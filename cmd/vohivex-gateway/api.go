package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"
)

func proxyRows(value any) []map[string]any {
	var raw []any
	switch item := value.(type) {
	case []any:
		raw = item
	case map[string]any:
		raw, _ = item["proxies"].([]any)
	}
	result := []map[string]any{}
	for _, item := range raw {
		if row, ok := item.(map[string]any); ok {
			result = append(result, row)
		}
	}
	return result
}

func (app *application) builtInData(enabled bool) map[string]any {
	return map[string]any{"id": builtInProxyID, "name": "订阅节点 · 内置代理", "addr": app.proxy.endpoint(), "username": "", "password": "", "enabled": enabled}
}

func (app *application) coreLoginRequest(ctx *http.Request) (string, error) {
	username, password, err := configCredentials(app.configPath)
	if err != nil {
		return "", err
	}
	status, value, err := app.upstreamJSON(ctx, http.MethodPost, "/api/auth/login", map[string]any{"username": username, "password": password}, 10*time.Second)
	if err != nil || status != http.StatusOK {
		return "", errors.New("本机登录尚未就绪")
	}
	object, _ := value.(map[string]any)
	token, _ := object["token"].(string)
	if token == "" {
		return "", errors.New("本机登录尚未就绪")
	}
	return "Bearer " + token, nil
}

func (app *application) ensureBuiltInProxy(_ any) {
	request, _ := http.NewRequest(http.MethodGet, "http://local/", nil)
	token, err := app.coreLoginRequest(request)
	if err != nil {
		return
	}
	request.Header.Set("Authorization", token)
	status, value, err := app.upstreamJSON(request, http.MethodGet, "/api/upstream-proxies", nil, 10*time.Second)
	if err != nil || status != http.StatusOK {
		return
	}
	var existing map[string]any
	for _, row := range proxyRows(value) {
		if row["id"] == builtInProxyID {
			existing = row
			break
		}
	}
	enabled := false
	if existing != nil {
		enabled, _ = existing["enabled"].(bool)
	}
	data := app.builtInData(enabled)
	if existing != nil && existing["name"] == data["name"] && existing["addr"] == data["addr"] && fmt.Sprint(existing["username"]) == "" {
		return
	}
	path := "/api/upstream-proxies"
	method := http.MethodPost
	if existing != nil {
		path += "/" + builtInProxyID
		method = http.MethodPut
	}
	_, _, _ = app.upstreamJSON(request, method, path, data, 25*time.Second)
}

func apiError(w http.ResponseWriter, err error) {
	var conflict conflictError
	if errors.As(err, &conflict) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
}

func (app *application) managedProxyAPI(w http.ResponseWriter, r *http.Request) {
	devices, status, err := app.validateAuth(r)
	if err != nil {
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	path := r.URL.Path
	if r.Method == http.MethodGet && path == "/api/managed-proxy/public-ip" {
		snapshot := app.egressSnapshot(r.URL.Query().Get("refresh") == "1")
		rulesStatus, rules, _ := app.upstreamJSON(r, http.MethodGet, "/api/upstream-proxy-country-rules", nil, 10*time.Second)
		proxyStatus, proxies, _ := app.upstreamJSON(r, http.MethodGet, "/api/upstream-proxies", nil, 10*time.Second)
		var rulesValue any
		if rulesStatus == http.StatusOK {
			rulesValue = rules
		}
		var proxyValue any
		if proxyStatus == http.StatusOK {
			proxyValue = proxies
		}
		snapshot["devices"] = deviceEgress(deviceRows(devices), rulesValue, proxyValue, snapshot)
		writeJSON(w, http.StatusOK, snapshot)
		return
	}
	if r.Method == http.MethodGet && path == "/api/managed-proxy" {
		writeJSON(w, http.StatusOK, app.proxy.Snapshot())
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	if !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "不允许跨站修改代理"})
		return
	}
	payload := map[string]any{}
	if !decodeJSON(w, r, &payload, 4<<20) {
		return
	}
	action := strings.TrimPrefix(path, "/api/managed-proxy/")
	switch action {
	case "import":
		err = app.proxy.Import(payload)
	case "select", "pause":
		before := app.proxy.StateCopy()
		err = app.proxy.Change(action, payload)
		if err == nil {
			err = app.syncBuiltInEnabled(r, action == "select")
			if err != nil {
				app.proxy.Restore(before)
			}
		}
	case "refresh", "delete":
		err = app.proxy.Change(action, payload)
	case "test":
		var result map[string]any
		result, err = app.proxy.Test(fmt.Sprint(payload["node_id"]))
		if err == nil {
			writeJSON(w, http.StatusOK, result)
			return
		}
	case "attach":
		app.proxy.mu.Lock()
		err = app.proxy.checkVersion(payload)
		enabled := app.proxy.state.Enabled
		app.proxy.mu.Unlock()
		if err == nil && !enabled {
			err = invalidError{"请先选择并启用一个节点"}
		}
		if err == nil {
			err = app.syncBuiltInEnabled(r, true)
		}
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"message": "已关联前置代理，请在下方为它绑定运营商国家"})
			return
		}
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "接口不存在"})
		return
	}
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.proxy.Snapshot())
}

