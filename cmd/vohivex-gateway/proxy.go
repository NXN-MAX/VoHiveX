package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

const (
	proxySOCKSPort      = 17890
	proxyControllerPort = 17891
	maxSubscriptionSize = 4 << 20
)

type invalidError struct{ message string }

func (e invalidError) Error() string { return e.message }

type conflictError struct{ message string }

func (e conflictError) Error() string { return e.message }

type proxyNode struct {
	ID     string         `json:"id"`
	Key    string         `json:"key,omitempty"`
	Name   string         `json:"name"`
	Config map[string]any `json:"config"`
}

type proxySource struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Kind    string      `json:"kind"`
	URL     string      `json:"url,omitempty"`
	Nodes   []proxyNode `json:"nodes"`
	Updated int64       `json:"updated"`
}

type proxyState struct {
	Version  int           `json:"version"`
	Sources  []proxySource `json:"sources"`
	Selected string        `json:"selected,omitempty"`
	Enabled  bool          `json:"enabled"`
	Secret   string        `json:"secret"`
}

type proxyManager struct {
	mu          sync.Mutex
	dir         string
	statePath   string
	confPath    string
	binary      string
	state       proxyState
	process     *exec.Cmd
	processDone chan struct{}
	errorText   string
	closed      bool
	lastStart   time.Time
}

func randomHex(bytes int) string {
	raw := make([]byte, bytes)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

func newProxyManager(directory, binary string) (*proxyManager, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	manager := &proxyManager{dir: directory, statePath: filepath.Join(directory, "state.json"), confPath: filepath.Join(directory, "config.json"), binary: binary}
	raw, err := os.ReadFile(manager.statePath)
	if errors.Is(err, os.ErrNotExist) {
		manager.state = proxyState{Version: 1, Sources: []proxySource{}, Secret: randomHex(32)}
		if err := manager.persist(manager.state); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else if err := json.Unmarshal(raw, &manager.state); err != nil {
		return nil, fmt.Errorf("invalid managed proxy state: %w", err)
	}
	if manager.state.Version < 1 || manager.state.Secret == "" {
		return nil, errors.New("managed proxy state is incompatible")
	}
	if err := manager.startLocked(); err != nil {
		manager.errorText = "代理核心未启动，请检查容器"
	}
	return manager, nil
}

func (m *proxyManager) Version() string { return commandVersion(m.binary) }

func (m *proxyManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.stopLocked()
}

func (m *proxyManager) endpoint() string { return fmt.Sprintf("127.0.0.1:%d", proxySOCKSPort) }

func (m *proxyManager) nodes(state proxyState) []proxyNode {
	var result []proxyNode
	for _, source := range state.Sources {
		result = append(result, source.Nodes...)
	}
	return result
}

func (m *proxyManager) snapshotLocked() map[string]any {
	sources := make([]map[string]any, 0, len(m.state.Sources))
	nodes := make([]map[string]any, 0)
	for _, source := range m.state.Sources {
		sources = append(sources, map[string]any{"id": source.ID, "name": source.Name, "kind": source.Kind, "updated": source.Updated, "count": len(source.Nodes)})
		for _, node := range source.Nodes {
			nodes = append(nodes, map[string]any{"id": node.ID, "name": node.Name, "source_id": source.ID, "type": node.Config["type"], "udp": node.Config["udp"] != false})
		}
	}
	running := m.processRunningLocked()
	return map[string]any{"version": m.state.Version, "enabled": m.state.Enabled, "selected": m.state.Selected, "running": running, "error": m.errorText, "sources": sources, "nodes": nodes, "endpoint": m.endpoint()}
}

func (m *proxyManager) Snapshot() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked()
}

func (m *proxyManager) StateCopy() proxyState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneProxyState(m.state)
}

func (m *proxyManager) Restore(state proxyState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = m.persist(state)
	m.state = cloneProxyState(state)
	_ = m.startLocked()
}

func (m *proxyManager) persist(state proxyState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return writeAtomic(m.statePath, raw, 0o600)
}

