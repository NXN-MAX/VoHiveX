package main

import (
	"strings"
	"testing"
	"time"
)

func TestTaskLifecycleAndNotificationQueue(t *testing.T) {
	for _, result := range []string{"success", "failed", "unknown"} {
		t.Run(result, func(t *testing.T) {
			store, err := newTaskStore(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			payload := map[string]any{"name": "Test reminder", "device_id": "demo", "phone": "+12025550123", "message": "Synthetic message", "mode": "once", "first_run": time.Now().Unix() + 3600, "interval_seconds": 0}
			id, err := store.Create(payload)
			if err != nil {
				t.Fatal(err)
			}
			list, _ := store.List()
			if len(list) != 1 || list[0].State != "paused" || list[0].NextRun != nil {
				t.Fatalf("new task must be paused: %+v", list)
			}
			if err = store.Change(id, "start", map[string]any{"version": list[0].Version}); err != nil {
				t.Fatal(err)
			}
			if err = store.Change(id, "pause", map[string]any{"version": list[0].Version}); err == nil {
				t.Fatal("accepted stale version")
			}
			list, _ = store.List()
			payload["version"] = list[0].Version
			if err = store.Change(id, "edit", payload); err != nil {
				t.Fatal(err)
			}
			list, _ = store.List()
			if list[0].State != "paused" || list[0].NextRun != nil {
				t.Fatal("editing did not pause task")
			}
			if err = store.Change(id, "start", map[string]any{"version": list[0].Version}); err != nil {
				t.Fatal(err)
			}
			if _, err = store.db.Exec(`UPDATE tasks SET next_run=? WHERE id=?`, time.Now().Unix(), id); err != nil {
				t.Fatal(err)
			}
			task, runID, err := store.Claim()
			if err != nil || task == nil {
				t.Fatalf("claim: %v", err)
			}
			task.DeviceName = "Demo device"
			if err = store.Finish(task, runID, result, "Synthetic result"); err != nil {
				t.Fatal(err)
			}
			list, _ = store.List()
			want := "paused"
			if result == "success" {
				want = "completed"
			}
			if list[0].State != want || list[0].NextRun != nil {
				t.Fatalf("finish state: %+v", list[0])
			}
			event, err := store.ClaimNotification()
			if err != nil || event == nil {
				t.Fatalf("notification missing: %v", err)
			}
			for key, want := range map[string]string{"device_name": "Demo device", "phone": "+12025550123", "message": "Synthetic message", "status": result} {
				if event.Event[key] != want {
					t.Fatalf("notification missing %s", key)
				}
			}
			if event.Event["finished_at"] == nil {
				t.Fatal("notification missing execution time")
			}
			if err = store.Finish(task, runID, result, "duplicate completion"); err != nil {
				t.Fatal(err)
			}
			if err = store.Change(id, "delete", map[string]any{"version": list[0].Version}); err != nil {
				t.Fatal(err)
			}
			list, _ = store.List()
			if len(list) != 0 {
				t.Fatal("delete did not remove task")
			}
		})
	}
}

func TestDeliveryClassificationAcceptsBothResponseFields(t *testing.T) {
	for _, key := range []string{"state", "delivery_state"} {
		for _, tc := range []struct{ input, want string }{{"acked", "success"}, {"failed", "failed"}, {"timeout", "unknown"}} {
			status, _, done := deliveryClassification(map[string]any{key: tc.input, "parts_total": 2, "acks": 2}, true)
			if !done || status != tc.want {
				t.Fatalf("%s=%s: got %s, done=%v", key, tc.input, status, done)
			}
		}
	}
	if _, _, done := deliveryClassification(map[string]any{"delivery_state": "acked", "parts_total": 2, "acks": 1}, true); done {
		t.Fatal("partial delivery treated as complete")
	}
	if _, _, done := deliveryClassification(map[string]any{}, true); done {
		t.Fatal("missing status treated as complete")
	}
	message := formatNotification(map[string]any{"device_name": "Demo device", "phone": "+12025550123", "message": "Synthetic message", "finished_at": time.Now().Unix(), "status": "success"})
	for _, part := range []string{"Demo device", "+12025550123", "Synthetic message"} {
		if !strings.Contains(message, part) {
			t.Fatalf("notification text missing %q", part)
		}
	}
}
