package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	stdhtml "html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const (
	networkDebugSessionTTL     = 30 * time.Minute
	networkDebugRequestLimit   = int64(16 << 20)
	networkDebugTransformLimit = int64(12 << 20)
	networkDebugResponseLimit  = int64(128 << 20)
)

type networkDebugManager struct {
	mu       sync.Mutex
	sessions map[string]*networkDebugSession
	now      func() time.Time
}

type networkDebugSession struct {
	mu             sync.Mutex
	id             string
	owner          string
	deviceID       string
	deviceName     string
	interfaceName  string
	client         *http.Client
	startedAt      time.Time
	lastActivity   time.Time
	lastSampleAt   time.Time
	lastSampleByte uint64
	received       uint64
	sent           uint64
	requests       uint64
	active         int
	lastURL        string
	lastError      string
	lastStatus     int
}

type networkDebugTrafficReader struct {
	reader  io.ReadCloser
	session *networkDebugSession
	upload  bool
}

func (reader *networkDebugTrafficReader) Read(buffer []byte) (int, error) {
	count, err := reader.reader.Read(buffer)
	if count > 0 {
		reader.session.addTraffic(uint64(count), reader.upload)
	}
	return count, err
}

func (reader *networkDebugTrafficReader) Close() error { return reader.reader.Close() }

func newNetworkDebugManager() *networkDebugManager {
	return &networkDebugManager{sessions: map[string]*networkDebugSession{}, now: time.Now}
}

func (manager *networkDebugManager) Close() {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	for id, session := range manager.sessions {
		if transport, ok := session.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
		delete(manager.sessions, id)
	}
}

func (manager *networkDebugManager) purgeLocked(now time.Time) {
	for id, session := range manager.sessions {
		session.mu.Lock()
		expired := now.Sub(session.lastActivity) > networkDebugSessionTTL
		session.mu.Unlock()
		if !expired {
			continue
		}
		if transport, ok := session.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
		delete(manager.sessions, id)
	}
}

func randomNetworkDebugID() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func networkDebugOwner(request *http.Request) string {
	sum := sha256.Sum256([]byte(request.Header.Get("Authorization")))
	return hex.EncodeToString(sum[:])
}

func (manager *networkDebugManager) create(owner, deviceID, deviceName, interfaceName string) (*networkDebugSession, error) {
	transport, err := newCellularTransport(interfaceName)
	if err != nil {
		return nil, err
	}
	return manager.createTransport(owner, deviceID, deviceName, interfaceName, transport)
}

func (manager *networkDebugManager) createTransport(owner, deviceID, deviceName, interfaceName string, transport *http.Transport) (*networkDebugSession, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("创建浏览会话失败: %w", err)
	}
	id, err := randomNetworkDebugID()
	if err != nil {
		return nil, fmt.Errorf("创建浏览会话失败: %w", err)
	}
	now := manager.now()
	session := &networkDebugSession{
		id:            id,
		owner:         owner,
		deviceID:      deviceID,
		deviceName:    deviceName,
		interfaceName: interfaceName,
		startedAt:     now,
		lastActivity:  now,
		lastSampleAt:  now,
	}
	client := &http.Client{
		Transport: transport,
		Jar:       jar,
		Timeout:   90 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("网页重定向次数过多")
			}
			return validateNetworkDebugURL(request.URL)
		},
	}
	session.client = client
	manager.mu.Lock()
	manager.purgeLocked(now)
	manager.sessions[id] = session
	manager.mu.Unlock()
	return session, nil
}

func (manager *networkDebugManager) get(id string) *networkDebugSession {
	now := manager.now()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.purgeLocked(now)
	session := manager.sessions[id]
	if session != nil {
		session.mu.Lock()
		session.lastActivity = now
		session.mu.Unlock()
	}
	return session
}

func (manager *networkDebugManager) remove(id string) {
	manager.mu.Lock()
	session := manager.sessions[id]
	delete(manager.sessions, id)
	manager.mu.Unlock()
	if session != nil {
		if transport, ok := session.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}
}

func (session *networkDebugSession) begin(target string) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.requests++
	session.active++
	session.lastURL = target
	session.lastError = ""
	session.lastActivity = time.Now()
}

func (session *networkDebugSession) finish(status int, target string, requestErr error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.active > 0 {
		session.active--
	}
	session.lastStatus = status
	if target != "" {
		session.lastURL = target
	}
	if requestErr != nil {
		session.lastError = requestErr.Error()
	}
	session.lastActivity = time.Now()
}

