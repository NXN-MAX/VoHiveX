# 定时短信

2.1.0 起，定时任务、短信归档、推送队列及反向代理均由 `cmd/vohivex-gateway` 下的 Go 服务实现；前端源码位于 `web/`，此目录仅保留资源构建脚本与模块说明。

## 使用步骤

1. 打开「定时任务」，新增任务并填写名称、发信设备、一个收信号码和短信内容。
2. 选择指定时间发送一次，或按天、小时、分钟、秒设置重复间隔。
3. 设置首次执行时间，使用北京时间（UTC+8），可精确到秒。
4. 保存后任务处于暂停状态，点击「开始」启用并显示下次执行时间。
5. 需要时暂停、修改、删除任务或查看执行记录。修改后需重新开始；已完成的单次任务需先修改执行时间。

## 执行规则

- 重复时间以首次时间为基准；停机或延迟超过 60 秒的执行不补发。单次任务过期后暂停，重复任务跳至下一个未来时间。
- 发送失败、结果不确定或重启前执行中断时自动暂停，不自动重发。
- 暂停不能撤回已经提交的短信。设备离线或未启用短信时，不改用其他设备。
- 最多 1000 个任务；每个任务保留最近 100 条结果，界面显示最近 20 条。
- 长短信可能分段计费；实际发送时间受设备、队列和运营商响应影响。
- 删除任务会删除其执行记录，保留短信中心已有消息。

## 消息推送

在「消息推送」配置并启用接收渠道。每次实际执行结束后，发送任务名、设备名称、收信号码、内容、时间及状态。未执行的过期任务不发送完成通知。

首次通知表示发送请求的执行结果；后续状态通知按以下规则判断：

- 有 `message_id`：只读查询 `/api/sms/delivery/{message_id}`，核对消息和设备 ID。`state=acked` 且 `acks>=parts_total>0` 表示全部分段得到确认；`failed` 表示明确失败。
- 无 `message_id`：仅在发送接口明确返回 `delivery_state=acked` 时确认接口发送成功。
- 无明确状态、网络请求结果不确定或超时均记为未知。每 15 秒查询，最长等待 24 小时。
- 网络或接口确认不代表收件人已收到或已阅读，不据此自动重发。

推送使用当前已启用渠道，独立处理，不阻塞任务。推送失败不改变短信状态，也不触发短信重发。

## 数据与接口

任务、结果和推送队列保存在 `data/scheduled-sms.sqlite3`，应与配置一并备份。重启后继续处理等待中的状态查询；推送中断记为未知，不自动重复发送。

管理接口使用 `Authorization: Bearer <token>`：

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/api/schedules` | 任务列表及服务器时间 |
| GET | `/api/schedules/devices` | 可用发信设备 |
| POST | `/api/schedules` | 创建暂停任务 |
| PUT | `/api/schedules/{id}` | 修改并暂停 |
| DELETE | `/api/schedules/{id}` | 删除任务 |
| POST | `/api/schedules/{id}/start` | 开始 |
| POST | `/api/schedules/{id}/pause` | 暂停 |
| GET | `/api/schedules/{id}/history` | 最近执行记录 |

创建和修改字段为 `name`、`device_id`、`phone`、`message`、`mode`（`once` 或 `interval`）、`first_run`（Unix 秒）、`interval_seconds`。修改、删除、开始和暂停需要当前 `version`，避免覆盖其他页面操作。

Webhook 首次事件为 `scheduled_sms.completed`，最终状态事件为 `scheduled_sms.delivery`（`phase=delivery`），使用相同 `run_id`。提供设备、号码、消息、时间、状态和详情字段；最终事件的 `finished_at` 为确认时间，`execution_finished_at` 为原执行结束时间。沿用自定义请求头及 `X-Vohive-Signature` 签名，完整结果位于 `text`。

## 资源构建

从项目根目录执行 `python3 app/scheduler/build-assets.py`；环境准备及部署步骤见[构建与部署](../README.md)。
