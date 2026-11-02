package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type webhookTransport func(*http.Request) (*http.Response, error)

func (f webhookTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testDingConfig() map[string]any {
	return map[string]any{"format": "dingtalk", "enabled": true, "urls": []any{"https://oapi.dingtalk.com/robot/send?access_token=synthetic"}, "secret": "SECsynthetic", "timeout_ms": 5000, "retry_max": 0, "text_template": "服务警报\n{{device_label}}\n{{text}}"}
}

func newTestWebhook(t *testing.T) *webhookAdapter {
	t.Helper()
	a, err := newWebhookAdapter(filepath.Join(t.TempDir(), "adapter.json"), "http://127.0.0.1:7575"+webhookRelayPath)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestDingTalkPayloadSignatureAndAPIResult(t *testing.T) {
	a := newTestWebhook(t)
	cfg := testDingConfig()
	message := "收到短信\n号码：+12025550123\n内容：完整正文"
	a.client.Transport = webhookTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "oapi.dingtalk.com" {
			t.Fatal("unexpected destination")
		}
		q := r.URL.Query()
		mac := hmac.New(sha256.New, []byte("SECsynthetic"))
		mac.Write([]byte(q.Get("timestamp") + "\nSECsynthetic"))
		if q.Get("sign") != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
			t.Error("invalid DingTalk signature")
		}
		var data map[string]any
		_ = json.NewDecoder(r.Body).Decode(&data)
		if data["msgtype"] != "text" || configMap(data["text"])["content"] != "服务警报\n测试设备\n"+message || configMap(data["at"])["isAtAll"] != false {
			t.Errorf("unexpected payload: %v", data)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"errcode":0}`)), Header: make(http.Header)}, nil
	})
	if err := a.send(context.Background(), cfg, map[string]any{"text": message, "meta": map[string]any{"device_label": "测试设备"}}); err != nil {
		t.Fatal(err)
	}
	for _, response := range []string{`{"errcode":310000}`, `{"ok":true}`, `not-json`} {
		a.client.Transport = webhookTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
		})
		if err := a.send(context.Background(), cfg, map[string]any{"text": "test"}); err == nil {
			t.Fatalf("HTTP 200 falsely accepted: %s", response)
		}
	}
	a.client.Transport = webhookTransport(func(r *http.Request) (*http.Response, error) { return nil, fmt.Errorf("request failed at %s", r.URL) })
	if err := a.send(context.Background(), cfg, map[string]any{"text": "test"}); err == nil || strings.Contains(err.Error(), "synthetic") {
		t.Fatal("network error leaked token", err)
	}
	for _, endpoint := range []string{"http://oapi.dingtalk.com/robot/send?access_token=x", "https://oapi.dingtalk.com.evil.invalid/robot/send?access_token=x", "https://oapi.dingtalk.com/robot/send", "https://u:p@oapi.dingtalk.com/robot/send?access_token=x"} {
		cfg["urls"] = []any{endpoint}
		if validateRobotConfig(cfg) == nil {
			t.Fatal("accepted invalid destination")
		}
	}
	plain := "https://oapi.dingtalk.com/robot/send?access_token=x"
	if dingTalkURL(plain, "", time.Now()) != plain {
		t.Fatal("keyword-only URL changed")
	}
}

func TestWebhookRelayAuthenticatesAndPreservesContent(t *testing.T) {
	a := newTestWebhook(t)
	state := a.snapshot()
	state.Format = "dingtalk"
	state.Config = testDingConfig()
	_ = a.save(state)
	calls := 0
	a.client.Transport = webhookTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"errcode":0}`))}, nil
	})
	app := &application{webhook: a}
	body := `{"event":"sms","text":"Complete SMS +12025550123","meta":{"device_label":"Demo"}}`
	mac := hmac.New(sha256.New, []byte(state.RelaySecret))
	mac.Write([]byte(body))
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	for _, tc := range []struct {
		address, signature, body string
		want                     int
	}{
		{"192.0.2.1:1234", signature, body, 403}, {"127.0.0.1:1234", "", body, 403}, {"127.0.0.1:1234", signature, body + " ", 403}, {"127.0.0.1:1234", signature, body, 200},
	} {
		r := httptest.NewRequest("POST", "http://localhost"+webhookRelayPath, strings.NewReader(tc.body))
		r.RemoteAddr = tc.address
		r.Header.Set("X-Vohive-Signature", tc.signature)
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if calls != 1 {
		t.Fatal("unauthorized relay forwarded a message")
	}
}