func cloneProxyState(state proxyState) proxyState {
	raw, _ := json.Marshal(state)
	var result proxyState
	_ = json.Unmarshal(raw, &result)
	return result
}

func (m *proxyManager) mihomoConfig(state proxyState) map[string]any {
	nodes := m.nodes(state)
	proxies := make([]map[string]any, 0, len(nodes))
	selected := ""
	for _, node := range nodes {
		config := make(map[string]any, len(node.Config)+1)
		for key, value := range node.Config {
			config[key] = value
		}
		config["name"] = node.ID
		proxies = append(proxies, config)
		if node.ID == state.Selected {
			selected = node.ID
		}
	}
	route := "REJECT"
	if state.Enabled && selected != "" {
		route = selected
	}
	return map[string]any{
		"socks-port": proxySOCKSPort, "bind-address": "127.0.0.1", "allow-lan": false,
		"mode": "rule", "log-level": "silent", "ipv6": false,
		"external-controller": fmt.Sprintf("127.0.0.1:%d", proxyControllerPort), "secret": state.Secret,
		"profile":      map[string]any{"store-selected": false, "store-fake-ip": false},
		"geodata-mode": false, "geo-auto-update": false,
		"dns": map[string]any{"enable": false}, "tun": map[string]any{"enable": false}, "sniffer": map[string]any{"enable": false},
		"proxies": proxies, "rules": []string{"MATCH," + route},
	}
}

func (m *proxyManager) writeConfig(path string, state proxyState) error {
	raw, err := json.Marshal(m.mihomoConfig(state))
	if err != nil {
		return err
	}
	return writeAtomic(path, raw, 0o600)
}

func (m *proxyManager) validate(state proxyState) error {
	check := filepath.Join(m.dir, "check.json")
	defer os.Remove(check)
	if err := m.writeConfig(check, state); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, m.binary, "-t", "-d", m.dir, "-f", check)
	if err := cmd.Run(); err != nil {
		return invalidError{"代理核心不接受部分节点参数，未应用本次修改"}
	}
	return nil
}

func (m *proxyManager) stopLocked() {
	if !m.processRunningLocked() {
		m.process = nil
		m.processDone = nil
		return
	}
	_ = m.process.Process.Signal(syscall.SIGTERM)
	select {
	case <-m.processDone:
	case <-time.After(8 * time.Second):
		_ = m.process.Process.Kill()
		<-m.processDone
	}
	m.process = nil
	m.processDone = nil
}

func (m *proxyManager) processRunningLocked() bool {
	if m.process == nil || m.process.Process == nil || m.processDone == nil {
		return false
	}
	select {
	case <-m.processDone:
		return false
	default:
		return true
	}
}

func (m *proxyManager) startLocked() error {
	if m.closed {
		return nil
	}
	if err := m.writeConfig(m.confPath, m.state); err != nil {
		return err
	}
	m.stopLocked()
	cmd := exec.Command(m.binary, "-d", m.dir, "-f", m.confPath)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}
	m.process = cmd
	m.processDone = make(chan struct{})
	go func(done chan struct{}) { _ = cmd.Wait(); close(done) }(m.processDone)
	m.lastStart = time.Now()
	for index := 0; index < 40; index++ {
		if !m.processRunningLocked() {
			break
		}
		if _, err := m.control("/version"); err == nil {
			m.errorText = ""
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	m.stopLocked()
	return invalidError{"代理核心未启动，请重试或检查容器"}
}

func (m *proxyManager) Maintain() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.processRunningLocked() || time.Since(m.lastStart) < 30*time.Second {
		return
	}
	if err := m.startLocked(); err != nil {
		m.errorText = "代理核心未启动，请检查容器"
	}
}

func (m *proxyManager) control(path string) (map[string]any, error) {
	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", proxyControllerPort, path), nil)
	request.Header.Set("Authorization", "Bearer "+m.state.Secret)
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("proxy core rejected request")
	}
	var value map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func (m *proxyManager) commitLocked(next proxyState) error {
	if err := m.validate(next); err != nil {
		return err
	}
	old := cloneProxyState(m.state)
	next.Version = old.Version + 1
	if err := m.persist(next); err != nil {
		return err
	}
	m.state = next
	if err := m.startLocked(); err != nil {
		_ = m.persist(old)
		m.state = old
		_ = m.startLocked()
		return invalidError{"代理配置应用失败，已恢复原配置"}
	}
	return nil
}

