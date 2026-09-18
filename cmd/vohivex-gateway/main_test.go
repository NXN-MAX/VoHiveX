package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestPrepareConfigPreservesCredentialsAndComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := "# retained\nserver:\n  port: \"0.0.0.0:7575\" # internal listener\nweb:\n  username: \"kept-user\"\n  password: \"kept-secret\"\ndevices: []\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareConfig(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{"# retained", "# internal listener", "kept-user", "kept-secret", coreListen} {
		if !strings.Contains(text, required) {
			t.Fatalf("prepared config lost %q:\n%s", required, text)
		}
	}
	if err := prepareConfig(path); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(second) != text {
		t.Fatal("config preparation is not idempotent")
	}
}

func TestSMSArchiveKeepsStatusZeroAndDeduplicates(t *testing.T) {
	archive, err := newSMSArchive(filepath.Join(t.TempDir(), "sms.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	device := map[string]any{"id": "device-1", "name": "Test device", "imsi": "001010123456789"}
	rows := []map[string]any{{"peer": "+12025550123", "content": "Example", "type": 1, "timestamp": "2026-09-17T10:00:00+08:00", "status": 0, "message_id": "0"}}
	inserted, err := archive.Import(device, rows)
	if err != nil || inserted != 1 {
		t.Fatalf("first import: inserted=%d err=%v", inserted, err)
	}
	inserted, err = archive.Import(device, rows)
	if err != nil || inserted != 0 {
		t.Fatalf("duplicate import: inserted=%d err=%v", inserted, err)
	}
	messages, err := archive.Messages("+12025550123", "device-1", "", 200, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0]["status"] != int64(0) {
		t.Fatalf("status zero was not preserved: %#v", messages)
	}
}

func TestSchedulerNeverCatchesUpMissedOneTimeTask(t *testing.T) {
	store, err := newTaskStore(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().Unix()
	id, err := store.Create(map[string]any{"name": "One time", "device_id": "device-1", "phone": "+12025550123", "message": "Example", "mode": "once", "first_run": now + 2, "interval_seconds": 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec(`UPDATE tasks SET state='active',next_run=?,first_run=? WHERE id=?`, now-120, now-120, id); err != nil {
		t.Fatal(err)
	}
	if err = store.Recover(); err != nil {
		t.Fatal(err)
	}
	var state string
	var next sql.NullInt64
	var result string
	if err = store.db.QueryRow(`SELECT state,next_run,last_result FROM tasks WHERE id=?`, id).Scan(&state, &next, &result); err != nil {
		t.Fatal(err)
	}
	if state != "paused" || next.Valid || !strings.Contains(result, "停机期间") {
		t.Fatalf("unsafe recovery state=%s next=%v result=%s", state, next, result)
	}
}

func TestNewerDatabaseSchemaIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	store, err := newTaskStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec(`UPDATE vohivex_meta SET value='999' WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}
	store.Close()
	if _, err = newTaskStore(path); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("expected newer schema rejection, got %v", err)
	}
}

func TestProxyShareLinkParsing(t *testing.T) {
	nodes, err := parseProxyNodes("socks5://user:pass@example.com:1080#Example")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Config["type"] != "socks5" || nodes[0].Config["port"] != 1080 || nodes[0].Config["udp"] != true {
		t.Fatalf("unexpected node: %#v", nodes)
	}
}

func TestTelegramEndpointCompatibility(t *testing.T) {
	tests := []struct {
		template string
		want     string
	}{
		{"", "https://api.telegram.org/bottoken/sendMessage"},
		{"https://telegram.example/api", "https://telegram.example/api/bottoken/sendMessage"},
		{"https://telegram.example/bot%s/%s", "https://telegram.example/bottoken/sendMessage"},
	}
	for _, test := range tests {
		got, err := telegramEndpoint(test.template, "token")
		if err != nil || got != test.want {
			t.Fatalf("template %q: got %q, %v; want %q", test.template, got, err, test.want)
		}
	}
	if _, err := telegramEndpoint("file:///tmp/telegram", "token"); err == nil {
		t.Fatal("non-HTTP Telegram endpoint was accepted")
	}
}

func TestInMemoryStoresDoNotRequireFilesystemPermissions(t *testing.T) {
	store, err := newTaskStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	archive, err := newSMSArchive(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	archive.Close()
}

func TestSMSContactsAllFansOutByDevice(t *testing.T) {
	requested := map[string]bool{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("authorization header was not forwarded: %q", r.Header.Get("Authorization"))
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		switch r.URL.Path {
		case "/api/devices":
			writeJSON(w, http.StatusOK, map[string]any{"devices": []map[string]any{
				{"id": "device-1", "imsi": "001010123456789"},
				{"id": "device-2", "imsi": "001010987654321"},
			}})
		case "/api/sms/contacts":
			deviceID := r.URL.Query().Get("device_id")
			if deviceID == "all" || deviceID == "" {
				t.Errorf("all-devices request was forwarded without expansion: %q", r.URL.RawQuery)
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "device_id was not expanded"})
				return
			}
			requested[deviceID] = true
			writeJSON(w, http.StatusOK, []map[string]any{{
				"peer":           "+12025550123",
				"last_timestamp": "2026-09-18T12:00:00Z",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	archive, err := newSMSArchive(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	app := &application{upstream: strings.TrimPrefix(upstream.URL, "http://"), archive: archive}
	request := httptest.NewRequest(http.MethodGet, "/api/sms/contacts?device_id=all&limit=20", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body.String())
	}
	var contacts []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &contacts); err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 2 || !requested["device-1"] || !requested["device-2"] {
		t.Fatalf("unexpected fan-out: requests=%v contacts=%#v", requested, contacts)
	}
	seen := map[string]string{}
	for _, contact := range contacts {
		seen[contact["device_id"].(string)] = contact["imsi"].(string)
	}
	if seen["device-1"] != "001010123456789" || seen["device-2"] != "001010987654321" {
		t.Fatalf("device metadata was not attached: %#v", contacts)
	}
}

func TestMergeOpenAPIAddsGatewayAndLegacyPaths(t *testing.T) {
	spec := map[string]any{
		"openapi": "3.0.3",
		"info":    map[string]any{"title": "Core API", "version": "test"},
		"paths": map[string]any{
			"/devices": map[string]any{"get": map[string]any{"summary": "Core devices"}},
			"/cards/{iccid}/policy": map[string]any{
				"get": map[string]any{"summary": "Keep the upstream definition"},
			},
		},
	}
	if err := mergeOpenAPI(spec); err != nil {
		t.Fatal(err)
	}
	paths := spec["paths"].(map[string]any)
	for _, path := range []string{
		"/cards/policies",
		"/cards/{iccid}/policy",
		"/devices/{device_id}/actions/ussd/continue",
		"/devices/{device_id}/esim/profiles/{iccid}",
		"/settings/notifications/bark/test",
		"/settings/notifications/email/test",
		"/schedules",
		"/managed-proxy",
		"/sms/archive/import",
		"/settings/username",
		"/settings/system",
	} {
		if _, found := paths[path]; !found {
			t.Fatalf("missing merged path %s", path)
		}
	}
	policy := paths["/cards/{iccid}/policy"].(map[string]any)
	if policy["get"].(map[string]any)["summary"] != "Keep the upstream definition" {
		t.Fatal("merge overwrote an existing upstream operation")
	}
	if _, found := policy["put"]; !found {
		t.Fatal("merge did not add a missing operation to an existing path")
	}
}

func TestMergeOpenAPIUsesAPIPrefixWhenCorePathsUseIt(t *testing.T) {
	spec := map[string]any{
		"openapi": "3.0.3",
		"paths": map[string]any{
			"/api/devices":     map[string]any{},
			"/api/system/info": map[string]any{},
		},
	}
	if err := mergeOpenAPI(spec); err != nil {
		t.Fatal(err)
	}
	paths := spec["paths"].(map[string]any)
	if _, found := paths["/api/schedules"]; !found {
		t.Fatal("gateway path did not follow the upstream /api prefix convention")
	}
	if _, found := paths["/schedules"]; found {
		t.Fatal("gateway path was added with the wrong convention")
	}
}