func TestNotificationSettingsPersistenceAndGenericCompatibility(t *testing.T) {
	a := newTestWebhook(t)
	stored := map[string]any{"webhook": map[string]any{"enabled": false}}
	reject := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test" {
			writeJSON(w, 401, map[string]any{})
			return
		}
		if r.URL.Path == "/api/devices" {
			writeJSON(w, 200, map[string]any{})
			return
		}
		if r.Method == "GET" {
			writeJSON(w, 200, stored)
			return
		}
		if reject {
			writeJSON(w, 400, map[string]any{"error": "test rejection"})
			return
		}
		if r.Method == "POST" {
			writeJSON(w, 200, map[string]any{"ok": true})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&stored)
		writeJSON(w, 200, map[string]any{"applied": true})
	}))
	defer upstream.Close()
	app := &application{webhook: a, upstream: strings.TrimPrefix(upstream.URL, "http://")}
	request := func(method, path, origin, token string, payload any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(payload)
		r := httptest.NewRequest(method, "http://local"+path, strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", token)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, r)
		return w
	}
	path := "/api/settings/notifications"
	cfg := testDingConfig()
	payload := map[string]any{"webhook": cfg, "telegram": map[string]any{"enabled": true, "bot_token": "preserved"}}
	if w := request("PUT", path, "https://other.invalid", "Bearer test", payload); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := request("GET", path, "", "", nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request("PUT", path, "http://local", "Bearer test", payload); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	core := configMap(stored["webhook"])
	if core["secret"] != a.snapshot().RelaySecret || core["text_template"] != "" || fmt.Sprint(core["urls"]) != "["+a.relayURL+"]" || configMap(stored["telegram"])["bot_token"] != "preserved" {
		t.Fatal("core config was not safely adapted")
	}
	reopened, err := newWebhookAdapter(a.path, a.relayURL)
	if err != nil || reopened.snapshot().Format != "dingtalk" {
		t.Fatal("restart lost configuration", err)
	}
	info, _ := os.Stat(a.path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("secret file not private")
	}
	w := request("GET", path, "", "Bearer test", nil)
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if configMap(got["webhook"])["format"] != "dingtalk" || strings.Contains(w.Body.String(), a.snapshot().RelaySecret) {
		t.Fatal("GET leaked internal relay config")
	}
	reject = true
	w = request("PUT", path, "", "Bearer test", map[string]any{"webhook": map[string]any{"format": "generic", "enabled": false}})
	if w.Code != 400 || a.snapshot().Format != "dingtalk" {
		t.Fatal("rejected update was not rolled back")
	}
	reject = false
	generic := map[string]any{"format": "generic", "enabled": true, "urls": []any{"https://example.invalid/webhook"}, "secret": "generic-secret"}
	w = request("PUT", path, "", "Bearer test", map[string]any{"webhook": generic})
	if w.Code != 200 || a.snapshot().Format != "generic" || configMap(stored["webhook"])["secret"] != "generic-secret" {
		t.Fatal("generic webhook changed")
	}
	w = request("POST", path+"/webhook/test", "", "Bearer test", generic)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if _, err := url.Parse(a.relayURL); err != nil {
		t.Fatal(err)
	}
}

func TestScheduledNotificationsReachDingTalkRelay(t *testing.T) {
	a := newTestWebhook(t)
	state := a.snapshot()
	state.Format, state.Config = "dingtalk", testDingConfig()
	if err := a.save(state); err != nil {
		t.Fatal(err)
	}
	app := &application{webhook: a}
	relay := httptest.NewServer(app.routes())
	defer relay.Close()
	calls := 0
	a.client.Transport = webhookTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		text := stringSetting(configMap(payload["text"]), "content")
		for _, part := range []string{"Demo device", "+12025550123", "完整短信正文"} {
			if !strings.Contains(text, part) {
				t.Errorf("notification lost %q", part)
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"errcode":0}`))}, nil
	})
	for _, phase := range []string{"execution", "delivery"} {
		event := map[string]any{"device_name": "Demo device", "phone": "+12025550123", "message": "完整短信正文", "status": "success", "phase": phase, "finished_at": time.Now().Unix()}
		cfg := map[string]any{"urls": []any{relay.URL + webhookRelayPath}, "secret": state.RelaySecret}
		if err := sendChannel(context.Background(), "webhook", cfg, formatNotification(event), event); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatal("scheduled notifications were lost")
	}
}

const syntheticFeishuHook = "https://open.feishu.cn/open-apis/bot/v2/hook/00000000-0000-0000-0000-000000000001"

func TestFeishuSignaturePayloadAndResults(t *testing.T) {
	a := newTestWebhook(t)
	cfg := testDingConfig()
	cfg["format"], cfg["urls"], cfg["feishu_secret"] = "chat_robots", []any{syntheticFeishuHook}, "feishu-synthetic-secret"
	response := `{"code":0,"msg":"success"}`
	a.client.Transport = webhookTransport(func(r *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if r.URL.String() != syntheticFeishuHook || payload["msg_type"] != "text" || configMap(payload["content"])["text"] != "服务警报\nDemo\n完整正文 +12025550123" {
			t.Fatal("unexpected Feishu message")
		}
		timestamp, ok := payload["timestamp"].(string)
		if !ok || len(timestamp) != 10 {
			t.Fatal("Feishu requires seconds, not milliseconds")
		}
		mac := hmac.New(sha256.New, []byte(timestamp+"\nfeishu-synthetic-secret"))
		if payload["sign"] != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
			t.Fatal("Feishu signing key or algorithm mismatch")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
	})
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"code":0}`, true}, {`{"StatusCode":0}`, true}, {`{"code":19021}`, false},
		{`{"code":19021,"StatusCode":0}`, false}, {`{"errcode":0}`, false}, {`invalid`, false},
	} {
		response = tc.body
		err := a.send(context.Background(), cfg, map[string]any{"text": "完整正文 +12025550123", "device_label": "Demo"})
		if (err == nil) != tc.valid {
			t.Fatalf("result %s accepted=%v", tc.body, err == nil)
		}
	}
	for _, endpoint := range []string{
		strings.Replace(syntheticFeishuHook, "https:", "http:", 1),
		strings.Replace(syntheticFeishuHook, "open.feishu.cn", "open.feishu.cn.evil.invalid", 1),
		syntheticFeishuHook + "?redirect=1", syntheticFeishuHook + "#fragment",
		"https://open.feishu.cn/open-apis/bot/v2/hook/invalid",
	} {
		cfg["urls"] = []any{endpoint}
		if validateRobotConfig(cfg) == nil {
			t.Fatal("accepted invalid Feishu endpoint")
		}
	}
}

