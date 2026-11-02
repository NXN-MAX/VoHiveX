package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const callStoreSchemaVersion = 1

var (
	callNumberPattern = regexp.MustCompile(`^\+?[0-9]{3,24}$`)
	callIDPattern     = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)
	clccLinePattern   = regexp.MustCompile(`(?i)^\+CLCC:\s*([0-9]+)\s*,\s*([0-9]+)\s*,\s*([0-9]+)\s*,\s*([0-9]+)\s*,\s*([0-9]+)(?:\s*,\s*"([^"\r\n]*)"\s*,\s*([0-9]+))?\s*$`)
	alsaDevicePattern = regexp.MustCompile(`^[A-Za-z0-9_.:,=-]{1,120}$`)
)

type callManager struct {
	db             *sql.DB
	recordingsDir  string
	locksMu        sync.Mutex
	deviceLocks    map[string]*sync.Mutex
	recorderMu     sync.Mutex
	recorder       *activeCallRecorder
	recordingError string
}

type parsedVoiceCall struct {
	Index     int    `json:"index"`
	Direction string `json:"direction"`
	State     string `json:"state"`
	Number    string `json:"number,omitempty"`
}

type callHistoryItem struct {
	ID              string `json:"id"`
	DeviceID        string `json:"device_id"`
	DeviceName      string `json:"device_name"`
	Index           int    `json:"index"`
	Direction       string `json:"direction"`
	State           string `json:"state"`
	Number          string `json:"number"`
	ContactName     string `json:"contact_name,omitempty"`
	StartedAt       int64  `json:"started_at"`
	ConnectedAt     *int64 `json:"connected_at,omitempty"`
	EndedAt         *int64 `json:"ended_at,omitempty"`
	DurationSeconds int64  `json:"duration_seconds"`
	Missed          bool   `json:"missed"`
	RecordingID     string `json:"recording_id,omitempty"`
}

type callContact struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Number    string `json:"number"`
	Note      string `json:"note,omitempty"`
	Favorite  bool   `json:"favorite"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type callRecording struct {
	ID              string `json:"id"`
	CallID          string `json:"call_id"`
	DeviceID        string `json:"device_id"`
	Number          string `json:"number"`
	Direction       string `json:"direction"`
	StartedAt       int64  `json:"started_at"`
	EndedAt         int64  `json:"ended_at"`
	DurationSeconds int64  `json:"duration_seconds"`
	MIMEType        string `json:"mime_type"`
	Size            int64  `json:"size"`
	AudioPath       string `json:"audio_path"`
	fileName        string
}

type activeCallRecorder struct {
	id         string
	call       callHistoryItem
	startedAt  time.Time
	tempPath   string
	finalPath  string
	cmd        *exec.Cmd
	done       chan struct{}
	processErr error
}

func newCallManager(path, recordingsDir string) (*callManager, error) {
	if recordingsDir == "" {
		return nil, errors.New("call recordings directory is required")
	}
	if err := os.MkdirAll(recordingsDir, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	statements := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=10000`,
		`CREATE TABLE IF NOT EXISTS call_meta(key TEXT PRIMARY KEY,value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS call_contacts(id TEXT PRIMARY KEY,name TEXT NOT NULL,number TEXT NOT NULL UNIQUE,note TEXT NOT NULL DEFAULT '',favorite INTEGER NOT NULL DEFAULT 0,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS call_history(id TEXT PRIMARY KEY,device_id TEXT NOT NULL,device_name TEXT NOT NULL DEFAULT '',call_index INTEGER NOT NULL DEFAULT -1,direction TEXT NOT NULL,state TEXT NOT NULL,number TEXT NOT NULL DEFAULT '',contact_name TEXT NOT NULL DEFAULT '',started_at INTEGER NOT NULL,connected_at INTEGER,ended_at INTEGER,duration_seconds INTEGER NOT NULL DEFAULT 0,missed INTEGER NOT NULL DEFAULT 0,recording_id TEXT NOT NULL DEFAULT '')`,
		`CREATE INDEX IF NOT EXISTS call_history_device_active ON call_history(device_id,ended_at,started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS call_history_started ON call_history(started_at DESC)`,
		`CREATE TABLE IF NOT EXISTS call_recordings(id TEXT PRIMARY KEY,call_id TEXT NOT NULL UNIQUE,device_id TEXT NOT NULL,number TEXT NOT NULL DEFAULT '',direction TEXT NOT NULL,started_at INTEGER NOT NULL,ended_at INTEGER NOT NULL,duration_seconds INTEGER NOT NULL,file_name TEXT NOT NULL UNIQUE,mime_type TEXT NOT NULL,size INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS call_recordings_started ON call_recordings(started_at DESC)`,
	}
	for _, statement := range statements {
		if _, err = db.Exec(statement); err != nil {
			db.Close()
			return nil, err
		}
	}
	var schema int
	err = db.QueryRow(`SELECT CAST(value AS INTEGER) FROM call_meta WHERE key='schema_version'`).Scan(&schema)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = db.Exec(`INSERT INTO call_meta(key,value)VALUES('schema_version',?)`, callStoreSchemaVersion)
		schema = callStoreSchemaVersion
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	if schema > callStoreSchemaVersion {
		db.Close()
		return nil, fmt.Errorf("call store schema %d is newer than supported schema %d", schema, callStoreSchemaVersion)
	}
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if err := os.Chmod(path, 0o600); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &callManager{db: db, recordingsDir: recordingsDir, deviceLocks: map[string]*sync.Mutex{}}, nil
}

func (manager *callManager) Close() error {
	manager.recorderMu.Lock()
	if manager.recorder != nil {
		_, _ = manager.stopRecordingLocked()
	}
	manager.recorderMu.Unlock()
	return manager.db.Close()
}

func (manager *callManager) deviceLock(deviceID string) *sync.Mutex {
	manager.locksMu.Lock()
	defer manager.locksMu.Unlock()
	if manager.deviceLocks[deviceID] == nil {
		manager.deviceLocks[deviceID] = &sync.Mutex{}
	}
	return manager.deviceLocks[deviceID]
}

func mapCLCCDirection(value int) (string, error) {
	switch value {
	case 0:
		return "outgoing", nil
	case 1:
		return "incoming", nil
	default:
		return "", errors.New("unknown CLCC direction")
	}
}

