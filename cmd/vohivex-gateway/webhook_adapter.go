package main

import (
	"bytes"
	"context"
	"crypto/hmac"
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
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const webhookRelayPath = "/api/internal/webhook-relay"

type webhookAdapterState struct {
	Format      string         `json:"format"`
	Config      map[string]any `json:"config"`
	RelaySecret string         `json:"relay_secret"`
}

type webhookAdapter struct {
	mu       sync.RWMutex
	applyMu  sync.Mutex
	path     string
	relayURL string
	state    webhookAdapterState
	client   *http.Client
}

func newWebhookAdapter(path, relayURL string) (*webhookAdapter, error) {
	adapter := &webhookAdapter{path: path, relayURL: relayURL, client: &http.Client{
		Timeout:       60 * time.Second,
		Transport:     &http.Transport{Proxy: nil},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}
	raw, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(raw, &adapter.state); err != nil {
			return nil, err
		}
		if len(adapter.state.RelaySecret) < 32 {
			return nil, errors.New("invalid webhook relay state")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		var secret [32]byte
		if _, err := rand.Read(secret[:]); err != nil {
			return nil, err
		}
		adapter.state = webhookAdapterState{Format: "generic", RelaySecret: hex.EncodeToString(secret[:])}
	} else {
		return nil, err
	}
	return adapter, nil
}

func (adapter *webhookAdapter) snapshot() webhookAdapterState {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return adapter.state // State maps are immutable after publication.
}

func (adapter *webhookAdapter) save(next webhookAdapterState) error {
	raw, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if err := writeAtomic(adapter.path, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	adapter.state = next
	return nil
}

func isRobotFormat(format string) bool { return format == "dingtalk" || format == "chat_robots" }

var feishuHookPath = regexp.MustCompile(`^/open-apis/bot/v2/hook/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func robotProvider(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" {
		return ""
	}
	if u.Host == "oapi.dingtalk.com" && u.Path == "/robot/send" && u.Query().Get("access_token") != "" {
		return "dingtalk"
	}
	if u.Host == "open.feishu.cn" && feishuHookPath.MatchString(u.Path) && u.RawQuery == "" {
		return "feishu"
	}
	return ""
}

func validateRobotConfig(cfg map[string]any) error {
	targets := recipients(cfg["urls"])
	if len(targets) == 0 || len(targets) > 10 {
		return errors.New("请填写 1–10 个群机器人 Webhook 地址")
	}
	for _, endpoint := range targets {
		provider := robotProvider(endpoint)
		if provider == "" || (stringSetting(cfg, "format") == "dingtalk" && provider != "dingtalk") {
			return errors.New("请填写与所选格式匹配的钉钉或飞书 HTTPS Webhook 地址")
		}
	}

	if len(stringSetting(cfg, "text_template")) > 4096 {
		return errors.New("文本模板过长")
	}
	for key, bounds := range map[string][2]int64{"timeout_ms": {1000, 60000}, "retry_max": {0, 10}} {
		if cfg[key] == nil {
			continue
		}
		value, err := numberAny64(cfg[key])
		if err != nil || value < bounds[0] || value > bounds[1] {
			return fmt.Errorf("%s 超出允许范围", key)
		}
	}
	return nil
}

func dingTalkURL(endpoint, secret string, now time.Time) string {
	if secret == "" {
		return endpoint
	}
	u, _ := url.Parse(endpoint)
	timestamp := strconv.FormatInt(now.UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "\n" + secret))
	query := u.Query()
	query.Set("timestamp", timestamp)
	query.Set("sign", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	u.RawQuery = query.Encode()
	return u.String()
}

func webhookText(cfg, event map[string]any) string {
	text, _ := event["text"].(string)
	template := stringSetting(cfg, "text_template")
	if template == "" {
		return text
	}
	meta := configMap(event["meta"])
	values := []string{}
	for _, key := range []string{"text", "event", "timestamp", "device_id", "device_name", "device_label"} {
		value, _ := event[key].(string)
		if value == "" {
			value, _ = meta[key].(string)
		}
		if key == "device_label" && value == "" {
			value, _ = event["device_name"].(string)
		}
		values = append(values, "{{"+key+"}}", value)
	}
	return strings.NewReplacer(values...).Replace(template)
}

func (adapter *webhookAdapter) send(ctx context.Context, cfg, event map[string]any) error {
	if err := validateRobotConfig(cfg); err != nil {
		return err
	}
	text := webhookText(cfg, event)
	if strings.TrimSpace(text) == "" {
		return errors.New("群机器人消息内容为空")
	}
	var failures []string
	for _, endpoint := range recipients(cfg["urls"]) {
		if err := adapter.sendOne(ctx, cfg, endpoint, text); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "；"))
	}
	return nil
}

func (adapter *webhookAdapter) sendOne(ctx context.Context, cfg map[string]any, endpoint, text string) error {
	provider, label := robotProvider(endpoint), "钉钉"
	payload := map[string]any{"msgtype": "text", "text": map[string]string{"content": text}, "at": map[string]any{"isAtAll": false}}
	if provider == "feishu" {
		label = "飞书"
		payload = map[string]any{"msg_type": "text", "content": map[string]string{"text": text}}
		if secret := stringSetting(cfg, "feishu_secret"); secret != "" {
			timestamp := strconv.FormatInt(time.Now().Unix(), 10)
			mac := hmac.New(sha256.New, []byte(timestamp+"\n"+secret))
			payload["timestamp"], payload["sign"] = timestamp, base64.StdEncoding.EncodeToString(mac.Sum(nil))
		}
	} else {
		endpoint = dingTalkURL(endpoint, stringSetting(cfg, "secret"), time.Now())
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	timeout, _ := numberAny64(cfg["timeout_ms"])
	if timeout == 0 {
		timeout = 5000
	}
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("%s请求创建失败", label)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := adapter.client.Do(request)
	if err != nil {
		return fmt.Errorf("%s连接失败或超时", label)
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if readErr != nil || response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s返回 HTTP %d", label, response.StatusCode)
	}
	var result struct {
		DingCode   *int `json:"errcode"`
		FeishuCode *int `json:"code"`
		LegacyCode *int `json:"StatusCode"`
	}
	if json.Unmarshal(body, &result) != nil {
		return fmt.Errorf("%s返回了无效的响应", label)
	}
	code := result.DingCode
	if provider == "feishu" {
		code = result.FeishuCode
		if code == nil {
			code = result.LegacyCode
		}
	}
	if code == nil {
		return fmt.Errorf("%s返回了无效的响应", label)
	}
	if *code != 0 {
		return fmt.Errorf("%s拒绝消息（错误码 %d），请检查关键词、签名或机器人状态", label, *code)
	}
	return nil
}

func (app *application) webhookRelay(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() || r.Method != http.MethodPost || app.webhook == nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "relay access denied"})
		return
	}
	state := app.webhook.snapshot()
	if !isRobotFormat(state.Format) || !boolSetting(state.Config, "enabled") {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "relay disabled"})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid payload"})
		return
	}
	mac := hmac.New(sha256.New, []byte(state.RelaySecret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(r.Header.Get("X-Vohive-Signature")), []byte(expected)) {
		writeJSON(w, 403, map[string]any{"error": "invalid relay signature"})
		return
	}
	var event map[string]any
	if json.Unmarshal(body, &event) != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid payload"})
		return
	}
	if err := app.webhook.send(r.Context(), state.Config, event); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (app *application) notificationSettings(w http.ResponseWriter, r *http.Request) {
	if _, status, err := app.validateAuth(r); err != nil {
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	if app.webhook == nil {
		writeJSON(w, 503, map[string]any{"error": "消息适配器暂不可用"})
		return
	}
	isTest := r.URL.Path == "/api/settings/notifications/webhook/test"
	if r.Method == http.MethodGet && !isTest {
		status, value, err := app.upstreamJSON(r, r.Method, r.URL.Path, nil, 15*time.Second)
		if err != nil {
			writeJSON(w, 502, map[string]any{"error": "无法读取消息配置"})
			return
		}
		state := app.webhook.snapshot()
		if root, ok := value.(map[string]any); ok && status == 200 && isRobotFormat(state.Format) {
			root["webhook"] = state.Config
		}
		writeJSON(w, status, value)
		return
	}
	if !sameOrigin(r) {
		writeJSON(w, 403, map[string]any{"error": "不允许跨站修改消息推送"})
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "方法不支持"})
		return
	}
	var payload map[string]any
	if !decodeJSON(w, r, &payload, 128<<10) {
		return
	}
	if isTest && r.Method != http.MethodPost || !isTest && r.Method != http.MethodPut {
		writeJSON(w, 405, map[string]any{"error": "方法不支持"})
		return
	}
	cfg := configMap(payload["webhook"])
	if isTest {
		cfg = payload
	}
	format := stringSetting(cfg, "format")
	if format != "" && format != "generic" && !isRobotFormat(format) {
		writeJSON(w, 400, map[string]any{"error": "未知 Webhook 格式"})
		return
	}
	if isRobotFormat(format) {
		if err := validateRobotConfig(cfg); err != nil {
			writeJSON(w, 400, map[string]any{"error": err.Error()})
			return
		}
		if isTest {
			if !boolSetting(cfg, "enabled") {
				writeJSON(w, 400, map[string]any{"error": "请先启用 Webhook 后再测试"})
				return
			}
			event := map[string]any{"text": "VoHiveX Webhook 测试提醒：消息推送连接正常。此为测试消息，无需处理。", "event": "test", "timestamp": time.Now().Format(time.RFC3339), "device_label": "VoHiveX"}
			if err := app.webhook.send(r.Context(), cfg, event); err != nil {
				writeJSON(w, 502, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, 200, map[string]any{"ok": true, "message": "群机器人已接收测试消息"})
			return
		}
	}
	if isTest {
		status, value, err := app.upstreamJSON(r, r.Method, r.URL.Path, payload, 30*time.Second)
		if err != nil {
			writeJSON(w, 502, map[string]any{"error": "测试服务暂不可用"})
			return
		}
		writeJSON(w, status, value)
		return
	}
	app.webhook.applyMu.Lock()
	defer app.webhook.applyMu.Unlock()
	before := app.webhook.snapshot()
	next := webhookAdapterState{Format: "generic", RelaySecret: before.RelaySecret}
	if isRobotFormat(format) {
		next.Format, next.Config = format, cfg
		payload["webhook"] = map[string]any{"enabled": boolSetting(cfg, "enabled"), "urls": []string{app.webhook.relayURL}, "secret": next.RelaySecret, "text_template": "", "timeout_ms": 60000, "retry_max": cfg["retry_max"]}
	}
	if err := app.webhook.save(next); err != nil {
		writeJSON(w, 500, map[string]any{"error": "消息配置保存失败"})
		return
	}
	status, value, err := app.upstreamJSON(r, r.Method, r.URL.Path, payload, 30*time.Second)
	// A transport timeout may happen after the core has saved the relay URL.
	// Keep the new adapter in that uncertain case, so notifications still have a destination.
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": "消息配置结果未确认，请重新读取配置核对"})
		return
	}
	if status != 200 {
		if restoreErr := app.webhook.save(before); restoreErr != nil {
			writeJSON(w, 500, map[string]any{"error": "消息配置恢复失败，请检查数据目录"})
			return
		}
	}
	writeJSON(w, status, value)
}
