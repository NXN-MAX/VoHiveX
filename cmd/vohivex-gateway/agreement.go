package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sync"
	"time"
)

const agreementConfirmation = "我同意并确认"

type agreementState struct {
	UserID     string `json:"user_id"`
	AcceptedAt int64  `json:"accepted_at"`
	Source     string `json:"source,omitempty"`
}

// The upstream authenticates one administrator account, without a user ID.
// Give that account a persistent identity independent of its editable username,
// password, session token, browser and application version.
type agreementStore struct {
	mu    sync.Mutex
	path  string
	state agreementState
}

func newAgreementStore(path string) (*agreementStore, error) {
	store := &agreementStore{path: path}
	raw, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(raw, &store.state); err != nil {
			return nil, err
		}
		if store.state.UserID == "" || store.state.AcceptedAt < 0 {
			return nil, errors.New("invalid account agreement state")
		}
		return store, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	store.state.UserID = hex.EncodeToString(id[:])
	if err := store.save(store.state); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *agreementStore) save(state agreementState) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(store.path, append(raw, '\n'), 0o600)
}

func (app *application) agreementAPI(w http.ResponseWriter, r *http.Request) {
	if _, status, err := app.validateAuth(r); err != nil {
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	if r.Method == http.MethodPost && !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "不允许跨站确认条款"})
		return
	}
	store := app.agreement
	if store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "确认记录暂时不可用，请重试"})
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if r.Method == http.MethodPost {
		var body struct {
			UserID             string `json:"user_id"`
			Confirmation       string `json:"confirmation"`
			PreviousAcceptedAt int64  `json:"previous_accepted_at"`
		}
		if !decodeJSON(w, r, &body, 4096) {
			return
		}
		if body.UserID != store.state.UserID {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "账号已变化，请刷新后重试"})
			return
		}
		if store.state.AcceptedAt == 0 {
			next := store.state
			now := time.Now().UnixMilli()
			switch {
			case body.Confirmation == agreementConfirmation:
				next.AcceptedAt, next.Source = now, "user"
			case body.Confirmation == "" && body.PreviousAcceptedAt >= 946684800000 && body.PreviousAcceptedAt <= now:
				// Import an existing explicit confirmation from the old browser key.
				// Its age is not an expiry condition.
				next.AcceptedAt, next.Source = body.PreviousAcceptedAt, "legacy_browser"
			default:
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请输入“我同意并确认”"})
				return
			}
			if err := store.save(next); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "确认记录保存失败，请重试"})
				return
			}
			store.state = next
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": store.state.UserID, "accepted": store.state.AcceptedAt > 0, "accepted_at": store.state.AcceptedAt,
	})
}
