package main

import (
	"errors"
	"net/http"
	"os"
)

// Clear only the active application log read by the core's history endpoint.
// Truncate in place so the logger's open append handle and rotation link remain valid.
func (app *application) clearLogHistory(w http.ResponseWriter, r *http.Request) {
	if _, status, err := app.validateAuth(r); err != nil {
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	if !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "不允许跨站清空日志"})
		return
	}
	app.logMu.Lock()
	defer app.logMu.Unlock()
	file, err := os.OpenFile(app.appLogPath, os.O_WRONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusOK, map[string]any{"cleared": true})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "无法清空日志，请检查日志目录权限"})
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "日志文件不可用"})
		return
	}
	if err := file.Truncate(0); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "清空日志失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": true})
}
