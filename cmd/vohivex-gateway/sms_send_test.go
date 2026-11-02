package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iniwex5/qmi-go/pkg/qmi"
)

type fakeQMISMSService struct {
	rawErr         error
	writeErr       error
	imsErr         error
	rawCalls       int
	writeCalls     int
	imsCalls       int
	deleteCalls    int
	writtenPDU     []byte
	writtenStorage uint8
	writtenFormat  uint8
	storageIndex   uint32
	imsMode        qmi.MessageMode
	imsEnabled     bool
}

func (service *fakeQMISMSService) SendRawMessage(_ context.Context, _ uint8, pdu []byte) error {
	service.rawCalls++
	service.writtenPDU = append([]byte(nil), pdu...)
	return service.rawErr
}

func (service *fakeQMISMSService) RawWriteMessage(_ context.Context, storage, format uint8, pdu []byte) (uint32, error) {
	service.writeCalls++
	service.writtenStorage = storage
	service.writtenFormat = format
	service.writtenPDU = append([]byte(nil), pdu...)
	if service.storageIndex == 0 {
		service.storageIndex = 23
	}
	return service.storageIndex, service.writeErr
}

func (service *fakeQMISMSService) SendFromStorage(_ context.Context, _ uint8, _ uint32, mode qmi.MessageMode, ims bool) (*qmi.WMSSendFromStorageResult, error) {
	service.imsCalls++
	service.imsMode = mode
	service.imsEnabled = ims
	return &qmi.WMSSendFromStorageResult{MessageID: 7, HasMessageID: true}, service.imsErr
}

func (service *fakeQMISMSService) DeleteMessageByIndex(_ context.Context, _ uint8, _ uint32, _ qmi.MessageMode) error {
	service.deleteCalls++
	return nil
}

func TestQMISMSSendsRawWhenAccepted(t *testing.T) {
	service := &fakeQMISMSService{}
	parts, err := sendQmiSMSWithService(context.Background(), service, "+12025550123", "VoHiveX test")
	if err != nil || parts != 1 {
		t.Fatalf("parts=%d err=%v", parts, err)
	}
	if service.rawCalls != 1 || service.writeCalls != 0 || service.imsCalls != 0 {
		t.Fatalf("unexpected calls: %#v", service)
	}
}

func TestQMISMSFallsBackToStorageIMSAfterExplicitRejection(t *testing.T) {
	service := &fakeQMISMSService{rawErr: &qmi.QMIError{
		Service: qmi.ServiceWMS, MessageID: qmi.WMSRawSend, Result: 1, ErrorCode: qmi.QMIErrIncompatibleState,
	}}
	parts, err := sendQmiSMSWithService(context.Background(), service, "+12025550123", "VoHiveX 短信测试")
	if err != nil || parts != 1 {
		t.Fatalf("parts=%d err=%v", parts, err)
	}
	if service.rawCalls != 1 || service.writeCalls != 1 || service.imsCalls != 1 || service.deleteCalls != 1 {
		t.Fatalf("unexpected calls: %#v", service)
	}
	if service.writtenStorage != qmiSMSStorageNV || service.writtenFormat != 0x06 || service.imsMode != qmi.MessageModeGW || !service.imsEnabled {
		t.Fatalf("unexpected IMS fallback parameters: %#v", service)
	}
	if len(service.writtenPDU) < 2 || service.writtenPDU[0] != 0x00 {
		t.Fatalf("PDU must begin with the empty SMSC length byte: %x", service.writtenPDU)
	}
	if !bytes.Contains(service.writtenPDU, []byte{0x77, 0xed, 0x4f, 0xe1}) {
		t.Fatalf("Chinese content was not encoded as UCS-2: %x", service.writtenPDU)
	}
}

func TestQMISMSDoesNotRetryAfterAmbiguousRawFailure(t *testing.T) {
	service := &fakeQMISMSService{rawErr: context.DeadlineExceeded}
	parts, err := sendQmiSMSWithService(context.Background(), service, "+12025550123", "test")
	if parts != 0 || err == nil {
		t.Fatalf("parts=%d err=%v", parts, err)
	}
	var uncertain *smsDeliveryUncertainError
	if !errors.As(err, &uncertain) {
		t.Fatalf("expected uncertain delivery error, got %T: %v", err, err)
	}
	if service.writeCalls != 0 || service.imsCalls != 0 {
		t.Fatalf("ambiguous result must not be retried: %#v", service)
	}
}

