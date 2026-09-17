package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"
)

type egressCacheEntry struct {
	key     string
	checked time.Time
	value   map[string]any
}

var egressCache struct {
	sync.Mutex
	entry egressCacheEntry
}

func nonCellularDefaultRoute() bool {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	first := true
	best := ""
	bestMetric := int64(1 << 62)
	ambiguous := false
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 || fields[1] != "00000000" {
			continue
		}
		var metric int64
		fmt.Sscanf(fields[6], "%d", &metric)
		if metric < bestMetric {
			bestMetric = metric
			best = fields[0]
			ambiguous = false
		} else if metric == bestMetric && fields[0] != best {
			ambiguous = true
		}
	}
	if best == "" || ambiguous {
		return false
	}
	for _, prefix := range []string{"wwan", "ppp", "usb", "rmnet"} {
		if strings.HasPrefix(best, prefix) {
			return false
		}
	}
	return true
}

func readFull(conn net.Conn, buffer []byte) error { _, err := io.ReadFull(conn, buffer); return err }

func socks5Dial(ctx context.Context, _ string, address string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: 6 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", proxySOCKSPort))
	if err != nil {
		return nil, err
	}
	fail := func(e error) (net.Conn, error) { conn.Close(); return nil, e }
	if _, err = conn.Write([]byte{5, 1, 0}); err != nil {
		return fail(err)
	}
	reply := make([]byte, 2)
	if err = readFull(conn, reply); err != nil || reply[0] != 5 || reply[1] != 0 {
		return fail(fmt.Errorf("SOCKS authentication failed"))
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return fail(err)
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil {
		return fail(err)
	}
	request := []byte{5, 1, 0, 3, byte(len(host))}
	request = append(request, []byte(host)...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	request = append(request, portBytes...)
	if _, err = conn.Write(request); err != nil {
		return fail(err)
	}
	header := make([]byte, 4)
	if err = readFull(conn, header); err != nil || header[0] != 5 || header[1] != 0 {
		return fail(fmt.Errorf("SOCKS connection failed"))
	}
	size := 0
	switch header[3] {
	case 1:
		size = 4
	case 4:
		size = 16
	case 3:
		length := []byte{0}
		if err = readFull(conn, length); err != nil {
			return fail(err)
		}
		size = int(length[0])
	default:
		return fail(fmt.Errorf("invalid SOCKS address"))
	}
	tail := make([]byte, size+2)
	if err = readFull(conn, tail); err != nil {
		return fail(err)
	}
	return conn, nil
}

func queryPublicIP(proxy bool) map[string]any {
	if !proxy && !nonCellularDefaultRoute() {
		return map[string]any{"ip": nil, "error": "无法确认设备默认网络，未执行查询"}
	}
	hosts := []string{"api64.ipify.org", "api.ipify.org"}
	for _, host := range hosts {
		transport := &http.Transport{Proxy: nil}
		if proxy {
			transport.DialContext = socks5Dial
		}
		client := http.Client{Timeout: 8 * time.Second, Transport: transport}
		request, _ := http.NewRequest(http.MethodGet, "https://"+host+"/", nil)
		request.Header.Set("User-Agent", "VoHiveX-IP/2.1")
		response, err := client.Do(request)
		if err != nil {
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 257))
		response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || len(raw) > 256 {
			continue
		}
		address, err := netip.ParseAddr(strings.TrimSpace(string(raw)))
		if err != nil || !address.IsGlobalUnicast() || address.IsPrivate() {
			continue
		}
		return map[string]any{"ip": address.String(), "error": nil, "checked_at": time.Now().Unix()}
	}
	return map[string]any{"ip": nil, "error": "查询失败或超时"}
}

func (app *application) egressSnapshot(force bool) map[string]any {
	app.proxy.mu.Lock()
	key := fmt.Sprintf("%d:%s:%t:%t", app.proxy.state.Version, app.proxy.state.Selected, app.proxy.state.Enabled, app.proxy.processRunningLocked())
	enabled := app.proxy.state.Enabled && app.proxy.state.Selected != ""
	versionNumber := app.proxy.state.Version
	selected := app.proxy.state.Selected
	app.proxy.mu.Unlock()
	egressCache.Lock()
	defer egressCache.Unlock()
	ttl := 60 * time.Second
	if force {
		ttl = 5 * time.Second
	}
	if egressCache.entry.key == key && time.Since(egressCache.entry.checked) < ttl {
		return cloneMap(egressCache.entry.value)
	}
	nas := queryPublicIP(false)
	proxy := map[string]any{"ip": nil, "error": "未连接"}
	if enabled {
		proxy = queryPublicIP(true)
	}
	value := map[string]any{"nas": nas, "proxy": proxy, "selected": selected, "version": versionNumber}
	egressCache.entry = egressCacheEntry{key: key, checked: time.Now(), value: value}
	return cloneMap(value)
}

func cloneMap(value map[string]any) map[string]any {
	result := map[string]any{}
	for key, item := range value {
		if nested, ok := item.(map[string]any); ok {
			result[key] = cloneMap(nested)
		} else {
			result[key] = item
		}
	}
	return result
}

func anySlice(value any) []any        { items, _ := value.([]any); return items }
func anyMap(value any) map[string]any { item, _ := value.(map[string]any); return item }

func deviceEgress(devices []map[string]any, rulesValue, proxiesValue any, snapshot map[string]any) map[string]any {
	rules := proxyRowsAsMaps(rulesValue)
	proxies := proxyRows(proxiesValue)
	result := map[string]any{}
	for _, device := range devices {
		id := fmt.Sprint(device["id"])
		item := map[string]any{"ip": nil, "label": "IP 地址", "error": "无法确认出口规则"}
		vowifi, _ := device["vowifi_enabled"].(bool)
		if !vowifi {
			var ip any
			for _, key := range []string{"public_ip", "public_ipv6", "private_ip", "private_ipv6"} {
				if value := device[key]; value != nil && fmt.Sprint(value) != "" {
					ip = value
					break
				}
			}
			item = map[string]any{"ip": ip, "label": "IP 地址", "error": func() any {
				if ip == nil {
					return "尚未获取到 SIM 卡 IP"
				}
				return nil
			}()}
		} else {
			mcc := fmt.Sprint(anyMap(device["modem"])["native_mcc"])
			matches := []map[string]any{}
			for _, rule := range rules {
				enabled, _ := rule["enabled"].(bool)
				if !enabled {
					continue
				}
				for _, candidate := range anySlice(rule["mccs"]) {
					if fmt.Sprint(candidate) == mcc {
						matches = append(matches, rule)
					}
				}
			}
			if len(matches) <= 1 {
				var matched map[string]any
				if len(matches) == 1 {
					proxyID := fmt.Sprint(matches[0]["upstream_proxy_id"])
					for _, candidate := range proxies {
						enabled, _ := candidate["enabled"].(bool)
						if enabled && fmt.Sprint(candidate["id"]) == proxyID {
							matched = candidate
						}
					}
				}
				if matched == nil {
					item = cloneMap(anyMap(snapshot["nas"]))
					item["label"] = "IP 地址"
				} else if matched["id"] == builtInProxyID {
					item = cloneMap(anyMap(snapshot["proxy"]))
					item["label"] = "代理 IP"
				} else {
					item = map[string]any{"ip": nil, "label": "代理 IP", "error": "此代理出口尚未查询"}
				}
			}
		}
		result[id] = item
	}
	return result
}
