package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const schedulerSchemaVersion = 1

type scheduledTask struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	DeviceID        string         `json:"device_id"`
	DeviceName      string         `json:"device_name,omitempty"`
	Phone           string         `json:"phone"`
	Message         string         `json:"message"`
	Mode            string         `json:"mode"`
	FirstRun        int64          `json:"first_run"`
	IntervalSeconds int64          `json:"interval_seconds"`
	State           string         `json:"state"`
	NextRun         *int64         `json:"next_run"`
	CreatedAt       int64          `json:"created_at"`
	UpdatedAt       int64          `json:"updated_at"`
	LastRun         *int64         `json:"last_run"`
	LastResult      string         `json:"last_result"`
	RunCount        int64          `json:"run_count"`
	Version         int64          `json:"version"`
	SendResult      map[string]any `json:"send_result,omitempty"`
}

type taskRun struct {
	ID           string `json:"id"`
	TaskID       string `json:"task_id"`
	ScheduledFor int64  `json:"scheduled_for"`
	StartedAt    int64  `json:"started_at"`
	FinishedAt   *int64 `json:"finished_at"`
	Status       string `json:"status"`
	Detail       string `json:"detail"`
}

type taskStore struct{ db *sql.DB }

func newTaskStore(path string) (*taskStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	statements := []string{
		`PRAGMA journal_mode=WAL`, `PRAGMA busy_timeout=10000`,
		`CREATE TABLE IF NOT EXISTS vohivex_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS tasks (id TEXT PRIMARY KEY, name TEXT NOT NULL, device_id TEXT NOT NULL, phone TEXT NOT NULL, message TEXT NOT NULL, mode TEXT NOT NULL, first_run INTEGER NOT NULL, interval_seconds INTEGER NOT NULL, state TEXT NOT NULL DEFAULT 'paused', next_run INTEGER, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, last_run INTEGER, last_result TEXT NOT NULL DEFAULT '', run_count INTEGER NOT NULL DEFAULT 0, version INTEGER NOT NULL DEFAULT 1)`,
		`CREATE TABLE IF NOT EXISTS runs (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, scheduled_for INTEGER NOT NULL, started_at INTEGER NOT NULL, finished_at INTEGER, status TEXT NOT NULL, detail TEXT NOT NULL DEFAULT '', UNIQUE(task_id, scheduled_for))`,
		`CREATE TABLE IF NOT EXISTS notifications (run_id TEXT PRIMARY KEY, task_id TEXT NOT NULL, event TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'pending', created_at INTEGER NOT NULL, detail TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS delivery_checks (run_id TEXT PRIMARY KEY, event TEXT NOT NULL, next_check INTEGER NOT NULL, deadline INTEGER NOT NULL, state TEXT NOT NULL DEFAULT 'waiting', detail TEXT NOT NULL DEFAULT '')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, err
		}
	}
	var versionNumber int
	err = db.QueryRow(`SELECT CAST(value AS INTEGER) FROM vohivex_meta WHERE key='schema_version'`).Scan(&versionNumber)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = db.Exec(`INSERT INTO vohivex_meta(key,value) VALUES('schema_version',?)`, schedulerSchemaVersion)
		versionNumber = schedulerSchemaVersion
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	if versionNumber > schedulerSchemaVersion {
		db.Close()
		return nil, fmt.Errorf("scheduler database schema %d is newer than supported schema %d", versionNumber, schedulerSchemaVersion)
	}
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if err := os.Chmod(path, 0o600); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &taskStore{db: db}, nil
}

func (s *taskStore) Close() error { return s.db.Close() }

func scanTask(scanner interface{ Scan(...any) error }) (scheduledTask, error) {
	var task scheduledTask
	var next, last sql.NullInt64
	err := scanner.Scan(&task.ID, &task.Name, &task.DeviceID, &task.Phone, &task.Message, &task.Mode, &task.FirstRun, &task.IntervalSeconds, &task.State, &next, &task.CreatedAt, &task.UpdatedAt, &last, &task.LastResult, &task.RunCount, &task.Version)
	if next.Valid {
		value := next.Int64
		task.NextRun = &value
	}
	if last.Valid {
		value := last.Int64
		task.LastRun = &value
	}
	return task, err
}

const taskColumns = `id,name,device_id,phone,message,mode,first_run,interval_seconds,state,next_run,created_at,updated_at,last_run,last_result,run_count,version`

func (s *taskStore) List() ([]scheduledTask, error) {
	rows, err := s.db.Query(`SELECT ` + taskColumns + ` FROM tasks ORDER BY created_at DESC,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []scheduledTask{}
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, task)
	}
	return result, rows.Err()
}

