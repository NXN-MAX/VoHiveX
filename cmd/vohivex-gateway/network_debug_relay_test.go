package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNetworkRelayRequiresPrivateConfigAndAuthentication(t *testing.T) {
	p := filepath.Join(t.TempDir(), "relay.json")
	t.Setenv("VOHIVEX_NETWORK_RELAY_CONFIG", p)
	v := map[string]any{"relays": []networkRelay{{ID: "mac", Name: "Mac", Interface: "remote", ProxyURL: "http://test:secret@127.0.0.1:19490"}}}
	b, _ := json.Marshal(v)
	os.WriteFile(p, b, 0600)
	if len(networkRelays()) != 1 {
		t.Fatal("valid relay not loaded")
	}
	os.Chmod(p, 0644)
	if len(networkRelays()) != 0 {
		t.Fatal("public credentials file accepted")
	}
	for _, s := range []string{"http://example.com:80", "http://user:pass@example.com:80", "http://127.0.0.1:80", "https://a:b@127.0.0.1:80"} {
		if _, e := (networkRelay{ProxyURL: s}).proxy(); e == nil {
			t.Fatal("unsafe relay accepted")
		}
	}
}
func TestNetworkRelayUsesAuthenticatedProxyWithoutDirectFallback(t *testing.T) {
	seen := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Host == "example.com" && r.Header.Get("Proxy-Authorization") == "Basic dGVzdDpzZWNyZXQ="
		w.Write([]byte("via modem"))
	}))
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	u.User = url.UserPassword("test", "secret")
	tr, e := (networkRelay{ProxyURL: u.String()}).transport()
	if e != nil {
		t.Fatal(e)
	}
	client := &http.Client{Transport: tr}
	response, e := client.Get("http://example.com/")
	if e != nil {
		t.Fatal(e)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !seen || string(body) != "via modem" {
		t.Fatal("did not use authenticated relay")
	}
	tr.CloseIdleConnections()
	proxy.Close()
	if _, e = client.Get("http://example.com/"); e == nil {
		t.Fatal("silently fell back to direct route")
	}
	// Never serialize a configured relay's password through the device endpoint.
	app := &application{}
	p := filepath.Join(t.TempDir(), "relay.json")
	t.Setenv("VOHIVEX_NETWORK_RELAY_CONFIG", p)
	os.WriteFile(p, []byte(`{"relays":[]}`), 0600)
	b, _ := json.Marshal(app.networkDebugDeviceRows(nil))
	if strings.Contains(string(b), "secret") {
		t.Fatal("secret exposed")
	}
}
