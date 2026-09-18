# Scheduled SMS

Since version 2.1.0, the Go service under `cmd/vohivex-gateway` implements scheduled tasks, the SMS archive, the notification queue, and reverse proxying. Frontend source is under `web/`; this directory contains only asset build scripts and module documentation.

## Use

1. Open **Scheduled Tasks**, create a task, and enter its name, sending device, one recipient number, and message.
2. Choose either a single specified time or a repeating interval in days, hours, minutes, and seconds.
3. Set the first execution time in China Standard Time (UTC+8), with second-level precision.
4. A saved task is paused. Click **Start** to enable it and display the next execution time.
5. Pause, edit, delete, or review task history when needed. An edited task must be started again. For a completed one-time task, change its execution time before restarting it.

## Execution Rules

- Repeating schedules use the first execution time as their baseline. Executions delayed by more than 60 seconds because of shutdown or another delay are skipped. An expired one-time task is paused; a repeating task advances to the next future time.
- A task pauses automatically when sending fails, the result is uncertain, or execution was interrupted before a restart. It is never retried automatically.
- Pausing cannot recall a message already submitted. If the selected device is offline or SMS is unavailable, another device is not substituted.
- The system supports up to 1,000 tasks. It retains the latest 100 results per task and displays the latest 20 in the interface.
- Long messages may be billed as multiple segments. Actual sending time depends on the device, queue, and carrier response.
- Deleting a task also deletes its execution history but preserves messages already present in the SMS center.

## Notifications

Configure and enable delivery channels under **Notifications**. After every actual execution, VoHiveX sends the task name, device name, recipient number, content, time, and status. No completion notification is sent for an expired task that did not run.

The first notification reports the sending request result. Later delivery-state notifications follow these rules:

- With a `message_id`, VoHiveX performs read-only queries to `/api/sms/delivery/{message_id}` and verifies both message and device IDs. `state=acked` with `acks>=parts_total>0` means that all segments were acknowledged; `failed` means an explicit failure.
- Without a `message_id`, success is confirmed only when the send API explicitly returns `delivery_state=acked`.
- Missing explicit state, uncertain network results, and timeouts are recorded as unknown. Status is checked every 15 seconds for up to 24 hours.
- Network or API confirmation does not prove that the recipient received or read the message, and never triggers an automatic resend.

Enabled channels are processed independently and do not block the task. A notification failure does not change SMS status or trigger an SMS resend.

## Data and API

Tasks, results, and the notification queue are stored in `data/scheduled-sms.sqlite3` and should be backed up with the configuration. Pending delivery-state checks continue after restart. An interrupted notification is recorded as unknown and is not sent again automatically.

Management endpoints use `Authorization: Bearer <token>`:

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/schedules` | Tasks and server time |
| GET | `/api/schedules/devices` | Available sending devices |
| POST | `/api/schedules` | Create a paused task |
| PUT | `/api/schedules/{id}` | Update and pause a task |
| DELETE | `/api/schedules/{id}` | Delete a task |
| POST | `/api/schedules/{id}/start` | Start a task |
| POST | `/api/schedules/{id}/pause` | Pause a task |
| GET | `/api/schedules/{id}/history` | Recent execution history |

Create and update requests use `name`, `device_id`, `phone`, `message`, `mode` (`once` or `interval`), `first_run` (Unix seconds), and `interval_seconds`. Update, delete, start, and pause operations require the current `version` to avoid overwriting changes from another page.

The initial webhook event is `scheduled_sms.completed`; the final delivery-state event is `scheduled_sms.delivery` with `phase=delivery`. Both use the same `run_id` and include device, number, message, time, status, and detail fields. For the final event, `finished_at` is the confirmation time and `execution_finished_at` is the original execution completion time. Custom headers and the `X-Vohive-Signature` signature remain supported, and the complete result is provided in `text`.

## Asset Build

Run `python3 app/scheduler/build-assets.py` from the repository root. See [Build and Deployment](../README.md) for environment preparation and deployment steps.
