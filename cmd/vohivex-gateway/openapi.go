package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// gatewayOpenAPI contains only endpoints that the upstream binary omits from
// its generated specification. Paths are written relative to the upstream
// API server and are prefixed automatically when an older upstream spec uses
// /api-prefixed path keys.
const gatewayOpenAPI = `{
  "tags": [
    {"name":"Card policy","description":"SIM card network and VoWiFi policy"},
    {"name":"USSD","description":"Interactive USSD sessions"},
    {"name":"Notification tests","description":"Validate notification channel settings"},
    {"name":"VoHiveX schedules","description":"Scheduled SMS tasks provided by the VoHiveX gateway"},
    {"name":"VoHiveX managed proxy","description":"Subscription and managed Mihomo egress provided by the VoHiveX gateway"},
    {"name":"VoHiveX SMS archive","description":"SMS archive import provided by the VoHiveX gateway"},
    {"name":"VoHiveX calls","description":"Voice-call control, contacts, history and recordings provided by the VoHiveX gateway"},
    {"name":"VoHiveX network debug","description":"Browse public web pages through a selected connected cellular interface"},
    {"name":"VoHiveX settings","description":"Account and runtime metadata provided by the VoHiveX gateway"},
    {"name":"Logs","description":"Application log history"}
  ],
  "components": {"schemas": {
    "VoHiveXError": {"type":"object","properties":{"error":{"type":"string"}},"required":["error"]},
    "CardPolicy": {"type":"object","properties":{"source":{"type":"string","readOnly":true},"ip_version":{"type":"string","enum":["v4","v6","v4v6"]},"apn":{"type":"string"},"network_enabled":{"type":"boolean"},"vowifi_enabled":{"type":"boolean"},"airplane_enabled":{"type":"boolean"}}},
    "USSDContinueRequest": {"type":"object","required":["session_id","input"],"properties":{"session_id":{"type":"string"},"input":{"type":"string"},"timeout_ms":{"type":"integer","minimum":1000,"default":30000}}},
    "USSDCancelRequest": {"type":"object","required":["session_id"],"properties":{"session_id":{"type":"string"}}},
    "ESIMProfileRename": {"type":"object","required":["name"],"properties":{"name":{"type":"string","maxLength":64,"description":"Profile nickname stored on the eUICC"},"aid_hex":{"type":"string","description":"Target eUICC application identifier when more than one eUICC is present"}}},
    "WebhookAdapterConfig": {"type":"object","properties":{"format":{"type":"string","enum":["generic","dingtalk","chat_robots"],"default":"generic"},"enabled":{"type":"boolean"},"urls":{"type":"array","items":{"type":"string","format":"uri"}},"secret":{"type":"string","format":"password","description":"Generic HMAC secret or DingTalk SEC signing secret"},"feishu_secret":{"type":"string","format":"password","description":"Feishu custom-bot signing secret"},"text_template":{"type":"string","description":"DingTalk keyword requirements also apply to this template. Full notification text and phone numbers are forwarded."},"timeout_ms":{"type":"integer","minimum":1000,"maximum":60000},"retry_max":{"type":"integer","minimum":0,"maximum":10}}},
    "BarkTestRequest": {"type":"object","required":["urls"],"properties":{"enabled":{"type":"boolean"},"urls":{"type":"array","items":{"type":"string","format":"uri"}},"group":{"type":"string"},"icon":{"type":"string"},"level":{"type":"string","enum":["timeSensitive","active","passive"]}}},
    "EmailTestRequest": {"type":"object","required":["smtp_host","smtp_port","from_address","to_addresses"],"properties":{"enabled":{"type":"boolean"},"use_ssl":{"type":"boolean"},"smtp_host":{"type":"string"},"smtp_port":{"type":"integer"},"username":{"type":"string"},"password":{"type":"string","format":"password","writeOnly":true},"from_address":{"type":"string","format":"email"},"to_addresses":{"type":"array","items":{"type":"string","format":"email"}}}},
    "ScheduleWrite": {"type":"object","required":["name","device_id","phone","message","mode","first_run","interval_seconds"],"properties":{"name":{"type":"string","maxLength":80},"device_id":{"type":"string"},"phone":{"type":"string","pattern":"^\\+?[0-9]{3,20}$"},"message":{"type":"string","maxLength":2000},"mode":{"type":"string","enum":["once","interval"]},"first_run":{"type":"integer","format":"int64","description":"First execution time as a Unix timestamp in seconds"},"interval_seconds":{"type":"integer","format":"int64","minimum":0},"version":{"type":"integer","format":"int64","description":"Required for update, start, pause and delete"}}},
    "ScheduleTask": {"allOf":[{"$ref":"#/components/schemas/ScheduleWrite"},{"type":"object","properties":{"id":{"type":"string"},"state":{"type":"string","enum":["paused","active","completed"]},"next_run":{"type":"integer","format":"int64","nullable":true},"last_run":{"type":"integer","format":"int64","nullable":true},"last_result":{"type":"string"},"run_count":{"type":"integer","format":"int64"}}}]},
    "ScheduleVersion": {"type":"object","required":["version"],"properties":{"version":{"type":"integer","format":"int64"}}},
    "ManagedProxyVersion": {"type":"object","required":["version"],"properties":{"version":{"type":"integer"}}},
    "ManagedProxyImport": {"allOf":[{"$ref":"#/components/schemas/ManagedProxyVersion"},{"type":"object","required":["name","kind","content"],"properties":{"name":{"type":"string"},"kind":{"type":"string","enum":["subscription","nodes"]},"content":{"type":"string","description":"Subscription URL or proxy share-link content"}}}]},
    "ManagedProxySourceAction": {"allOf":[{"$ref":"#/components/schemas/ManagedProxyVersion"},{"type":"object","required":["source_id"],"properties":{"source_id":{"type":"string"}}}]},
    "ManagedProxyNodeAction": {"allOf":[{"$ref":"#/components/schemas/ManagedProxyVersion"},{"type":"object","required":["node_id"],"properties":{"node_id":{"type":"string"}}}]},
    "ManagedProxySnapshot": {"type":"object","properties":{"version":{"type":"integer"},"enabled":{"type":"boolean"},"selected":{"type":"string"},"running":{"type":"boolean"},"error":{"type":"string"},"endpoint":{"type":"string","example":"127.0.0.1:17890"},"sources":{"type":"array","items":{"type":"object"}},"nodes":{"type":"array","items":{"type":"object"}}}},
    "SMSArchiveImport": {"type":"object","required":["device_id","messages"],"properties":{"device_id":{"type":"string"},"messages":{"type":"array","minItems":1,"maxItems":20000,"items":{"type":"object","properties":{"peer":{"type":"string"},"phone":{"type":"string"},"content":{"type":"string"},"type":{"type":"integer","description":"1 received, 2 sent"},"timestamp":{"oneOf":[{"type":"string","format":"date-time"},{"type":"integer","format":"int64"}]},"status":{"oneOf":[{"type":"integer"},{"type":"string"}]},"message_id":{"type":"string"},"imsi":{"type":"string"}}}}}},
    "UsernameChange": {"type":"object","required":["username","current_password"],"properties":{"username":{"type":"string"},"current_password":{"type":"string","format":"password","writeOnly":true}}},
    "CallDeviceRequest": {"type":"object","required":["device_id"],"properties":{"device_id":{"type":"string"},"call_id":{"type":"string"},"transport":{"type":"string","default":"auto","enum":["auto","cellular","vowifi"],"description":"Auto (also the omitted default) follows fresh device VoWiFi policy, including while reconnecting. Explicit transports must match the device or identified active call. VoWiFi currently returns 501 without an AT command or cellular fallback; unknown mode returns 503."}}},
    "CallDialRequest": {"allOf":[{"$ref":"#/components/schemas/CallDeviceRequest"},{"type":"object","required":["number"],"properties":{"number":{"type":"string","pattern":"^\\+?[0-9]{3,24}$"}}}]},
    "CallDTMFRequest": {"allOf":[{"$ref":"#/components/schemas/CallDeviceRequest"},{"type":"object","required":["digits"],"properties":{"digits":{"type":"string","pattern":"^[0-9*#]{1,20}$"}}}]},
    "CallContactWrite": {"type":"object","required":["name","number"],"properties":{"name":{"type":"string","maxLength":80},"number":{"type":"string","pattern":"^\\+?[0-9]{3,24}$"},"note":{"type":"string","maxLength":240},"favorite":{"type":"boolean"}}},
    "CallRecordingStart": {"type":"object","required":["call_id"],"properties":{"call_id":{"type":"string"}}}
    ,"NetworkDebugSessionWrite": {"type":"object","required":["device_id"],"properties":{"device_id":{"type":"string"}}}
    ,"NetworkDebugSession": {"type":"object","properties":{"id":{"type":"string"},"device_id":{"type":"string"},"interface":{"type":"string"},"entry_url":{"type":"string"},"expires_in":{"type":"integer"}}}
    ,"NetworkDebugStats": {"type":"object","properties":{"received_bytes":{"type":"integer","format":"int64"},"sent_bytes":{"type":"integer","format":"int64"},"total_bytes":{"type":"integer","format":"int64"},"current_speed_bps":{"type":"number"},"average_speed_bps":{"type":"number"},"request_count":{"type":"integer"},"active_requests":{"type":"integer"},"last_url":{"type":"string"},"last_status":{"type":"integer"},"last_error":{"type":"string"}}}
  }},
  "paths": {
    "/logs/history": {"delete":{"tags":["Logs"],"summary":"Clear the active application log","description":"Truncates the active log in place. Rotated archives are retained and new events continue to be logged.","responses":{"200":{"description":"Active log cleared","content":{"application/json":{"schema":{"type":"object","properties":{"cleared":{"type":"boolean"}}}}}},"401":{"description":"Authentication required"},"403":{"description":"Cross-origin request rejected"},"500":{"description":"Log file could not be cleared"}}}},
    "/cards/policies": {"get":{"tags":["Card policy"],"summary":"List saved card policies","responses":{"200":{"description":"Saved card policies"},"401":{"description":"Unauthorized"}}}},
    "/cards/{iccid}/policy": {
      "parameters":[{"name":"iccid","in":"path","required":true,"schema":{"type":"string"}}],
      "get":{"tags":["Card policy"],"summary":"Get card policy","responses":{"200":{"description":"Card policy","content":{"application/json":{"schema":{"$ref":"#/components/schemas/CardPolicy"}}}},"404":{"description":"Card not found"}}},
      "put":{"tags":["Card policy"],"summary":"Update card policy","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/CardPolicy"}}}},"responses":{"200":{"description":"Policy saved"},"400":{"description":"Invalid policy"}}}
    },
    "/devices/{device_id}/actions/ussd/continue": {"post":{"tags":["USSD"],"summary":"Continue an interactive USSD session","parameters":[{"name":"device_id","in":"path","required":true,"schema":{"type":"string"}}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/USSDContinueRequest"}}}},"responses":{"200":{"description":"USSD response"},"400":{"description":"Invalid or expired session"}}}},
    "/devices/{device_id}/actions/ussd/cancel": {"post":{"tags":["USSD"],"summary":"Cancel an interactive USSD session","parameters":[{"name":"device_id","in":"path","required":true,"schema":{"type":"string"}}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/USSDCancelRequest"}}}},"responses":{"200":{"description":"Session cancelled"},"400":{"description":"Invalid or expired session"}}}},
    "/devices/{device_id}/esim/profiles/{iccid}": {"patch":{"tags":["eSIM"],"summary":"Update an eSIM Profile nickname","description":"Writes the profile nickname to the selected eUICC without enabling, disabling or deleting the Profile.","parameters":[{"name":"device_id","in":"path","required":true,"schema":{"type":"string"}},{"name":"iccid","in":"path","required":true,"schema":{"type":"string"}}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ESIMProfileRename"}}}},"responses":{"200":{"description":"Profile nickname updated"},"400":{"description":"Invalid nickname or Profile"},"409":{"description":"eUICC is busy"}}}},
    "/settings/notifications/webhook/test": {"post":{"tags":["Notification tests"],"summary":"Test generic Webhooks or DingTalk/Feishu robots without saving settings","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/WebhookAdapterConfig"}}}},"responses":{"200":{"description":"Notification accepted by the destination API"},"400":{"description":"Invalid settings"},"502":{"description":"Destination rejected the message or could not be reached"}}}},
    "/settings/notifications/bark/test": {"post":{"tags":["Notification tests"],"summary":"Test Bark settings without saving them","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/BarkTestRequest"}}}},"responses":{"200":{"description":"Test result"},"400":{"description":"Invalid Bark settings"}}}},
    "/settings/notifications/email/test": {"post":{"tags":["Notification tests"],"summary":"Test email settings without saving them","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/EmailTestRequest"}}}},"responses":{"200":{"description":"Test result"},"400":{"description":"Invalid email settings"}}}},
    "/schedules": {
      "get":{"tags":["VoHiveX schedules"],"summary":"List scheduled SMS tasks","responses":{"200":{"description":"Tasks and server time"}}},
      "post":{"tags":["VoHiveX schedules"],"summary":"Create a paused scheduled SMS task","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ScheduleWrite"}}}},"responses":{"201":{"description":"Task created"},"400":{"description":"Invalid task"}}}
    },
    "/schedules/devices": {"get":{"tags":["VoHiveX schedules"],"summary":"List devices eligible for scheduled SMS","responses":{"200":{"description":"Eligible devices"}}}},
    "/schedules/{id}": {
      "parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],
      "put":{"tags":["VoHiveX schedules"],"summary":"Update a task and return it to paused state","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ScheduleWrite"}}}},"responses":{"200":{"description":"Task updated"},"409":{"description":"Version conflict"}}},
      "delete":{"tags":["VoHiveX schedules"],"summary":"Delete a task and its execution history","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ScheduleVersion"}}}},"responses":{"200":{"description":"Task deleted"},"409":{"description":"Version conflict"}}}
    },
    "/schedules/{id}/start": {"post":{"tags":["VoHiveX schedules"],"summary":"Start a scheduled task","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ScheduleVersion"}}}},"responses":{"200":{"description":"Task started"},"409":{"description":"Version conflict"}}}},
    "/schedules/{id}/pause": {"post":{"tags":["VoHiveX schedules"],"summary":"Pause a scheduled task","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ScheduleVersion"}}}},"responses":{"200":{"description":"Task paused"},"409":{"description":"Version conflict"}}}},
    "/schedules/{id}/history": {"get":{"tags":["VoHiveX schedules"],"summary":"Get the 20 most recent task executions","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"Execution history"}}}},
    "/managed-proxy": {"get":{"tags":["VoHiveX managed proxy"],"summary":"Get managed proxy subscriptions, nodes and runtime state","responses":{"200":{"description":"Managed proxy state","content":{"application/json":{"schema":{"$ref":"#/components/schemas/ManagedProxySnapshot"}}}}}}},
    "/managed-proxy/public-ip": {"get":{"tags":["VoHiveX managed proxy"],"summary":"Get device, NAS and selected proxy egress IP addresses","parameters":[{"name":"refresh","in":"query","schema":{"type":"integer","enum":[0,1]}}],"responses":{"200":{"description":"Egress IP snapshot"}}}},
    "/managed-proxy/import": {"post":{"tags":["VoHiveX managed proxy"],"summary":"Import a subscription or proxy share links","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ManagedProxyImport"}}}},"responses":{"200":{"description":"Import applied"},"409":{"description":"Version conflict"}}}},
    "/managed-proxy/refresh": {"post":{"tags":["VoHiveX managed proxy"],"summary":"Refresh a subscription","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ManagedProxySourceAction"}}}},"responses":{"200":{"description":"Subscription refreshed"}}}},
    "/managed-proxy/delete": {"post":{"tags":["VoHiveX managed proxy"],"summary":"Delete a subscription or imported source","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ManagedProxySourceAction"}}}},"responses":{"200":{"description":"Source deleted"}}}},
    "/managed-proxy/select": {"post":{"tags":["VoHiveX managed proxy"],"summary":"Select and enable a proxy node","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ManagedProxyNodeAction"}}}},"responses":{"200":{"description":"Node selected"}}}},
    "/managed-proxy/pause": {"post":{"tags":["VoHiveX managed proxy"],"summary":"Disable the selected proxy","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ManagedProxyVersion"}}}},"responses":{"200":{"description":"Proxy disabled"}}}},
    "/managed-proxy/test": {"post":{"tags":["VoHiveX managed proxy"],"summary":"Test HTTPS latency for a node","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ManagedProxyNodeAction"}}}},"responses":{"200":{"description":"Latency result"}}}},
    "/managed-proxy/attach": {"post":{"tags":["VoHiveX managed proxy"],"summary":"Enable the built-in VoWiFi upstream proxy association","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ManagedProxyVersion"}}}},"responses":{"200":{"description":"Association enabled"}}}},
    "/sms/archive/import": {"post":{"tags":["VoHiveX SMS archive"],"summary":"Import SMS messages into the local archive","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/SMSArchiveImport"}}}},"responses":{"200":{"description":"Import counts"},"400":{"description":"Invalid archive"}}}},
    "/sms/contacts": {"get":{"tags":["VoHiveX SMS archive"],"summary":"List SMS conversations merged from live devices and the local archive","parameters":[{"name":"device_id","in":"query","schema":{"type":"string"},"description":"A device ID or all"},{"name":"limit","in":"query","schema":{"type":"integer","minimum":1,"maximum":500,"default":200}}],"responses":{"200":{"description":"Conversation list"}}}},
    "/sms/thread": {"get":{"tags":["VoHiveX SMS archive"],"summary":"Get a conversation merged from a live device and the local archive","parameters":[{"name":"peer","in":"query","required":true,"schema":{"type":"string"}},{"name":"device_id","in":"query","schema":{"type":"string"}},{"name":"imsi","in":"query","schema":{"type":"string"}},{"name":"limit","in":"query","schema":{"type":"integer","minimum":1,"maximum":1000,"default":500}}],"responses":{"200":{"description":"Conversation messages"}}}},
    "/sms/messages/{id}": {"delete":{"tags":["VoHiveX SMS archive"],"summary":"Delete a message from the local archive or the live device","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}},{"name":"device_id","in":"query","schema":{"type":"string"}},{"name":"imsi","in":"query","schema":{"type":"string"}}],"responses":{"200":{"description":"Message deleted"},"404":{"description":"Message not found"}}}},
    "/calls/devices": {"get":{"tags":["VoHiveX calls"],"summary":"List devices eligible for phone calls and the audio capability","responses":{"200":{"description":"Call-capable devices and audio state"}}}},
    "/calls/status": {"get":{"tags":["VoHiveX calls"],"summary":"Follow device mode and read the corresponding phone capability and call state","parameters":[{"name":"device_id","in":"query","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"Resolved transport, device_transport, call_control_available, call_control_detail, current calls and recording state. A VoWiFi capability response does not issue AT queries. Existing native calls retain their original transport until ended."},"502":{"description":"Modem status unavailable"}}}},
    "/calls/diagnostics": {"get":{"tags":["VoHiveX calls"],"summary":"Read modem call diagnostics and separate host VoWiFi observations","description":"Authenticated, read-only diagnostic. Queries CEER first, followed by CFUN, CS/EPS registration and modem IMS configuration. Does not dial, hang up or activate cellular data. Fields may be unavailable; a running host voice agent does not imply a connected external control API.","parameters":[{"name":"device_id","in":"query","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"Sanitized modem observations, host VoWiFi agent state and control capabilities"},"401":{"description":"Authentication required"},"404":{"description":"Device not found"}}}},
    "/calls/dial": {"post":{"tags":["VoHiveX calls"],"summary":"Dial using the current device phone mode","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/CallDialRequest"}}}},"responses":{"200":{"description":"Dial command accepted; not proof that the destination rang or answered"},"409":{"description":"Another voice call is active, or the requested transport is stale"},"501":{"description":"This gateway has no connected VoWiFi call-control adapter; no dial was submitted"},"503":{"description":"Device phone mode is unknown; no command was submitted"}}}},
    "/calls/answer": {"post":{"tags":["VoHiveX calls"],"summary":"Answer the current incoming call","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/CallDeviceRequest"}}}},"responses":{"200":{"description":"Answer command accepted"},"409":{"description":"No incoming call"},"501":{"description":"VoWiFi call-control adapter unavailable; no cellular fallback"},"503":{"description":"Device phone mode is unknown; no command was submitted"}}}},
    "/calls/hangup": {"post":{"tags":["VoHiveX calls"],"summary":"Hang up the current voice call","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/CallDeviceRequest"}}}},"responses":{"200":{"description":"Hangup command accepted"},"409":{"description":"Call topology changed"},"501":{"description":"VoWiFi call-control adapter unavailable; no cellular fallback"},"503":{"description":"Device phone mode is unknown; no command was submitted"}}}},
    "/calls/reject": {"post":{"tags":["VoHiveX calls"],"summary":"Reject the current incoming call","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/CallDeviceRequest"}}}},"responses":{"200":{"description":"Reject command accepted"},"501":{"description":"VoWiFi call-control adapter unavailable; no cellular fallback"},"503":{"description":"Device phone mode is unknown; no command was submitted"}}}},
    "/calls/dtmf": {"post":{"tags":["VoHiveX calls"],"summary":"Send DTMF digits during an active call","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/CallDTMFRequest"}}}},"responses":{"200":{"description":"DTMF accepted"},"501":{"description":"VoWiFi call-control adapter unavailable; no cellular fallback"},"503":{"description":"Device phone mode is unknown; no command was submitted"}}}},
    "/calls/contacts": {"get":{"tags":["VoHiveX calls"],"summary":"List phone contacts","responses":{"200":{"description":"Contact list"}}},"post":{"tags":["VoHiveX calls"],"summary":"Create a phone contact","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/CallContactWrite"}}}},"responses":{"201":{"description":"Contact created"}}}},
    "/calls/contacts/{id}": {"parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"put":{"tags":["VoHiveX calls"],"summary":"Update a phone contact","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/CallContactWrite"}}}},"responses":{"200":{"description":"Contact updated"}}},"delete":{"tags":["VoHiveX calls"],"summary":"Delete a phone contact","responses":{"200":{"description":"Contact deleted"}}}},
    "/calls/history": {"get":{"tags":["VoHiveX calls"],"summary":"List persisted call history","parameters":[{"name":"device_id","in":"query","schema":{"type":"string"}},{"name":"limit","in":"query","schema":{"type":"integer","minimum":1,"maximum":500}}],"responses":{"200":{"description":"Call history"}}}},
    "/calls/history/{id}": {"delete":{"tags":["VoHiveX calls"],"summary":"Delete a completed call without a recording","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"Call history deleted"}}}},
    "/calls/audio/prepare": {"post":{"tags":["VoHiveX calls"],"summary":"Prepare optional local USB audio","description":"Requires an authenticated same-origin request and an idle local modem. Verifies pinned runtime hashes, saves original USB composition, enables temporary audio and rolls back on failure. Does not dial or change SIM/APN/IMS settings.","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["device_id"],"properties":{"device_id":{"type":"string","enum":["local-usb-modem"]}}}}}},"responses":{"200":{"description":"Prepared"},"409":{"description":"Busy or unsupported device"},"502":{"description":"Preparation failed; inspect restore_required"}}}},
    "/calls/audio/restore": {"post":{"tags":["VoHiveX calls"],"summary":"Restore original USB composition and reboot an idle local modem","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["device_id"],"properties":{"device_id":{"type":"string","enum":["local-usb-modem"]}}}}}},"responses":{"200":{"description":"Restored"},"409":{"description":"Disconnect browser audio first"}}}},
    "/calls/audio/ticket": {"post":{"tags":["VoHiveX calls"],"summary":"Issue a single-use browser audio ticket","description":"Requires login and a prepared local modem. Ticket expires after 20 seconds; no audio ticket is put in URLs or logs.","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["device_id"],"properties":{"device_id":{"type":"string","enum":["local-usb-modem"]}}}}}},"responses":{"201":{"description":"Ticket and sample_rate (8000)"},"409":{"description":"Audio unavailable or already occupied"}}}},
    "/calls/audio/stream": {"get":{"tags":["VoHiveX calls"],"summary":"Upgrade to authenticated duplex browser audio WebSocket","description":"Same Origin required. First frame must be JSON {ticket:...}. Server sends ready only after actual capture frames arrive. Subsequent binary messages are mono 8 kHz signed PCM16 little-endian, at most 3200 bytes. Only one client may connect. Disconnect stops audio and saves an active recording; it never hangs up the telephone call.","security":[],"responses":{"101":{"description":"WebSocket; first message authenticates the short-lived ticket"},"403":{"description":"Origin denied"}}}},
    "/calls/recordings": {"get":{"tags":["VoHiveX calls"],"summary":"List call recordings and current recorder capability","responses":{"200":{"description":"Recordings and recorder state"}}}},
    "/calls/recordings/start": {"post":{"tags":["VoHiveX calls"],"summary":"Start recording for an active call using ALSA or the connected browser audio bridge","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/CallRecordingStart"}}}},"responses":{"201":{"description":"Recording started"},"400":{"description":"Audio capture unavailable"}}}},
    "/calls/recordings/stop": {"post":{"tags":["VoHiveX calls"],"summary":"Stop and persist the current call recording","responses":{"200":{"description":"Recording saved"}}}},
    "/calls/recordings/{id}": {"delete":{"tags":["VoHiveX calls"],"summary":"Delete a call recording","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"Recording deleted"}}}},
    "/calls/recordings/{id}/audio": {"get":{"tags":["VoHiveX calls"],"summary":"Stream or download a call recording","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"WAV recording","content":{"audio/wav":{"schema":{"type":"string","format":"binary"}}}}}}},
    "/network-debug/devices": {"get":{"tags":["VoHiveX network debug"],"summary":"List devices with an enabled and connected cellular network","responses":{"200":{"description":"Eligible cellular devices"}}}},
    "/network-debug/sessions": {"post":{"tags":["VoHiveX network debug"],"summary":"Create a cellular-bound browsing session","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/NetworkDebugSessionWrite"}}}},"responses":{"201":{"description":"Browsing session created","content":{"application/json":{"schema":{"$ref":"#/components/schemas/NetworkDebugSession"}}}},"400":{"description":"Cellular interface is unavailable"}}}},
    "/network-debug/sessions/{id}": {"delete":{"tags":["VoHiveX network debug"],"summary":"Close a browsing session","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"Browsing session closed"}}}},
    "/network-debug/sessions/{id}/stats": {"get":{"tags":["VoHiveX network debug"],"summary":"Read session speed and traffic totals","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"Session traffic statistics","content":{"application/json":{"schema":{"$ref":"#/components/schemas/NetworkDebugStats"}}}}}}},
    "/network-debug/view/{session_id}": {"get":{"tags":["VoHiveX network debug"],"summary":"Open a public web URL through a browsing session","description":"The unguessable session ID is a short-lived capability used by the sandboxed iframe. Private, loopback and link-local targets are rejected.","security":[],"parameters":[{"name":"session_id","in":"path","required":true,"schema":{"type":"string"}},{"name":"url","in":"query","required":true,"schema":{"type":"string","format":"uri"}}],"responses":{"307":{"description":"Redirect to the canonical proxied URL"},"400":{"description":"Invalid or prohibited target"}}}},
    "/settings/agreement": {"get":{"tags":["VoHiveX settings"],"summary":"Read the authenticated administrator's permanent agreement confirmation","responses":{"200":{"description":"Persistent user_id, accepted and accepted_at (Unix milliseconds); no expiry"},"401":{"description":"Authentication required"}}},"post":{"tags":["VoHiveX settings"],"summary":"Confirm once for the authenticated account or migrate an existing browser confirmation","description":"The upstream has one administrator account. Its persisted ID survives username/password changes, new sessions, browser changes and restarts. The server chooses the account from authentication; user_id must match. previous_accepted_at migrates a prior explicit browser confirmation without a new expiry.","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["user_id"],"properties":{"user_id":{"type":"string"},"confirmation":{"type":"string","enum":["我同意并确认"]},"previous_accepted_at":{"type":"integer","format":"int64","description":"Prior confirmation time in Unix milliseconds, only for migration"}}}}}},"responses":{"200":{"description":"Persisted confirmation state"},"400":{"description":"Invalid confirmation"},"401":{"description":"Authentication required"},"403":{"description":"Cross-origin write refused"},"409":{"description":"Account ID mismatch"},"500":{"description":"Confirmation could not be saved"}}}},
    "/settings/username": {
      "get":{"tags":["VoHiveX settings"],"summary":"Get the current login username","responses":{"200":{"description":"Current username"}}},
      "post":{"tags":["VoHiveX settings"],"summary":"Change the login username","description":"The current password is required. A successful change restarts the service and invalidates the current login.","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/UsernameChange"}}}},"responses":{"200":{"description":"Username changed; restart required"},"403":{"description":"Current password is incorrect"}}}
    },
    "/settings/system": {"get":{"tags":["VoHiveX settings"],"summary":"Get VoHiveX runtime and build information","responses":{"200":{"description":"System time, build time, config path and component versions"}}}}
  }
}`

