package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run explicitly in a container with access to the existing core and its
// configuration. No workers or HTTP listeners are started. This test never
// calls dial/answer/end, changes radio settings or opens the QMI device.
func TestLiveCallDiagnosticsReadOnly(t *testing.T) {
	if os.Getenv("VOHIVEX_LIVE_CALL_TEST") != "1" {
		t.Skip("live modem diagnostic requires explicit opt-in")
	}
	deviceID := os.Getenv("VOHIVEX_CALL_TEST_DEVICE")
	configPath := os.Getenv("CONFIG_PATH")
	upstream := os.Getenv("VOHIVEX_CALL_TEST_UPSTREAM")
	if deviceID == "" || configPath == "" || !strings.HasPrefix(upstream, "127.0.0.1:") {
		t.Fatal("device, existing configuration and loopback core address are required")
	}
	manager, err := newCallManager(":memory:", filepath.Join(t.TempDir(), "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	app := &application{configPath: configPath, upstream: upstream, calls: manager}
	r := httptest.NewRequest(http.MethodGet, "/api/calls/diagnostics?device_id="+deviceID, nil)
	token, err := app.coreLoginRequest(r)
	if err != nil {
		t.Fatal("could not authenticate with existing core")
	}
	unauthenticated := httptest.NewRecorder()
	app.callsAPI(unauthenticated, r)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatal("diagnostics did not require authentication")
	}
	r.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	app.callsAPI(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("live diagnostics HTTP %d", w.Code)
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
	if !result.ReadOnly || w.Header().Get("Cache-Control") != "no-store" || result.Control["cellular"] != "at" {
		t.Fatal("unexpected diagnostic contract")
	}
	if result.Modem["radio_function"].Status != "available" || result.Host["status"] != "available" {
		t.Fatal("live radio or host observation was unavailable")
	}
	// Parsed fields are bounded protocol values; no raw body or credentials.
	t.Logf("modem=%+v host=%+v control=%+v", result.Modem, result.Host, result.Control)
}