func (session *networkDebugSession) addTraffic(count uint64, upload bool) {
	session.mu.Lock()
	if upload {
		session.sent += count
	} else {
		session.received += count
	}
	session.lastActivity = time.Now()
	session.mu.Unlock()
}

func (session *networkDebugSession) snapshot(now time.Time) map[string]any {
	session.mu.Lock()
	defer session.mu.Unlock()
	total := session.received + session.sent
	deltaTime := now.Sub(session.lastSampleAt).Seconds()
	current := float64(0)
	if deltaTime > 0 {
		current = float64(total-session.lastSampleByte) / deltaTime
	}
	session.lastSampleAt = now
	session.lastSampleByte = total
	elapsed := now.Sub(session.startedAt).Seconds()
	average := float64(0)
	if elapsed > 0 {
		average = float64(total) / elapsed
	}
	return map[string]any{
		"id":                 session.id,
		"device_id":          session.deviceID,
		"device_name":        session.deviceName,
		"interface":          session.interfaceName,
		"received_bytes":     session.received,
		"sent_bytes":         session.sent,
		"total_bytes":        total,
		"current_speed_bps":  current,
		"average_speed_bps":  average,
		"request_count":      session.requests,
		"active_requests":    session.active,
		"last_url":           session.lastURL,
		"last_status":        session.lastStatus,
		"last_error":         session.lastError,
		"started_at":         session.startedAt.Unix(),
		"expires_in_seconds": int(networkDebugSessionTTL.Seconds()),
	}
}

func boolValue(value any) bool {
	switch item := value.(type) {
	case bool:
		return item
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(item))
		return parsed
	case float64:
		return item != 0
	case int:
		return item != 0
	case int64:
		return item != 0
	default:
		return false
	}
}

func firstText(row map[string]any, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(fmt.Sprint(row[key]))
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func networkDebugDevices(rows []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if !boolValue(row["running"]) || !boolValue(row["healthy"]) || !boolValue(row["network_enabled"]) || !boolValue(row["network_connected"]) {
			continue
		}
		interfaceName := firstText(row, "interface", "iface", "net_interface")
		if interfaceName == "" {
			continue
		}
		modem, _ := row["modem"].(map[string]any)
		operator := firstText(row, "operator")
		networkMode := firstText(row, "network_mode")
		if modem != nil {
			if value := firstText(modem, "operator", "native_spn"); value != "" {
				operator = value
			}
			if value := firstText(modem, "network_mode"); value != "" {
				networkMode = value
			}
		}
		id := firstText(row, "id")
		name := firstText(row, "name")
		if name == "" {
			name = id
		}
		result = append(result, map[string]any{
			"id":           id,
			"name":         name,
			"interface":    interfaceName,
			"operator":     operator,
			"network_mode": networkMode,
			"local_phone":  firstText(row, "local_phone", "msisdn"),
		})
	}
	return result
}

func networkDebugInterfaceName(value string) error {
	if value == "" || len(value) > 15 {
		return fmt.Errorf("蜂窝网卡名称无效")
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '_' || character == '-' || character == '.' {
			continue
		}
		return fmt.Errorf("蜂窝网卡名称无效")
	}
	return nil
}

