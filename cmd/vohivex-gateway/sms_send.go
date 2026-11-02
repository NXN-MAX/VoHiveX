package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/iniwex5/qmi-go/pkg/qmi"
	"github.com/warthog618/sms"
	"github.com/warthog618/sms/encoding/tpdu"
)

var qmiControlName = regexp.MustCompile(`^(?:cdc-wdm\d+|wwan\d+qmi\d+|mhi_QMI\d+)$`)

const qmiSMSStorageNV uint8 = 0x01

type qmiSMSService interface {
	SendRawMessage(context.Context, uint8, []byte) error
	RawWriteMessage(context.Context, uint8, uint8, []byte) (uint32, error)
	SendFromStorage(context.Context, uint8, uint32, qmi.MessageMode, bool) (*qmi.WMSSendFromStorageResult, error)
	DeleteMessageByIndex(context.Context, uint8, uint32, qmi.MessageMode) error
}

type smsDeliveryUncertainError struct {
	err error
}

func (e *smsDeliveryUncertainError) Error() string {
	return e.err.Error()
}

func (e *smsDeliveryUncertainError) Unwrap() error {
	return e.err
}

type qmiSMSDevice struct {
	ControlPath     string
	UseProxy        bool
	ProxyPath       string
	ProxyExecutable string
	Backend         string
	PreferSMS       bool
}

type smsSendPayload struct {
	DeviceID string `json:"device_id"`
	IMSI     string `json:"imsi"`
	Phone    string `json:"phone"`
	Message  string `json:"message"`
}

func probeQMIWMS(ctx context.Context, device qmiSMSDevice) (string, error) {
	client, err := qmi.NewClientWithOptions(ctx, device.ControlPath, qmi.ClientOptions{
		UseProxy:           device.UseProxy,
		ProxyPath:          device.ProxyPath,
		ProxyExecutable:    device.ProxyExecutable,
		ProxyFallbackToRaw: false,
	})
	if err != nil {
		return "", fmt.Errorf("QMI control open failed: %w", err)
	}
	defer client.Close()
	wms, err := qmi.NewWMSServiceWithContext(ctx, client)
	if err != nil {
		return "", fmt.Errorf("WMS service open failed: %w", err)
	}
	defer wms.Close()
	parts := make([]string, 0, 5)
	if supported, supportedErr := wms.GetSupportedMessages(ctx); supportedErr != nil {
		parts = append(parts, "supported_error="+supportedErr.Error())
	} else {
		parts = append(parts, fmt.Sprintf("supported_bytes=%d", len(supported)))
	}
	if protocol, protocolErr := wms.GetMessageProtocol(ctx); protocolErr != nil {
		parts = append(parts, "protocol_error="+protocolErr.Error())
	} else {
		parts = append(parts, fmt.Sprintf("protocol=%d", protocol))
	}
	if state, stateErr := wms.GetTransportNetworkRegistrationStatus(ctx); stateErr != nil {
		parts = append(parts, "transport_error="+stateErr.Error())
	} else {
		parts = append(parts, "transport="+state.String())
	}
	if routes, routesErr := wms.GetRoutes(ctx); routesErr != nil {
		parts = append(parts, "routes_error="+routesErr.Error())
	} else if routes == nil {
		parts = append(parts, "routes=0")
	} else {
		parts = append(parts, fmt.Sprintf("routes=%d", len(routes.Routes)))
	}
	if smsc, smscErr := wms.GetSMSCAddress(ctx); smscErr != nil {
		parts = append(parts, "smsc_error="+smscErr.Error())
	} else {
		parts = append(parts, "smsc="+smsc)
	}
	return strings.Join(parts, "\n"), nil
}

func configBool(value any, fallback bool) bool {
	if value == nil {
		return fallback
	}
	parsed, ok := value.(bool)
	if ok {
		return parsed
	}
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(value))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func mergeQMISMSConfig(result *qmiSMSDevice, row map[string]any) {
	if result == nil || row == nil {
		return
	}
	if value := strings.TrimSpace(fmt.Sprint(row["control_device"])); value != "" && value != "<nil>" {
		result.ControlPath = value
	}
	if value := strings.TrimSpace(fmt.Sprint(row["qmi_device"])); result.ControlPath == "" && value != "" && value != "<nil>" {
		result.ControlPath = value
	}
	if value, exists := row["qmi_use_proxy"]; exists {
		result.UseProxy = configBool(value, result.UseProxy)
	}
	if value, exists := row["qmi_sms_preferred"]; exists {
		result.PreferSMS = configBool(value, result.PreferSMS)
	}
	if value := strings.TrimSpace(fmt.Sprint(row["qmi_proxy_path"])); value != "" && value != "<nil>" {
		result.ProxyPath = value
	}
	if value := strings.TrimSpace(fmt.Sprint(row["qmi_proxy_executable"])); value != "" && value != "<nil>" {
		result.ProxyExecutable = value
	}
	if value := strings.ToLower(strings.TrimSpace(fmt.Sprint(row["device_backend"]))); value != "" && value != "<nil>" {
		result.Backend = value
	}
}