func (app *application) syncBuiltInEnabled(r *http.Request, enabled bool) error {
	status, value, err := app.upstreamJSON(r, http.MethodGet, "/api/upstream-proxies", nil, 10*time.Second)
	if err != nil || status != http.StatusOK {
		return invalidError{"无法读取内置代理状态"}
	}
	present := false
	for _, row := range proxyRows(value) {
		if row["id"] == builtInProxyID {
			present = true
		}
	}
	method, path := http.MethodPost, "/api/upstream-proxies"
	if present {
		method, path = http.MethodPut, path+"/"+builtInProxyID
	}
	status, _, err = app.upstreamJSON(r, method, path, app.builtInData(enabled), 25*time.Second)
	if err != nil || status < 200 || status >= 300 {
		return invalidError{"内置代理状态同步失败，请重试"}
	}
	return nil
}

func (app *application) upstreamProxyAPI(w http.ResponseWriter, r *http.Request, reverse *httputil.ReverseProxy) {
	_, status, err := app.validateAuth(r)
	if err != nil {
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	fixed := strings.TrimSuffix(r.URL.Path, "/") == "/api/upstream-proxies/"+builtInProxyID
	if fixed && r.Method == http.MethodDelete {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "内置代理为固定项，不可删除；可通过列表开关停用"})
		return
	}
	if r.Method == http.MethodGet && strings.TrimSuffix(r.URL.Path, "/") == "/api/upstream-proxies" {
		code, value, err := app.upstreamJSON(r, http.MethodGet, "/api/upstream-proxies", nil, 10*time.Second)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": "无法读取前置代理"})
			return
		}
		rows := proxyRows(value)
		var existing map[string]any
		others := []map[string]any{}
		for _, row := range rows {
			if row["id"] == builtInProxyID {
				existing = row
			} else {
				others = append(others, row)
			}
		}
		if existing == nil {
			existing = app.builtInData(false)
		}
		merged := append([]map[string]any{existing}, others...)
		if _, ok := value.([]any); ok {
			writeJSON(w, code, merged)
		} else {
			object, _ := value.(map[string]any)
			if object == nil {
				object = map[string]any{}
			}
			object["proxies"] = merged
			writeJSON(w, code, object)
		}
		return
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
		if !sameOrigin(r) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "不允许跨站修改代理"})
			return
		}
		payload := map[string]any{}
		if !decodeJSON(w, r, &payload, 1<<20) {
			return
		}
		if fixed || payload["id"] == builtInProxyID {
			if !fixed || r.Method != http.MethodPut {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "内置代理为固定项，请使用列表启用开关"})
				return
			}
			enabled, ok := payload["enabled"].(bool)
			if !ok {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "启用状态必须为布尔值"})
				return
			}
			if err := app.syncBuiltInEnabled(r, enabled); err != nil {
				apiError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, app.builtInData(enabled))
			return
		}
		code, value, err := app.upstreamJSON(r, r.Method, r.URL.RequestURI(), payload, 25*time.Second)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": "前置代理更新失败"})
			return
		}
		writeJSON(w, code, value)
		return
	}
	reverse.ServeHTTP(w, r)
}