func newCellularTransport(interfaceName string) (*http.Transport, error) {
	if err := networkDebugInterfaceName(interfaceName); err != nil {
		return nil, err
	}
	if !cellularBindingAvailable() {
		return nil, fmt.Errorf("网络调试仅支持 Linux 容器")
	}
	device, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, fmt.Errorf("容器内未找到蜂窝网卡 %s", interfaceName)
	}
	if device.Flags&net.FlagUp == 0 {
		return nil, fmt.Errorf("蜂窝网卡 %s 尚未启用", interfaceName)
	}
	addresses, err := device.Addrs()
	if err != nil {
		return nil, fmt.Errorf("读取蜂窝网卡 %s 地址失败", interfaceName)
	}
	hasIPv4, hasIPv6 := false, false
	for _, raw := range addresses {
		prefix, parseErr := netip.ParsePrefix(raw.String())
		if parseErr != nil {
			continue
		}
		address := prefix.Addr().Unmap()
		if address.Is4() {
			hasIPv4 = true
		} else if address.Is6() {
			hasIPv6 = true
		}
	}
	if !hasIPv4 && !hasIPv6 {
		return nil, fmt.Errorf("蜂窝网卡 %s 尚未获取 IP 地址", interfaceName)
	}

	baseDialer := &net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, _ string, connection syscall.RawConn) error {
			var socketErr error
			if err := connection.Control(func(fd uintptr) {
				socketErr = bindSocketToInterface(fd, interfaceName)
			}); err != nil {
				return err
			}
			return socketErr
		},
	}
	resolver := &net.Resolver{
		PreferGo:     true,
		StrictErrors: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			if hasIPv4 {
				return baseDialer.DialContext(ctx, "udp4", "1.1.1.1:53")
			}
			return baseDialer.DialContext(ctx, "udp6", "[2606:4700:4700::1111]:53")
		},
	}
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		var candidates []netip.Addr
		if literal, parseErr := netip.ParseAddr(strings.Trim(host, "[]")); parseErr == nil {
			candidates = []netip.Addr{literal.Unmap()}
		} else {
			resolved, resolveErr := resolver.LookupNetIP(ctx, "ip", host)
			if resolveErr != nil {
				return nil, fmt.Errorf("解析 %s 失败: %w", host, resolveErr)
			}
			for _, item := range resolved {
				candidates = append(candidates, item.Unmap())
			}
		}
		if len(candidates) == 0 {
			return nil, fmt.Errorf("%s 没有可用公网地址", host)
		}
		for _, address := range candidates {
			if !publicNetworkDebugAddress(address) {
				return nil, fmt.Errorf("网络调试不允许访问内网或保留地址")
			}
		}
		var lastErr error
		for _, candidate := range candidates {
			if candidate.Is4() && !hasIPv4 || candidate.Is6() && !hasIPv6 {
				continue
			}
			connection, dialErr := baseDialer.DialContext(ctx, "tcp", net.JoinHostPort(candidate.String(), port))
			if dialErr == nil {
				return connection, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("蜂窝接口没有兼容的 IP 地址")
		}
		return nil, lastErr
	}
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           dial,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          24,
		MaxIdleConnsPerHost:   6,
		IdleConnTimeout:       45 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
	}, nil
}

var carrierGradeNAT = netip.MustParsePrefix("100.64.0.0/10")

func publicNetworkDebugAddress(address netip.Addr) bool {
	address = address.Unmap()
	return address.IsValid() && address.IsGlobalUnicast() && !address.IsPrivate() && !address.IsLoopback() && !address.IsLinkLocalUnicast() && !address.IsLinkLocalMulticast() && !address.IsMulticast() && !address.IsUnspecified() && !carrierGradeNAT.Contains(address)
}

func validateNetworkDebugURL(target *url.URL) error {
	if target == nil || (target.Scheme != "http" && target.Scheme != "https") {
		return fmt.Errorf("仅支持完整的 HTTP 或 HTTPS 网页地址")
	}
	if target.User != nil || target.Hostname() == "" {
		return fmt.Errorf("网页地址无效")
	}
	host := strings.ToLower(strings.TrimSuffix(target.Hostname(), "."))
	if literal, err := netip.ParseAddr(host); err == nil {
		if !publicNetworkDebugAddress(literal) {
			return fmt.Errorf("网络调试不允许访问内网或保留地址")
		}
	} else if host == "localhost" || !strings.Contains(host, ".") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".lan") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("网络调试不允许访问本机或内网主机")
	}
	if portText := target.Port(); portText != "" {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("网页端口无效")
		}
	}
	return nil
}

func networkDebugViewURL(sessionID string, target *url.URL) string {
	host := base64.RawURLEncoding.EncodeToString([]byte(target.Host))
	path := target.EscapedPath()
	if path == "" {
		path = "/"
	}
	result := "/api/network-debug/view/" + sessionID + "/" + target.Scheme + "/" + host + path
	if target.RawQuery != "" {
		result += "?" + target.RawQuery
	}
	if target.Fragment != "" {
		result += "#" + url.PathEscape(target.Fragment)
	}
	return result
}