func versionNumber(payload map[string]any) (int, bool) {
	switch value := payload["version"].(type) {
	case json.Number:
		integer, err := value.Int64()
		return int(integer), err == nil
	case float64:
		return int(value), value == float64(int(value))
	case int:
		return value, true
	default:
		return 0, false
	}
}

func (m *proxyManager) checkVersion(payload map[string]any) error {
	value, ok := versionNumber(payload)
	if !ok || value != m.state.Version {
		return conflictError{"配置已变化，请刷新后重试"}
	}
	return nil
}

func cleanProxyName(value any) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", invalidError{"名称格式无效"}
	}
	text = strings.TrimSpace(strings.Map(func(char rune) rune {
		if char < 32 || char == 127 {
			return -1
		}
		return char
	}, text))
	if text == "" {
		text = "未命名"
	}
	runes := []rune(text)
	if len(runes) > 100 {
		text = string(runes[:100])
	}
	return text, nil
}

var supportedProxyTypes = map[string]bool{"vmess": true, "vless": true, "anytls": true, "trojan": true, "ss": true, "hysteria2": true, "tuic": true, "socks5": true}

func sanitizeProxyNode(input map[string]any) (map[string]any, error) {
	typeName, _ := input["type"].(string)
	if !supportedProxyTypes[typeName] {
		return nil, invalidError{"包含暂不支持的节点类型"}
	}
	allowed := map[string]bool{"type": true, "name": true, "server": true, "port": true, "udp": true, "uuid": true, "password": true, "username": true, "cipher": true, "alterId": true, "tls": true, "servername": true, "sni": true, "skip-cert-verify": true, "client-fingerprint": true, "fingerprint": true, "alpn": true, "network": true, "flow": true, "packet-encoding": true, "encryption": true, "obfs": true, "obfs-password": true, "up": true, "down": true, "congestion-controller": true, "udp-relay-mode": true, "reduce-rtt": true, "disable-sni": true, "heartbeat-interval": true, "idle-session-check-interval": true, "idle-session-timeout": true, "min-idle-session": true, "ws-opts": true, "grpc-opts": true, "reality-opts": true, "http-opts": true, "h2-opts": true}
	result := map[string]any{}
	for key, value := range input {
		if allowed[key] {
			result[key] = value
		}
	}
	name, err := cleanProxyName(result["name"])
	if err != nil {
		return nil, err
	}
	result["name"] = name
	server, ok := result["server"].(string)
	if !ok || server == "" || len(server) > 253 || strings.ContainsAny(server, " \t\r\n/@?#\x00") {
		return nil, invalidError{"节点服务器地址无效"}
	}
	port, err := anyInt(result["port"])
	if err != nil || port < 1 || port > 65535 {
		return nil, invalidError{"节点端口无效"}
	}
	result["port"] = port
	if value, exists := result["udp"]; exists {
		if _, ok := value.(bool); !ok {
			return nil, invalidError{"UDP 参数无效"}
		}
	} else {
		result["udp"] = true
	}
	encoded, _ := json.Marshal(result)
	if len(encoded) > 16000 {
		return nil, invalidError{"单个节点参数过大"}
	}
	return result, nil
}

func anyInt(value any) (int, error) {
	switch item := value.(type) {
	case int:
		return item, nil
	case float64:
		if item != float64(int(item)) {
			return 0, errors.New("not integer")
		}
		return int(item), nil
	case json.Number:
		result, err := strconv.Atoi(string(item))
		return result, err
	case string:
		return strconv.Atoi(item)
	default:
		return 0, errors.New("not integer")
	}
}

func decodeURLBase64(text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if result, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(text, "=")); err == nil {
		return result, nil
	}
	if result, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(text, "=")); err == nil {
		return result, nil
	}
	return base64.StdEncoding.DecodeString(text)
}

