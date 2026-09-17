package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const smsArchiveSchemaVersion = 1

type smsArchive struct{ db *sql.DB }

func newSMSArchive(path string) (*smsArchive, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	statements := []string{`PRAGMA journal_mode=WAL`, `PRAGMA busy_timeout=10000`, `CREATE TABLE IF NOT EXISTS vohivex_meta(key TEXT PRIMARY KEY,value TEXT NOT NULL)`, `CREATE TABLE IF NOT EXISTS imported_sms(id INTEGER PRIMARY KEY AUTOINCREMENT,device_id TEXT NOT NULL,device_name TEXT NOT NULL DEFAULT '',local_phone TEXT NOT NULL DEFAULT '',imsi TEXT NOT NULL,peer TEXT NOT NULL,message_type INTEGER NOT NULL,content TEXT NOT NULL,timestamp TEXT NOT NULL,timestamp_ms INTEGER NOT NULL,sender TEXT NOT NULL DEFAULT '',status INTEGER,source_message_id TEXT NOT NULL DEFAULT '',signature TEXT NOT NULL UNIQUE,imported_at INTEGER NOT NULL)`, `CREATE INDEX IF NOT EXISTS imported_sms_thread ON imported_sms(device_id,imsi,peer,timestamp_ms,id)`}
	for _, statement := range statements {
		if _, err = db.Exec(statement); err != nil {
			db.Close()
			return nil, err
		}
	}
	var versionNumber int
	err = db.QueryRow(`SELECT CAST(value AS INTEGER) FROM vohivex_meta WHERE key='schema_version'`).Scan(&versionNumber)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = db.Exec(`INSERT INTO vohivex_meta(key,value)VALUES('schema_version',?)`, smsArchiveSchemaVersion)
		versionNumber = smsArchiveSchemaVersion
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	if versionNumber > smsArchiveSchemaVersion {
		db.Close()
		return nil, fmt.Errorf("SMS archive schema %d is newer than supported schema %d", versionNumber, smsArchiveSchemaVersion)
	}
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if err := os.Chmod(path, 0o600); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &smsArchive{db: db}, nil
}
func (a *smsArchive) Close() error { return a.db.Close() }

func timestampMillis(value any) (int64, string, error) {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		now := time.Now().UTC()
		return now.UnixMilli(), now.Format(time.RFC3339), nil
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
			if parsed, err = time.ParseInLocation(layout, text, time.Local); err == nil {
				break
			}
		}
	}
	if err != nil {
		return 0, "", invalidError{"短信时间格式无效"}
	}
	return parsed.UnixMilli(), text, nil
}

func deviceModemText(device map[string]any, key string) string {
	if value := strings.TrimSpace(fmt.Sprint(device[key])); value != "" && value != "<nil>" {
		return value
	}
	if modem, ok := device["modem"].(map[string]any); ok {
		return strings.TrimSpace(fmt.Sprint(modem[key]))
	}
	return ""
}

