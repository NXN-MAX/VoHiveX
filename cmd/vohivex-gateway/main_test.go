package main

import (
	"database/sql"
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
