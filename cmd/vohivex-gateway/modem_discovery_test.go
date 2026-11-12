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
		configured := []map[string]any{{"id": "configured-modem", key: row[key], "modem_imei": strings.Repeat("2", 15)}}
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