func (a *smsArchive) Import(device map[string]any, rows []map[string]any) (int, error) {
	deviceID := strings.TrimSpace(fmt.Sprint(device["id"]))
	if deviceID == "" {
		return 0, invalidError{"请选择导入设备"}
	}
	deviceName := strings.TrimSpace(fmt.Sprint(device["name"]))
	if deviceName == "" {
		deviceName = deviceID
	}
	defaultIMSI := deviceModemText(device, "imsi")
	if defaultIMSI == "" {
		defaultIMSI = "archive:" + deviceID
	}
	localPhone := deviceModemText(device, "local_phone")
	if localPhone == "" {
		localPhone = deviceModemText(device, "msisdn")
	}
	tx, err := a.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	inserted := 0
	now := time.Now().Unix()
	for _, row := range rows {
		peer := strings.TrimSpace(fmt.Sprint(row["peer"]))
		content := fmt.Sprint(row["content"])
		if peer == "" || len([]rune(peer)) > 200 || content == "" || len([]rune(content)) > 20000 {
			return 0, invalidError{"短信联系人或内容无效"}
		}
		messageType, err := anyInt(row["type"])
		if err != nil || (messageType != 1 && messageType != 2) {
			return 0, invalidError{"短信方向无效"}
		}
		stamp, timestamp, err := timestampMillis(row["timestamp"])
		if err != nil {
			return 0, err
		}
		imsi := strings.TrimSpace(fmt.Sprint(row["imsi"]))
		if imsi == "" || imsi == "<nil>" {
			imsi = defaultIMSI
		}
		sender := strings.TrimSpace(fmt.Sprint(row["sender"]))
		if sender == "<nil>" {
			sender = ""
		}
		sourceID := strings.TrimSpace(fmt.Sprint(row["message_id"]))
		if sourceID == "<nil>" {
			sourceID = ""
		}
		var status any = nil
		if row["status"] != nil && fmt.Sprint(row["status"]) != "" {
			if value, e := anyInt(row["status"]); e == nil && value >= 0 && value <= 3 {
				status = value
			}
		}
		material := strings.Join([]string{deviceID, imsi, peer, strconv.Itoa(messageType), timestamp, content, sender, sourceID}, "\x00")
		sum := sha256.Sum256([]byte(material))
		signature := hex.EncodeToString(sum[:])
		result, err := tx.Exec(`INSERT OR IGNORE INTO imported_sms(device_id,device_name,local_phone,imsi,peer,message_type,content,timestamp,timestamp_ms,sender,status,source_message_id,signature,imported_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, deviceID, deviceName, localPhone, imsi, peer, messageType, content, timestamp, stamp, sender, status, sourceID, signature, now)
		if err != nil {
			return 0, err
		}
		count, _ := result.RowsAffected()
		inserted += int(count)
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}

func scanArchiveContact(rows *sql.Rows) (map[string]any, error) {
	var id int64
	var deviceID, deviceName, localPhone, imsi, peer, content, timestamp string
	err := rows.Scan(&id, &deviceID, &deviceName, &localPhone, &imsi, &peer, &content, &timestamp)
	return map[string]any{"imsi": imsi, "peer": peer, "device_id": deviceID, "device_name": deviceName, "local_phone": localPhone, "last_timestamp": timestamp, "last_sms_id": -id, "last_content": content, "imported": true}, err
}

func (a *smsArchive) Contacts(deviceID string) ([]map[string]any, error) {
	query := `SELECT id,device_id,device_name,local_phone,imsi,peer,content,timestamp FROM imported_sms`
	args := []any{}
	if deviceID != "" && deviceID != "all" {
		query += ` WHERE device_id=?`
		args = append(args, deviceID)
	}
	query += ` ORDER BY timestamp_ms DESC,id DESC`
	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	result := []map[string]any{}
	for rows.Next() {
		row, err := scanArchiveContact(rows)
		if err != nil {
			return nil, err
		}
		key := fmt.Sprint(row["imsi"]) + "\x00" + fmt.Sprint(row["peer"])
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, row)
	}
	return result, rows.Err()
}

func (a *smsArchive) Messages(peer, deviceID, imsi string, limit int, beforeTS, beforeID string) ([]map[string]any, error) {
	clauses := []string{"peer=?"}
	args := []any{peer}
	if deviceID != "" && deviceID != "all" {
		clauses = append(clauses, "device_id=?")
		args = append(args, deviceID)
	} else if imsi != "" {
		clauses = append(clauses, "imsi=?")
		args = append(args, imsi)
	}
	if beforeTS != "" {
		stamp, _, err := timestampMillis(beforeTS)
		if err != nil {
			return nil, err
		}
		id, _ := strconv.ParseInt(beforeID, 10, 64)
		if id < 0 {
			id = -id
		}
		clauses = append(clauses, "(timestamp_ms<? OR (timestamp_ms=? AND id>?))")
		args = append(args, stamp, stamp, id)
	}
	if limit < 1 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	args = append(args, limit)
	query := `SELECT id,device_id,device_name,local_phone,imsi,peer,message_type,content,timestamp,sender,status FROM imported_sms WHERE ` + strings.Join(clauses, " AND ") + ` ORDER BY timestamp_ms DESC,id DESC LIMIT ?`
	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id int64
		var deviceID, deviceName, localPhone, imsi, peer, content, timestamp, sender string
		var messageType int
		var status sql.NullInt64
		if err := rows.Scan(&id, &deviceID, &deviceName, &localPhone, &imsi, &peer, &messageType, &content, &timestamp, &sender, &status); err != nil {
			return nil, err
		}
		var state any = nil
		if status.Valid {
			state = status.Int64
		}
		result = append(result, map[string]any{"id": -id, "type": messageType, "content": content, "timestamp": timestamp, "sender": sender, "status": state, "device_id": deviceID, "device_name": deviceName, "local_phone": localPhone, "imsi": imsi, "peer": peer, "imported": true})
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result, rows.Err()
}

func (a *smsArchive) DeleteMessage(messageID int64) (map[string]any, error) {
	if messageID < 0 {
		messageID = -messageID
	}
	tx, err := a.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var deviceID, imsi, peer string
	rowErr := tx.QueryRow(`SELECT device_id,imsi,peer FROM imported_sms WHERE id=?`, messageID).Scan(&deviceID, &imsi, &peer)
	result, err := tx.Exec(`DELETE FROM imported_sms WHERE id=?`, messageID)
	if err != nil {
		return nil, err
	}
	deleted, _ := result.RowsAffected()
	remaining := int64(0)
	if rowErr == nil {
		_ = tx.QueryRow(`SELECT COUNT(*) FROM imported_sms WHERE device_id=? AND imsi=? AND peer=?`, deviceID, imsi, peer).Scan(&remaining)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{"deleted": deleted, "thread_empty": rowErr == nil && remaining == 0}, nil
}

func (a *smsArchive) DeleteThread(peer, deviceID, imsi string) (int64, error) {
	clauses := []string{"peer=?"}
	args := []any{peer}
	if deviceID != "" && deviceID != "all" {
		clauses = append(clauses, "device_id=?")
		args = append(args, deviceID)
	} else if imsi != "" {
		clauses = append(clauses, "imsi=?")
		args = append(args, imsi)
	}
	result, err := a.db.Exec(`DELETE FROM imported_sms WHERE `+strings.Join(clauses, " AND "), args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