func textField(payload map[string]any, key, label string, limit int, preserve bool) (string, error) {
	value, ok := payload[key].(string)
	if !ok || strings.TrimSpace(value) == "" || len([]rune(value)) > limit {
		return "", invalidError{fmt.Sprintf("%s不能为空，且不能超过 %d 个字符", label, limit)}
	}
	if !preserve {
		value = strings.TrimSpace(value)
	}
	return value, nil
}

func numberField(payload map[string]any, key string) (int64, error) {
	switch value := payload[key].(type) {
	case json.Number:
		return value.Int64()
	case float64:
		if value != float64(int64(value)) {
			return 0, errors.New("not integer")
		}
		return int64(value), nil
	case int64:
		return value, nil
	case int:
		return int64(value), nil
	default:
		return 0, errors.New("not integer")
	}
}

func validateTask(payload map[string]any, now int64) (scheduledTask, error) {
	var task scheduledTask
	var err error
	if task.Name, err = textField(payload, "name", "任务名称", 80, false); err != nil {
		return task, err
	}
	if task.DeviceID, err = textField(payload, "device_id", "发信设备", 128, false); err != nil {
		return task, err
	}
	if task.Phone, err = textField(payload, "phone", "收信号码", 24, false); err != nil {
		return task, err
	}
	if task.Message, err = textField(payload, "message", "短信内容", 2000, true); err != nil {
		return task, err
	}
	if !regexp.MustCompile(`^\+?[0-9]{3,20}$`).MatchString(task.Phone) {
		return task, invalidError{"收信号码仅支持一个号码：数字及可选的 + 国家区号"}
	}
	task.Mode, _ = payload["mode"].(string)
	if task.Mode != "once" && task.Mode != "interval" {
		return task, invalidError{"请选择单次或间隔重复"}
	}
	task.FirstRun, err = numberField(payload, "first_run")
	if err != nil || task.FirstRun <= now {
		return task, invalidError{"首次执行时间必须晚于当前时间"}
	}
	if task.FirstRun > now+366*86400*10 {
		return task, invalidError{"首次执行时间不能超过十年"}
	}
	interval, _ := numberField(payload, "interval_seconds")
	if task.Mode == "interval" && (interval < 1 || interval > 366*86400*10) {
		return task, invalidError{"重复间隔至少 1 秒，最多十年"}
	}
	if task.Mode == "interval" {
		task.IntervalSeconds = interval
	}
	return task, nil
}

func (s *taskStore) Create(payload map[string]any) (string, error) {
	now := time.Now().Unix()
	task, err := validateTask(payload, now)
	if err != nil {
		return "", err
	}
	var count int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&count); err != nil {
		return "", err
	}
	if count >= 1000 {
		return "", invalidError{"最多保存 1000 个任务"}
	}
	task.ID = strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = s.db.Exec(`INSERT INTO tasks(id,name,device_id,phone,message,mode,first_run,interval_seconds,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, task.ID, task.Name, task.DeviceID, task.Phone, task.Message, task.Mode, task.FirstRun, task.IntervalSeconds, now, now)
	return task.ID, err
}

func (s *taskStore) taskTx(tx *sql.Tx, id string) (scheduledTask, error) {
	return scanTask(tx.QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE id=?`, id))
}

func futureRun(task scheduledTask, now int64) int64 {
	return task.FirstRun + maxInt64(0, (now-task.FirstRun)/task.IntervalSeconds+1)*task.IntervalSeconds
}
func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func expectedVersion(payload map[string]any) (int64, error) { return numberField(payload, "version") }

