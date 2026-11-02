package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCLCCSnapshotFiltersDataSessions(t *testing.T) {
	response := "AT+CLCC\r\n" +
		"+CLCC: 1,1,0,1,0,\"\",128\r\n" +
		"+CLCC: 3,1,4,0,0,\"+12025550123\",145\r\nOK\r\n"
	calls, err := parseCLCCSnapshot(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].Index != 3 || calls[0].Direction != "incoming" || calls[0].State != "incoming" || calls[0].Number != "+12025550123" {
		t.Fatalf("unexpected calls: %#v", calls)
	}
}

func TestParseCLCCSnapshotAcceptsCoreSanitizedRows(t *testing.T) {
	response := "+CLCC: 1,1,0,1,0,\"\",128"
	calls, err := parseCLCCSnapshot(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 0 {
		t.Fatalf("data session leaked into voice calls: %#v", calls)
	}
}

func TestParseCLCCSnapshotFailsClosed(t *testing.T) {
	invalid := []string{
		"AT+CLCC",
		"+CLCC: 1,1,4,0,0,\"10086\",129\r\nRING\r\nOK",
		"+CLCC: 1,1,4,0,0\r\n+CLCC: 1,0,0,0,0\r\nOK",
		"+CLCC: 1,1,4,0,1\r\nOK",
		"+CLCC: 1,1,4,1,0\r\nAT+CLCC\r\nOK",
	}
	for _, response := range invalid {
		if calls, err := parseCLCCSnapshot(response); err == nil {
			t.Fatalf("accepted unsafe CLCC response %q as %#v", response, calls)
		}
	}
}

func TestCallStoreReconcilesHistoryAndContacts(t *testing.T) {
	manager, err := newCallManager(":memory:", filepath.Join(t.TempDir(), "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	contact, err := manager.saveContact("", "Example", "+12025550123", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if contact.Name != "Example" {
		t.Fatalf("unexpected contact: %#v", contact)
	}
	if _, err := manager.saveContact("", "Duplicate", "+12025550123", "", false); err == nil {
		t.Fatal("duplicate contact number was accepted")
	}
	device := map[string]any{"id": "device-1", "name": "Test modem"}
	started := time.Unix(1_800_000_000, 0)
	active, err := manager.reconcile(device, []parsedVoiceCall{{Index: 2, Direction: "incoming", State: "active", Number: "+12025550123"}}, started)
	if err != nil || len(active) != 1 || active[0].ContactName != "Example" || active[0].ConnectedAt == nil {
		t.Fatalf("active call reconcile = %#v, %v", active, err)
	}
	if _, err = manager.reconcile(device, nil, started.Add(65*time.Second)); err != nil {
		t.Fatal(err)
	}
	history, err := manager.history("device-1", 10)
	if err != nil || len(history) != 1 || history[0].State != "ended" || history[0].DurationSeconds != 65 || history[0].Missed {
		t.Fatalf("ended history = %#v, %v", history, err)
	}
}

func TestCallsAPIDialsOnlyValidatedNumber(t *testing.T) {
	commands := []string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/devices":
			writeJSON(w, http.StatusOK, map[string]any{"devices": []map[string]any{{"id": "device-1", "name": "Test modem", "running": true, "vowifi_enabled": false}}})
		case "/api/devices/device-1/actions/at":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			command := body["cmd"].(string)
			commands = append(commands, command)
			if command == "AT+CLCC" {
				writeJSON(w, http.StatusOK, map[string]any{"response": "AT+CLCC\r\nOK\r\n"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"response": command + "\r\nOK\r\n"})
		default:
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

	request := httptest.NewRequest(http.MethodPost, "/api/calls/dial", strings.NewReader(`{"device_id":"device-1","number":"+12025550123"}`))
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(commands) != 2 || commands[0] != "AT+CLCC" || commands[1] != "ATD+12025550123;" {
		t.Fatalf("dial response=%d %s commands=%#v", response.Code, response.Body.String(), commands)
	}

	commands = nil
	request = httptest.NewRequest(http.MethodPost, "/api/calls/dial", strings.NewReader(`{"device_id":"device-1","number":"10086;ATH"}`))
	request.Header.Set("Authorization", "Bearer test-token")
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	app.routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unsafe dial response=%d %s", response.Code, response.Body.String())
	}
	for _, command := range commands {
		if strings.HasPrefix(command, "ATD") {
			t.Fatalf("unsafe number reached dial command: %q", command)
		}
	}
}

func TestCallsDevicesUsesMSISDNFallback(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/devices" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"devices": []map[string]any{{
			"id":      "device-1",
			"name":    "Test modem",
			"running": true,
			"modem":   map[string]any{"msisdn": "+12025550123"},
		}}})
	}))
	defer upstream.Close()
	manager, err := newCallManager(":memory:", filepath.Join(t.TempDir(), "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	app := &application{upstream: strings.TrimPrefix(upstream.URL, "http://"), calls: manager}

	request := httptest.NewRequest(http.MethodGet, "/api/calls/devices", nil)
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
	if len(payload.Devices) != 1 || payload.Devices[0]["local_phone"] != "+12025550123" {
		t.Fatalf("unexpected devices payload: %#v", payload.Devices)
	}
}

func TestCallsDevicesUsesOverviewLocalPhone(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/devices":
			writeJSON(w, http.StatusOK, map[string]any{"devices": []map[string]any{{
				"id": "device-1", "name": "Test modem", "running": true, "vowifi_enabled": false,
			}}})
		case "/api/devices/device-1/overview":
			writeJSON(w, http.StatusOK, map[string]any{"devices": []map[string]any{{
				"id": "device-1", "local_phone": "+12025550124",
			}}})
		default:
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

	request := httptest.NewRequest(http.MethodGet, "/api/calls/devices", nil)
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
	if len(payload.Devices) != 1 || payload.Devices[0]["local_phone"] != "+12025550124" {
		t.Fatalf("unexpected devices payload: %#v", payload.Devices)
	}
}

func TestCallDialRejectionAndUncertainResult(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		payload   map[string]any
		want      int
		state     string
		errorText string
	}{
		{"modem error", 200, map[string]any{"status": "ok", "response": "\r\nERROR\r\n"}, 502, "failed", "ERROR"},
		{"extended modem error", 200, map[string]any{"status": "ok", "response": "\r\n+CME ERROR: 3\r\n"}, 502, "failed", "+CME ERROR: 3"},
		{"busy serial port", 500, map[string]any{"status": "error", "message": "打开 AT 端口 /dev/ttyUSB2 失败: Serial port busy"}, 500, "failed", "Serial port busy"},
		{"timeout may have dialed", 500, map[string]any{"status": "error", "message": "timeout"}, 202, "dialing", ""},
		{"missing final is uncertain", 200, map[string]any{"status": "ok", "response": "ATD+12025550123;\r\n"}, 202, "dialing", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialCount := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/devices" {
					writeJSON(w, 200, map[string]any{"devices": []map[string]any{{"id": "device-1", "name": "Test modem", "running": true, "vowifi_enabled": false}}})
					return
				}
				if r.URL.Path != "/api/devices/device-1/actions/at" {
					http.NotFound(w, r)
					return
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if body["cmd"] == "AT+CLCC" {
					writeJSON(w, 200, map[string]any{"response": "\r\nOK\r\n"})
					return
				}
				dialCount++
				writeJSON(w, tt.status, tt.payload)
			}))
			defer upstream.Close()
			manager, err := newCallManager(":memory:", filepath.Join(t.TempDir(), "recordings"))
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			app := &application{upstream: strings.TrimPrefix(upstream.URL, "http://"), calls: manager}
			dial := func() *httptest.ResponseRecorder {
				request := httptest.NewRequest(http.MethodPost, "/api/calls/dial", strings.NewReader(`{"device_id":"device-1","number":"+12025550123"}`))
				request.Header.Set("Authorization", "Bearer test-token")
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				app.routes().ServeHTTP(response, request)
				return response
			}
			response := dial()
			if response.Code != tt.want || (tt.errorText != "" && !strings.Contains(response.Body.String(), tt.errorText)) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			history, err := manager.history("device-1", 10)
			if err != nil || len(history) != 1 || history[0].State != tt.state || (history[0].EndedAt != nil) != (tt.state == "failed") {
				t.Fatalf("history = %#v, %v", history, err)
			}
			if tt.want == 202 {
				repeat := dial()
				if repeat.Code != http.StatusConflict || dialCount != 1 {
					t.Fatalf("uncertain dial was repeated: %d %s (commands %d)", repeat.Code, repeat.Body.String(), dialCount)
				}
			}
		})
	}
}

