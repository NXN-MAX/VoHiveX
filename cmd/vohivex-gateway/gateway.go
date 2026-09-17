package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const builtInProxyID = "vohive-mihomo"

var requestCount atomic.Uint64

func (app *application) routes() http.Handler {
	target := &url.URL{Scheme: "http", Host: app.upstream}
	reverse := httputil.NewSingleHostReverseProxy(target)
	reverse.ErrorLog = log.New(io.Discard, "", 0)
	reverse.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "服务暂时不可用，请稍后重试"})
	}
	reverse.ModifyResponse = func(response *http.Response) error {
		response.Header.Set("X-Content-Type-Options", "nosniff")
		return nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		path := r.URL.Path
		switch {
		case path == "/healthz":
			app.health(w, r)
		case path == "/metrics":
			app.metrics(w, r)
		case path == "/api/settings/system":
			app.systemMetadata(w, r)
		case path == "/api/settings/username":
			app.usernameSettings(w, r)
		case path == "/api/managed-proxy" || strings.HasPrefix(path, "/api/managed-proxy/"):
			app.managedProxyAPI(w, r)
		case path == "/api/upstream-proxies" || strings.HasPrefix(path, "/api/upstream-proxies/"):
			app.upstreamProxyAPI(w, r, reverse)
		case path == "/api/schedules" || strings.HasPrefix(path, "/api/schedules/"):
			app.schedulerAPI(w, r)
		case path == "/api/sms/archive/import" || path == "/api/sms/contacts" || path == "/api/sms/thread" || strings.HasPrefix(path, "/api/sms/messages/"):
			app.smsHistoryAPI(w, r, reverse)
		case app.serveAsset(w, r):
		default:
			reverse.ServeHTTP(w, r)
		}
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any, limit int64) bool {
	if r.Header.Get("Transfer-Encoding") != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "不支持分块上传"})
		return false
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{"error": "仅接受 JSON 请求"})
		return false
	}
	body := http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(body)
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式无效"})
		return false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求只能包含一个 JSON 对象"})
		return false
	}
	return true
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

func (app *application) upstreamJSON(r *http.Request, method, path string, value any, timeout time.Duration) (int, any, error) {
	var body io.Reader
	if value != nil {
		raw, err := json.Marshal(value)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(r.Context(), method, "http://"+app.upstream+path, body)
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if auth := r.Header.Get("Authorization"); auth != "" {
		request.Header.Set("Authorization", auth)
	}
	client := &http.Client{Timeout: timeout}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return response.StatusCode, nil, err
	}
	var result any
	if len(raw) > 0 && json.Unmarshal(raw, &result) != nil {
		result = map[string]any{}
	}
	return response.StatusCode, result, nil
}

func (app *application) validateAuth(r *http.Request) (map[string]any, int, error) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || len(auth) > 8192 {
		return nil, http.StatusUnauthorized, fmt.Errorf("请先登录 VoHiveX")
	}
	status, value, err := app.upstreamJSON(r, http.MethodGet, "/api/devices", nil, 15*time.Second)
	if err != nil {
		return nil, http.StatusServiceUnavailable, fmt.Errorf("VoHiveX 正在启动，请稍后重试")
	}
	if status != http.StatusOK {
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return nil, status, fmt.Errorf("登录已失效，请重新登录")
		}
		return nil, http.StatusServiceUnavailable, fmt.Errorf("暂时无法验证登录状态")
	}
	data, _ := value.(map[string]any)
	return data, 0, nil
}

func deviceRows(value map[string]any) []map[string]any {
	raw, _ := value["devices"].([]any)
	rows := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if row, ok := item.(map[string]any); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func (app *application) serveAsset(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" || name == "index.html" {
		name = "index.html"
	} else if strings.TrimSuffix(name, "/") == "api/docs" {
		name = "api/docs/index.html"
	}
	if strings.Contains(name, "..") {
		return false
	}
	root, _ := filepath.Abs(app.assets)
	path, _ := filepath.Abs(filepath.Join(root, filepath.FromSlash(name)))
	if path != root && !strings.HasPrefix(path, root+string(os.PathSeparator)) {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = http.DetectContentType(raw)
	}
	w.Header().Set("Content-Type", contentType)
	if filepath.Ext(path) == ".html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	if r.Method == http.MethodGet {
		_, _ = w.Write(raw)
	}
	return true
}

