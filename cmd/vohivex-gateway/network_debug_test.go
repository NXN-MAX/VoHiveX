package main

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNetworkDebugDevicesOnlyIncludesConnectedCellularRows(t *testing.T) {
	rows := []map[string]any{
		{"id": "ready", "name": "Ready modem", "running": true, "healthy": true, "network_enabled": true, "network_connected": true, "interface": "wwan0"},
		{"id": "disabled", "running": true, "healthy": true, "network_enabled": false, "network_connected": true, "interface": "wwan1"},
		{"id": "searching", "running": true, "healthy": true, "network_enabled": true, "network_connected": false, "interface": "wwan2"},
		{"id": "missing-interface", "running": true, "healthy": true, "network_enabled": true, "network_connected": true},
	}
	devices := networkDebugDevices(rows)
	if len(devices) != 1 || devices[0]["id"] != "ready" || devices[0]["interface"] != "wwan0" {
		t.Fatalf("unexpected eligible devices: %#v", devices)
	}
}

func TestNetworkDebugRejectsPrivateAndLocalTargets(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1/",
		"http://[::1]/",
		"http://192.168.1.10/",
		"http://localhost/",
		"http://router.local/",
		"http://nas/",
	}
	for _, raw := range blocked {
		target, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateNetworkDebugURL(target); err == nil {
			t.Fatalf("private target was accepted: %s", raw)
		}
	}
	publicTarget, _ := url.Parse("https://example.com/path")
	if err := validateNetworkDebugURL(publicTarget); err != nil {
		t.Fatalf("public target was rejected: %v", err)
	}
	publicIPv6Target, _ := url.Parse("https://[2606:4700:4700::1111]/")
	if err := validateNetworkDebugURL(publicIPv6Target); err != nil {
		t.Fatalf("public IPv6 target was rejected: %v", err)
	}
	if publicNetworkDebugAddress(netip.MustParseAddr("100.64.0.1")) {
		t.Fatal("carrier-grade NAT address was accepted")
	}
}

func TestNetworkDebugHTMLRoutesResourcesThroughSession(t *testing.T) {
	base, _ := url.Parse("https://example.com/docs/page.html")
	raw := []byte(`<!doctype html><html><head><meta http-equiv="Content-Security-Policy" content="default-src 'none'"><style>.hero{background:url('/image.png')}</style></head><body><img src="../logo.png"><a href="https://cdn.example.net/file?q=1" target="_blank">Link</a></body></html>`)
	rewritten, err := rewriteNetworkDebugHTML(raw, "text/html; charset=utf-8", base, "session-test")
	if err != nil {
		t.Fatal(err)
	}
	text := string(rewritten)
	for _, expected := range []string{
		"/api/network-debug/view/session-test/https/ZXhhbXBsZS5jb20/image.png",
		"/api/network-debug/view/session-test/https/ZXhhbXBsZS5jb20/logo.png",
		"/api/network-debug/view/session-test/https/Y2RuLmV4YW1wbGUubmV0/file?q=1",
		`target="_self"`,
		"x-vohivex-removed-csp",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("rewritten HTML is missing %q:\n%s", expected, text)
		}
	}
}

func TestNetworkDebugViewEntryAndStats(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/network-debug/view/session-id?url=https%3A%2F%2Fexample.com%2Fpath%3Fa%3D1", nil)
	id, target, entry, err := parseNetworkDebugView(request)
	if err != nil || id != "session-id" || !entry || target.String() != "https://example.com/path?a=1" {
		t.Fatalf("id=%q target=%v entry=%v err=%v", id, target, entry, err)
	}
	start := time.Unix(1_700_000_000, 0)
	session := &networkDebugSession{id: id, startedAt: start, lastActivity: start, lastSampleAt: start}
	session.addTraffic(3_000, false)
	session.addTraffic(1_000, true)
	snapshot := session.snapshot(start.Add(2 * time.Second))
	if snapshot["total_bytes"] != uint64(4_000) || snapshot["current_speed_bps"] != float64(2_000) {
		t.Fatalf("unexpected stats: %#v", snapshot)
	}
}
