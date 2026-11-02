package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func coreAuthTestConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("server:\n  port: 127.0.0.1:7576\nweb:\n  username: admin\n  password: admin\ndevices: []\n")
	if err := writeAtomic(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCoreLoginRequestCachesAndInvalidatesToken(t *testing.T) {
	loginCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/login" {
			http.NotFound(w, r)
			return
		}
		loginCalls++
		writeJSON(w, http.StatusOK, map[string]any{"token": "core-token"})
	}))
	defer server.Close()
	app := &application{
		configPath: coreAuthTestConfig(t),
		upstream:   strings.TrimPrefix(server.URL, "http://"),
	}
	request := httptest.NewRequest(http.MethodGet, "http://local/", nil)
	first, err := app.coreLoginRequest(request)
	if err != nil || first != "Bearer core-token" {
		t.Fatalf("first token=%q err=%v", first, err)
	}
	second, err := app.coreLoginRequest(request)
	if err != nil || second != first || loginCalls != 1 {
		t.Fatalf("second token=%q err=%v calls=%d", second, err, loginCalls)
	}
	app.invalidateCoreLogin(first)
	third, err := app.coreLoginRequest(request)
	if err != nil || third != first || loginCalls != 2 {
		t.Fatalf("third token=%q err=%v calls=%d", third, err, loginCalls)
	}
}

func TestCoreLoginRequestBacksOffAfterRateLimit(t *testing.T) {
	loginCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loginCalls++
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate limited"})
	}))
	defer server.Close()
	app := &application{
		configPath: coreAuthTestConfig(t),
		upstream:   strings.TrimPrefix(server.URL, "http://"),
	}
	request := httptest.NewRequest(http.MethodGet, "http://local/", nil)
	if _, err := app.coreLoginRequest(request); err == nil {
		t.Fatal("expected the first login to fail")
	}
	if _, err := app.coreLoginRequest(request); err == nil {
		t.Fatal("expected the backed-off login to fail locally")
	}
	if loginCalls != 1 {
		t.Fatalf("backoff must suppress repeated upstream logins, calls=%d", loginCalls)
	}
}

func TestAuthLoginAPICachesSuccessfulUserToken(t *testing.T) {
	loginCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/login" {
			http.NotFound(w, r)
			return
		}
		loginCalls++
		writeJSON(w, http.StatusOK, map[string]any{"token": "user-token"})
	}))
	defer server.Close()
	app := &application{
		configPath: coreAuthTestConfig(t),
		upstream:   strings.TrimPrefix(server.URL, "http://"),
	}
	body, err := json.Marshal(map[string]string{"username": "admin", "password": "changed-password"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("login response=%d body=%s", response.Code, response.Body.String())
	}
	token, err := app.coreLoginRequest(httptest.NewRequest(http.MethodGet, "http://local/", nil))
	if err != nil || token != "Bearer user-token" || loginCalls != 1 {
		t.Fatalf("token=%q err=%v calls=%d", token, err, loginCalls)
	}
}
