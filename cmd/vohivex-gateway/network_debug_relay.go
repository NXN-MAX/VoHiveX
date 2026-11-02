package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Optional dedicated instance of the container's bundled Mihomo. It serves
// only Network Debug; the ordinary subscription proxy and VoWiFi routing are
// unchanged. The operator supplies the private, separately scoped config.
func runNetworkDebugProxy(ctx context.Context) {
	dir := filepath.Join(getenv("VOHIVEX_DATA", "/app/data"), "network-debug-mihomo")
	path := filepath.Join(dir, "config.json")
	for {
		info, err := os.Stat(path)
		if err == nil && info.Mode().Perm()&0077 == 0 {
			cmd := exec.CommandContext(ctx, getenv("MIHOMO_BINARY", "/opt/vohivex/proxy/mihomo"), "-d", dir, "-f", path)
			_ = cmd.Run()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// Relay credentials are operator-managed private runtime configuration, never
// accepted from a browser, returned by the device list, or put in browse URLs.
type networkRelay struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Interface string `json:"interface"`
	ProxyURL  string `json:"proxy_url"`
}

func (r networkRelay) proxy() (*url.URL, error) {
	u, e := url.Parse(r.ProxyURL)
	if e != nil || u.Scheme != "http" || u.User == nil || u.Hostname() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("中继配置无效")
	}
	ip, e := netip.ParseAddr(u.Hostname())
	if e != nil || (!ip.IsPrivate() && !ip.IsLoopback()) {
		return nil, fmt.Errorf("中继地址必须是明确的局域网地址")
	}
	if _, _, e = net.SplitHostPort(u.Host); e != nil {
		return nil, fmt.Errorf("中继端口无效")
	}
	pw, ok := u.User.Password()
	if !ok || pw == "" || u.User.Username() == "" {
		return nil, fmt.Errorf("中继必须配置独立鉴权")
	}
	return u, nil
}
func networkRelays() []networkRelay {
	path := getenv("VOHIVEX_NETWORK_RELAY_CONFIG", filepath.Join(getenv("VOHIVEX_DATA", "/app/data"), "network-debug-relay.json"))
	info, e := os.Stat(path)
	if e != nil || info.Size() > 16384 || info.Mode().Perm()&0077 != 0 {
		return nil
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return nil
	}
	var config struct {
		Relays []networkRelay `json:"relays"`
	}
	if json.Unmarshal(b, &config) != nil || len(config.Relays) > 4 {
		return nil
	}
	var result []networkRelay
	seen := map[string]bool{}
	for _, r := range config.Relays {
		if r.ID == "" || r.Name == "" || len(r.ID) > 80 || seen[r.ID] {
			continue
		}
		if _, e := r.proxy(); e != nil {
			continue
		}
		seen[r.ID] = true
		result = append(result, r)
	}
	return result
}
func configuredNetworkRelay(id string) *networkRelay {
	for _, r := range networkRelays() {
		if r.ID == id {
			return &r
		}
	}
	return nil
}
func (r networkRelay) transport() (*http.Transport, error) {
	u, e := r.proxy()
	if e != nil {
		return nil, e
	}
	return &http.Transport{Proxy: http.ProxyURL(u), DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext, MaxIdleConns: 24, MaxIdleConnsPerHost: 6, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second}, nil
}
func (app *application) networkDebugDeviceRows(rows []map[string]any) []map[string]any {
	result := networkDebugDevices(rows)
	for _, r := range networkRelays() {
		u, _ := r.proxy()
		c, e := net.DialTimeout("tcp", u.Host, 300*time.Millisecond)
		if e != nil {
			continue
		}
		c.Close()
		result = append(result, map[string]any{"id": r.ID, "name": r.Name, "interface": r.Interface, "network_mode": "蜂窝中继", "relay": true})
	}
	return result
}