func TestMixedRobotsDoNotSkipSecondDestinationAfterRejection(t *testing.T) {
	a := newTestWebhook(t)
	cfg := testDingConfig()
	cfg["format"] = "chat_robots"
	cfg["urls"] = append(recipients(cfg["urls"]), syntheticFeishuHook)
	var destinations []string
	a.client.Transport = webhookTransport(func(r *http.Request) (*http.Response, error) {
		destinations = append(destinations, r.URL.Host)
		body := `{"code":0}`
		if r.URL.Host == "oapi.dingtalk.com" {
			body = `{"errcode":310000}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	if err := a.send(context.Background(), cfg, map[string]any{"text": "test"}); err == nil || strings.Contains(err.Error(), "synthetic") {
		t.Fatal("failure was hidden or secret exposed")
	}
	if len(destinations) != 2 || destinations[1] != "open.feishu.cn" {
		t.Fatal("Feishu was skipped", destinations)
	}
	state := a.snapshot()
	state.Format, state.Config = "chat_robots", cfg
	if err := a.save(state); err != nil {
		t.Fatal(err)
	}
	reopened, err := newWebhookAdapter(a.path, a.relayURL)
	if err != nil || reopened.snapshot().Format != "chat_robots" || len(recipients(reopened.snapshot().Config["urls"])) != 2 {
		t.Fatal("mixed config lost on restart", err)
	}
}