func nodeFromURI(text string) (map[string]any, error) {
	u, err := url.Parse(strings.TrimSpace(text))
	if err != nil {
		return nil, invalidError{"节点链接格式无效"}
	}
	typeName := strings.ToLower(u.Scheme)
	if typeName == "hy2" {
		typeName = "hysteria2"
	}
	if typeName == "vmess" {
		raw, err := decodeURLBase64(strings.TrimPrefix(text, "vmess://"))
		if err != nil {
			return nil, invalidError{"VMess 链接格式无效"}
		}
		var value map[string]any
		if json.Unmarshal(raw, &value) != nil {
			return nil, invalidError{"VMess 链接格式无效"}
		}
		port, _ := anyInt(value["port"])
		node := map[string]any{"type": "vmess", "name": fmt.Sprint(value["ps"]), "server": fmt.Sprint(value["add"]), "port": port, "uuid": fmt.Sprint(value["id"]), "alterId": value["aid"], "cipher": "auto", "udp": true, "tls": value["tls"] == "tls"}
		if network := fmt.Sprint(value["net"]); network != "" && network != "tcp" {
			node["network"] = network
		}
		return sanitizeProxyNode(node)
	}
	if !supportedProxyTypes[typeName] {
		return nil, invalidError{"不支持的节点链接类型"}
	}
	name, _ := url.QueryUnescape(u.Fragment)
	if name == "" {
		name = strings.ToUpper(typeName)
	}
	port := 443
	if parsed := u.Port(); parsed != "" {
		port, _ = strconv.Atoi(parsed)
	}
	node := map[string]any{"type": typeName, "name": name, "server": u.Hostname(), "port": port, "udp": true}
	password, _ := u.User.Password()
	username := ""
	if u.User != nil {
		username = u.User.Username()
	}
	switch typeName {
	case "vless", "tuic":
		node["uuid"] = username
	case "trojan", "anytls", "hysteria2":
		node["password"] = username
	case "socks5":
		node["username"], node["password"] = username, password
	case "ss":
		credentials := username
		if decoded, err := decodeURLBase64(credentials); err == nil {
			credentials = string(decoded)
		}
		parts := strings.SplitN(credentials, ":", 2)
		if len(parts) != 2 {
			return nil, invalidError{"Shadowsocks 链接格式无效"}
		}
		node["cipher"], node["password"] = parts[0], parts[1]
	}
	query := u.Query()
	if value := query.Get("sni"); value != "" {
		node["servername"] = value
	}
	if value := query.Get("flow"); value != "" {
		node["flow"] = value
	}
	if value := query.Get("type"); value != "" && value != "tcp" {
		node["network"] = value
	}
	if query.Get("security") == "tls" || query.Get("security") == "reality" || map[string]bool{"trojan": true, "anytls": true, "hysteria2": true, "tuic": true}[typeName] {
		node["tls"] = true
	}
	return sanitizeProxyNode(node)
}

func normalizeYAML(value any) any {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			item[key] = normalizeYAML(child)
		}
		return item
	case map[any]any:
		result := map[string]any{}
		for key, child := range item {
			result[fmt.Sprint(key)] = normalizeYAML(child)
		}
		return result
	case []any:
		for index, child := range item {
			item[index] = normalizeYAML(child)
		}
		return item
	default:
		return value
	}
}

