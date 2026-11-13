package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func discoveryFixture(t *testing.T) *modemDiscovery {
	t.Helper()
	root := t.TempDir()
	d := newModemDiscovery(filepath.Join(root, "sys"), filepath.Join(root, "dev"))
	write := func(path, value string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	usb := filepath.Join(d.sysRoot, "bus/usb/devices/1-1")
	for name, value := range map[string]string{"idVendor": "2ca3", "idProduct": "4006", "bConfigurationValue": "1"} {
		write(filepath.Join(usb, name), value)
	}
	for suffix, value := range map[string]string{
		":1.2/bInterfaceClass": "ff", ":1.4/bInterfaceClass": "02", ":1.4/bInterfaceSubClass": "06", ":1.5/bInterfaceClass": "0a",
		":1.2/ttyUSB2/tty/ttyUSB2": "", ":1.0/tty/ttyUSB0": "", ":1.3/tty/ttyUSB3": "",
	} {
		write(usb+suffix, value)
	}
	for suffix, driver := range map[string]string{":1.2": "option", ":1.4": "cdc_ether", ":1.5": "cdc_ether"} {
		if err := os.Symlink("/fixture/drivers/"+driver, usb+suffix+"/driver"); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(usb+":1.4/net/wwan0", 0o700); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(d.devRoot, "ttyUSB2"), "")
	d.probe = func(_ context.Context, port string) (string, error) {
		if port != filepath.Join(d.devRoot, "ttyUSB2") {
			t.Errorf("unexpected AT port: %s", port)
		}
		return strings.Repeat("1", 15), nil
	}
	return d
}

func TestECMFallbackIdentityAndDeduplication(t *testing.T) {
	d := discoveryFixture(t)
	rows := d.appendECM(context.Background(), []map[string]any{{"usb_path": "2-1", "mode": "qmi", "custom": true}}, nil, nil, true)
	if len(rows) != 2 || rows[0]["custom"] != true || rows[1]["imei"] != strings.Repeat("1", 15) || rows[1]["mode"] != "ecm" || rows[1]["net_interface"] != "wwan0" {
		t.Fatalf("unexpected discovery: %#v", rows)
	}
	for _, key := range []string{"usb_path", "at_port"} {
		known := map[string]any{key: rows[1][key], "upstream": true}
		got := d.appendECM(context.Background(), []map[string]any{known}, nil, nil, true)
		if len(got) != 1 || got[0]["upstream"] != true {
			t.Fatalf("duplicate %s: %#v", key, got)
		}
	}
}

func TestECMDiscoveryDoesNotProbeConfiguredOrPassiveDevices(t *testing.T) {
	d := discoveryFixture(t)
	d.probe = func(context.Context, string) (string, error) {
		t.Fatal("must not probe an owned or passive port")
		return "", nil
	}
	row := d.ecmCandidates()[0]
	for _, key := range []string{"usb_path", "at_port"} {
		configured := []map[string]any{{"id": "configured-modem", key: row[key], "modem_imei": strings.Repeat("2", 15), "_runtime_running": true}}
		got := d.appendECM(context.Background(), nil, configured, nil, true)[0]
		if got["configured"] != true || got["configured_id"] != "configured-modem" || got["imei"] != strings.Repeat("2", 15) {
			t.Fatalf("configured: %#v", got)
		}
	}
	if got := d.appendECM(context.Background(), nil, nil, nil, false)[0]; got["imei"] != "" {
		t.Fatalf("passive discovery: %#v", got)
	}
	if got := d.appendECM(context.Background(), nil, nil, errors.New("bad config"), true)[0]; got["discovery_error"] != "configuration_unavailable" {
		t.Fatalf("bad config: %#v", got)
	}
}

func TestECMBusyAndMissingDriverRemainVisible(t *testing.T) {
	d := discoveryFixture(t)
	d.probe = func(context.Context, string) (string, error) { return "", errSerialBusy }
	row := d.appendECM(context.Background(), nil, nil, nil, true)[0]
	if row["discovery_error"] != "serial_busy" || row["degraded"] != true || !strings.Contains(row["degraded_reason"].(string), "ModemManager") {
		t.Fatalf("busy candidate hidden: %#v", row)
	}
	d.probe = func(context.Context, string) (string, error) {
		t.Fatal("must not probe unavailable port")
		return "", nil
	}
	if err := os.Remove(filepath.Join(d.devRoot, "ttyUSB2")); err != nil {
		t.Fatal(err)
	}
	row = d.appendECM(context.Background(), nil, nil, nil, true)[0]
	if row["discovery_error"] != "missing_at_port" {
		t.Fatalf("missing port: %#v", row)
	}
}

func TestECMDiscoveryRejectsOtherUSBLayouts(t *testing.T) {
	for _, change := range []string{"idProduct", ":1.4/bInterfaceClass"} {
		d := discoveryFixture(t)
		path := filepath.Join(d.sysRoot, "bus/usb/devices/1-1")
		if strings.HasPrefix(change, ":") {
			path += change
		} else {
			path = filepath.Join(path, change)
		}
		if err := os.WriteFile(path, []byte("ff"), 0o600); err != nil {
			t.Fatal(err)
		}
		if rows := d.ecmCandidates(); len(rows) != 0 {
			t.Fatalf("unsupported device matched: %#v", rows)
		}
	}
}

func TestECMDiscoverySerializesProbesAndHonorsCancellation(t *testing.T) {
	d := discoveryFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	d.probe = func(context.Context, string) (string, error) {
		calls.Add(1)
		close(entered)
		<-release
		return strings.Repeat("1", 15), nil
	}
	done := make(chan struct{})
	go func() { defer close(done); d.appendECM(context.Background(), nil, nil, nil, true) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("probe did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	row := d.appendECM(ctx, nil, nil, nil, true)[0]
	close(release)
	<-done
	if calls.Load() != 1 || row["discovery_error"] != "discovery_timeout" {
		t.Fatalf("concurrent probe/cancellation: calls=%d row=%#v", calls.Load(), row)
	}
}

func TestDiscoveryHTTPAuthAndUpstreamFailures(t *testing.T) {
	for _, scenario := range []struct {
		name, body                        string
		authStatus, discoveryStatus, want int
		token                             bool
	}{
		{"anonymous", `{"devices":[]}`, 200, 200, 401, false},
		{"expired", `{"devices":[]}`, 401, 200, 401, true},
		{"upstream-error", `{"error":"unavailable"}`, 200, 503, 503, true},
		{"bad-shape", `{"devices":["invalid"]}`, 200, 200, 502, true},
		{"ecm-fallback", `{"devices":[],"extra":"preserved"}`, 200, 200, 200, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			d := discoveryFixture(t)
			calls := 0
			d.probe = func(context.Context, string) (string, error) { calls++; return strings.Repeat("1", 15), nil }
			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/devices" {
					writeJSON(w, scenario.authStatus, map[string]any{"devices": []any{}})
					return
				}
				if r.URL.Path != "/api/devices/discovered" || r.URL.Query().Get("with_imei") != "1" {
					t.Errorf("unexpected upstream request %s", r.URL)
				}
				w.WriteHeader(scenario.discoveryStatus)
				_, _ = w.Write([]byte(scenario.body))
			}))
			defer core.Close()
			app := &application{upstream: strings.TrimPrefix(core.URL, "http://"), configPath: coreAuthTestConfig(t), discovery: d}
			req := httptest.NewRequest(http.MethodGet, "/api/devices/discovered?with_imei=1", nil)
			if scenario.token {
				req.Header.Set("Authorization", "Bearer test-token")
			}
			w := httptest.NewRecorder()
			app.discoveredDevicesAPI(w, req)
			if w.Code != scenario.want {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if scenario.want != 200 && calls != 0 {
				t.Fatal("probed port after auth/upstream failure")
			}
			if scenario.want == 200 {
				var data map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
					t.Fatal(err)
				}
				if calls != 1 || len(deviceRows(data)) != 1 || data["extra"] != "preserved" {
					t.Fatalf("fallback response: %s", w.Body.String())
				}
			}
		})
	}
}