func mapCLCCState(value int) (string, error) {
	switch value {
	case 0:
		return "active", nil
	case 1:
		return "held", nil
	case 2:
		return "dialing", nil
	case 3:
		return "alerting", nil
	case 4:
		return "incoming", nil
	case 5:
		return "waiting", nil
	default:
		return "", errors.New("unknown CLCC state")
	}
}

// parseCLCCSnapshot validates every returned row before exposing a voice call.
// The core AT endpoint may strip the final OK line from a successful response,
// so a non-empty response made exclusively of valid +CLCC rows is accepted.
// QDC507 and EC20 expose packet data as mode 1 rows; those rows are valid but
// deliberately excluded from the phone topology.
func parseCLCCSnapshot(response string) ([]parsedVoiceCall, error) {
	normalized := strings.ReplaceAll(strings.ReplaceAll(response, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(normalized, "\n")
	result := make([]parsedVoiceCall, 0)
	seenIndexes := map[int]bool{}
	seenEcho, seenRecord, seenOK := false, false, false
	for lineNumber, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if seenOK {
			return nil, fmt.Errorf("CLCC response has content after OK at line %d", lineNumber+1)
		}
		if strings.EqualFold(line, "AT+CLCC") {
			if seenEcho || seenRecord {
				return nil, fmt.Errorf("CLCC response has a misplaced command echo at line %d", lineNumber+1)
			}
			seenEcho = true
			continue
		}
		if strings.EqualFold(line, "OK") {
			seenOK = true
			continue
		}
		match := clccLinePattern.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("CLCC response has an unexpected line at %d", lineNumber+1)
		}
		seenRecord = true
		values := make([]int, 5)
		for index := range values {
			value, err := strconv.Atoi(match[index+1])
			if err != nil {
				return nil, fmt.Errorf("CLCC response has an invalid field at line %d", lineNumber+1)
			}
			values[index] = value
		}
		index, directionValue, stateValue, mode, multiparty := values[0], values[1], values[2], values[3], values[4]
		if index < 1 || seenIndexes[index] || mode < 0 || mode > 2 || multiparty != 0 {
			return nil, fmt.Errorf("CLCC response has an unsupported topology at line %d", lineNumber+1)
		}
		seenIndexes[index] = true
		direction, err := mapCLCCDirection(directionValue)
		if err != nil {
			return nil, fmt.Errorf("CLCC response has an unknown direction at line %d", lineNumber+1)
		}
		state, err := mapCLCCState(stateValue)
		if err != nil {
			return nil, fmt.Errorf("CLCC response has an unknown state at line %d", lineNumber+1)
		}
		if match[7] != "" {
			numberType, err := strconv.Atoi(match[7])
			if err != nil || numberType < 0 || numberType > 255 {
				return nil, fmt.Errorf("CLCC response has an invalid number type at line %d", lineNumber+1)
			}
		}
		if mode == 0 {
			result = append(result, parsedVoiceCall{Index: index, Direction: direction, State: state, Number: strings.TrimSpace(match[6])})
		}
	}
	if !seenOK && !seenRecord {
		return nil, errors.New("CLCC response is missing terminal OK")
	}
	return result, nil
}

