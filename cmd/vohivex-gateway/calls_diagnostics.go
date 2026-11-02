package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/iniwex5/qmi-go/pkg/qmi"
)

type voiceDiagnosticQuery struct {
	key, command string
	pattern      *regexp.Regexp
}

// Read CEER first: subsequent modem operations may replace the last cause.
// Only protocol fields are retained; AT echoes and SMS URCs may contain PII.
// This diagnostic never activates RF, PDP contexts, audio routing or a call.
var voiceDiagnosticQueries = []voiceDiagnosticQuery{
	{"last_cause", "AT+CEER", regexp.MustCompile(`^\+CEER:\s*((?:0\s*,\s*-1|[0-6]\s*,\s*[0-9]{1,5}))$`)},
	{"radio_function", "AT+CFUN?", regexp.MustCompile(`^\+CFUN:\s*([0-9]{1,2})$`)},
	{"cs_registration", "AT+CREG?", regexp.MustCompile(`^\+CREG:\s*[0-9]{1,2}\s*,\s*([0-9]{1,2})(?:\s*,.*)?$`)},
	{"eps_registration", "AT+CEREG?", regexp.MustCompile(`^\+CEREG:\s*[0-9]{1,2}\s*,\s*([0-9]{1,2})(?:\s*,.*)?$`)},
	{"modem_ims_config", `AT+QCFG="ims"`, regexp.MustCompile(`^\+QCFG:\s*"ims"\s*,\s*([012]\s*,\s*[01])$`)},
}

type voiceDiagnosticField struct {
	Status string `json:"status"`
	Value  string `json:"value,omitempty"`
	Domain string `json:"domain,omitempty"`
}

func parseVoiceDiagnostic(query voiceDiagnosticQuery, raw string, err error) voiceDiagnosticField {
	result := voiceDiagnosticField{Status: "unavailable"}
	if err != nil || callATRejection(raw) != "" {
		result.Status = "query_failed"
		return result
	}
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r", "\n"), "\n") {
		match := query.pattern.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		value := strings.ReplaceAll(match[1], " ", "")
		// Conflicting result lines are not a reliable observation.
		if result.Status == "available" && result.Value != value {
			return voiceDiagnosticField{Status: "unavailable"}
		}
		result = voiceDiagnosticField{Status: "available", Value: value}
		if query.key == "last_cause" {
			// Quectel CEER categories 3-6 describe packet-data failures, not
			// necessarily the voice call being investigated. No-cause is 0,-1.
			if value == "0,-1" {
				result.Domain = "no_cause"
			} else {
				result.Domain = [...]string{"cs_local", "cs_network", "cs_reject", "ps_local", "ps_network", "lte_network", "lte_local"}[value[0]-'0']
			}
		}
	}
	return result
}

func (app *application) callDiagnosticsAPI(w http.ResponseWriter, r *http.Request, devices []map[string]any) {
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	if _, found := callDevice(devices, deviceID); !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "电话设备不存在"})
		return
	}
	lock := app.calls.deviceLock(deviceID)
	lock.Lock()
	defer lock.Unlock()
	fields := make(map[string]voiceDiagnosticField, len(voiceDiagnosticQueries))
	for _, query := range voiceDiagnosticQueries {
		raw, _, err := app.runCallAT(r, deviceID, query.command, 2*time.Second)
		fields[query.key] = parseVoiceDiagnostic(query, raw, err)
		if r.Context().Err() != nil {
			return
		}
	}
	// Keep host IMS observations separate from native modem IMS configuration.
	// An existing voice agent does not imply an externally controllable call API.
	host := map[string]any{"status": "unavailable"}
	status, value, err := app.upstreamJSON(r, http.MethodGet, "/api/devices/"+url.PathEscape(deviceID)+"/status", nil, 5*time.Second)
	if err == nil && status == http.StatusOK {
		if detail, ok := value.(map[string]any); ok {
			if vowifi, ok := detail["vowifi"].(map[string]any); ok {
				host["status"] = "available"
				if active, ok := vowifi["active"].(bool); ok {
					host["active"] = active
				}
				voice, _ := vowifi["voice"].(map[string]any)
				agent, _ := voice["agent"].(map[string]any)
				for _, key := range []string{"running", "active_call"} {
					if v, ok := agent[key].(bool); ok {
						host["agent_"+key] = v
					}
				}
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id": deviceID, "read_only": true, "modem": fields, "host_vowifi": host,
		"control": map[string]any{"cellular": "at", "vowifi": "not_connected"},
		"notes": []string{
			"Registration and command acceptance do not prove that a remote phone rang or answered.",
			"QCFG ims mode 0 is MBN-controlled; it does not by itself mean IMS is disabled.",
			"CEER is the modem's last cause and may be stale or unavailable; it is not a host IMS SIP response.",
			"CEER packet-data categories do not establish a voice failure reason; normal call clearing does not prove the recipient rang or answered.",
		},
		"updated_at": time.Now().Unix(),
	})
}

// A separate opt-in probe checks QMI VOICE without calling DialCall, EndCall,
// radio configuration or WDS. Do not fall back to raw control-device access
// while the running core owns the modem through qmi-proxy.
func probeQMIVoice(ctx context.Context, device qmiSMSDevice) (string, error) {
	client, err := qmi.NewClientWithOptions(ctx, device.ControlPath, qmi.ClientOptions{
		UseProxy: true, ProxyPath: device.ProxyPath, ProxyExecutable: device.ProxyExecutable,
		ProxyFallbackToRaw: false,
	})
	if err != nil {
		return "", fmt.Errorf("QMI control open failed: %w", err)
	}
	defer client.Close()
	voice, err := qmi.NewVOICEServiceWithContext(ctx, client)
	if err != nil {
		return "", fmt.Errorf("VOICE service open failed: %w", err)
	}
	defer voice.Close()
	result := map[string]any{"read_only": true, "voice_service": true}
	if supported, err := voice.GetSupportedMessages(ctx); err != nil {
		result["supported_messages"] = "unavailable"
	} else {
		capabilities := map[string]bool{}
		for name, id := range map[string]uint16{"dial": qmi.VOICEDialCall, "answer": qmi.VOICEAnswerCall, "end": qmi.VOICEEndCall, "status": qmi.VOICEGetAllCallInfo, "dtmf": qmi.VOICEBurstDTMF} {
			capabilities[name] = qmiVoiceMessageSupported(supported, id)
		}
		result["supported_messages"] = capabilities
	}
	if info, err := voice.GetAllCallInfo(ctx); err != nil || info == nil {
		result["call_status"] = "unavailable"
	} else {
		// Intentionally omit remote-party numbers and private call handles.
		calls := make([]map[string]any, 0, len(info.Calls))
		for _, call := range info.Calls {
			calls = append(calls, map[string]any{"state": call.State, "type": call.Type, "mode": call.Mode, "direction": call.Direction})
		}
		result["call_status"] = "available"
		result["calls"] = calls
	}
	encoded, err := json.Marshal(result)
	return string(encoded), err
}

func qmiVoiceMessageSupported(mask []byte, message uint16) bool {
	index := int(message) / 8
	return index < len(mask) && mask[index]&(1<<uint(message%8)) != 0
}
