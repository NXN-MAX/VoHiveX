package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVoiceDiagnosticsDiscardUnsolicitedPrivateData(t *testing.T) {
	query := voiceDiagnosticQueries[0]
	raw := "ATD+12025550123;\r\n+CMT: \"+12025550123\"\r\nprivate SMS\r\n+CEER: 1, 16\r\nOK"
	got := parseVoiceDiagnostic(query, raw, nil)
	if got.Status != "available" || got.Value != "1,16" {
		t.Fatalf("cause = %#v", got)
	}
	for _, raw := range []string{"OK", "+CEER: unknown private text", "+CEER: 1,16\n+CEER: 1,17", "+CEER: 123456789012345"} {
		if got := parseVoiceDiagnostic(query, raw, nil); got.Status == "available" || got.Value != "" {
			t.Fatalf("accepted ambiguous/private cause: %#v", got)
		}
	}
	if got := parseVoiceDiagnostic(query, "+CEER: 1,16", errors.New("private transport error")); got.Status != "query_failed" || got.Value != "" {
		t.Fatalf("query error not isolated: %#v", got)
	}
	got = parseVoiceDiagnostic(voiceDiagnosticQueries[2], `+CREG: 2,5,"1234","12345678",7`, nil)
	if got.Value != "5" {
		t.Fatalf("registration leaked cell location: %#v", got)
	}
}

func TestVoiceDiagnosticCauseDomains(t *testing.T) {
	for _, tc := range []struct{ raw, value, domain string }{
		{"+CEER: 0,-1", "0,-1", "no_cause"},
		{"+CEER: 1,16", "1,16", "cs_network"},
		{"+CEER: 4,36", "4,36", "ps_network"},
		{"+CEER: 6,259", "6,259", "lte_local"},
	} {
		got := parseVoiceDiagnostic(voiceDiagnosticQueries[0], tc.raw, nil)
		if got.Status != "available" || got.Value != tc.value || got.Domain != tc.domain {
			t.Fatalf("cause %q: %#v", tc.raw, got)
		}
	}
	for _, raw := range []string{"+CEER: 7,16", "+CEER: 1,-1", "+CEER: 1,16,99", "+CEER: 4,36\n+CEER: 1,16"} {
		if got := parseVoiceDiagnostic(voiceDiagnosticQueries[0], raw, nil); got.Status == "available" || got.Domain != "" {
			t.Fatalf("accepted unsupported or conflicting cause: %#v", got)
		}
	}
}

func TestCallDiagnosticsReadOnlyAndSeparateIMS(t *testing.T) {
	var commands []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/devices":
			writeJSON(w, 200, map[string]any{"devices": []map[string]any{{"id": "modem-1"}}})
		case "/api/devices/modem-1/actions/at":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			command, _ := body["cmd"].(string)
			commands = append(commands, command)
			responses := map[string]string{
				"AT+CEER": "+CEER: 1,16", "AT+CFUN?": "+CFUN: 4",
				"AT+CREG?": "+CREG: 0,0", "AT+CEREG?": "ERROR",
				`AT+QCFG="ims"`: `+QCFG: "ims",0,0`,
			}
			writeJSON(w, 200, map[string]any{"response": responses[command]})
		case "/api/devices/modem-1/status":
			writeJSON(w, 200, map[string]any{"imsi": "private", "vowifi": map[string]any{
				"active": true, "private_key": "private", "voice": map[string]any{"agent": map[string]any{"running": true, "active_call": false, "secret": "private"}},
			}})
		default:
			t.Errorf("unexpected modem operation %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	manager, err := newCallManager(":memory:", filepath.Join(t.TempDir(), "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	app := &application{upstream: strings.TrimPrefix(upstream.URL, "http://"), calls: manager}
	r := httptest.NewRequest(http.MethodGet, "/api/calls/diagnostics?device_id=modem-1", nil)
	r.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	want := []string{"AT+CEER", "AT+CFUN?", "AT+CREG?", "AT+CEREG?", `AT+QCFG="ims"`}
	if w.Code != 200 || !reflect.DeepEqual(commands, want) {
		t.Fatalf("status=%d commands=%#v response=%s", w.Code, commands, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "private") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("diagnostics leaked private upstream fields or allowed caching")
	}
	var result struct {
		ReadOnly bool                            `json:"read_only"`
		Modem    map[string]voiceDiagnosticField `json:"modem"`
		Host     map[string]any                  `json:"host_vowifi"`
		Control  map[string]string               `json:"control"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.ReadOnly || result.Control["vowifi"] != "not_connected" || result.Host["agent_running"] != true || result.Modem["modem_ims_config"].Value != "0,0" || result.Modem["eps_registration"].Status != "query_failed" {
		t.Fatalf("modem configuration and host voice readiness conflated: %#v", result)
	}
	commands = nil
	r = httptest.NewRequest(http.MethodGet, "/api/calls/diagnostics?device_id=missing", nil)
	r.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	if w.Code != 404 || len(commands) != 0 {
		t.Fatal("unknown device reached AT diagnostics")
	}
}

func TestCallDiagnosticsRequireAuthentication(t *testing.T) {
	app := &application{}
	r := httptest.NewRequest(http.MethodGet, "/api/calls/diagnostics?device_id=modem-1", nil)
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated diagnostics status=%d", w.Code)
	}
}

func TestQMIVoiceCapabilityMaskBoundaries(t *testing.T) {
	mask := []byte{0x80, 0x01, 0x00, 0x00, 0x04}
	for _, id := range []uint16{7, 8, 34} {
		if !qmiVoiceMessageSupported(mask, id) {
			t.Fatalf("missing supported message %d", id)
		}
	}
	for _, id := range []uint16{0, 9, 33, 40, 65535} {
		if qmiVoiceMessageSupported(mask, id) {
			t.Fatalf("falsely supported message %d", id)
		}
	}
	if qmiVoiceMessageSupported(nil, 34) {
		t.Fatal("empty mask considered supported")
	}
}