func callATResponse(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		for _, key := range []string{"response", "result", "data", "output"} {
			if nested, found := typed[key]; found {
				if text := callATResponse(nested); text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func callATExplicitOK(response string) bool {
	if callATRejection(response) != "" {
		return false
	}
	for _, line := range strings.FieldsFunc(response, func(r rune) bool { return r == '\r' || r == '\n' }) {
		if strings.EqualFold(strings.TrimSpace(line), "OK") {
			return true
		}
	}
	return false
}

// Only explicit modem finals prove a rejected command. Transport timeouts do
// not: a dial may have reached the modem before its response was lost.
func callATRejection(response string) string {
	for _, line := range strings.FieldsFunc(response, func(r rune) bool { return r == '\r' || r == '\n' }) {
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)
		if upper == "ERROR" || upper == "NO CARRIER" || upper == "NO ANSWER" || upper == "NO DIALTONE" || upper == "BUSY" || strings.HasPrefix(upper, "+CME ERROR:") || strings.HasPrefix(upper, "+CMS ERROR:") {
			return line
		}
	}
	return ""
}

type callATError struct {
	message  string
	rejected bool
}

func (err *callATError) Error() string { return err.message }

func callDevice(devices []map[string]any, id string) (map[string]any, bool) {
	for _, device := range devices {
		if strings.TrimSpace(fmt.Sprint(device["id"])) == id {
			return device, true
		}
	}
	return nil, false
}

func callDeviceName(device map[string]any) string {
	name := strings.TrimSpace(fmt.Sprint(device["name"]))
	if name == "" || name == "<nil>" {
		name = strings.TrimSpace(fmt.Sprint(device["id"]))
	}
	return name
}

func overviewDeviceRow(value any, deviceID string) map[string]any {
	object, _ := value.(map[string]any)
	if object == nil {
		return nil
	}
	if rows := deviceRows(object); len(rows) > 0 {
		for _, row := range rows {
			if strings.TrimSpace(fmt.Sprint(row["id"])) == deviceID {
				return row
			}
		}
		return rows[0]
	}
	return object
}

func (app *application) deviceLocalPhone(r *http.Request, device map[string]any) string {
	if app.media != nil && device["id"] == nativeCallDevice {
		return ""
	}
	for _, key := range []string{"local_phone", "msisdn", "phone_number"} {
		if value := deviceModemText(device, key); value != "" {
			return value
		}
	}
	deviceID := strings.TrimSpace(fmt.Sprint(device["id"]))
	if deviceID == "" || deviceID == "<nil>" {
		return ""
	}
	status, value, err := app.upstreamJSON(r, http.MethodGet, "/api/devices/"+url.PathEscape(deviceID)+"/overview", nil, 10*time.Second)
	if err != nil || status != http.StatusOK {
		return ""
	}
	detail := overviewDeviceRow(value, deviceID)
	for _, key := range []string{"local_phone", "msisdn", "phone_number"} {
		if phone := deviceModemText(detail, key); phone != "" {
			return phone
		}
	}
	return ""
}

func (app *application) deviceSummaries(r *http.Request, devices []map[string]any) []map[string]any {
	items := make([]map[string]any, 0, len(devices))
	for _, device := range devices {
		item := map[string]any{
			"id":          device["id"],
			"name":        callDeviceName(device),
			"running":     device["running"],
			"healthy":     device["healthy"],
			"imei":        deviceModemText(device, "imei"),
			"local_phone": app.deviceLocalPhone(r, device),
		}
		resolveCallRoute(device, nil).writeFields(item)
		items = append(items, item)
	}
	return items
}

func (app *application) runCallAT(r *http.Request, deviceID, command string, timeout time.Duration) (string, int, error) {
	if app.media != nil && deviceID == nativeCallDevice {
		ctx, cancel := context.WithTimeout(r.Context(), timeout+5*time.Second)
		defer cancel()
		value, err := app.media.run(ctx, "at", command)
		if err != nil {
			return "", http.StatusBadGateway, err
		}
		response, _ := value["response"].(string)
		return response, http.StatusOK, nil
	}
	path := "/api/devices/" + url.PathEscape(deviceID) + "/actions/at"
	status, value, err := app.upstreamJSON(r, http.MethodPost, path, map[string]any{"cmd": command, "timeout_ms": timeout.Milliseconds()}, timeout+5*time.Second)
	if err != nil {
		return "", http.StatusBadGateway, err
	}
	if status != http.StatusOK {
		message := "模组未接受电话指令"
		if object, ok := value.(map[string]any); ok {
			for _, key := range []string{"error", "message"} {
				if text, ok := object[key].(string); ok && strings.TrimSpace(text) != "" {
					message = strings.TrimSpace(text)
					break
				}
			}
		}
		// An unavailable port or a rejected request never submitted the ATD.
		rejected := status == http.StatusBadRequest || status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound ||
			(strings.HasPrefix(message, "打开 AT 端口 ") && strings.Contains(message, "失败")) ||
			message == "当前设备没有可用 AT 端口" || callATRejection(message) != ""
		return "", status, &callATError{message: message, rejected: rejected}
	}
	response := callATResponse(value)
	if response == "" {
		return "", http.StatusBadGateway, errors.New("模组返回了空的 AT 响应")
	}
	return response, http.StatusOK, nil
}

func (app *application) currentVoiceCalls(r *http.Request, deviceID string) ([]parsedVoiceCall, error) {
	response, _, err := app.runCallAT(r, deviceID, "AT+CLCC", 4*time.Second)
	if err != nil {
		return nil, err
	}
	return parseCLCCSnapshot(response)
}

func scanCallHistory(scanner interface{ Scan(...any) error }) (callHistoryItem, error) {
	var item callHistoryItem
	var connected, ended sql.NullInt64
	var missed int
	err := scanner.Scan(&item.ID, &item.DeviceID, &item.DeviceName, &item.Index, &item.Direction, &item.State, &item.Number, &item.ContactName, &item.StartedAt, &connected, &ended, &item.DurationSeconds, &missed, &item.RecordingID)
	if connected.Valid {
		item.ConnectedAt = &connected.Int64
	}
	if ended.Valid {
		item.EndedAt = &ended.Int64
	}
	item.Missed = missed != 0
	return item, err
}

const callHistoryColumns = `id,device_id,device_name,call_index,direction,state,number,contact_name,started_at,connected_at,ended_at,duration_seconds,missed,recording_id`

func (manager *callManager) contactName(number string) string {
	var name string
	_ = manager.db.QueryRow(`SELECT name FROM call_contacts WHERE number=?`, number).Scan(&name)
	return name
}

func callContactNameTx(tx *sql.Tx, number string) string {
	var name string
	_ = tx.QueryRow(`SELECT name FROM call_contacts WHERE number=?`, number).Scan(&name)
	return name
}

func (manager *callManager) activeCalls(deviceID string) ([]callHistoryItem, error) {
	rows, err := manager.db.Query(`SELECT `+callHistoryColumns+` FROM call_history WHERE device_id=? AND ended_at IS NULL ORDER BY started_at`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []callHistoryItem{}
	for rows.Next() {
		item, err := scanCallHistory(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (manager *callManager) reconcile(device map[string]any, calls []parsedVoiceCall, now time.Time) ([]callHistoryItem, error) {
	manager.locksMu.Lock()
	defer manager.locksMu.Unlock()
	deviceID := strings.TrimSpace(fmt.Sprint(device["id"]))
	deviceName := callDeviceName(device)
	tx, err := manager.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT `+callHistoryColumns+` FROM call_history WHERE device_id=? AND ended_at IS NULL ORDER BY started_at`, deviceID)
	if err != nil {
		return nil, err
	}
	existing := []callHistoryItem{}
	for rows.Next() {
		item, scanErr := scanCallHistory(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		existing = append(existing, item)
	}
	rows.Close()
	seen := map[string]bool{}
	endedIDs := map[string]bool{}
	for _, call := range calls {
		var matched *callHistoryItem
		for index := range existing {
			if seen[existing[index].ID] {
				continue
			}
			if existing[index].Index == call.Index || (existing[index].Index < 0 && existing[index].Direction == call.Direction && now.Unix()-existing[index].StartedAt <= 30) {
				matched = &existing[index]
				break
			}
		}
		if matched == nil {
			item := callHistoryItem{ID: uuid.NewString(), DeviceID: deviceID, DeviceName: deviceName, Index: call.Index, Direction: call.Direction, State: call.State, Number: call.Number, ContactName: callContactNameTx(tx, call.Number), StartedAt: now.Unix()}
			if call.State == "active" {
				connected := now.Unix()
				item.ConnectedAt = &connected
			}
			_, err = tx.Exec(`INSERT INTO call_history(id,device_id,device_name,call_index,direction,state,number,contact_name,started_at,connected_at)VALUES(?,?,?,?,?,?,?,?,?,?)`, item.ID, item.DeviceID, item.DeviceName, item.Index, item.Direction, item.State, item.Number, item.ContactName, item.StartedAt, item.ConnectedAt)
			if err != nil {
				return nil, err
			}
			existing = append(existing, item)
			matched = &existing[len(existing)-1]
		} else {
			if matched.Number == "" && call.Number != "" {
				matched.Number = call.Number
				matched.ContactName = callContactNameTx(tx, call.Number)
			}
			matched.Index = call.Index
			matched.State = call.State
			if call.State == "active" && matched.ConnectedAt == nil {
				connected := now.Unix()
				matched.ConnectedAt = &connected
			}
			_, err = tx.Exec(`UPDATE call_history SET device_name=?,call_index=?,state=?,number=?,contact_name=?,connected_at=? WHERE id=?`, deviceName, matched.Index, matched.State, matched.Number, matched.ContactName, matched.ConnectedAt, matched.ID)
			if err != nil {
				return nil, err
			}
		}
		seen[matched.ID] = true
	}
	for index := range existing {
		item := &existing[index]
		if seen[item.ID] {
			continue
		}
		if item.Index < 0 && now.Unix()-item.StartedAt < 12 {
			seen[item.ID] = true
			continue
		}
		ended := now.Unix()
		duration := int64(0)
		if item.ConnectedAt != nil && ended > *item.ConnectedAt {
			duration = ended - *item.ConnectedAt
		}
		missed := item.Direction == "incoming" && item.ConnectedAt == nil
		finalState := "ended"
		if item.Direction == "outgoing" && item.ConnectedAt == nil {
			finalState = "failed"
		}
		_, err = tx.Exec(`UPDATE call_history SET state=?,ended_at=?,duration_seconds=?,missed=? WHERE id=?`, finalState, ended, duration, missed, item.ID)
		if err != nil {
			return nil, err
		}
		endedIDs[item.ID] = true
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	manager.stopRecordingForEndedCalls(endedIDs)
	return manager.activeCalls(deviceID)
}

func (manager *callManager) stopRecordingForEndedCalls(ended map[string]bool) {
	if len(ended) == 0 {
		return
	}
	manager.recorderMu.Lock()
	shouldStop := manager.recorder != nil && ended[manager.recorder.call.ID]
	manager.recorderMu.Unlock()
	if shouldStop {
		go func() { _, _ = manager.stopRecording() }()
	}
}

func (manager *callManager) createOutgoing(device map[string]any, number string) (callHistoryItem, error) {
	item := callHistoryItem{ID: uuid.NewString(), DeviceID: strings.TrimSpace(fmt.Sprint(device["id"])), DeviceName: callDeviceName(device), Index: -1, Direction: "outgoing", State: "dialing", Number: number, ContactName: manager.contactName(number), StartedAt: time.Now().Unix()}
	_, err := manager.db.Exec(`INSERT INTO call_history(id,device_id,device_name,call_index,direction,state,number,contact_name,started_at)VALUES(?,?,?,?,?,?,?,?,?)`, item.ID, item.DeviceID, item.DeviceName, item.Index, item.Direction, item.State, item.Number, item.ContactName, item.StartedAt)
	return item, err
}

func (manager *callManager) failOutgoing(id string) error {
	_, err := manager.db.Exec(`UPDATE call_history SET state='failed',ended_at=? WHERE id=? AND call_index=-1 AND ended_at IS NULL`, time.Now().Unix(), id)
	return err
}

func (manager *callManager) history(deviceID string, limit int) ([]callHistoryItem, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	query := `SELECT ` + callHistoryColumns + ` FROM call_history`
	args := []any{}
	if deviceID != "" && deviceID != "all" {
		query += ` WHERE device_id=?`
		args = append(args, deviceID)
	}
	query += ` ORDER BY started_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := manager.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []callHistoryItem{}
	for rows.Next() {
		item, err := scanCallHistory(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (manager *callManager) deleteHistory(id string) error {
	result, err := manager.db.Exec(`DELETE FROM call_history WHERE id=? AND ended_at IS NOT NULL AND recording_id=''`, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return invalidError{"通话仍在进行、包含录音或记录不存在"}
	}
	return nil
}

func normalizeCallContact(name, number, note string) (string, string, string, error) {
	name = strings.TrimSpace(name)
	number = strings.TrimSpace(strings.ReplaceAll(number, " ", ""))
	note = strings.TrimSpace(note)
	if name == "" || len([]rune(name)) > 80 {
		return "", "", "", invalidError{"联系人姓名需为 1–80 个字符"}
	}
	if !callNumberPattern.MatchString(number) {
		return "", "", "", invalidError{"电话号码格式无效"}
	}
	if len([]rune(note)) > 240 {
		return "", "", "", invalidError{"备注不能超过 240 个字符"}
	}
	return name, number, note, nil
}

func (manager *callManager) contacts() ([]callContact, error) {
	rows, err := manager.db.Query(`SELECT id,name,number,note,favorite,created_at,updated_at FROM call_contacts ORDER BY favorite DESC,name COLLATE NOCASE,number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []callContact{}
	for rows.Next() {
		var item callContact
		var favorite int
		if err := rows.Scan(&item.ID, &item.Name, &item.Number, &item.Note, &favorite, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Favorite = favorite != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func (manager *callManager) saveContact(id, name, number, note string, favorite bool) (callContact, error) {
	name, number, note, err := normalizeCallContact(name, number, note)
	if err != nil {
		return callContact{}, err
	}
	now := time.Now().Unix()
	if id == "" {
		id = uuid.NewString()
		_, err = manager.db.Exec(`INSERT INTO call_contacts(id,name,number,note,favorite,created_at,updated_at)VALUES(?,?,?,?,?,?,?)`, id, name, number, note, favorite, now, now)
	} else {
		result, updateErr := manager.db.Exec(`UPDATE call_contacts SET name=?,number=?,note=?,favorite=?,updated_at=? WHERE id=?`, name, number, note, favorite, now, id)
		err = updateErr
		if err == nil {
			if count, _ := result.RowsAffected(); count == 0 {
				err = invalidError{"联系人不存在"}
			}
		}
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return callContact{}, invalidError{"该号码已存在于通讯录"}
		}
		return callContact{}, err
	}
	var item callContact
	var favoriteValue int
	err = manager.db.QueryRow(`SELECT id,name,number,note,favorite,created_at,updated_at FROM call_contacts WHERE id=?`, id).Scan(&item.ID, &item.Name, &item.Number, &item.Note, &favoriteValue, &item.CreatedAt, &item.UpdatedAt)
	item.Favorite = favoriteValue != 0
	return item, err
}

func (manager *callManager) deleteContact(id string) error {
	result, err := manager.db.Exec(`DELETE FROM call_contacts WHERE id=?`, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return invalidError{"联系人不存在"}
	}
	return nil
}

func detectCallAudioDevice() (string, bool, string) {
	if _, err := exec.LookPath("arecord"); err != nil {
		return "", false, "容器未包含 ALSA 录音工具"
	}
	configured := strings.TrimSpace(os.Getenv("VOHIVEX_CALL_AUDIO_DEVICE"))
	if strings.EqualFold(configured, "disabled") {
		return "", false, "录音已通过环境变量禁用"
	}
	if configured != "" {
		if !alsaDevicePattern.MatchString(configured) {
			return "", false, "录音设备配置无效"
		}
		return configured, true, "已使用配置的 ALSA 捕获设备"
	}
	paths, _ := filepath.Glob("/dev/snd/pcmC*D*c")
	if len(paths) == 0 {
		return "", false, "未检测到 ALSA 捕获设备"
	}
	cardsRaw, _ := os.ReadFile("/proc/asound/cards")
	cardsText := string(cardsRaw)
	for _, path := range paths {
		match := regexp.MustCompile(`^pcmC([0-9]+)D([0-9]+)c$`).FindStringSubmatch(filepath.Base(path))
		if match == nil {
			continue
		}
		cardID, _ := os.ReadFile(filepath.Join("/sys/class/sound", "card"+match[1], "id"))
		if len(cardID) == 0 {
			cardID, _ = os.ReadFile(filepath.Join("/proc/asound", "card"+match[1], "id"))
		}
		usbID, _ := os.ReadFile(filepath.Join("/proc/asound", "card"+match[1], "usbid"))
		identity := strings.ToLower(string(cardID) + " " + string(usbID) + " " + usbSoundIdentity("/sys/class/sound/card"+match[1]) + " " + alsaCardDescription(cardsText, match[1]))
		for _, marker := range []string{"2ca3", "2c7c", "qdc507", "quectel", "dji", "mavo"} {
			if strings.Contains(identity, marker) {
				return "hw:" + match[1] + "," + match[2], true, "已识别模组 ALSA 捕获设备"
			}
		}
	}
	return "", false, "未检测到蜂窝模组录音声卡；NAS 主板声卡不能代替通话音频"
}

func alsaCardDescription(cards, cardIndex string) string {
	lines := strings.Split(cards, "\n")
	for index, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != cardIndex || !strings.Contains(line, "[") {
			continue
		}
		if index+1 < len(lines) {
			return line + " " + lines[index+1]
		}
		return line
	}
	return ""
}

func (manager *callManager) recordingState() map[string]any {
	device, available, detail := detectCallAudioDevice()
	manager.recorderMu.Lock()
	defer manager.recorderMu.Unlock()
	state := map[string]any{"available": available, "detail": detail, "active": false}
	if device != "" {
		state["device"] = device
	}
	if manager.recorder != nil && recorderFinished(manager.recorder) {
		if _, err := manager.stopRecordingLocked(); err != nil {
			manager.recordingError = "录音进程已退出，未生成有效音频；请检查模组音频通道"
		}
	}
	if manager.recordingError != "" {
		state["error"] = manager.recordingError
		state["detail"] = manager.recordingError
	}
	if manager.recorder != nil {
		state["active"] = true
		state["id"] = manager.recorder.id
		state["call_id"] = manager.recorder.call.ID
		state["device_id"] = manager.recorder.call.DeviceID
		state["started_at"] = manager.recorder.startedAt.Unix()
	}
	return state
}

func (manager *callManager) callByID(id string) (callHistoryItem, error) {
	return scanCallHistory(manager.db.QueryRow(`SELECT `+callHistoryColumns+` FROM call_history WHERE id=?`, id))
}

func (manager *callManager) startRecording(callID string) (map[string]any, error) {
	call, err := manager.callByID(callID)
	if err != nil || call.EndedAt != nil || call.State != "active" {
		return nil, invalidError{"仅能为当前已接通的电话录音"}
	}
	device, available, detail := detectCallAudioDevice()
	if !available {
		return nil, invalidError{detail}
	}
	manager.recorderMu.Lock()
	defer manager.recorderMu.Unlock()
	if manager.recorder != nil && recorderFinished(manager.recorder) {
		_, _ = manager.stopRecordingLocked()
	}
	if manager.recorder != nil {
		return nil, invalidError{"已有通话正在录音"}
	}
	manager.recordingError = ""
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	tempPath := filepath.Join(manager.recordingsDir, "."+id+".part.wav")
	finalPath := filepath.Join(manager.recordingsDir, id+".wav")
	// Keep live recordings private too, before the final archive chmod.
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("无法创建录音文件: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tempPath)
		return nil, err
	}
	command := exec.Command("arecord", "-q", "-D", device, "-t", "wav", "-f", "S16_LE", "-r", "8000", "-c", "1", tempPath)
	if err := command.Start(); err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("无法启动录音: %w", err)
	}
	recorder := &activeCallRecorder{id: id, call: call, startedAt: time.Now(), tempPath: tempPath, finalPath: finalPath, cmd: command, done: make(chan struct{})}
	manager.recorder = recorder
	go func() { recorder.processErr = command.Wait(); close(recorder.done) }()
	if err := awaitRecorderSamples(recorder, 3*time.Second); err != nil {
		stopRecorderProcess(recorder)
		manager.recorder = nil
		_ = os.Remove(tempPath)
		manager.recordingError = err.Error()
		return nil, invalidError{manager.recordingError}
	}
	return map[string]any{"active": true, "id": id, "call_id": callID, "started_at": recorder.startedAt.Unix(), "device": device}, nil
}

func (manager *callManager) stopRecording() (callRecording, error) {
	manager.recorderMu.Lock()
	defer manager.recorderMu.Unlock()
	return manager.stopRecordingLocked()
}

func (manager *callManager) stopRecordingLocked() (callRecording, error) {
	recorder := manager.recorder
	manager.recorder = nil
	if recorder == nil {
		return callRecording{}, invalidError{"当前没有正在录制的通话"}
	}
	stopRecorderProcess(recorder)
	processErr := recorder.processErr
	info, statErr := os.Stat(recorder.tempPath)
	if statErr != nil || info.Size() <= 44 {
		_ = os.Remove(recorder.tempPath)
		if processErr != nil {
			return callRecording{}, fmt.Errorf("录音失败: %w", processErr)
		}
		return callRecording{}, errors.New("录音文件没有有效音频")
	}
	duration, audioErr := wavDuration(recorder.tempPath)
	if audioErr != nil {
		_ = os.Remove(recorder.tempPath)
		return callRecording{}, errors.New("录音文件没有有效的 PCM 音频")
	}
	if err := os.Chmod(recorder.tempPath, 0o600); err != nil {
		return callRecording{}, err
	}
	if err := os.Rename(recorder.tempPath, recorder.finalPath); err != nil {
		_ = os.Remove(recorder.tempPath)
		return callRecording{}, err
	}
	ended := time.Now()
	item := callRecording{ID: recorder.id, CallID: recorder.call.ID, DeviceID: recorder.call.DeviceID, Number: recorder.call.Number, Direction: recorder.call.Direction, StartedAt: recorder.startedAt.Unix(), EndedAt: ended.Unix(), DurationSeconds: duration, MIMEType: "audio/wav", Size: info.Size(), AudioPath: "/api/calls/recordings/" + recorder.id + "/audio", fileName: filepath.Base(recorder.finalPath)}
	tx, err := manager.db.Begin()
	if err != nil {
		_ = os.Remove(recorder.finalPath)
		return callRecording{}, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO call_recordings(id,call_id,device_id,number,direction,started_at,ended_at,duration_seconds,file_name,mime_type,size)VALUES(?,?,?,?,?,?,?,?,?,?,?)`, item.ID, item.CallID, item.DeviceID, item.Number, item.Direction, item.StartedAt, item.EndedAt, item.DurationSeconds, item.fileName, item.MIMEType, item.Size)
	if err == nil {
		_, err = tx.Exec(`UPDATE call_history SET recording_id=? WHERE id=?`, item.ID, item.CallID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		_ = os.Remove(recorder.finalPath)
		return callRecording{}, err
	}
	return item, nil
}

func scanCallRecording(scanner interface{ Scan(...any) error }) (callRecording, error) {
	var item callRecording
	err := scanner.Scan(&item.ID, &item.CallID, &item.DeviceID, &item.Number, &item.Direction, &item.StartedAt, &item.EndedAt, &item.DurationSeconds, &item.fileName, &item.MIMEType, &item.Size)
	item.AudioPath = "/api/calls/recordings/" + item.ID + "/audio"
	return item, err
}

const callRecordingColumns = `id,call_id,device_id,number,direction,started_at,ended_at,duration_seconds,file_name,mime_type,size`

func (manager *callManager) recordings() ([]callRecording, error) {
	rows, err := manager.db.Query(`SELECT ` + callRecordingColumns + ` FROM call_recordings ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []callRecording{}
	for rows.Next() {
		item, err := scanCallRecording(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (manager *callManager) recording(id string) (callRecording, error) {
	return scanCallRecording(manager.db.QueryRow(`SELECT `+callRecordingColumns+` FROM call_recordings WHERE id=?`, id))
}

func (manager *callManager) deleteRecording(id string) error {
	item, err := manager.recording(id)
	if err != nil {
		return invalidError{"录音不存在"}
	}
	path := filepath.Join(manager.recordingsDir, item.fileName)
	if filepath.Dir(path) != filepath.Clean(manager.recordingsDir) {
		return errors.New("invalid recording path")
	}
	tx, err := manager.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE call_history SET recording_id='' WHERE recording_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM call_recordings WHERE id=?`, id); err != nil {
		return err
	}
	if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return tx.Commit()
}

func callAPIError(w http.ResponseWriter, err error) {
	var invalid invalidError
	if errors.As(err, &invalid) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": invalid.Error()})
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "记录不存在"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "电话模块处理失败"})
}

func (app *application) callsAPI(w http.ResponseWriter, r *http.Request) {
	snapshot, authStatus, authErr := app.validateAuth(r)
	if authErr != nil {
		writeJSON(w, authStatus, map[string]any{"error": authErr.Error()})
		return
	}
	if app.localCallsOnly {
		app.rememberCoreLogin(r.Header.Get("Authorization"))
	}
	if r.Method != http.MethodGet && !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "不允许跨站控制电话"})
		return
	}
	devices := app.callDevices(snapshot)
	path := strings.TrimSuffix(r.URL.Path, "/")
	if strings.HasPrefix(path, "/api/calls/audio/") && r.Method == http.MethodPost {
		app.browserAudioAPI(w, r, devices)
		return
	}
	if path == "/api/calls/diagnostics" && r.Method == http.MethodGet {
		app.callDiagnosticsAPI(w, r, devices)
		return
	}
	if path == "/api/calls/devices" && r.Method == http.MethodGet {
		items := app.deviceSummaries(r, devices)
		writeJSON(w, http.StatusOK, map[string]any{"devices": items, "audio": app.calls.recordingState()})
		return
	}
	if path == "/api/calls/status" && r.Method == http.MethodGet {
		deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
		device, found := callDevice(devices, deviceID)
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "电话设备不存在"})
			return
		}
		if device["audio_preparing"] == true {
			active, _ := app.calls.activeCalls(deviceID)
			result := map[string]any{"device_id": deviceID, "calls": active, "audio": app.browserAudioState(deviceID), "updated_at": time.Now().Unix()}
			resolveCallRoute(device, active).writeFields(result)
			writeJSON(w, http.StatusOK, result)
			return
		}
		lock := app.calls.deviceLock(deviceID)
		lock.Lock()
		active, historyErr := app.calls.activeCalls(deviceID)
		route := resolveCallRoute(device, active)
		var err error
		if historyErr == nil && route.Transport == "cellular" {
			var calls []parsedVoiceCall
			calls, err = app.currentVoiceCalls(r, deviceID)
			if err == nil {
				active, err = app.calls.reconcile(device, calls, time.Now())
				route = resolveCallRoute(device, active)
			}
		}
		lock.Unlock()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": "无法读取通话状态: " + err.Error(), "calls": active, "audio": app.calls.recordingState()})
			return
		}
		if historyErr != nil {
			callAPIError(w, historyErr)
			return
		}
		app.finishBrowserRecording()
		audio := app.browserAudioState(deviceID)
		if route.Transport == "vowifi" && audio["active"] != true {
			audio["available"] = false
			audio["source"] = "ims"
			audio["detail"] = "当前 VoWiFi 核心未提供通话音频流；ALSA 声卡不能代替 IMS 录音"
		}
		result := map[string]any{"device_id": deviceID, "calls": active, "audio": audio, "updated_at": time.Now().Unix()}
		route.writeFields(result)
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, result)
		return
	}
	if path == "/api/calls/contacts" {
		app.callContactsAPI(w, r)
		return
	}
	if strings.HasPrefix(path, "/api/calls/contacts/") {
		app.callContactItemAPI(w, r, strings.TrimPrefix(path, "/api/calls/contacts/"))
		return
	}
	if path == "/api/calls/history" && r.Method == http.MethodGet {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items, err := app.calls.history(strings.TrimSpace(r.URL.Query().Get("device_id")), limit)
		if err != nil {
			callAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}
	if strings.HasPrefix(path, "/api/calls/history/") && r.Method == http.MethodDelete {
		id := strings.TrimPrefix(path, "/api/calls/history/")
		if !callIDPattern.MatchString(id) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "通话记录 ID 无效"})
			return
		}
		if err := app.calls.deleteHistory(id); err != nil {
			callAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
		return
	}
	if path == "/api/calls/recordings" && r.Method == http.MethodGet {
		items, err := app.calls.recordings()
		if err != nil {
			callAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "state": app.calls.recordingState()})
		return
	}
	if path == "/api/calls/recordings/start" && r.Method == http.MethodPost {
		var body struct {
			CallID string `json:"call_id"`
		}
		if !decodeJSON(w, r, &body, 4096) {
			return
		}
		call, err := app.calls.callByID(body.CallID)
		if err != nil {
			callAPIError(w, err)
			return
		}
		device, found := callDevice(devices, call.DeviceID)
		if !found {
			writeJSON(w, 404, map[string]any{"error": "电话设备不存在"})
			return
		}
		lock := app.calls.deviceLock(call.DeviceID)
		lock.Lock()
		defer lock.Unlock()
		// Do not start capture based on a stale database row. Read the active
		// provider and reconcile the exact call before touching an audio device.
		route := resolveCallRoute(device, []callHistoryItem{call})
		if !route.Available {
			writeJSON(w, 409, map[string]any{"error": route.Detail})
			return
		}
		calls, err := app.currentVoiceCalls(r, call.DeviceID)
		if err != nil {
			writeJSON(w, 502, map[string]any{"error": "无法确认通话状态，未启动录音"})
			return
		}
		if _, err = app.calls.reconcile(device, calls, time.Now()); err != nil {
			callAPIError(w, err)
			return
		}
		var state map[string]any
		if app.media != nil && call.DeviceID == nativeCallDevice {
			current, lookupErr := app.calls.callByID(body.CallID)
			if lookupErr != nil || current.EndedAt != nil || current.State != "active" {
				writeJSON(w, 409, map[string]any{"error": "仅能为当前已接通的电话录音"})
				return
			}
			state, err = app.media.startRecording(current, app.calls.recordingsDir)
		} else {
			state, err = app.calls.startRecording(body.CallID)
		}
		if err != nil {
			callAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, state)
		return
	}
	if path == "/api/calls/recordings/stop" && r.Method == http.MethodPost {
		var item callRecording
		var err error
		browserRecording := false
		if app.media != nil {
			app.media.recMu.Lock()
			browserRecording = app.media.recording != nil
			app.media.recMu.Unlock()
		}
		if browserRecording {
			item, err = app.media.stopRecording(app.calls)
		} else {
			item, err = app.calls.stopRecording()
		}
		if err != nil {
			callAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if strings.HasPrefix(path, "/api/calls/recordings/") {
		app.callRecordingItemAPI(w, r, strings.TrimPrefix(path, "/api/calls/recordings/"))
		return
	}
	if r.Method == http.MethodPost && (path == "/api/calls/dial" || path == "/api/calls/answer" || path == "/api/calls/hangup" || path == "/api/calls/reject" || path == "/api/calls/dtmf") {
		app.callControlAPI(w, r, path, devices)
		return
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "电话接口或方法不支持"})
}