func parseNetworkDebugView(request *http.Request) (string, *url.URL, bool, error) {
	const prefix = "/api/network-debug/view/"
	rest := strings.TrimPrefix(request.URL.Path, prefix)
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", nil, false, fmt.Errorf("浏览会话无效")
	}
	sessionID := parts[0]
	if len(parts) == 1 || parts[1] == "" {
		raw := strings.TrimSpace(request.URL.Query().Get("url"))
		if raw == "" {
			return sessionID, nil, true, fmt.Errorf("请输入网页地址")
		}
		target, err := url.Parse(raw)
		if err != nil {
			return sessionID, nil, true, fmt.Errorf("网页地址无效")
		}
		return sessionID, target, true, validateNetworkDebugURL(target)
	}
	canonical := strings.SplitN(parts[1], "/", 3)
	if len(canonical) < 2 {
		return sessionID, nil, false, fmt.Errorf("网页地址无效")
	}
	hostRaw, err := base64.RawURLEncoding.DecodeString(canonical[1])
	if err != nil {
		return sessionID, nil, false, fmt.Errorf("网页地址无效")
	}
	targetPath := "/"
	if len(canonical) == 3 {
		targetPath += canonical[2]
	}
	target := &url.URL{Scheme: canonical[0], Host: string(hostRaw), Path: targetPath, RawQuery: request.URL.RawQuery}
	return sessionID, target, false, validateNetworkDebugURL(target)
}

var cssURLPattern = regexp.MustCompile(`(?i)url\(\s*(['"]?)([^'"\)]+)['"]?\s*\)`)

func rewriteNetworkDebugURL(raw string, base *url.URL, sessionID string) string {
	value := strings.TrimSpace(stdhtml.UnescapeString(raw))
	if value == "" || strings.HasPrefix(value, "#") {
		return raw
	}
	reference, err := url.Parse(value)
	if err != nil {
		return raw
	}
	if reference.Scheme != "" && reference.Scheme != "http" && reference.Scheme != "https" {
		return raw
	}
	resolved := base.ResolveReference(reference)
	if validateNetworkDebugURL(resolved) != nil {
		return raw
	}
	return networkDebugViewURL(sessionID, resolved)
}

func rewriteNetworkDebugCSS(raw string, base *url.URL, sessionID string) string {
	return cssURLPattern.ReplaceAllStringFunc(raw, func(match string) string {
		parts := cssURLPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		value := rewriteNetworkDebugURL(parts[2], base, sessionID)
		quote := parts[1]
		if quote == "" {
			quote = `"`
		}
		return "url(" + quote + value + quote + ")"
	})
}

func rewriteNetworkDebugSrcset(raw string, base *url.URL, sessionID string) string {
	if strings.Contains(strings.ToLower(raw), "data:") {
		return raw
	}
	items := strings.Split(raw, ",")
	for index, item := range items {
		fields := strings.Fields(strings.TrimSpace(item))
		if len(fields) == 0 {
			continue
		}
		fields[0] = rewriteNetworkDebugURL(fields[0], base, sessionID)
		items[index] = strings.Join(fields, " ")
	}
	return strings.Join(items, ", ")
}