func (app *application) schedulerAPI(w http.ResponseWriter, r *http.Request) {
	deviceData, status, err := app.validateAuth(r)
	if err != nil {
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if r.Method == http.MethodGet {
		if r.URL.Path == "/api/schedules" {
			tasks, err := app.tasks.List()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "无法读取任务"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks, "server_time": time.Now().Unix(), "timezone": "Asia/Shanghai"})
			return
		}
		if r.URL.Path == "/api/schedules/devices" {
			rows := deviceRows(deviceData)
			result := []map[string]any{}
			for _, row := range rows {
				result = append(result, map[string]any{"id": row["id"], "name": row["name"], "running": row["running"], "sms_enabled": row["sms_enabled"]})
			}
			writeJSON(w, http.StatusOK, map[string]any{"devices": result})
			return
		}
		if len(parts) == 4 && parts[3] == "history" {
			runs, err := app.tasks.History(parts[2])
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "无法读取执行记录"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "接口不存在"})
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	if !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "不允许跨站修改任务"})
		return
	}
	payload := map[string]any{}
	if !decodeJSON(w, r, &payload, 16<<10) {
		return
	}
	deviceExists := func(id string) bool {
		for _, row := range deviceRows(deviceData) {
			if fmt.Sprint(row["id"]) == id {
				return true
			}
		}
		return false
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/schedules" {
		if !deviceExists(fmt.Sprint(payload["device_id"])) {
			apiError(w, invalidError{"发信设备不存在，请重新选择"})
			return
		}
		id, err := app.tasks.Create(payload)
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
		return
	}
	if len(parts) == 3 && (r.Method == http.MethodPut || r.Method == http.MethodDelete) {
		action := "edit"
		if r.Method == http.MethodDelete {
			action = "delete"
		}
		if action == "edit" && !deviceExists(fmt.Sprint(payload["device_id"])) {
			apiError(w, invalidError{"发信设备不存在，请重新选择"})
			return
		}
		err = app.tasks.Change(parts[2], action, payload)
	} else if len(parts) == 4 && r.Method == http.MethodPost && (parts[3] == "start" || parts[3] == "pause") {
		err = app.tasks.Change(parts[2], parts[3], payload)
	} else {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "接口不存在"})
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "任务不存在"})
		return
	}
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func parseLimit(value string, fallback, maxValue int) int {
	number, err := strconv.Atoi(value)
	if err != nil || number < 1 {
		return fallback
	}
	if number > maxValue {
		return maxValue
	}
	return number
}

func timestampSortValue(value any) int64 {
	text := fmt.Sprint(value)
	if parsed, err := time.Parse(time.RFC3339, text); err == nil {
		return parsed.UnixMilli()
	}
	return 0
}

