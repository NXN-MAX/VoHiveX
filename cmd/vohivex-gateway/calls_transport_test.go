package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func callRouteTestApp(t *testing.T, device map[string]any, clcc *string) (*application, *[]string) {
	t.Helper()
	commands := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/devices":
			writeJSON(w, 200, map[string]any{"devices": []map[string]any{device}})
		case "/api/devices/modem-1/actions/at":
			var body struct {
				Command string `json:"cmd"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			commands = append(commands, body.Command)
			response := "OK"
			if body.Command == "AT+CLCC" {
				response = *clcc
			}
			writeJSON(w, 200, map[string]any{"response": response})
		default:
			t.Errorf("unexpected upstream request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	manager, err := newCallManager(":memory:", filepath.Join(t.TempDir(), "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	return &application{upstream: strings.TrimPrefix(server.URL, "http://"), calls: manager}, &commands
}

func routeRequest(app *application, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer test-token")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	return w
}

func TestCallTransportFollowsPolicyWithoutFallback(t *testing.T) {
	for _, tc := range []struct {
		name      string
		policy    map[string]any
		requested string
		status    int
		commands  []string
	}{
		{"cellular data off", map[string]any{"vowifi_enabled": false, "network_enabled": false}, "auto", 200, []string{"AT+CLCC", "ATD+12025550123;"}},
		{"omitted transport", map[string]any{"vowifi_enabled": false}, "", 200, []string{"AT+CLCC", "ATD+12025550123;"}},
		{"vowifi registered", map[string]any{"vowifi_enabled": true, "vowifi_active": true}, "auto", 501, nil},
		{"vowifi reconnecting", map[string]any{"vowifi_enabled": true, "vowifi_active": false}, "", 501, nil},
		{"still active after disable", map[string]any{"vowifi_enabled": false, "vowifi_active": true}, "auto", 501, nil},
		{"runtime vowifi", map[string]any{"vowifi_runtime": map[string]any{"dataplane_mode": "vowifi"}}, "auto", 501, nil},
		{"missing policy", map[string]any{}, "auto", 503, nil},
		{"invalid policy", map[string]any{"vowifi_enabled": "false"}, "auto", 503, nil},
		{"stale cellular request", map[string]any{"vowifi_enabled": true}, "cellular", 409, nil},
		{"stale vowifi request", map[string]any{"vowifi_enabled": false}, "vowifi", 409, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			device := map[string]any{"id": "modem-1", "running": true}
			for k, v := range tc.policy {
				device[k] = v
			}
			clcc := "OK"
			app, commands := callRouteTestApp(t, device, &clcc)
			payload, _ := json.Marshal(map[string]any{"device_id": "modem-1", "number": "+12025550123", "transport": tc.requested})
			w := routeRequest(app, "POST", "/api/calls/dial", string(payload))
			if w.Code != tc.status || strings.Join(*commands, "|") != strings.Join(tc.commands, "|") {
				t.Fatalf("status=%d commands=%v body=%s", w.Code, *commands, w.Body.String())
			}
			history, err := app.calls.history("modem-1", 10)
			if err != nil || (tc.status != 200 && len(history) != 0) {
				t.Fatalf("failed route created call history: %v %v", history, err)
			}
		})
	}
}

func TestCallStatusAndActionsDoNotQueryCellularInVoWiFiMode(t *testing.T) {
	device := map[string]any{"id": "modem-1", "running": true, "vowifi_enabled": true}
	clcc := "OK"
	app, commands := callRouteTestApp(t, device, &clcc)
	w := routeRequest(app, "GET", "/api/calls/status?device_id=modem-1", "")
	var result struct {
		callRoute
		Calls []callHistoryItem `json:"calls"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || result.Transport != "vowifi" || result.Available || len(result.Calls) != 0 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("bad mode status: %s", w.Body.String())
	}
	for _, action := range []string{"answer", "hangup", "reject", "dtmf"} {
		w = routeRequest(app, "POST", "/api/calls/"+action, `{"device_id":"modem-1","transport":"auto","digits":"1"}`)
		if w.Code != 501 {
			t.Fatalf("%s status=%d", action, w.Code)
		}
	}
	if len(*commands) != 0 {
		t.Fatalf("VoWiFi issued native commands: %v", *commands)
	}
}

func TestCallModeRecheckedAfterBrowserSnapshot(t *testing.T) {
	device := map[string]any{"id": "modem-1", "running": true, "vowifi_enabled": false}
	clcc := "OK"
	app, commands := callRouteTestApp(t, device, &clcc)
	if w := routeRequest(app, "GET", "/api/calls/status?device_id=modem-1", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	device["vowifi_enabled"] = true
	w := routeRequest(app, "POST", "/api/calls/dial", `{"device_id":"modem-1","transport":"auto","number":"+12025550123"}`)
	if w.Code != 501 || !reflect.DeepEqual(*commands, []string{"AT+CLCC"}) {
		t.Fatalf("stale route: %d %v", w.Code, *commands)
	}
}

func TestActiveNativeCallKeepsTransportAfterPolicyChange(t *testing.T) {
	device := map[string]any{"id": "modem-1", "running": true, "vowifi_enabled": true}
	clcc := `+CLCC: 1,0,0,0,0,"+12025550123",145` + "\r\nOK"
	app, commands := callRouteTestApp(t, device, &clcc)
	active, err := app.calls.reconcile(device, []parsedVoiceCall{{Index: 1, State: "active", Direction: "outgoing", Number: "+12025550123"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	w := routeRequest(app, "GET", "/api/calls/status?device_id=modem-1", "")
	var route callRoute
	_ = json.Unmarshal(w.Body.Bytes(), &route)
	if w.Code != 200 || route.Transport != "cellular" || route.DeviceTransport != "vowifi" || !route.Available {
		t.Fatalf("lost original route: %s", w.Body.String())
	}
	payload, _ := json.Marshal(map[string]any{"device_id": "modem-1", "call_id": active[0].ID, "transport": "cellular"})
	w = routeRequest(app, "POST", "/api/calls/hangup", string(payload))
	if w.Code != 200 || !reflect.DeepEqual(*commands, []string{"AT+CLCC", "AT+CLCC", "ATH"}) {
		t.Fatalf("wrong active route: %d %v", w.Code, *commands)
	}
	clcc = "OK"
	w = routeRequest(app, "GET", "/api/calls/status?device_id=modem-1", "")
	_ = json.Unmarshal(w.Body.Bytes(), &route)
	if route.Transport != "vowifi" || route.Available {
		t.Fatalf("did not follow policy after end: %s", w.Body.String())
	}
	before := len(*commands)
	w = routeRequest(app, "POST", "/api/calls/hangup", string(payload))
	if w.Code != 409 || len(*commands) != before {
		t.Fatal("ended cellular call was reused")
	}
}