func (app *application) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	request, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://"+app.upstream+"/", nil)
	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "starting", "version": version})
		return
	}
	response.Body.Close()
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": version, "uptime_seconds": int(time.Since(app.started).Seconds())})
}

func (app *application) metrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP vohivex_info VoHiveX build information.\n# TYPE vohivex_info gauge\nvohivex_info{version=%q} 1\n", version)
	fmt.Fprintf(w, "# HELP vohivex_http_requests_total Requests handled by the gateway.\n# TYPE vohivex_http_requests_total counter\nvohivex_http_requests_total %d\n", requestCount.Load())
	fmt.Fprintf(w, "# HELP vohivex_uptime_seconds Gateway uptime.\n# TYPE vohivex_uptime_seconds gauge\nvohivex_uptime_seconds %.0f\n", time.Since(app.started).Seconds())
}

func driverVersion() string {
	if runtime.GOOS != "linux" {
		return "option / qmi_wwan · 当前系统未加载"
	}
	root := filepath.Join(getenv("QMI_SYS_ROOT", "/host-sys"), "module")
	parts := make([]string, 0, 2)
	for _, name := range []string{"option", "qmi_wwan"} {
		path := filepath.Join(root, name)
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			parts = append(parts, name+" · 未加载")
			continue
		}
		if raw, err := os.ReadFile(filepath.Join(path, "version")); err == nil && strings.TrimSpace(string(raw)) != "" {
			parts = append(parts, name+" · "+strings.TrimSpace(string(raw)))
		} else {
			parts = append(parts, name+" · 已加载")
		}
	}
	return strings.Join(parts, "\n")
}

func (app *application) systemMetadata(w http.ResponseWriter, r *http.Request) {
	if _, status, err := app.validateAuth(r); err != nil {
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	var buildTime any
	if raw, err := os.ReadFile(filepath.Join(app.assets, "build-info.json")); err == nil {
		var info map[string]any
		if json.Unmarshal(raw, &info) == nil {
			buildTime = info["build_time"]
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"system_time": time.Now().Unix(), "build_time": buildTime,
		"config_path": app.configPath, "driver_version": driverVersion(),
		"proxy_version": app.proxy.Version(), "gateway_version": version,
	})
}

func (app *application) usernameSettings(w http.ResponseWriter, r *http.Request) {
	if _, status, err := app.validateAuth(r); err != nil {
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	if r.Method == http.MethodGet {
		username, err := configUsername(app.configPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "用户名加载失败"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"username": username})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	if !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "不允许跨站修改账号"})
		return
	}
	var payload struct {
		Username string `json:"username"`
		Password string `json:"current_password"`
	}
	if !decodeJSON(w, r, &payload, 4096) {
		return
	}
	current, err := configUsername(app.configPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "无法读取当前账号"})
		return
	}
	status, result, err := app.upstreamJSON(r, http.MethodPost, "/api/auth/login", map[string]any{"username": current, "password": payload.Password}, 10*time.Second)
	valid := false
	if object, ok := result.(map[string]any); ok {
		_, valid = object["token"].(string)
	}
	if err != nil || status != http.StatusOK || !valid {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "当前密码不正确"})
		return
	}
	if payload.Username == current {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "新用户名与当前用户名相同"})
		return
	}
	if err := renameConfigUsername(app.configPath, payload.Username); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": payload.Username, "restart_required": true})
	go func() {
		time.Sleep(2 * time.Second)
		_ = syscallKillParent()
	}()
}

func syscallKillParent() error {
	process, err := os.FindProcess(os.Getppid())
	if err != nil {
		return err
	}
	return process.Signal(os.Interrupt)
}

func commandVersion(binary string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, "-v").CombinedOutput()
	if err != nil {
		return "暂不可用"
	}
	text := strings.TrimSpace(string(output))
	if len(text) > 100 {
		text = text[:100]
	}
	return text
}