func (app *application) callContactsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		items, err := app.calls.contacts()
		if err != nil {
			callAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	var body struct {
		Name     string `json:"name"`
		Number   string `json:"number"`
		Note     string `json:"note"`
		Favorite bool   `json:"favorite"`
	}
	if !decodeJSON(w, r, &body, 8192) {
		return
	}
	item, err := app.calls.saveContact("", body.Name, body.Number, body.Note, body.Favorite)
	if err != nil {
		callAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (app *application) callContactItemAPI(w http.ResponseWriter, r *http.Request, id string) {
	if !callIDPattern.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "联系人 ID 无效"})
		return
	}
	if r.Method == http.MethodDelete {
		if err := app.calls.deleteContact(id); err != nil {
			callAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
		return
	}
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	var body struct {
		Name     string `json:"name"`
		Number   string `json:"number"`
		Note     string `json:"note"`
		Favorite bool   `json:"favorite"`
	}
	if !decodeJSON(w, r, &body, 8192) {
		return
	}
	item, err := app.calls.saveContact(id, body.Name, body.Number, body.Note, body.Favorite)
	if err != nil {
		callAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *application) callRecordingItemAPI(w http.ResponseWriter, r *http.Request, suffix string) {
	parts := strings.Split(suffix, "/")
	id := parts[0]
	if !callIDPattern.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "audio" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		item, err := app.calls.recording(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		path := filepath.Join(app.calls.recordingsDir, item.fileName)
		file, err := os.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		mediaType, _, _ := mime.ParseMediaType(item.MIMEType)
		w.Header().Set("Content-Type", mediaType)
		w.Header().Set("Content-Disposition", `inline; filename="call-recording.wav"`)
		w.Header().Set("Accept-Ranges", "bytes")
		http.ServeContent(w, r, item.fileName, info.ModTime(), file)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := app.calls.deleteRecording(id); err != nil {
			callAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
		return
	}
	http.NotFound(w, r)
}

func (app *application) callControlAPI(w http.ResponseWriter, r *http.Request, path string, devices []map[string]any) {
	var body struct {
		DeviceID  string `json:"device_id"`
		Number    string `json:"number"`
		CallID    string `json:"call_id"`
		Digits    string `json:"digits"`
		Transport string `json:"transport"`
	}
	if !decodeJSON(w, r, &body, 8192) {
		return
	}
	body.Transport = strings.ToLower(strings.TrimSpace(body.Transport))
	switch body.Transport {
	case "", "auto", "cellular", "vowifi":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "电话通道无效"})
		return
	}
	body.DeviceID = strings.TrimSpace(body.DeviceID)
	device, found := callDevice(devices, body.DeviceID)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "电话设备不存在"})
		return
	}
	if device["audio_preparing"] == true {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "正在准备或恢复音频接口，请稍后再操作电话"})
		return
	}
	lock := app.calls.deviceLock(body.DeviceID)
	lock.Lock()
	defer lock.Unlock()
	existing, err := app.calls.activeCalls(body.DeviceID)
	if err != nil {
		callAPIError(w, err)
		return
	}
	route := resolveCallRoute(device, nil)
	if path != "/api/calls/dial" {
		for _, call := range existing {
			if body.CallID != "" && call.ID == body.CallID {
				route = resolveCallRoute(device, []callHistoryItem{call})
				break
			}
		}
	}
	if body.Transport != "" && body.Transport != "auto" && body.Transport != route.Transport {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "设备电话通道已变化，请刷新后重试", "transport": route.Transport})
		return
	}
	if !route.Available {
		status := http.StatusServiceUnavailable
		if route.Transport == "vowifi" {
			status = http.StatusNotImplemented
		}
		writeJSON(w, status, map[string]any{"error": route.Detail, "transport": route.Transport})
		return
	}
	calls, err := app.currentVoiceCalls(r, body.DeviceID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "无法确认当前通话状态: " + err.Error()})
		return
	}
	active, err := app.calls.reconcile(device, calls, time.Now())
	if err != nil {
		callAPIError(w, err)
		return
	}
	if path != "/api/calls/dial" && body.CallID != "" {
		matched := false
		for _, call := range active {
			matched = matched || call.ID == body.CallID
		}
		if !matched {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "通话已经变化，请刷新后重试"})
			return
		}
	}
	command, successKey := "", "ok"
	var pendingCall *callHistoryItem
	switch path {
	case "/api/calls/dial":
		body.Number = strings.ReplaceAll(strings.TrimSpace(body.Number), " ", "")
		if !callNumberPattern.MatchString(body.Number) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "电话号码格式无效"})
			return
		}
		if len(calls) != 0 || len(active) != 0 {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "当前已有通话或拨号结果待确认，不能重复拨号"})
			return
		}
		command, successKey = "ATD"+body.Number+";", "dialing"
	case "/api/calls/answer":
		if len(calls) != 1 || (calls[0].State != "incoming" && calls[0].State != "waiting") {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "没有可接听的来电"})
			return
		}
		command, successKey = "ATA", "answered"
	case "/api/calls/hangup", "/api/calls/reject":
		if len(calls) == 0 {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "当前没有可挂断的电话"})
			return
		}
		if body.CallID != "" {
			matched := false
			for _, item := range active {
				matched = matched || item.ID == body.CallID
			}
			if !matched {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "通话已经变化，请刷新后重试"})
				return
			}
		}
		command, successKey = "ATH", "hung_up"
	case "/api/calls/dtmf":
		if len(calls) != 1 || calls[0].State != "active" {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "DTMF 只能在已接通的单路电话中发送"})
			return
		}
		if len(body.Digits) < 1 || len(body.Digits) > 20 || strings.Trim(body.Digits, "0123456789*#") != "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "DTMF 仅支持 1–20 位数字、* 或 #"})
			return
		}
		toneParts := make([]string, 0, len(body.Digits))
		for _, digit := range body.Digits {
			toneParts = append(toneParts, string(digit))
		}
		response, _, runErr := app.runCallAT(r, body.DeviceID, `AT+CLDTMF=1,"`+strings.Join(toneParts, ",")+`"`, 4*time.Second)
		if runErr != nil || !callATExplicitOK(response) {
			for _, digit := range body.Digits {
				response, _, runErr = app.runCallAT(r, body.DeviceID, fmt.Sprintf(`AT+VTS="%c"`, digit), 4*time.Second)
				if runErr != nil || !callATExplicitOK(response) {
					writeJSON(w, http.StatusBadGateway, map[string]any{"error": "模组未接受 DTMF"})
					return
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"sent": true})
		return
	}
	if path == "/api/calls/dial" {
		item, createErr := app.calls.createOutgoing(device, body.Number)
		if createErr != nil {
			callAPIError(w, createErr)
			return
		}
		pendingCall = &item
	}
	response, status, runErr := app.runCallAT(r, body.DeviceID, command, 8*time.Second)
	rejection := callATRejection(response)
	var commandErr *callATError
	definitelyRejected := rejection != "" || (errors.As(runErr, &commandErr) && commandErr.rejected)
	if definitelyRejected {
		if pendingCall != nil {
			if err := app.calls.failOutgoing(pendingCall.ID); err != nil {
				callAPIError(w, err)
				return
			}
		}
		message := "模组拒绝电话指令: " + rejection
		if runErr != nil {
			message = runErr.Error()
		} else {
			status = http.StatusBadGateway
		}
		writeJSON(w, status, map[string]any{"error": message})
		return
	}
	if runErr != nil {
		if pendingCall != nil {
			writeJSON(w, http.StatusAccepted, map[string]any{"pending_confirmation": true, "call": pendingCall})
			return
		}
		writeJSON(w, status, map[string]any{"error": runErr.Error()})
		return
	}
	if !callATExplicitOK(response) {
		if pendingCall != nil {
			writeJSON(w, http.StatusAccepted, map[string]any{"pending_confirmation": true, "call": pendingCall})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "模组没有明确确认电话指令"})
		return
	}
	result := map[string]any{successKey: true, "transport": "cellular"}
	if pendingCall != nil {
		result["call"] = pendingCall
	}
	writeJSON(w, http.StatusOK, result)
}