func parseProxyNodes(text string) ([]proxyNode, error) {
	if len([]byte(text)) > maxSubscriptionSize {
		return nil, invalidError{"订阅内容超过 4 MB"}
	}
	text = strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
	if text == "" {
		return nil, invalidError{"订阅没有节点"}
	}
	var raw []map[string]any
	first := strings.ToLower(strings.SplitN(text, "\n", 2)[0])
	share := false
	for kind := range supportedProxyTypes {
		if strings.HasPrefix(first, kind+"://") {
			share = true
		}
	}
	if strings.HasPrefix(first, "hy2://") {
		share = true
	}
	if share {
		for _, line := range strings.Split(text, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			node, err := nodeFromURI(line)
			if err != nil {
				return nil, err
			}
			raw = append(raw, node)
		}
	} else {
		var value any
		var document yaml.Node
		if err := yaml.Unmarshal([]byte(text), &document); err == nil {
			var inspect func(*yaml.Node) error
			inspect = func(node *yaml.Node) error {
				if node.Kind == yaml.AliasNode || node.Anchor != "" {
					return invalidError{"不接受 YAML 引用，请使用普通节点列表"}
				}
				for _, child := range node.Content {
					if err := inspect(child); err != nil {
						return err
					}
				}
				return nil
			}
			if err := inspect(&document); err != nil {
				return nil, err
			}
		}
		if err := yaml.Unmarshal([]byte(text), &value); err == nil {
			value = normalizeYAML(value)
			if object, ok := value.(map[string]any); ok {
				if items, ok := object["proxies"].([]any); ok {
					for _, item := range items {
						if node, ok := item.(map[string]any); ok {
							raw = append(raw, node)
						}
					}
				}
			}
			if encoded, ok := value.(string); ok && len(raw) == 0 {
				decoded, decodeErr := decodeURLBase64(strings.Join(strings.Fields(encoded), ""))
				if decodeErr == nil {
					return parseProxyNodes(string(decoded))
				}
			}
		}
		if len(raw) == 0 {
			decoded, err := decodeURLBase64(strings.Join(strings.Fields(text), ""))
			if err == nil {
				return parseProxyNodes(string(decoded))
			}
			return nil, invalidError{"请使用 Clash YAML、Base64 订阅或节点分享链接"}
		}
	}
	if len(raw) < 1 || len(raw) > 1000 {
		return nil, invalidError{"每个订阅支持 1–1000 个节点"}
	}
	seen := map[string]bool{}
	result := make([]proxyNode, 0, len(raw))
	for _, item := range raw {
		node, err := sanitizeProxyNode(item)
		if err != nil {
			return nil, err
		}
		canonical, _ := json.Marshal(node)
		sum := sha256.Sum256(canonical)
		key := hex.EncodeToString(sum[:])[:24]
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, proxyNode{Key: key, Name: fmt.Sprint(node["name"]), Config: node})
	}
	return result, nil
}

func publicAddress(address string) bool {
	ip, err := netip.ParseAddr(address)
	if err != nil {
		return false
	}
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}

func fetchSubscription(rawURL string) (string, error) {
	current := rawURL
	for redirect := 0; redirect < 4; redirect++ {
		u, err := url.Parse(current)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(current) > 4096 {
			return "", invalidError{"请填写 HTTPS 订阅地址"}
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(context.Background(), u.Hostname())
		if err != nil || len(addresses) == 0 {
			return "", invalidError{"无法获取订阅，请检查链接、证书或网络"}
		}
		for _, address := range addresses {
			if !publicAddress(address.IP.String()) {
				return "", invalidError{"订阅地址必须指向公网服务器"}
			}
		}
		pinned := addresses[0].IP.String()
		port := u.Port()
		if port == "" {
			port = "443"
		}
		transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, network, net.JoinHostPort(pinned, port))
		}}
		client := &http.Client{Timeout: 20 * time.Second, Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
		request, _ := http.NewRequest(http.MethodGet, current, nil)
		request.Header.Set("User-Agent", "clash.meta")
		request.Header.Set("Accept-Encoding", "identity")
		response, err := client.Do(request)
		if err != nil {
			return "", invalidError{"无法获取订阅，请检查链接、证书或网络"}
		}
		if response.StatusCode >= 300 && response.StatusCode <= 399 {
			location := response.Header.Get("Location")
			response.Body.Close()
			next, err := u.Parse(location)
			if err != nil {
				return "", invalidError{"订阅重定向地址无效"}
			}
			current = next.String()
			continue
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return "", invalidError{fmt.Sprintf("订阅下载失败（HTTP %d）", response.StatusCode)}
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, maxSubscriptionSize+1))
		response.Body.Close()
		if err != nil {
			return "", invalidError{"无法读取订阅内容"}
		}
		if len(raw) > maxSubscriptionSize {
			return "", invalidError{"订阅内容超过 4 MB"}
		}
		return strings.TrimPrefix(string(raw), "\ufeff"), nil
	}
	return "", invalidError{"订阅重定向次数过多"}
}

