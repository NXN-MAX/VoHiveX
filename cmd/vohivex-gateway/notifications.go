package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func (app *application) sendScheduledSMS(ctx context.Context, task *scheduledTask) (string, string) {
	username, password, err := configCredentials(app.configPath)
	if err != nil {
		return "failed", "无法登录本机 VoHiveX，已暂停，请检查应用配置"
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://local/", nil)
	status, value, err := app.upstreamJSON(request, http.MethodPost, "/api/auth/login", map[string]any{"username": username, "password": password}, 10*time.Second)
	if err != nil || status != http.StatusOK {
		return "failed", "本机 VoHiveX 登录失败，已暂停"
	}
	login, _ := value.(map[string]any)
	token, _ := login["token"].(string)
	if token == "" {
		return "failed", "本机 VoHiveX 登录失败，已暂停"
	}
	request.Header.Set("Authorization", "Bearer "+token)
	status, value, err = app.upstreamJSON(request, http.MethodGet, "/api/devices", nil, 15*time.Second)
	if err != nil || status != http.StatusOK {
		return "failed", "发信设备不存在、未在线或未启用短信，已暂停"
	}
	data, _ := value.(map[string]any)
	var device map[string]any
	for _, row := range deviceRows(data) {
		if fmt.Sprint(row["id"]) == task.DeviceID {
			device = row
		}
	}
	if device == nil || device["running"] != true || device["sms_enabled"] != true {
		return "failed", "发信设备不存在、未在线或未启用短信，已暂停"
	}
	task.DeviceName = fmt.Sprint(device["name"])
	if task.DeviceName == "" {
		task.DeviceName = task.DeviceID
	}
	status, value, err = app.upstreamJSON(request, http.MethodPost, "/api/sms/send", map[string]any{"device_id": task.DeviceID, "phone": task.Phone, "message": task.Message}, 120*time.Second)
	if err != nil {
		return "unknown", "发送结果未知，已暂停，请核对短信记录后再开始"
	}
	result, _ := value.(map[string]any)
	task.SendResult = map[string]any{}
	for _, key := range []string{"message_id", "delivery_state", "parts_total"} {
		if item, exists := result[key]; exists {
			task.SendResult[key] = item
		}
	}
	if status < 200 || status >= 300 || result["status"] == "error" || result["status"] == "failed" || result["delivery_state"] == "failed" {
		return "failed", fmt.Sprintf("发送接口未成功（HTTP %d），已暂停，请查看短信记录", status)
	}
	return "success", "已提交发送；实际投递状态请在短信中心查看"
}

func recipients(value any) []string {
	var source []string
	if items, ok := value.([]any); ok {
		for _, item := range items {
			source = append(source, fmt.Sprint(item))
		}
	} else {
		replacer := strings.NewReplacer(",", " ", "，", " ", ";", " ", "；", " ")
		source = strings.Fields(replacer.Replace(fmt.Sprint(value)))
	}
	seen := map[string]bool{}
	result := []string{}
	for _, item := range source {
		item = strings.TrimSpace(item)
		if item != "" && !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func eventStatus(event map[string]any) string {
	status := fmt.Sprint(event["status"])
	if event["phase"] == "delivery" && status == "success" {
		return "发送成功"
	}
	switch status {
	case "success":
		return "已提交发送（实际投递状态请查看短信中心）"
	case "failed":
		return "发送失败"
	default:
		return "发送结果未知，请核对短信记录"
	}
}
func eventTitle(event map[string]any) string {
	if event["phase"] == "delivery" {
		return "定时短信发送结果"
	}
	return "定时短信执行结果"
}
func formatNotification(event map[string]any) string {
	finished, _ := numberAny64(event["finished_at"])
	stamp := time.Unix(finished, 0).In(time.Local).Format("2006-01-02 15:04:05 -07:00")
	device := fmt.Sprint(event["device_name"])
	if device == "" {
		device = fmt.Sprint(event["device_id"])
	}
	return strings.Join([]string{eventTitle(event), "任务：" + fmt.Sprint(event["name"]), "发信设备：" + device, "收信号码：" + fmt.Sprint(event["phone"]), "时间：" + stamp, "状态：" + eventStatus(event), "详情：" + fmt.Sprint(event["detail"]), "短信内容：", fmt.Sprint(event["message"])}, "\n")
}
func numberAny64(value any) (int64, error) {
	switch item := value.(type) {
	case float64:
		return int64(item), nil
	case json.Number:
		return item.Int64()
	case int64:
		return item, nil
	case int:
		return int64(item), nil
	default:
		return strconv.ParseInt(fmt.Sprint(item), 10, 64)
	}
}

func postJSONWithOptions(ctx context.Context, endpoint string, payload any, headers map[string]string, secret, proxyAddress string, timeout time.Duration) (map[string]any, error) {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid endpoint")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		lower := strings.ToLower(key)
		if lower != "host" && lower != "content-length" && lower != "connection" && lower != "transfer-encoding" && lower != "x-vohive-signature" {
			request.Header.Set(key, value)
		}
	}
	if secret != "" {
		signature := hmac.New(sha256.New, []byte(secret))
		signature.Write(raw)
		request.Header.Set("X-Vohive-Signature", "sha256="+hex.EncodeToString(signature.Sum(nil)))
	}
	transport := &http.Transport{Proxy: nil}
	if proxyAddress != "" {
		proxyURL, parseErr := url.Parse(proxyAddress)
		if parseErr != nil || (proxyURL.Scheme != "http" && proxyURL.Scheme != "https") || proxyURL.Host == "" {
			return nil, fmt.Errorf("invalid HTTP proxy")
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	result := map[string]any{}
	if len(data) > 0 {
		_ = json.Unmarshal(data, &result)
	}
	return result, nil
}

func postJSON(ctx context.Context, endpoint string, payload any, headers map[string]string, secret string) (map[string]any, error) {
	return postJSONWithOptions(ctx, endpoint, payload, headers, secret, "", 10*time.Second)
}

func stringSetting(config map[string]any, key string) string {
	value := strings.TrimSpace(fmt.Sprint(config[key]))
	if value == "<nil>" {
		return ""
	}
	return value
}

func telegramEndpoint(template, token string) (string, error) {
	if template == "" {
		return "https://api.telegram.org/bot" + token + "/sendMessage", nil
	}
	if strings.Count(template, "%s") >= 2 {
		return fmt.Sprintf(template, token, "sendMessage"), nil
	}
	u, err := url.Parse(template)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("invalid Telegram base URL")
	}
	return strings.TrimRight(template, "/") + "/bot" + token + "/sendMessage", nil
}

func configMap(value any) map[string]any { result, _ := value.(map[string]any); return result }
func boolSetting(config map[string]any, key string) bool {
	value, _ := config[key].(bool)
	return value
}

func (app *application) dispatchNotification(ctx context.Context, event map[string]any) (map[string]string, error) {
	raw, err := osReadFile(app.configPath)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err = yaml.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	text := formatNotification(event)
	results := map[string]string{}
	channels := []string{"telegram", "feishu", "qq", "bark", "email", "pushplus", "webhook"}
	for _, channel := range channels {
		cfg := configMap(root[channel])
		if !boolSetting(cfg, "enabled") {
			continue
		}
		if err := sendChannel(ctx, channel, cfg, text, event); err != nil {
			results[channel] = "failed"
		} else {
			results[channel] = "success"
		}
	}
	return results, nil
}

var osReadFile = func(path string) ([]byte, error) { return os.ReadFile(path) }

func sendChannel(ctx context.Context, channel string, cfg map[string]any, text string, event map[string]any) error {
	switch channel {
	case "telegram":
		token := stringSetting(cfg, "bot_token")
		chat := stringSetting(cfg, "chat_id")
		if token == "" || chat == "" {
			return fmt.Errorf("missing recipient")
		}
		endpoint, err := telegramEndpoint(stringSetting(cfg, "base_url"), token)
		if err != nil {
			return err
		}
		parts := splitUTF16(text, 4000)
		for _, part := range parts {
			result, err := postJSONWithOptions(ctx, endpoint, map[string]any{"chat_id": chat, "text": part}, nil, "", stringSetting(cfg, "proxy"), 10*time.Second)
			if err != nil || result["ok"] != true {
				return fmt.Errorf("telegram rejected")
			}
		}
		return nil
	case "feishu":
		token, err := postJSON(ctx, "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal", map[string]any{"app_id": cfg["app_id"], "app_secret": cfg["app_secret"]}, nil, "")
		if err != nil || fmt.Sprint(token["code"]) != "0" {
			return fmt.Errorf("feishu auth rejected")
		}
		targets := recipients(cfg["chat_ids"])
		if len(targets) == 0 {
			targets = recipients(cfg["chat_id"])
		}
		if len(targets) == 0 {
			return fmt.Errorf("missing recipient")
		}
		for _, target := range targets {
			content, _ := json.Marshal(map[string]string{"text": text})
			idMaterial := fmt.Sprint(event["run_id"]) + fmt.Sprint(event["phase"]) + target
			idSum := sha256.Sum256([]byte(idMaterial))
			payload := map[string]any{"receive_id": target, "msg_type": "text", "content": string(content), "uuid": hex.EncodeToString(idSum[:16])}
			result, err := postJSON(ctx, "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id", payload, map[string]string{"Authorization": "Bearer " + fmt.Sprint(token["tenant_access_token"])}, "")
			if err != nil || fmt.Sprint(result["code"]) != "0" {
				return fmt.Errorf("feishu rejected")
			}
		}
		return nil
	case "qq":
		token, err := postJSON(ctx, "https://bots.qq.com/app/getAppAccessToken", map[string]any{"appId": cfg["app_id"], "clientSecret": cfg["app_secret"]}, nil, "")
		if err != nil {
			return err
		}
		headers := map[string]string{"Authorization": "QQBot " + fmt.Sprint(token["access_token"]), "X-Union-Appid": fmt.Sprint(cfg["app_id"])}
		targets := []struct{ kind, id string }{}
		for _, target := range recipients(cfg["group_ids"]) {
			targets = append(targets, struct{ kind, id string }{"groups", target})
		}
		for _, target := range recipients(cfg["direct_ids"]) {
			targets = append(targets, struct{ kind, id string }{"users", target})
		}
		if len(targets) == 0 {
			return fmt.Errorf("missing recipient")
		}
		for _, target := range targets {
			result, err := postJSON(ctx, "https://api.sgroup.qq.com/v2/"+target.kind+"/"+url.PathEscape(target.id)+"/messages", map[string]any{"content": text, "msg_type": 0}, headers, "")
			if err != nil || fmt.Sprint(result["id"]) == "" {
				return fmt.Errorf("QQ rejected")
			}
		}
		return nil
	case "bark":
		targets := recipients(cfg["urls"])
		if len(targets) == 0 {
			return fmt.Errorf("missing recipient")
		}
		payload := map[string]any{"title": eventTitle(event), "body": text}
		for _, key := range []string{"group", "icon", "level"} {
			if value := stringSetting(cfg, key); value != "" {
				payload[key] = value
			}
		}
		for _, endpoint := range targets {
			result, err := postJSON(ctx, endpoint, payload, nil, "")
			if err != nil || fmt.Sprint(result["code"]) != "200" {
				return fmt.Errorf("bark rejected")
			}
		}
		return nil
	case "pushplus":
		result, err := postJSON(ctx, "https://www.pushplus.plus/send", map[string]any{"token": cfg["token"], "title": eventTitle(event), "content": text, "template": "txt", "topic": cfg["topic"], "channel": cfg["channel"]}, nil, "")
		if err != nil || fmt.Sprint(result["code"]) != "200" {
			return fmt.Errorf("pushplus rejected")
		}
		return nil
	case "webhook":
		headers := map[string]string{}
		for key, value := range configMap(cfg["headers"]) {
			headers[key] = fmt.Sprint(value)
		}
		payload := map[string]any{"event": func() string {
			if event["phase"] == "delivery" {
				return "scheduled_sms.delivery"
			}
			return "scheduled_sms.completed"
		}(), "text": text, "title": eventTitle(event), "device_label": event["device_name"]}
		for key, value := range event {
			payload[key] = value
		}
		targets := recipients(cfg["urls"])
		if len(targets) == 0 {
			return fmt.Errorf("missing recipient")
		}
		timeout := 5 * time.Second
		if milliseconds, err := numberAny64(cfg["timeout_ms"]); err == nil {
			if milliseconds < 1000 {
				milliseconds = 1000
			}
			if milliseconds > 30000 {
				milliseconds = 30000
			}
			timeout = time.Duration(milliseconds) * time.Millisecond
		}
		for _, endpoint := range targets {
			if _, err := postJSONWithOptions(ctx, endpoint, payload, headers, stringSetting(cfg, "secret"), "", timeout); err != nil {
				return err
			}
		}
		return nil
	case "email":
		return sendEmail(cfg, eventTitle(event), text)
	default:
		return fmt.Errorf("unsupported channel")
	}
}

func splitUTF16(text string, limit int) []string {
	result := []string{}
	current := strings.Builder{}
	units := 0
	for _, char := range text {
		size := 1
		if char > 0xffff {
			size = 2
		}
		if units+size > limit {
			result = append(result, current.String())
			current.Reset()
			units = 0
		}
		current.WriteRune(char)
		units += size
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}

func sendEmail(cfg map[string]any, title, text string) error {
	host := fmt.Sprint(cfg["smtp_host"])
	port := fmt.Sprint(cfg["smtp_port"])
	secure, _ := cfg["use_ssl"].(bool)
	if port == "" || port == "<nil>" {
		if secure {
			port = "465"
		} else {
			port = "587"
		}
	}
	username := fmt.Sprint(cfg["username"])
	password := fmt.Sprint(cfg["password"])
	from := fmt.Sprint(cfg["from_address"])
	if from == "" || from == "<nil>" {
		from = username
	}
	targets := recipients(cfg["to_addresses"])
	if host == "" || len(targets) == 0 {
		return fmt.Errorf("missing email config")
	}
	address := net.JoinHostPort(host, port)
	var client *smtp.Client
	var err error
	if secure {
		connection, e := tls.Dial("tcp", address, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if e != nil {
			return e
		}
		client, err = smtp.NewClient(connection, host)
	} else {
		client, err = smtp.Dial(address)
	}
	if err != nil {
		return err
	}
	defer client.Close()
	if !secure {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err = client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		}
	}
	if username != "" && username != "<nil>" {
		if err = client.Auth(smtp.PlainAuth("", username, password, host)); err != nil {
			return err
		}
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	for _, target := range targets {
		if err = client.Rcpt(target); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: VoHiveX · %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", from, strings.Join(targets, ", "), title, text)
	if _, err = writer.Write([]byte(message)); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (app *application) processNotification(ctx context.Context) {
	row, err := app.tasks.ClaimNotification()
	if err != nil || row == nil {
		return
	}
	results, dispatchErr := app.dispatchNotification(ctx, row.Event)
	state := "sent"
	detailBytes, _ := json.Marshal(results)
	detail := string(detailBytes)
	if dispatchErr != nil {
		state = "failed"
		detail = "无法读取或处理消息推送配置"
	} else if len(results) == 0 {
		state = "disabled"
		detail = "未启用消息推送渠道"
	} else {
		for _, value := range results {
			if value == "failed" {
				state = "failed"
			}
		}
	}
	_ = app.tasks.FinishNotification(row.ID, state, detail)
}

func deliveryClassification(value map[string]any, requireCounts bool) (string, string, bool) {
	state := fmt.Sprint(value["state"])
	if state == "" {
		state = fmt.Sprint(value["delivery_state"])
	}
	switch state {
	case "failed":
		return "failed", "短信发送失败，短信状态接口已确认失败。", true
	case "timeout":
		return "unknown", "短信确认超时，不能确定是否发送成功，请核对短信中心。", true
	case "acked":
		if requireCounts {
			total, e1 := numberAny64(value["parts_total"])
			acks, e2 := numberAny64(value["acks"])
			if e1 != nil || e2 != nil || total <= 0 || acks < total {
				return "", "", false
			}
		}
		return "success", "短信发送成功，全部分段已获网络确认；不代表对方已收到或已阅读。", true
	}
	return "", "", false
}

func (app *application) queryDelivery(ctx context.Context, messageID string) (map[string]any, error) {
	username, password, err := configCredentials(app.configPath)
	if err != nil {
		return nil, err
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://local/", nil)
	status, value, err := app.upstreamJSON(request, http.MethodPost, "/api/auth/login", map[string]any{"username": username, "password": password}, 10*time.Second)
	if err != nil || status != http.StatusOK {
		return nil, fmt.Errorf("login failed")
	}
	login, _ := value.(map[string]any)
	request.Header.Set("Authorization", "Bearer "+fmt.Sprint(login["token"]))
	status, value, err = app.upstreamJSON(request, http.MethodGet, "/api/sms/delivery/"+url.PathEscape(messageID), nil, 10*time.Second)
	if err != nil || status != http.StatusOK {
		return nil, fmt.Errorf("query failed")
	}
	object, _ := value.(map[string]any)
	delivery, _ := object["delivery"].(map[string]any)
	return delivery, nil
}

func (app *application) processDelivery(ctx context.Context) {
	row, err := app.tasks.NextDelivery()
	if err != nil || row == nil {
		return
	}
	event := row.Event
	status := fmt.Sprint(event["status"])
	result := configMap(event["send_result"])
	messageID := fmt.Sprint(result["message_id"])
	finalStatus, detail, done := "", "", false
	if status == "failed" {
		finalStatus, detail, done = "failed", fmt.Sprint(event["detail"]), true
	} else if status != "success" {
		finalStatus, detail, done = "unknown", "发送请求结果不确定，无法安全关联短信状态，未自动重发。", true
	} else if messageID == "" || messageID == "<nil>" {
		finalStatus, detail, done = deliveryClassification(result, false)
		if !done {
			finalStatus, detail, done = "unknown", "短信已提交，但该通道没有返回可查询的消息ID或明确发送状态。", true
		}
	} else if value, queryErr := app.queryDelivery(ctx, messageID); queryErr == nil && fmt.Sprint(value["message_id"]) == messageID && (fmt.Sprint(value["device_id"]) == "" || fmt.Sprint(value["device_id"]) == fmt.Sprint(event["device_id"])) {
		finalStatus, detail, done = deliveryClassification(value, true)
	}
	if !done && time.Now().Unix() >= row.Deadline {
		finalStatus, detail, done = "unknown", "24小时内未获取到明确发送结果，已停止查询；未自动重发短信。", true
	}
	if done {
		_ = app.tasks.FinishDelivery(row, finalStatus, detail)
	} else {
		_ = app.tasks.DeferDelivery(row.RunID)
	}
}