func rewriteNetworkDebugHTML(raw []byte, contentType string, base *url.URL, sessionID string) ([]byte, error) {
	reader, err := charset.NewReader(bytes.NewReader(raw), contentType)
	if err != nil {
		return nil, err
	}
	document, err := nethtml.Parse(reader)
	if err != nil {
		return nil, err
	}
	urlAttributes := map[string]bool{"href": true, "src": true, "action": true, "formaction": true, "poster": true, "data": true, "cite": true}
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node.Type == nethtml.ElementNode {
			if node.Data == "style" && node.FirstChild != nil && node.FirstChild.Type == nethtml.TextNode {
				node.FirstChild.Data = rewriteNetworkDebugCSS(node.FirstChild.Data, base, sessionID)
			}
			isRefresh := node.Data == "meta"
			for index := range node.Attr {
				attribute := &node.Attr[index]
				name := strings.ToLower(attribute.Key)
				switch {
				case urlAttributes[name]:
					attribute.Val = rewriteNetworkDebugURL(attribute.Val, base, sessionID)
				case name == "srcset":
					attribute.Val = rewriteNetworkDebugSrcset(attribute.Val, base, sessionID)
				case name == "style":
					attribute.Val = rewriteNetworkDebugCSS(attribute.Val, base, sessionID)
				case name == "target" && (attribute.Val == "_blank" || attribute.Val == "_top" || attribute.Val == "_parent"):
					attribute.Val = "_self"
				case isRefresh && name == "http-equiv" && strings.EqualFold(attribute.Val, "content-security-policy"):
					attribute.Val = "x-vohivex-removed-csp"
				}
			}
			if isRefresh {
				refresh := false
				for _, attribute := range node.Attr {
					if strings.EqualFold(attribute.Key, "http-equiv") && strings.EqualFold(attribute.Val, "refresh") {
						refresh = true
					}
				}
				if refresh {
					for index := range node.Attr {
						if strings.EqualFold(node.Attr[index].Key, "content") {
							pieces := strings.SplitN(node.Attr[index].Val, ";", 2)
							if len(pieces) == 2 && strings.Contains(strings.ToLower(pieces[1]), "url=") {
								rawURL := strings.TrimSpace(strings.SplitN(pieces[1], "=", 2)[1])
								node.Attr[index].Val = pieces[0] + ";url=" + rewriteNetworkDebugURL(rawURL, base, sessionID)
							}
						}
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	var output bytes.Buffer
	if err := nethtml.Render(&output, document); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func copyNetworkDebugHeaders(destination http.Header, source http.Header) {
	allowed := map[string]bool{
		"accept-ranges": true, "cache-control": true, "content-disposition": true,
		"content-language": true, "content-range": true, "content-type": true,
		"etag": true, "expires": true, "last-modified": true,
	}
	for name, values := range source {
		if !allowed[strings.ToLower(name)] {
			continue
		}
		for _, value := range values {
			destination.Add(name, value)
		}
	}
	destination.Set("Cache-Control", "no-store")
	destination.Set("Referrer-Policy", "no-referrer")
	destination.Set("X-Content-Type-Options", "nosniff")
	destination.Set("Content-Security-Policy", "frame-ancestors 'self'")
}

func copyNetworkDebugRequestHeaders(destination http.Header, source http.Header, target *url.URL) {
	allowed := map[string]bool{
		"accept": true, "accept-language": true, "content-type": true,
		"if-match": true, "if-modified-since": true, "if-none-match": true,
		"if-unmodified-since": true, "range": true, "user-agent": true,
	}
	for name, values := range source {
		if !allowed[strings.ToLower(name)] {
			continue
		}
		for _, value := range values {
			destination.Add(name, value)
		}
	}
	if source.Get("Origin") != "" {
		destination.Set("Origin", target.Scheme+"://"+target.Host)
	}
}

func writeNetworkDebugViewError(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'self'")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>网络调试</title><style>body{margin:0;padding:48px;background:#fff;color:#242722;font:14px/1.6 system-ui,sans-serif}.box{max-width:560px;margin:auto;padding:28px;border-radius:22px;background:#edefec}h1{margin:0 0 8px;color:#163300;font-size:22px}p{margin:0}</style><div class="box"><h1>网页无法打开</h1><p>%s</p></div></html>`, stdhtml.EscapeString(detail))
}

func (app *application) networkDebugAPI(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/network-debug/view/") {
		app.networkDebugView(w, r)
		return
	}
	if app.network == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "网络调试模块尚未就绪"})
		return
	}
	if r.URL.Path == "/api/network-debug/devices" {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
			return
		}
		devices, status, err := app.validateAuth(r)
		if err != nil {
			writeJSON(w, status, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"devices": app.networkDebugDeviceRows(deviceRows(devices))})
		return
	}
	if r.URL.Path == "/api/network-debug/sessions" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
			return
		}
		if !sameOrigin(r) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "不允许跨站创建浏览会话"})
			return
		}
		devices, status, err := app.validateAuth(r)
		if err != nil {
			writeJSON(w, status, map[string]any{"error": err.Error()})
			return
		}
		payload := struct {
			DeviceID string `json:"device_id"`
		}{}
		if !decodeJSON(w, r, &payload, 8<<10) {
			return
		}
		var selected map[string]any
		for _, device := range app.networkDebugDeviceRows(deviceRows(devices)) {
			if fmt.Sprint(device["id"]) == payload.DeviceID {
				selected = device
				break
			}
		}
		if selected == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "所选设备的蜂窝网络未启用或尚未连接"})
			return
		}
		var session *networkDebugSession
		if relay := configuredNetworkRelay(payload.DeviceID); relay != nil {
			var transport *http.Transport
			transport, err = relay.transport()
			if err == nil {
				session, err = app.network.createTransport(networkDebugOwner(r), payload.DeviceID, relay.Name, relay.Interface, transport)
			}
		} else {
			session, err = app.network.create(networkDebugOwner(r), payload.DeviceID, fmt.Sprint(selected["name"]), fmt.Sprint(selected["interface"]))
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":         session.id,
			"device_id":  session.deviceID,
			"interface":  session.interfaceName,
			"entry_url":  "/api/network-debug/view/" + session.id,
			"expires_in": int(networkDebugSessionTTL.Seconds()),
		})
		return
	}
	const sessionPrefix = "/api/network-debug/sessions/"
	if strings.HasPrefix(r.URL.Path, sessionPrefix) {
		rest := strings.TrimPrefix(r.URL.Path, sessionPrefix)
		parts := strings.Split(strings.Trim(rest, "/"), "/")
		id := ""
		if len(parts) > 0 {
			id = parts[0]
		}
		session := app.network.get(id)
		if session == nil || session.owner != networkDebugOwner(r) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "浏览会话不存在或已过期"})
			return
		}
		if len(parts) == 2 && parts[1] == "stats" && r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, session.snapshot(time.Now()))
			return
		}
		if len(parts) == 1 && r.Method == http.MethodDelete {
			if !sameOrigin(r) {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "不允许跨站关闭浏览会话"})
				return
			}
			app.network.remove(id)
			writeJSON(w, http.StatusOK, map[string]any{"closed": true})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"error": "接口不存在"})
}