func TestECMMatchesSavedIMEIWithoutTransientPaths(t *testing.T) {
	d := discoveryFixture(t)
	configured := []map[string]any{{"id": "saved-modem", "modem_imei": strings.Repeat("1", 15)}}
	row := d.appendECM(context.Background(), nil, configured, nil, true)[0]
	if row["configured"] != true || row["configured_id"] != "saved-modem" {
		t.Fatalf("saved identity was not recognized: %#v", row)
	}
}

func TestECMDoesNotTrustAReusedUSBPathForIdentity(t *testing.T) {
	d := discoveryFixture(t)
	configured := []map[string]any{{"id": "previous-modem", "usb_path": "1-1", "modem_imei": strings.Repeat("2", 15)}}
	row := d.appendECM(context.Background(), nil, configured, nil, true)[0]
	if row["configured"] != false || row["imei"] != strings.Repeat("1", 15) {
		t.Fatalf("stale path hid a replacement device: %#v", row)
	}
}

func TestECMDuplicateIdentityFailsClosed(t *testing.T) {
	d := discoveryFixture(t)
	configured := []map[string]any{{"id": "first", "modem_imei": strings.Repeat("1", 15)}, {"id": "second", "modem_imei": strings.Repeat("1", 15)}}
	row := d.appendECM(context.Background(), nil, configured, nil, true)[0]
	if row["discovery_error"] != "configuration_conflict" || row["degraded"] != true {
		t.Fatalf("ambiguous binding accepted: %#v", row)
	}
}