func TestCallsStatusPreservesCoreMessage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 500, map[string]any{"status": "error", "message": "打开 AT 端口 /dev/ttyUSB2 失败: Serial port busy"})
	}))
	defer upstream.Close()
	app := &application{upstream: strings.TrimPrefix(upstream.URL, "http://")}
	request := httptest.NewRequest(http.MethodGet, "/api/calls/status", nil)
	_, err := app.currentVoiceCalls(request, "device-1")
	if err == nil || !strings.Contains(err.Error(), "Serial port busy") {
		t.Fatalf("lost upstream error: %v", err)
	}
	if callATExplicitOK("ERROR\r\nOK\r\n") {
		t.Fatal("mixed rejected response was accepted")
	}
}

func TestCallsRejectUnsupportedTransportWithoutSendingAT(t *testing.T) {
	for _, tc := range []struct {
		transport string
		status    int
	}{{"vowifi", http.StatusNotImplemented}, {"unknown", http.StatusBadRequest}} {
		t.Run(tc.transport, func(t *testing.T) {
			atRequests := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/devices" {
					writeJSON(w, http.StatusOK, map[string]any{"devices": []map[string]any{{"id": "device-1", "running": true, "vowifi_enabled": true}}})
					return
				}
				atRequests++
				http.NotFound(w, r)
			}))
			defer upstream.Close()
			manager, err := newCallManager(":memory:", filepath.Join(t.TempDir(), "recordings"))
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			app := &application{upstream: strings.TrimPrefix(upstream.URL, "http://"), calls: manager}
			request := httptest.NewRequest(http.MethodPost, "/api/calls/dial", strings.NewReader(`{"device_id":"device-1","number":"+12025550123","transport":"`+tc.transport+`"}`))
			request.Header.Set("Authorization", "Bearer test-token")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			app.routes().ServeHTTP(response, request)
			if response.Code != tc.status || atRequests != 0 {
				t.Fatalf("status=%d AT requests=%d body=%s", response.Code, atRequests, response.Body.String())
			}
		})
	}
}