func mapValue(parent map[string]any, key string) map[string]any {
	value, _ := parent[key].(map[string]any)
	if value == nil {
		value = map[string]any{}
		parent[key] = value
	}
	return value
}

func openAPIPathPrefix(paths map[string]any) string {
	prefixed, relative := 0, 0
	for path := range paths {
		if strings.HasPrefix(path, "/api/") {
			prefixed++
		} else if strings.HasPrefix(path, "/") {
			relative++
		}
	}
	if prefixed > relative {
		return "/api"
	}
	return ""
}

func mergeOpenAPI(spec map[string]any) error {
	var additions map[string]any
	if err := json.Unmarshal([]byte(gatewayOpenAPI), &additions); err != nil {
		return fmt.Errorf("decode gateway OpenAPI additions: %w", err)
	}

	paths := mapValue(spec, "paths")
	prefix := openAPIPathPrefix(paths)
	for path, raw := range mapValue(additions, "paths") {
		incoming, _ := raw.(map[string]any)
		targetPath := prefix + path
		existing := mapValue(paths, targetPath)
		for method, operation := range incoming {
			if method != "parameters" {
				if object, ok := operation.(map[string]any); ok {
					if _, found := object["security"]; !found {
						object["security"] = []any{map[string]any{"BearerAuth": []any{}}}
					}
				}
			}
			if _, found := existing[method]; !found || path == "/settings/notifications/webhook/test" {
				existing[method] = operation
			}
		}
	}

	components := mapValue(spec, "components")
	schemas := mapValue(components, "schemas")
	for name, schema := range mapValue(mapValue(additions, "components"), "schemas") {
		if _, found := schemas[name]; !found {
			schemas[name] = schema
		}
	}
	securitySchemes := mapValue(components, "securitySchemes")
	if _, found := securitySchemes["BearerAuth"]; !found {
		securitySchemes["BearerAuth"] = map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}
	}

	existingTags, _ := spec["tags"].([]any)
	tagNames := map[string]bool{}
	for _, raw := range existingTags {
		if tag, ok := raw.(map[string]any); ok {
			tagNames[fmt.Sprint(tag["name"])] = true
		}
	}
	for _, raw := range additions["tags"].([]any) {
		tag, _ := raw.(map[string]any)
		if !tagNames[fmt.Sprint(tag["name"])] {
			existingTags = append(existingTags, tag)
		}
	}
	spec["tags"] = existingTags
	if info := mapValue(spec, "info"); strings.TrimSpace(fmt.Sprint(info["title"])) == "" {
		info["title"] = "VoHiveX API"
	}
	return nil
}

func (app *application) openAPISpec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不支持"})
		return
	}
	status, value, err := app.upstreamJSON(r, http.MethodGet, "/api/openapi.json", nil, 10*time.Second)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "无法读取核心 API 文档"})
		return
	}
	if status != http.StatusOK {
		writeJSON(w, status, value)
		return
	}
	spec, ok := value.(map[string]any)
	if !ok {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "核心 API 文档格式无效"})
		return
	}
	if err := mergeOpenAPI(spec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "API 文档合并失败"})
		return
	}

	var raw []byte
	if strings.HasSuffix(r.URL.Path, ".yaml") {
		raw, err = yaml.Marshal(spec)
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	} else {
		raw, err = json.MarshalIndent(spec, "", "  ")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "API 文档序列化失败"})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", fmt.Sprint(len(raw)))
	if r.Method == http.MethodGet {
		_, _ = w.Write(raw)
	}
}
