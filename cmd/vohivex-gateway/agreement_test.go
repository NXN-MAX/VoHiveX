package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func agreementTestApp(t *testing.T) (*application, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/devices" {
			t.Fatalf("unexpected upstream request: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer session-a" && r.Header.Get("Authorization") != "Bearer session-b" {
			writeJSON(w, 401, map[string]any{"error": "invalid session"})
			return
		}
		writeJSON(w, 200, map[string]any{"devices": []any{}})
	}))
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "account-agreement.json")
	store, err := newAgreementStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return &application{agreement: store, upstream: strings.TrimPrefix(server.URL, "http://")}, path
}

func agreementRequest(app *application, method, token, origin string, body map[string]any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, "http://local/api/settings/agreement", strings.NewReader(string(raw)))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	return w
}

func TestAgreementPersistsAcrossSessionsRestartsAndUsernameChanges(t *testing.T) {
	app, path := agreementTestApp(t)
	first := app.agreement.state
	if first.UserID == "" || first.AcceptedAt != 0 {
		t.Fatalf("unexpected initial state: %+v", first)
	}
	result := agreementRequest(app, "POST", "session-a", "http://local", map[string]any{"user_id": first.UserID, "confirmation": agreementConfirmation})
	if result.Code != 200 {
		t.Fatal(result.Code, result.Body.String())
	}
	saved := app.agreement.state
	if saved.AcceptedAt <= 0 {
		t.Fatal("confirmation was not persisted")
	}
	// A different session/browser reads the same server record without a timestamp.
	result = agreementRequest(app, "GET", "session-b", "", nil)
	var response map[string]any
	_ = json.Unmarshal(result.Body.Bytes(), &response)
	if result.Code != 200 || response["accepted"] != true || response["user_id"] != first.UserID {
		t.Fatal(result.Code, result.Body.String())
	}
	// Renaming the one upstream admin does not replace its persistent identity.
	app.configPath = coreAuthTestConfig(t)
	if err := renameConfigUsername(app.configPath, "renamed-admin"); err != nil {
		t.Fatal(err)
	}
	reopened, err := newAgreementStore(path)
	if err != nil || reopened.state != saved {
		t.Fatalf("restart lost agreement: %v %+v", err, reopened)
	}
	app.agreement = reopened
	result = agreementRequest(app, "POST", "session-b", "http://local", map[string]any{"user_id": first.UserID, "confirmation": agreementConfirmation})
	if result.Code != 200 || app.agreement.state != saved {
		t.Fatal("repeat confirmation changed the original record")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("agreement file permissions are not private")
	}
}

func TestAgreementMigratesExpiredLegacyBrowserConfirmation(t *testing.T) {
	app, path := agreementTestApp(t)
	previous := time.Now().Add(-365 * 24 * time.Hour).UnixMilli()
	result := agreementRequest(app, "POST", "session-a", "http://local", map[string]any{"user_id": app.agreement.state.UserID, "previous_accepted_at": previous})
	if result.Code != 200 || app.agreement.state.AcceptedAt != previous || app.agreement.state.Source != "legacy_browser" {
		t.Fatal(result.Code, result.Body.String())
	}
	reopened, err := newAgreementStore(path)
	if err != nil || reopened.state.AcceptedAt != previous {
		t.Fatal("legacy confirmation was not durable", err)
	}
}

func TestAgreementRejectsUnauthenticatedCrossOriginAndInvalidConfirmation(t *testing.T) {
	app, path := agreementTestApp(t)
	before, _ := os.ReadFile(path)
	for _, tc := range []struct {
		name, token, origin string
		body                map[string]any
		want                int
	}{
		{"anonymous", "", "", map[string]any{"confirmation": agreementConfirmation}, 401},
		{"expired session", "invalid", "", map[string]any{"confirmation": agreementConfirmation}, 401},
		{"cross origin", "session-a", "https://other.invalid", map[string]any{"confirmation": agreementConfirmation}, 403},
		{"wrong account", "session-a", "", map[string]any{"user_id": "another-account", "confirmation": agreementConfirmation}, 409},
		{"missing confirmation", "session-a", "", map[string]any{}, 400},
		{"wrong phrase", "session-a", "", map[string]any{"confirmation": "yes"}, 400},
		{"future legacy date", "session-a", "", map[string]any{"previous_accepted_at": time.Now().Add(time.Hour).UnixMilli()}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, exists := tc.body["user_id"]; !exists {
				tc.body["user_id"] = app.agreement.state.UserID
			}
			result := agreementRequest(app, "POST", tc.token, tc.origin, tc.body)
			after, _ := os.ReadFile(path)
			if result.Code != tc.want || string(before) != string(after) {
				t.Fatal(result.Code, result.Body.String(), "changed", string(before) != string(after))
			}
		})
	}
}

func TestAgreementSaveFailureDoesNotClaimAcceptance(t *testing.T) {
	app, _ := agreementTestApp(t)
	app.agreement.path = t.TempDir() // Rename cannot replace a directory.
	result := agreementRequest(app, "POST", "session-a", "", map[string]any{"user_id": app.agreement.state.UserID, "confirmation": agreementConfirmation})
	if result.Code != 500 || app.agreement.state.AcceptedAt != 0 {
		t.Fatal(result.Code, result.Body.String())
	}
}