func (m *proxyManager) Import(payload map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkVersion(payload); err != nil {
		return err
	}
	name, err := cleanProxyName(payload["name"])
	if err != nil {
		return err
	}
	kind, _ := payload["kind"].(string)
	content, _ := payload["content"].(string)
	if kind != "subscription" && kind != "nodes" {
		return invalidError{"导入方式无效"}
	}
	text := content
	if kind == "subscription" {
		text, err = fetchSubscription(content)
		if err != nil {
			return err
		}
	}
	nodes, err := parseProxyNodes(text)
	if err != nil {
		return err
	}
	if len(m.state.Sources) >= 20 || len(m.nodes(m.state))+len(nodes) > 1000 {
		return invalidError{"最多 20 个来源、合计 1000 个节点"}
	}
	sourceID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	for index := range nodes {
		nodes[index].ID = sourceID + "-" + nodes[index].Key
		nodes[index].Key = ""
	}
	next := cloneProxyState(m.state)
	next.Sources = append(next.Sources, proxySource{ID: sourceID, Name: name, Kind: kind, URL: func() string {
		if kind == "subscription" {
			return content
		}
		return ""
	}(), Nodes: nodes, Updated: time.Now().Unix()})
	return m.commitLocked(next)
}

func (m *proxyManager) Change(action string, payload map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkVersion(payload); err != nil {
		return err
	}
	next := cloneProxyState(m.state)
	switch action {
	case "select":
		id, _ := payload["node_id"].(string)
		found := false
		for _, node := range m.nodes(next) {
			if node.ID == id {
				if node.Config["udp"] == false {
					return invalidError{"此节点未启用 UDP，不能作为 VoWiFi 出口"}
				}
				found = true
			}
		}
		if !found {
			return invalidError{"节点不存在"}
		}
		next.Selected = id
		next.Enabled = true
	case "pause":
		next.Enabled = false
	case "refresh", "delete":
		id, _ := payload["source_id"].(string)
		index := -1
		for i := range next.Sources {
			if next.Sources[i].ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			return invalidError{"订阅不存在"}
		}
		if action == "refresh" {
			source := &next.Sources[index]
			if source.Kind != "subscription" {
				return invalidError{"手动导入的节点没有订阅地址"}
			}
			text, err := fetchSubscription(source.URL)
			if err != nil {
				return err
			}
			nodes, err := parseProxyNodes(text)
			if err != nil {
				return err
			}
			if len(m.nodes(next))-len(source.Nodes)+len(nodes) > 1000 {
				return invalidError{"节点总数不能超过 1000"}
			}
			for i := range nodes {
				nodes[i].ID = source.ID + "-" + nodes[i].Key
				nodes[i].Key = ""
			}
			source.Nodes = nodes
			source.Updated = time.Now().Unix()
		} else {
			next.Sources = append(next.Sources[:index], next.Sources[index+1:]...)
		}
		selectedExists := false
		for _, node := range m.nodes(next) {
			if node.ID == next.Selected {
				selectedExists = true
			}
		}
		if !selectedExists {
			if next.Enabled {
				return invalidError{"操作会移除当前出口，请先暂停或切换节点"}
			}
			next.Selected = ""
		}
	default:
		return invalidError{"操作无效"}
	}
	return m.commitLocked(next)
}

func (m *proxyManager) Test(nodeID string) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := false
	for _, node := range m.nodes(m.state) {
		if node.ID == nodeID {
			found = true
		}
	}
	if !found {
		return nil, invalidError{"节点不存在"}
	}
	value, err := m.control("/proxies/" + url.PathEscape(nodeID) + "/delay?timeout=6000&url=https%3A%2F%2Fwww.gstatic.com%2Fgenerate_204")
	if err != nil {
		return nil, invalidError{"HTTPS 连接测试失败或超时；不能据此判断 UDP"}
	}
	return map[string]any{"delay_ms": value["delay"], "message": "HTTPS 连接成功；UDP 和 VoWiFi 注册需单独验证"}, nil
}