func (app *application) networkDebugView(w http.ResponseWriter, r *http.Request) {
	if app.network == nil {
		writeNetworkDebugViewError(w, http.StatusServiceUnavailable, "网络调试模块尚未就绪")
		return
	}
	if r.Method == http.MethodConnect || r.Method == http.MethodTrace {
		writeNetworkDebugViewError(w, http.StatusMethodNotAllowed, "该请求方式不受支持")
		return
	}
	sessionID, target, entry, err := parseNetworkDebugView(r)
	if err != nil {
		writeNetworkDebugViewError(w, http.StatusBadRequest, err.Error())
		return
	}
	session := app.network.get(sessionID)
	if session == nil {
		writeNetworkDebugViewError(w, http.StatusGone, "浏览会话已结束，请返回网络调试重新开始")
		return
	}
	if entry {
		http.Redirect(w, r, networkDebugViewURL(sessionID, target), http.StatusTemporaryRedirect)
		return
	}
	target.Fragment = ""
	upstreamBody := io.ReadCloser(http.NoBody)
	if r.Body != nil && r.Body != http.NoBody {
		upstreamBody = &networkDebugTrafficReader{reader: http.MaxBytesReader(w, r.Body, networkDebugRequestLimit), session: session, upload: true}
	}
	request, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), upstreamBody)
	if err != nil {
		writeNetworkDebugViewError(w, http.StatusBadRequest, "网页请求无效")
		return
	}
	copyNetworkDebugRequestHeaders(request.Header, r.Header, target)
	session.begin(target.String())
	statusCode := 0
	finalURL := target.String()
	var finalErr error
	defer func() { session.finish(statusCode, finalURL, finalErr) }()
	response, err := session.client.Do(request)
	if err != nil {
		finalErr = err
		writeNetworkDebugViewError(w, http.StatusBadGateway, "无法通过所选蜂窝网络访问该网页："+err.Error())
		return
	}
	defer response.Body.Close()
	statusCode = response.StatusCode
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL.String()
		target = response.Request.URL
	}
	copyNetworkDebugHeaders(w.Header(), response.Header)
	contentType := response.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	counted := &networkDebugTrafficReader{reader: response.Body, session: session}
	if mediaType == "text/html" || mediaType == "application/xhtml+xml" || mediaType == "text/css" {
		raw, readErr := io.ReadAll(io.LimitReader(counted, networkDebugTransformLimit+1))
		if readErr != nil {
			finalErr = readErr
			writeNetworkDebugViewError(w, http.StatusBadGateway, "读取网页内容失败")
			return
		}
		if int64(len(raw)) > networkDebugTransformLimit {
			finalErr = fmt.Errorf("网页内容超过单个响应限制")
			writeNetworkDebugViewError(w, http.StatusRequestEntityTooLarge, finalErr.Error())
			return
		}
		if mediaType == "text/css" {
			raw = []byte(rewriteNetworkDebugCSS(string(raw), target, sessionID))
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		} else {
			raw, err = rewriteNetworkDebugHTML(raw, contentType, target, sessionID)
			if err != nil {
				finalErr = err
				writeNetworkDebugViewError(w, http.StatusBadGateway, "网页内容解析失败")
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
		w.WriteHeader(response.StatusCode)
		if r.Method != http.MethodHead {
			_, _ = w.Write(raw)
		}
		return
	}
	w.Header().Del("Content-Length")
	w.WriteHeader(response.StatusCode)
	if r.Method == http.MethodHead {
		return
	}
	_, copyErr := io.Copy(w, io.LimitReader(counted, networkDebugResponseLimit))
	if copyErr != nil {
		finalErr = copyErr
	}
}