func (s *taskStore) Change(id, action string, payload map[string]any) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	task, err := s.taskTx(tx, id)
	if err != nil {
		return err
	}
	expected, err := expectedVersion(payload)
	if err != nil || expected != task.Version {
		return conflictError{"任务已被修改，请刷新后再操作"}
	}
	var busy int
	_ = tx.QueryRow(`SELECT COUNT(*) FROM runs WHERE task_id=? AND status='sending'`, id).Scan(&busy)
	if busy > 0 && action != "pause" {
		return conflictError{"该任务正在发送，请等待本次执行结束；可先暂停后续执行"}
	}
	now := time.Now().Unix()
	switch action {
	case "delete":
		_, err = tx.Exec(`DELETE FROM runs WHERE task_id=?`, id)
		if err == nil {
			_, err = tx.Exec(`DELETE FROM tasks WHERE id=?`, id)
		}
	case "edit":
		var replacement scheduledTask
		replacement, err = validateTask(payload, now)
		if err == nil {
			_, err = tx.Exec(`UPDATE tasks SET name=?,device_id=?,phone=?,message=?,mode=?,first_run=?,interval_seconds=?,state='paused',next_run=NULL,last_result='已修改，等待开始',updated_at=?,version=version+1 WHERE id=?`, replacement.Name, replacement.DeviceID, replacement.Phone, replacement.Message, replacement.Mode, replacement.FirstRun, replacement.IntervalSeconds, now, id)
		}
	case "start":
		if task.State == "active" {
			return conflictError{"任务已经开始"}
		}
		if task.Mode == "once" && task.FirstRun <= now {
			return invalidError{"单次执行时间已过，请修改时间后再开始"}
		}
		due := task.FirstRun
		if due <= now {
			due = futureRun(task, now)
		}
		_, err = tx.Exec(`UPDATE tasks SET state='active',next_run=?,last_result='等待执行',updated_at=?,version=version+1 WHERE id=?`, due, now, id)
	case "pause":
		_, err = tx.Exec(`UPDATE tasks SET state='paused',next_run=NULL,updated_at=?,version=version+1 WHERE id=?`, now, id)
	default:
		return invalidError{"不支持的操作"}
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *taskStore) History(id string) ([]taskRun, error) {
	rows, err := s.db.Query(`SELECT id,task_id,scheduled_for,started_at,finished_at,status,detail FROM runs WHERE task_id=? ORDER BY started_at DESC LIMIT 20`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []taskRun{}
	for rows.Next() {
		var row taskRun
		var finished sql.NullInt64
		if err := rows.Scan(&row.ID, &row.TaskID, &row.ScheduledFor, &row.StartedAt, &finished, &row.Status, &row.Detail); err != nil {
			return nil, err
		}
		if finished.Valid {
			value := finished.Int64
			row.FinishedAt = &value
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *taskStore) Recover() error {
	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`UPDATE tasks SET state='paused',next_run=NULL,last_result='上次发送结果未知，已暂停，请核对短信记录',version=version+1 WHERE id IN (SELECT task_id FROM runs WHERE status='sending')`)
	if err == nil {
		_, err = tx.Exec(`UPDATE runs SET status='unknown',finished_at=?,detail='进程中断，未自动重发' WHERE status='sending'`, now)
	}
	if err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT `+taskColumns+` FROM tasks WHERE state='active' AND next_run<=?`, now)
	if err != nil {
		return err
	}
	var tasks []scheduledTask
	for rows.Next() {
		task, e := scanTask(rows)
		if e != nil {
			rows.Close()
			return e
		}
		tasks = append(tasks, task)
	}
	rows.Close()
	for _, task := range tasks {
		if task.Mode == "interval" {
			_, err = tx.Exec(`UPDATE tasks SET next_run=?,last_result='已跳过停机期间的执行时间',version=version+1 WHERE id=?`, futureRun(task, now), task.ID)
		} else {
			_, err = tx.Exec(`UPDATE tasks SET state='paused',next_run=NULL,last_result='执行时间在停机期间已过，请修改后开始',version=version+1 WHERE id=?`, task.ID)
		}
		if err != nil {
			return err
		}
	}
	_, _ = tx.Exec(`UPDATE notifications SET state='unknown',detail='推送进程中断，结果未知，未自动重发' WHERE state='sending'`)
	return tx.Commit()
}

func (s *taskStore) Claim() (*scheduledTask, string, error) {
	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()
	task, err := scanTask(tx.QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE state='active' AND next_run<=? ORDER BY next_run,id LIMIT 1`, now))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	runID := strings.ReplaceAll(uuid.NewString(), "-", "")
	scheduled := *task.NextRun
	if now-scheduled > 60 {
		_, err = tx.Exec(`INSERT INTO runs VALUES(?,?,?,?,?,?,?)`, runID, task.ID, scheduled, now, now, "skipped", "超过执行时间 60 秒，未补发")
		if err == nil {
			if task.Mode == "interval" {
				_, err = tx.Exec(`UPDATE tasks SET next_run=?,last_result='已跳过错过的执行时间',version=version+1 WHERE id=?`, futureRun(task, now), task.ID)
			} else {
				_, err = tx.Exec(`UPDATE tasks SET state='paused',next_run=NULL,last_result='已跳过错过的执行时间',version=version+1 WHERE id=?`, task.ID)
			}
		}
		if err != nil {
			return nil, "", err
		}
		return nil, "", tx.Commit()
	}
	_, err = tx.Exec(`INSERT INTO runs(id,task_id,scheduled_for,started_at,status,detail) VALUES(?,?,?,?,?,?)`, runID, task.ID, scheduled, now, "sending", "")
	if err == nil {
		_, err = tx.Exec(`UPDATE tasks SET next_run=NULL,last_run=?,last_result='发送中',version=version+1 WHERE id=?`, now, task.ID)
	}
	if err != nil {
		return nil, "", err
	}
	return &task, runID, tx.Commit()
}