func (app *application) smsHistoryAPI(w http.ResponseWriter, r *http.Request, reverse *httputil.ReverseProxy) {
	deviceData, status, err := app.validateAuth(r)
	if err != nil {
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	path := r.URL.Path
	query := r.URL.Query()
	if path == "/api/sms/archive/import" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
			return
		}
		if !sameOrigin(r) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "不允许跨站导入短信"})
			return
		}
		var payload struct {
			DeviceID string           `json:"device_id"`
			Messages []map[string]any `json:"messages"`
		}
		if !decodeJSON(w, r, &payload, 16<<20) {
			return
		}
		if len(payload.Messages) < 1 || len(payload.Messages) > 20000 {
			apiError(w, invalidError{"归档必须包含 1–20000 条短信"})
			return
		}
		var device map[string]any
		for _, row := range deviceRows(deviceData) {
			if fmt.Sprint(row["id"]) == payload.DeviceID {
				device = row
			}
		}
		if device == nil {
			apiError(w, invalidError{"导入设备不存在，请重新选择"})
			return
		}
		inserted, err := app.archive.Import(device, payload.Messages)
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"inserted": inserted, "skipped": len(payload.Messages) - inserted})
		return
	}
	if r.Method == http.MethodGet && path == "/api/sms/contacts" {
		code, value, err := app.upstreamJSON(r, http.MethodGet, r.URL.RequestURI(), nil, 25*time.Second)
		if err != nil || code != http.StatusOK {
			writeJSON(w, code, value)
			return
		}
		archive, err := app.archive.Contacts(query.Get("device_id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "无法读取短信归档"})
			return
		}
		merged := map[string]map[string]any{}
		for _, row := range append(proxyRowsAsMaps(value), archive...) {
			key := fmt.Sprint(row["imsi"]) + "\x00" + fmt.Sprint(row["peer"])
			old := merged[key]
			if old == nil || timestampSortValue(row["last_timestamp"]) >= timestampSortValue(old["last_timestamp"]) {
				merged[key] = row
			}
		}
		rows := []map[string]any{}
		for _, row := range merged {
			rows = append(rows, row)
		}
		sortMapsByTime(rows, "last_timestamp", true)
		limit := parseLimit(query.Get("limit"), 200, 500)
		if len(rows) > limit {
			rows = rows[:limit]
		}
		writeJSON(w, http.StatusOK, rows)
		return
	}
	if r.Method == http.MethodGet && path == "/api/sms/thread" {
		code, value, err := app.upstreamJSON(r, http.MethodGet, r.URL.RequestURI(), nil, 25*time.Second)
		if err != nil || code != http.StatusOK {
			writeJSON(w, code, value)
			return
		}
		peer := strings.TrimSpace(query.Get("peer"))
		if peer == "" {
			apiError(w, invalidError{"缺少短信联系人"})
			return
		}
		limit := parseLimit(query.Get("limit"), 200, 500)
		archive, err := app.archive.Messages(peer, query.Get("device_id"), query.Get("imsi"), limit, query.Get("before_ts"), query.Get("before_id"))
		if err != nil {
			apiError(w, err)
			return
		}
		rows := append(proxyRowsAsMaps(value), archive...)
		sortMapsByTime(rows, "timestamp", false)
		if len(rows) > limit {
			rows = rows[len(rows)-limit:]
		}
		writeJSON(w, http.StatusOK, rows)
		return
	}
	if r.Method == http.MethodDelete && strings.HasPrefix(path, "/api/sms/messages/") {
		id, parseErr := strconv.ParseInt(strings.TrimPrefix(path, "/api/sms/messages/"), 10, 64)
		if parseErr != nil || id >= 0 {
			reverse.ServeHTTP(w, r)
			return
		}
		result, err := app.archive.DeleteMessage(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "无法删除短信"})
			return
		}
		status := http.StatusOK
		if result["deleted"] == int64(0) {
			status = http.StatusNotFound
			result = map[string]any{"error": "短信不存在"}
		}
		writeJSON(w, status, result)
		return
	}
	if r.Method == http.MethodDelete && path == "/api/sms/thread" {
		peer := strings.TrimSpace(query.Get("peer"))
		if peer == "" {
			apiError(w, invalidError{"缺少短信联系人"})
			return
		}
		archived, err := app.archive.DeleteThread(peer, query.Get("device_id"), query.Get("imsi"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "无法删除归档会话"})
			return
		}
		code, value, _ := app.upstreamJSON(r, http.MethodDelete, r.URL.RequestURI(), nil, 25*time.Second)
		if code != http.StatusOK && code != http.StatusNoContent && code != http.StatusNotFound && archived == 0 {
			writeJSON(w, code, value)
			return
		}
		result, _ := value.(map[string]any)
		if result == nil {
			result = map[string]any{}
		}
		result["deleted_imported"] = archived
		result["thread_empty"] = true
		writeJSON(w, http.StatusOK, result)
		return
	}
	reverse.ServeHTTP(w, r)
}

func proxyRowsAsMaps(value any) []map[string]any {
	raw, ok := value.([]any)
	if !ok {
		return []map[string]any{}
	}
	result := []map[string]any{}
	for _, item := range raw {
		if row, ok := item.(map[string]any); ok {
			result = append(result, row)
		}
	}
	return result
}
func sortMapsByTime(rows []map[string]any, key string, descending bool) {
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			left, right := timestampSortValue(rows[i][key]), timestampSortValue(rows[j][key])
			if (descending && left < right) || (!descending && left > right) {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
}