func runtimeDeviceConfig(value any) map[string]any {
	object, _ := value.(map[string]any)
	if nested, ok := object["config"].(map[string]any); ok {
		return nested
	}
	return object
}

func (app *application) qmiSMSConfig(r *http.Request, deviceID string) (qmiSMSDevice, error) {
	result := qmiSMSDevice{UseProxy: true, Backend: "at"}
	found := false
	configMu.Lock()
	cfg, _, err := readCoreConfig(app.configPath)
	configMu.Unlock()
	if err == nil {
		for _, row := range cfg.Devices {
			if strings.TrimSpace(fmt.Sprint(row["id"])) != deviceID {
				continue
			}
			found = true
			mergeQMISMSConfig(&result, row)
			break
		}
	}

	// Device paths are deliberately omitted from persistent configuration and
	// resolved from the modem at runtime. Read the effective config so an AT
	// device can still use its QMI WMS service when AT+CMGS is incompatible.
	status, value, runtimeErr := app.upstreamJSON(r, http.MethodGet, "/api/devices/"+url.PathEscape(deviceID)+"/config", nil, 10*time.Second)
	if runtimeErr == nil && status == http.StatusOK {
		found = true
		mergeQMISMSConfig(&result, runtimeDeviceConfig(value))
	}
	if !found {
		if err != nil {
			return qmiSMSDevice{}, err
		}
		return qmiSMSDevice{}, errors.New("设备配置不存在")
	}
	if result.ControlPath == "" || result.ControlPath == "<nil>" {
		return qmiSMSDevice{}, errors.New("设备没有可用的 QMI 控制口")
	}
	parts := strings.Split(result.ControlPath, "/")
	if !strings.HasPrefix(result.ControlPath, "/dev/") || !qmiControlName.MatchString(parts[len(parts)-1]) {
		return qmiSMSDevice{}, errors.New("QMI 控制口格式无效")
	}
	return result, nil
}

func sendQmiSMSWithService(ctx context.Context, wms qmiSMSService, phone, content string) (int, error) {
	pdus, err := sms.Encode([]byte(content), sms.AsSubmit, sms.To(phone))
	if err != nil || len(pdus) == 0 {
		return 0, fmt.Errorf("短信编码失败: %w", err)
	}
	for index := range pdus {
		if isQMIShortCode(phone) {
			address := pdus[index].DA
			address.SetTypeOfNumber(tpdu.TonUnknown)
			address.SetNumberingPlan(tpdu.NpISDN)
			pdus[index].DA = address
		}
		raw, marshalErr := pdus[index].MarshalBinary()
		if marshalErr != nil {
			return index, fmt.Errorf("短信第 %d 段编码失败: %w", index+1, marshalErr)
		}
		// WMS Raw Send expects one SMSC-length byte followed by the TPDU.
		pdu := append([]byte{0x00}, raw...)
		sendErr := wms.SendRawMessage(ctx, 0x06, pdu)
		if sendErr == nil {
			continue
		}

		// A protocol-level rejection guarantees that Raw Send did not accept the
		// segment, so it is safe to try the LTE/IMS path. Transport failures and
		// timeouts are ambiguous and must never trigger an automatic retry.
		if qmi.GetQMIError(sendErr) == nil {
			return index, &smsDeliveryUncertainError{err: fmt.Errorf("QMI 短信第 %d/%d 段发送结果未知: %w", index+1, len(pdus), sendErr)}
		}

		storageIndex, writeErr := wms.RawWriteMessage(ctx, qmiSMSStorageNV, 0x06, pdu)
		if writeErr != nil {
			return index, fmt.Errorf("QMI 短信第 %d/%d 段原始发送被拒绝（%v），IMS 暂存失败: %w", index+1, len(pdus), sendErr, writeErr)
		}
		_, imsErr := wms.SendFromStorage(ctx, qmiSMSStorageNV, storageIndex, qmi.MessageModeGW, true)
		if imsErr != nil {
			if qmi.GetQMIError(imsErr) == nil {
				return index, &smsDeliveryUncertainError{err: fmt.Errorf("QMI 短信第 %d/%d 段 IMS 发送结果未知: %w", index+1, len(pdus), imsErr)}
			}
			if deleteErr := wms.DeleteMessageByIndex(ctx, qmiSMSStorageNV, storageIndex, qmi.MessageModeGW); deleteErr != nil {
				log.Printf("QMI SMS cleanup failed for NV index %d: %v", storageIndex, deleteErr)
			}
			return index, fmt.Errorf("QMI 短信第 %d/%d 段 IMS 发送失败（原始发送错误：%v）: %w", index+1, len(pdus), sendErr, imsErr)
		}
		if deleteErr := wms.DeleteMessageByIndex(ctx, qmiSMSStorageNV, storageIndex, qmi.MessageModeGW); deleteErr != nil {
			log.Printf("QMI SMS cleanup failed for NV index %d: %v", storageIndex, deleteErr)
		}
	}
	return len(pdus), nil
}