func (s *taskStore) Finish(task *scheduledTask, runID, status, detail string) error {
	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := s.taskTx(tx, task.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var started int64
	if err = tx.QueryRow(`SELECT started_at FROM runs WHERE id=? AND status='sending'`, runID).Scan(&started); errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	state := current.State
	var due any
	if status != "success" {
		state = "paused"
	} else if state == "active" {
		if current.Mode == "once" {
			state = "completed"
		} else {
			due = futureRun(current, now)
		}
	}
	_, err = tx.Exec(`UPDATE runs SET status=?,detail=?,finished_at=? WHERE id=?`, status, detail, now, runID)
	if err != nil {
		return err
	}
	event := map[string]any{"name": task.Name, "device_id": task.DeviceID, "device_name": task.DeviceName, "phone": task.Phone, "message": task.Message, "task_id": task.ID, "run_id": runID, "status": status, "detail": detail, "started_at": started, "finished_at": now, "send_result": task.SendResult}
	encoded, _ := json.Marshal(event)
	_, err = tx.Exec(`INSERT OR IGNORE INTO notifications(run_id,task_id,event,created_at) VALUES(?,?,?,?)`, runID, task.ID, string(encoded), now)
	if err == nil {
		_, err = tx.Exec(`INSERT OR IGNORE INTO delivery_checks(run_id,event,next_check,deadline) VALUES(?,?,?,?)`, runID, string(encoded), now, now+86400)
	}
	if err == nil {
		increment := 0
		if status == "success" {
			increment = 1
		}
		_, err = tx.Exec(`UPDATE tasks SET state=?,next_run=?,last_result=?,run_count=run_count+?,updated_at=?,version=version+1 WHERE id=?`, state, due, detail, increment, now, task.ID)
	}
	if err == nil {
		_, err = tx.Exec(`DELETE FROM runs WHERE task_id=? AND id NOT IN (SELECT id FROM runs WHERE task_id=? ORDER BY started_at DESC LIMIT 100)`, task.ID, task.ID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

type notificationRow struct {
	ID    string
	Event map[string]any
}

func (s *taskStore) ClaimNotification() (*notificationRow, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id, raw string
	err = tx.QueryRow(`SELECT run_id,event FROM notifications WHERE state='pending' ORDER BY created_at,rowid LIMIT 1`).Scan(&id, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`UPDATE notifications SET state='sending' WHERE run_id=?`, id); err != nil {
		return nil, err
	}
	var event map[string]any
	if err = json.Unmarshal([]byte(raw), &event); err != nil {
		return nil, err
	}
	return &notificationRow{ID: id, Event: event}, tx.Commit()
}
func (s *taskStore) FinishNotification(id, state, detail string) error {
	_, err := s.db.Exec(`UPDATE notifications SET state=?,detail=? WHERE run_id=?`, state, detail, id)
	if err == nil {
		_, _ = s.db.Exec(`DELETE FROM notifications WHERE state NOT IN ('pending','sending') AND rowid NOT IN (SELECT rowid FROM notifications ORDER BY created_at DESC,rowid DESC LIMIT 1000)`)
	}
	return err
}

type deliveryRow struct {
	RunID    string
	Event    map[string]any
	Deadline int64
}

func (s *taskStore) NextDelivery() (*deliveryRow, error) {
	var id, raw string
	var deadline int64
	err := s.db.QueryRow(`SELECT run_id,event,deadline FROM delivery_checks WHERE state='waiting' AND next_check<=? ORDER BY next_check,rowid LIMIT 1`, time.Now().Unix()).Scan(&id, &raw, &deadline)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var event map[string]any
	if err = json.Unmarshal([]byte(raw), &event); err != nil {
		return nil, err
	}
	return &deliveryRow{RunID: id, Event: event, Deadline: deadline}, nil
}
func (s *taskStore) DeferDelivery(id string) error {
	_, err := s.db.Exec(`UPDATE delivery_checks SET next_check=? WHERE run_id=? AND state='waiting'`, time.Now().Add(15*time.Second).Unix(), id)
	return err
}
func (s *taskStore) FinishDelivery(row *deliveryRow, status, detail string) error {
	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	event := row.Event
	event["phase"] = "delivery"
	event["status"] = status
	event["detail"] = detail
	event["execution_finished_at"] = event["finished_at"]
	event["finished_at"] = now
	raw, _ := json.Marshal(event)
	taskID := fmt.Sprint(event["task_id"])
	_, err = tx.Exec(`INSERT OR IGNORE INTO notifications(run_id,task_id,event,created_at) VALUES(?,?,?,?)`, row.RunID+":delivery", taskID, string(raw), now)
	if err == nil {
		_, err = tx.Exec(`UPDATE delivery_checks SET state=?,detail=? WHERE run_id=?`, status, detail, row.RunID)
	}
	if err == nil {
		_, err = tx.Exec(`DELETE FROM delivery_checks WHERE state!='waiting' AND rowid NOT IN (SELECT rowid FROM delivery_checks ORDER BY rowid DESC LIMIT 1000)`)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func sortDeviceRows(rows []map[string]any) {
	sort.SliceStable(rows, func(i, j int) bool { return fmt.Sprint(rows[i]["id"]) < fmt.Sprint(rows[j]["id"]) })
}

func (app *application) runWorkers(ctx context.Context) {
	if err := app.tasks.Recover(); err != nil {
		logSafe("scheduler recovery", err)
	}
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if task, runID, err := app.tasks.Claim(); err != nil {
					logSafe("scheduler claim", err)
				} else if task != nil {
					status, detail := app.sendScheduledSMS(ctx, task)
					if err := app.tasks.Finish(task, runID, status, detail); err != nil {
						logSafe("scheduler finish", err)
					}
				}
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				app.processNotification(ctx)
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				app.processDelivery(ctx)
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				app.proxy.Maintain()
				app.ensureBuiltInProxy(ctx)
			}
		}
	}()
	<-ctx.Done()
}

func logSafe(scope string, err error) { fmt.Printf("VoHiveX %s: %T\n", scope, err) }