func TestECMCanceledProbeDoesNotAcquireFreePort(t *testing.T) {
	d := discoveryFixture(t)
	d.probe = func(context.Context, string) (string, error) {
		t.Fatal("canceled request probed a port")
		return "", nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.probeIMEI(ctx, "unused"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestDiscoveryUsesLiveBindingAndPreservesConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name            string
		status          int
		missingBindings bool
	}{
		{"live binding", http.StatusOK, false},
		{"unavailable config", http.StatusServiceUnavailable, false},
		{"null bindings", http.StatusOK, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := discoveryFixture(t)
			d.probe = func(context.Context, string) (string, error) {
				t.Fatal("attempted to probe an active worker's port")
				return "", nil
			}
			path := coreAuthTestConfig(t)
			text := "web:\n  username: admin\n  password: admin\ndevices:\n  - id: owned-modem\n    modem_imei: '" + strings.Repeat("1", 15) + "'\n    device_backend: at\n"
			if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				switch r.URL.Path {
				case "/api/devices":
					writeJSON(w, 200, map[string]any{"devices": []any{map[string]any{"id": "owned-modem", "running": true}}})
				case "/api/devices/discovered":
					writeJSON(w, 200, map[string]any{"devices": []any{}})
				case "/api/devices/owned-modem/config":
					config := map[string]any{"id": "owned-modem", "modem_imei": strings.Repeat("1", 15), "device_backend": "at"}
					if !tc.missingBindings {
						config["usb_path"], config["at_port"] = "1-1", filepath.Join(d.devRoot, "ttyUSB2")
					}
					writeJSON(w, tc.status, map[string]any{"config": config})
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer core.Close()
			app := &application{configPath: path, upstream: strings.TrimPrefix(core.URL, "http://"), discovery: d}
			req := httptest.NewRequest(http.MethodGet, "/api/devices/discovered?with_imei=1", nil)
			req.Header.Set("Authorization", "Bearer test-token")
			w := httptest.NewRecorder()
			app.discoveredDevicesAPI(w, req)
			var data map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
			rows := deviceRows(data)
			if w.Code != 200 || len(rows) != 1 {
				t.Fatalf("bad response: %d %s", w.Code, w.Body.String())
			}
			if tc.status == 200 && !tc.missingBindings && (rows[0]["configured"] != true || rows[0]["configured_id"] != "owned-modem") {
				t.Fatalf("lost active binding: %#v", rows[0])
			}
			if (tc.status != 200 || tc.missingBindings) && rows[0]["discovery_error"] != "configuration_unavailable" {
				t.Fatalf("unsafe fallback on core config failure: %#v", rows[0])
			}
			stored, _ := os.ReadFile(path)
			if string(stored) != text {
				t.Fatal("discovery changed saved config")
			}
		})
	}
}

func TestNativeECMDiscoveryDoesNotPerformFallbackWork(t *testing.T) {
	d := discoveryFixture(t)
	d.probe = func(context.Context, string) (string, error) {
		t.Fatal("native result was probed again")
		return "", nil
	}
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/devices":
			writeJSON(w, 200, map[string]any{"devices": []any{map[string]any{"id": "owned-modem", "running": true}}})
		case "/api/devices/discovered":
			writeJSON(w, 200, map[string]any{"extra": "preserved", "devices": []any{map[string]any{"at_port": filepath.Join(d.devRoot, "ttyUSB2"), "configured": true, "configured_id": "owned-modem", "native": true}}})
		default:
			t.Errorf("unnecessary request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer core.Close()
	app := &application{configPath: "missing", upstream: strings.TrimPrefix(core.URL, "http://"), discovery: d}
	req := httptest.NewRequest(http.MethodGet, "/api/devices/discovered?with_imei=1", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	app.discoveredDevicesAPI(w, req)
	var data map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &data)
	if w.Code != 200 || data["extra"] != "preserved" || deviceRows(data)[0]["native"] != true {
		t.Fatalf("native response changed: %s", w.Body.String())
	}
}