// pollCallsBackground keeps incoming calls and completed call history visible
// even when no browser currently has the phone page open. Unsupported modems
// simply reject the read-only CLCC query and are retried on a later cycle.
func (app *application) pollCallsBackground(ctx context.Context) {
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://local/", nil)
	token, err := app.coreLoginRequest(request)
	if err != nil {
		return
	}
	request.Header.Set("Authorization", token)
	status, value, err := app.upstreamJSON(request, http.MethodGet, "/api/devices", nil, 10*time.Second)
	if err != nil || status != http.StatusOK {
		if status == http.StatusUnauthorized {
			app.invalidateCoreLogin(token)
		}
		return
	}
	snapshot, _ := value.(map[string]any)
	for _, device := range app.callDevices(snapshot) {
		if device["audio_preparing"] == true {
			continue
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		deviceID := strings.TrimSpace(fmt.Sprint(device["id"]))
		if deviceID == "" {
			continue
		}
		if running, ok := device["running"].(bool); ok && !running {
			continue
		}
		lock := app.calls.deviceLock(deviceID)
		if !lock.TryLock() {
			continue
		}
		active, historyErr := app.calls.activeCalls(deviceID)
		if historyErr != nil || resolveCallRoute(device, active).Transport != "cellular" {
			lock.Unlock()
			continue
		}
		calls, pollErr := app.currentVoiceCalls(request, deviceID)
		if pollErr == nil {
			_, _ = app.calls.reconcile(device, calls, time.Now())
		}
		lock.Unlock()
	}
	app.finishBrowserRecording()
}
