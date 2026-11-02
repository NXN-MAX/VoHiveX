package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func logTestApp(t *testing.T, path string) *application {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/devices" || r.Header.Get("Authorization") != "Bearer valid-test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"devices": []any{}})
	}))
	t.Cleanup(upstream.Close)
	return &application{appLogPath: path, upstream: strings.TrimPrefix(upstream.URL, "http://")}
}

func clearTestLog(app *application, auth, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodDelete, "http://local/api/logs/history", nil)
	r.Header.Set("Authorization", auth)
	r.Header.Set("Origin", origin)
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	return w
}

func TestClearLogHistoryPreservesRotationAndAppendHandle(t *testing.T) {
	dir := t.TempDir()
	active, archive, link := filepath.Join(dir, "today.log"), filepath.Join(dir, "yesterday.log"), filepath.Join(dir, "app.log")
	for _, path := range []string{active, archive} {
		if err := os.WriteFile(path, []byte("old event\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(active, link); err != nil {
		t.Fatal(err)
	}
	logger, err := os.OpenFile(active, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	app := logTestApp(t, link)
	for i := 0; i < 2; i++ {
		if w := clearTestLog(app, "Bearer valid-test-token", "http://local"); w.Code != 200 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if data, err := os.ReadFile(active); err != nil || len(data) != 0 {
			t.Fatalf("active log not empty: %q %v", data, err)
		}
	}
	if _, err := logger.WriteString("new event\n"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(link); string(data) != "new event\n" {
		t.Fatalf("append handle broken: %q", data)
	}
	if data, _ := os.ReadFile(archive); string(data) != "old event\n" {
		t.Fatalf("archive changed: %q", data)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("rotation link was replaced")
	}
}

func TestClearLogHistoryRequiresAuthAndSameOrigin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("preserve\n"), 0600); err != nil {
		t.Fatal(err)
	}
	app := logTestApp(t, path)
	for _, tc := range []struct {
		auth, origin string
		status       int
	}{
		{"", "http://local", 401}, {"Bearer invalid", "http://local", 401},
		{"Bearer valid-test-token", "http://foreign", 403},
	} {
		w := clearTestLog(app, tc.auth, tc.origin)
		if w.Code != tc.status {
			t.Fatalf("status=%d want=%d", w.Code, tc.status)
		}
		if data, _ := os.ReadFile(path); string(data) != "preserve\n" {
			t.Fatal("unauthorized clear modified the log")
		}
	}
}

func TestClearLogHistoryAbsentAndInvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.log")
	app := logTestApp(t, path)
	if w := clearTestLog(app, "Bearer valid-test-token", ""); w.Code != 200 {
		t.Fatalf("absent log status=%d", w.Code)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("clear created a log file")
	}
	app.appLogPath = dir
	if w := clearTestLog(app, "Bearer valid-test-token", ""); w.Code != 500 {
		t.Fatalf("invalid file status=%d", w.Code)
	}
}

func TestLogOpenAPIPreservesExistingReadOperation(t *testing.T) {
	spec := map[string]any{"paths": map[string]any{"/logs/history": map[string]any{"get": map[string]any{"summary": "Original log history"}}}}
	if err := mergeOpenAPI(spec); err != nil {
		t.Fatal(err)
	}
	path := mapValue(mapValue(spec, "paths"), "/logs/history")
	if mapValue(path, "get")["summary"] != "Original log history" || path["delete"] == nil {
		t.Fatal("missing original GET or new DELETE operation")
	}
}