func TestQMISMSPreservesStoredMessageAfterAmbiguousIMSResult(t *testing.T) {
	service := &fakeQMISMSService{
		rawErr: &qmi.QMIError{
			Service: qmi.ServiceWMS, MessageID: qmi.WMSRawSend, Result: 1, ErrorCode: qmi.QMIErrIncompatibleState,
		},
		imsErr: context.DeadlineExceeded,
	}
	parts, err := sendQmiSMSWithService(context.Background(), service, "+12025550123", "test")
	if parts != 0 || err == nil {
		t.Fatalf("parts=%d err=%v", parts, err)
	}
	var uncertain *smsDeliveryUncertainError
	if !errors.As(err, &uncertain) {
		t.Fatalf("expected uncertain delivery error, got %T: %v", err, err)
	}
	if service.writeCalls != 1 || service.imsCalls != 1 || service.deleteCalls != 0 {
		t.Fatalf("ambiguous IMS result must keep the stored message for diagnosis: %#v", service)
	}
}

func TestSMSSendDiagnosticMasksRecipientAndOmitsContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "sms-send.log")
	app := &application{smsLogPath: path}
	app.recordSMSSend("ec20", "+12025550123", "qmi-direct", "failed", 0, errors.New("QMI error 0x005a"))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "+12025550123") || !strings.Contains(text, "+12***0123") {
		t.Fatalf("recipient was not masked: %s", text)
	}
	if !strings.Contains(text, "QMI error 0x005a") {
		t.Fatalf("diagnostic error missing: %s", text)
	}
}