func sendQmiSMS(ctx context.Context, device qmiSMSDevice, phone, content string) (int, error) {
	client, err := qmi.NewClientWithOptions(ctx, device.ControlPath, qmi.ClientOptions{
		UseProxy:           device.UseProxy,
		ProxyPath:          device.ProxyPath,
		ProxyExecutable:    device.ProxyExecutable,
		ProxyFallbackToRaw: false,
	})
	if err != nil {
		return 0, fmt.Errorf("QMI 控制口打开失败: %w", err)
	}
	defer client.Close()
	wms, err := qmi.NewWMSServiceWithContext(ctx, client)
	if err != nil {
		return 0, fmt.Errorf("QMI 短信服务启动失败: %w", err)
	}
	defer wms.Close()
	return sendQmiSMSWithService(ctx, wms, phone, content)
}

func isQMIShortCode(phone string) bool {
	digits := strings.TrimPrefix(strings.TrimSpace(phone), "+")
	if digits == "" || len(digits) > 6 {
		return false
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func smsFailureText(value any) string {
	object, _ := value.(map[string]any)
	for _, key := range []string{"error", "message", "detail"} {
		if text := strings.TrimSpace(fmt.Sprint(object[key])); text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func shouldFallbackToQMI(status int, value any, err error) bool {
	// A transport error or a generic 5xx after an AT send request is
	// ambiguous: the modem may already have received Ctrl-Z. VoCat keeps the
	// AT session result authoritative in that case and never retries through a
	// second transport, which prevents duplicate SMS submissions.
	if err != nil {
		return false
	}
	if status >= 200 && status < 300 {
		return false
	}
	// An absent/unsupported upstream endpoint is known to occur before a modem
	// submission, so the QMI compatibility path is safe here.
	if status == http.StatusNotFound || status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
		return true
	}
	message := strings.ToLower(smsFailureText(value))
	// Waiting for the CMGS prompt happens before the payload and Ctrl-Z are
	// written. Only these explicit pre-submit failures may use QMI fallback.
	for _, marker := range []string{"prompt unsupported", "prompt is not supported", "sms prompt unsupported", "提示符", "at 短信不支持", "unsupported at sms"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func findSMSDevice(devices map[string]any, id string) map[string]any {
	for _, row := range deviceRows(devices) {
		if strings.TrimSpace(fmt.Sprint(row["id"])) == id {
			return row
		}
	}
	return nil
}

func maskedSMSRecipient(phone string) string {
	phone = strings.TrimSpace(phone)
	runes := []rune(phone)
	if len(runes) <= 7 {
		return "***"
	}
	return string(runes[:3]) + "***" + string(runes[len(runes)-4:])
}

func singleLineSMSLogValue(value string) string {
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r < 0x20 {
			return ' '
		}
		return r
	}, value)
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > 600 {
		return string(runes[:600])
	}
	return string(runes)
}

func (app *application) recordSMSSend(deviceID, phone, transport, outcome string, parts int, sendErr error) {
	if app == nil || strings.TrimSpace(app.smsLogPath) == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(app.smsLogPath), 0o700); err != nil {
		log.Printf("SMS diagnostic directory unavailable: %v", err)
		return
	}
	file, err := os.OpenFile(app.smsLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("SMS diagnostic log unavailable: %v", err)
		return
	}
	defer file.Close()
	errorText := ""
	if sendErr != nil {
		errorText = singleLineSMSLogValue(sendErr.Error())
	}
	_, _ = fmt.Fprintf(file, "%s device=%q recipient=%q transport=%q outcome=%q parts=%d error=%q\n",
		time.Now().UTC().Format(time.RFC3339Nano), singleLineSMSLogValue(deviceID), maskedSMSRecipient(phone),
		singleLineSMSLogValue(transport), singleLineSMSLogValue(outcome), parts, errorText)
}

func (app *application) submitQMISMS(r *http.Request, device map[string]any, payload smsSendPayload, qmiDevice qmiSMSDevice, transport string) (int, map[string]any, error) {
	app.smsMu.Lock()
	defer app.smsMu.Unlock()
	sender := app.qmiSendSMS
	if sender == nil {
		sender = sendQmiSMS
	}
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	defer cancel()
	parts, sendErr := sender(ctx, qmiDevice, payload.Phone, payload.Message)
	if sendErr != nil {
		log.Printf("QMI SMS send failed for device %s: %v", payload.DeviceID, sendErr)
		outcome := "failed"
		var uncertain *smsDeliveryUncertainError
		if errors.As(sendErr, &uncertain) {
			outcome = "unknown"
		}
		app.recordSMSSend(payload.DeviceID, payload.Phone, transport, outcome, parts, sendErr)
		if outcome == "unknown" {
			return http.StatusBadGateway, map[string]any{"error": "短信发送结果未知，请核对短信记录后再重试：" + sendErr.Error()}, nil
		}
		return http.StatusBadGateway, map[string]any{"error": "QMI 短信发送失败：" + sendErr.Error()}, nil
	}
	if parts < 1 {
		parts = 1
	}
	now := time.Now().UTC()
	messageID := fmt.Sprintf("qmi-%d", now.UnixNano())
	_, _ = app.archive.Import(device, []map[string]any{{
		"peer":       payload.Phone,
		"content":    payload.Message,
		"type":       2,
		"timestamp":  now.Format(time.RFC3339),
		"status":     2,
		"message_id": messageID,
	}})
	app.recordSMSSend(payload.DeviceID, payload.Phone, transport, "submitted", parts, nil)
	return http.StatusOK, map[string]any{
		"status":         "success",
		"delivery_state": "acked",
		"message_id":     messageID,
		"parts_total":    parts,
		"acks":           parts,
		"transport":      transport,
	}, nil
}

func (app *application) submitSMS(r *http.Request, device map[string]any, payload smsSendPayload) (int, map[string]any, error) {
	qmiDevice, configErr := app.qmiSMSConfig(r, payload.DeviceID)
	// Keep the modem's AT+CMGS path authoritative. The previous
	// qmi_sms_preferred shortcut skipped AT entirely, but this hardware rejects
	// QMI WMS while still exposing a working serial SMS interface. QMI remains a
	// compatibility fallback only when AT cannot reach the pre-submit prompt.
	upstreamStatus, upstreamValue, upstreamErr := app.upstreamJSON(r, http.MethodPost, "/api/sms/send", map[string]any{
		"device_id": payload.DeviceID,
		"imsi":      payload.IMSI,
		"phone":     payload.Phone,
		"message":   payload.Message,
	}, 120*time.Second)
	if upstreamErr == nil && upstreamStatus >= 200 && upstreamStatus < 300 {
		result, _ := upstreamValue.(map[string]any)
		if result == nil {
			result = map[string]any{}
		}
		return upstreamStatus, result, nil
	}
	if !shouldFallbackToQMI(upstreamStatus, upstreamValue, upstreamErr) {
		result, _ := upstreamValue.(map[string]any)
		if result == nil {
			result = map[string]any{"error": "短信发送失败"}
		}
		return upstreamStatus, result, nil
	}

	if configErr != nil {
		qmiDevice, configErr = app.qmiSMSConfig(r, payload.DeviceID)
	}
	if configErr != nil || qmiDevice.Backend != "at" {
		if configErr != nil {
			log.Printf("QMI SMS fallback unavailable for device %s: %v", payload.DeviceID, configErr)
		}
		if upstreamErr != nil {
			return 0, nil, upstreamErr
		}
		result, _ := upstreamValue.(map[string]any)
		if result == nil {
			result = map[string]any{"error": "短信发送失败"}
		}
		return upstreamStatus, result, nil
	}

	return app.submitQMISMS(r, device, payload, qmiDevice, "qmi-fallback")
}

func (app *application) smsSendAPI(w http.ResponseWriter, r *http.Request) {
	devices, status, err := app.validateAuth(r)
	if err != nil {
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	if !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "不允许跨站发送短信"})
		return
	}
	var payload smsSendPayload
	if !decodeJSON(w, r, &payload, 16<<10) {
		return
	}
	payload.DeviceID = strings.TrimSpace(payload.DeviceID)
	payload.IMSI = strings.TrimSpace(payload.IMSI)
	payload.Phone = strings.TrimSpace(payload.Phone)
	payload.Message = strings.TrimSpace(payload.Message)
	if payload.DeviceID == "" || payload.Phone == "" || payload.Message == "" || len([]rune(payload.Message)) > 2000 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "发信设备、收信号码和短信内容不能为空"})
		return
	}
	device := findSMSDevice(devices, payload.DeviceID)
	if device == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "发信设备不存在"})
		return
	}
	status, result, submitErr := app.submitSMS(r, device, payload)
	if submitErr != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "发送结果未知，请核对短信记录后重试"})
		return
	}
	if status < 1 {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, result)
}

func (app *application) smsDevicesAPI(w http.ResponseWriter, r *http.Request) {
	devices, status, err := app.validateAuth(r)
	if err != nil {
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": app.deviceSummaries(r, deviceRows(devices))})
}