func TestSMSSendFallsBackToQMIAndArchivesMessage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/devices":
			writeJSON(w, http.StatusOK, map[string]any{"devices": []map[string]any{{
				"id": "ec20", "name": "电信卡", "running": true, "sms_enabled": true,
				"modem": map[string]any{"imsi": "460110000000000"},
			}}})
		case "/api/sms/send":
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": "AT+CMGS 等待提示符超时"})
		case "/api/devices/ec20/config":
			writeJSON(w, http.StatusOK, map[string]any{"config": map[string]any{
				"id": "ec20", "device_backend": "at", "control_device": "/dev/cdc-wdm1",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	archive, err := newSMSArchive(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	config := "server:\n  port: 127.0.0.1:7576\nweb:\n  username: admin\n  password: admin\ndevices:\n  - id: ec20\n    device_backend: at\n    qmi_use_proxy: true\n"
	if err := writeAtomic(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	app := &application{
		upstream:   strings.TrimPrefix(upstream.URL, "http://"),
		configPath: configPath,
		archive:    archive,
		qmiSendSMS: func(_ context.Context, device qmiSMSDevice, phone, message string) (int, error) {
			called = true
			if device.ControlPath != "/dev/cdc-wdm1" || !device.UseProxy || phone != "+12025550123" || message != "VoHiveX test" {
				t.Fatalf("unexpected QMI fallback: %#v %q %q", device, phone, message)
			}
			return 1, nil
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/sms/send", strings.NewReader(`{"device_id":"ec20","phone":"+12025550123","message":"VoHiveX test"}`))
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !called {
		t.Fatalf("response=%d called=%v body=%s", response.Code, called, response.Body.String())
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["transport"] != "qmi-fallback" || result["delivery_state"] != "acked" {
		t.Fatalf("unexpected response: %#v", result)
	}
	messages, err := archive.Messages("+12025550123", "ec20", "", 20, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0]["content"] != "VoHiveX test" || messages[0]["status"] != int64(2) {
		t.Fatalf("unexpected archive: %#v", messages)
	}
}

func TestSMSSendKeepsATFirstWhenQMIPreferred(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/devices":
			writeJSON(w, http.StatusOK, map[string]any{"devices": []map[string]any{{
				"id": "ec20", "name": "电信卡", "running": true, "sms_enabled": true,
			}}})
		case "/api/devices/ec20/config":
			writeJSON(w, http.StatusOK, map[string]any{"config": map[string]any{
				"id": "ec20", "device_backend": "at", "control_device": "/dev/cdc-wdm1",
			}})
		case "/api/sms/send":
			writeJSON(w, http.StatusOK, map[string]any{
				"status": "success", "delivery_state": "acked", "transport": "cellular-at",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	archive, err := newSMSArchive(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	config := "server:\n  port: 127.0.0.1:7576\nweb:\n  username: admin\n  password: admin\ndevices:\n  - id: ec20\n    device_backend: at\n    qmi_use_proxy: true\n    qmi_sms_preferred: true\n"
	if err := writeAtomic(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	app := &application{
		upstream:   strings.TrimPrefix(upstream.URL, "http://"),
		configPath: configPath,
		archive:    archive,
		qmiSendSMS: func(_ context.Context, device qmiSMSDevice, phone, message string) (int, error) {
			called = true
			return 0, errors.New("QMI must not run after a successful AT submission")
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/sms/send", strings.NewReader(`{"device_id":"ec20","phone":"+12025550123","message":"VoHiveX direct"}`))
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || called {
		t.Fatalf("response=%d called=%v body=%s", response.Code, called, response.Body.String())
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["transport"] != "cellular-at" || result["delivery_state"] != "acked" {
		t.Fatalf("unexpected response: %#v", result)
	}
}

func TestSMSSendDoesNotRetryExplicitModemRejectionThroughQMI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/devices":
			writeJSON(w, http.StatusOK, map[string]any{"devices": []map[string]any{{
				"id": "ec20", "name": "电信卡", "running": true, "sms_enabled": true,
			}}})
		case "/api/devices/ec20/config":
			writeJSON(w, http.StatusOK, map[string]any{"config": map[string]any{
				"id": "ec20", "device_backend": "at", "control_device": "/dev/cdc-wdm1",
			}})
		case "/api/sms/send":
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": "AT+CMGS: +CMS ERROR: 500"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	archive, err := newSMSArchive(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	config := "server:\n  port: 127.0.0.1:7576\nweb:\n  username: admin\n  password: admin\ndevices:\n  - id: ec20\n    device_backend: at\n    qmi_use_proxy: true\n    qmi_sms_preferred: true\n"
	if err := writeAtomic(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	app := &application{
		upstream:   strings.TrimPrefix(upstream.URL, "http://"),
		configPath: configPath,
		archive:    archive,
		qmiSendSMS: func(context.Context, qmiSMSDevice, string, string) (int, error) {
			called = true
			return 0, nil
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/sms/send", strings.NewReader(`{"device_id":"ec20","phone":"+12025550123","message":"VoHiveX test"}`))
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || called {
		t.Fatalf("response=%d called=%v body=%s", response.Code, called, response.Body.String())
	}
}

func TestSMSDevicesUsesOverviewLocalPhone(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/devices":
			writeJSON(w, http.StatusOK, map[string]any{"devices": []map[string]any{{
				"id": "ec20", "name": "电信卡", "running": true, "healthy": true,
				"modem": map[string]any{"imei": "868704046838858"},
			}}})
		case "/api/devices/ec20/overview":
			writeJSON(w, http.StatusOK, map[string]any{"devices": []map[string]any{{
				"id": "ec20", "modem": map[string]any{"phone_number": "+12025550125"},
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	app := &application{upstream: strings.TrimPrefix(upstream.URL, "http://")}

	request := httptest.NewRequest(http.MethodGet, "/api/sms/devices", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("devices response=%d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Devices []map[string]any `json:"devices"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Devices) != 1 || payload.Devices[0]["local_phone"] != "+12025550125" {
		t.Fatalf("unexpected devices payload: %#v", payload.Devices)
	}
}

func TestSMSSendDoesNotFallbackForValidationFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/devices" {
			writeJSON(w, http.StatusOK, map[string]any{"devices": []map[string]any{{"id": "ec20"}}})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "电话号码格式无效"})
	}))
	defer upstream.Close()
	archive, err := newSMSArchive(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	app := &application{
		upstream: strings.TrimPrefix(upstream.URL, "http://"),
		archive:  archive,
		qmiSendSMS: func(context.Context, qmiSMSDevice, string, string) (int, error) {
			t.Fatal("QMI fallback must not run for a validation error")
			return 0, nil
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/sms/send", strings.NewReader(`{"device_id":"ec20","phone":"bad","message":"test"}`))
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("response=%d body=%s", response.Code, response.Body.String())
	}
}
