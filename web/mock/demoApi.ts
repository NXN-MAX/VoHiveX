import type { Plugin } from 'vite'
import type { IncomingMessage, ServerResponse } from 'node:http'

const now = () => Math.floor(Date.now() / 1000)
let demoLogs = Array.from({ length: 80 }, (_, index) => index % 2
  ? `${new Date(Date.now() - (80 - index) * 1000).toISOString()} INFO demo/device-${index % 3} health check passed`
  : { timestamp: new Date(Date.now() - (80 - index) * 1000).toISOString(), level: index % 6 ? 'info' : 'warn', device_id: `demo/device-${index % 3}`, message: index % 5 === 0 ? 'A deliberately long fictional log line used to verify automatic wrapping across the available content width.' : 'structured health check passed' })
const devices = [
  { id: 'MODEM_DEMO_001', name: 'DJI 4G Module · Cellular Demo', running: true, healthy: true, operator: 'Demo Mobile', network_mode: 'LTE', network_duplex: 'FDD', signal_dbm: -72, vowifi_active: false, vowifi_enabled: false, network_enabled: true, network_connected: true, public_ip: '203.0.113.18', private_ip: '10.10.0.8', private_ipv6: '2001:db8:10::8', interface: 'wwan0', imei: '860000000000001', imsi: '460001234567890', iccid: '8986000000000000001', msisdn: '+1 202-555-0100', local_phone: '+1 202-555-0100', active_esim_profile_name: 'Demo Profile', modem: { imei: '860000000000001', imsi: '460001234567890', iccid: '8986000000000000001', firmware: 'Demo 1.0', operator: 'Demo Mobile', network_mode: 'LTE', network_duplex: 'FDD', signal_dbm: -72, signal_rsrp: -98, signal_rsrq: -10, signal_sinr: 18, radio_band: 'B3', radio_channel: 1650, reg_status_text: '已注册', native_spn: 'Demo Mobile', native_mcc: '310', native_mnc: '260' }, vowifi_runtime: { sim_ready: true, access_ready: true, tunnel_ready: false, ims_ready: false, sms_ready: true, dataplane_mode: 'cellular', last_reason: '蜂窝网络已连接', last_error_class: '' } },
  { id: 'RM520N-DEMO', name: 'Quectel RM520N · VoWiFi Demo', running: true, healthy: true, operator: 'Example Telecom', network_mode: '5G NSA', network_duplex: 'TDD', signal_dbm: -88, vowifi_active: true, vowifi_enabled: true, network_enabled: false, network_connected: false, interface: 'wwan1', imei: '860000000000002', imsi: '460031234567890', iccid: '8986030000000000002', msisdn: '+1 202-555-0102', local_phone: '+1 202-555-0102', active_esim_profile_name: 'Travel eSIM', modem: { imei: '860000000000002', imsi: '460031234567890', iccid: '8986030000000000002', firmware: 'Demo 2.0', operator: 'Example Telecom', network_mode: '5G NSA', network_duplex: 'TDD', signal_dbm: -88, signal_rsrp: -108, signal_rsrq: -13, signal_sinr: 9, nr5g_signal_sinr: 12, radio_band: 'n78', radio_channel: 633984, reg_status_text: 'VoWiFi', native_mcc: '440', native_mnc: '10', opl: [{ plmn: '44010', pnn_record: 1 }], pnn: [{ record: 1, full_name: 'Example Telecom' }] }, vowifi_runtime: { sim_ready: true, access_ready: true, tunnel_ready: true, ims_ready: true, sms_ready: true, dataplane_mode: 'vowifi', last_reason: 'IMS 已注册', last_error_class: '' } },
  { id: 'NO-NETWORK-DEMO', name: 'Online Modem · No Network or Data', running: true, healthy: true, operator: 'Stale Carrier', network_mode: 'LTE', network_duplex: 'FDD', signal_dbm: -999, vowifi_active: false, vowifi_enabled: false, network_connected: false, interface: 'wwan2', imei: '860000000000003', imsi: '460011234567890', iccid: '8986010000000000003', msisdn: '+1 202-555-0103', modem: { native_spn: 'Example MVNO', native_mcc: '460', native_mnc: '01' } },
  { id: 'OFFLINE-DEMO', name: 'Offline Modem with a Very Long Demonstration Name', running: false, healthy: false, operator: 'Stale Carrier', network_mode: 'LTE', signal_dbm: -83, vowifi_active: false, vowifi_enabled: false, network_connected: false, public_ip: '198.51.100.199', interface: 'wwan3', imei: '860000000000004', iccid: '8986010000000000004' },
]
const contacts = [
  { peer: '+1 202-555-0110', device_id: 'MODEM_DEMO_001', device_name: devices[0].name, imsi: devices[0].imsi, local_phone: devices[0].msisdn, last_content: 'Your scheduled test message was received.', last_timestamp: new Date(Date.now() - 90_000).toISOString(), unread_count: 1 },
  { peer: 'Demo Service', device_id: 'MODEM_DEMO_001', device_name: devices[0].name, imsi: devices[0].imsi, local_phone: devices[0].msisdn, last_content: 'Demo account balance notification', last_timestamp: new Date(Date.now() - 3_600_000).toISOString(), unread_count: 0 },
  ...Array.from({ length: 10 }, (_, index) => ({ peer: `+1 202-555-${String(111 + index).padStart(4, '0')}`, device_id: 'MODEM_DEMO_001', device_name: devices[0].name, imsi: devices[0].imsi, local_phone: devices[0].msisdn, last_content: `Example conversation ${index + 1}`, last_timestamp: new Date(Date.now() - (index + 2) * 7_200_000).toISOString(), unread_count: index % 3 === 0 ? 1 : 0 })),
]
const sms = [
  { id: 1, peer: contacts[0].peer, content: 'This is a fictional message used only for local UI testing.', timestamp: new Date(Date.now() - 600_000).toISOString(), type: 1, status: 0 },
  { id: 2, peer: contacts[0].peer, content: 'VoHiveX local demo message with a longer line that verifies wrapping and the maximum bubble width on desktop and mobile screens.', timestamp: new Date(Date.now() - 300_000).toISOString(), type: 2, status: 2 },
  { id: 3, peer: contacts[0].peer, content: 'Received, thank you.', timestamp: new Date(Date.now() - 90_000).toISOString(), type: 1, status: 0 },
]
const schedules = [
  { id: 1, version: 2, name: 'Daily demo reminder', device_id: 'MODEM_DEMO_001', phone: '+1 202-555-0110', message: 'This is a fictional scheduled SMS.', mode: 'interval', first_run: now() - 86400, interval_seconds: 86400, state: 'active', next_run: now() + 3600, last_run: now() - 82800, run_count: 3, last_result: 'Submitted' },
  { id: 2, version: 1, name: 'One-time test', device_id: 'RM520N-DEMO', phone: '+1 202-555-0111', message: 'Paused sample task.', mode: 'once', first_run: now() + 7200, interval_seconds: 0, state: 'paused', next_run: now() + 7200, run_count: 0, last_result: 'Waiting' },
]
const sources = [{ id: 'builtin', name: '订阅节点 · 内置代理', kind: 'built_in', count: 1, updated: now() - 300 }, { id: 'demo-sub', name: 'Demo Subscription', kind: 'subscription', count: 4, updated: now() - 1800 }]
const nodes = [{ id: 'builtin-socks', source_id: 'builtin', name: 'Socks5 · 127.0.0.1:17890', type: 'socks5' }, ...['Tokyo Demo', 'Singapore Demo', 'Frankfurt Demo', 'Los Angeles Demo'].map((name, index) => ({ id: `node-${index}`, source_id: 'demo-sub', name, type: index % 2 ? 'vmess' : 'trojan' }))]
let managed = { sources, nodes, version: 1, enabled: true, running: true, selected: 'node-0' }
let demoEsimProfileName = 'Demo Profile'
let demoEsimProfileDeleted = false
let callContacts = [
  { id: 'contact-demo-001', name: 'Example Contact', number: '+12025550110', note: 'Local UI sample', favorite: true, created_at: now() - 86400, updated_at: now() - 86400 },
  { id: 'contact-demo-002', name: 'Demo Support', number: '+12025550111', note: '', favorite: false, created_at: now() - 7200, updated_at: now() - 7200 },
]
let callHistory = [
  { id: 'call-demo-001', device_id: 'MODEM_DEMO_001', device_name: devices[0].name, index: 1, direction: 'incoming', state: 'ended', number: '+12025550110', contact_name: 'Example Contact', started_at: now() - 3800, connected_at: now() - 3790, ended_at: now() - 3625, duration_seconds: 165, missed: false, recording_id: '' },
  { id: 'call-demo-002', device_id: 'MODEM_DEMO_001', device_name: devices[0].name, index: 1, direction: 'incoming', state: 'ended', number: '+12025550112', contact_name: '', started_at: now() - 86000, ended_at: now() - 85970, duration_seconds: 0, missed: true, recording_id: '' },
]
let demoAgreementAcceptedAt = 0
let demoCall: any = null
let demoRecordingActive = false
let demoNetworkSession: { id: string; device_id: string; interface: string; started_at: number } | null = null
let demoNetworkLoadedAt = 0
let demoNetworkTarget = ''

function json(response: ServerResponse, status: number, value: unknown) {
  response.statusCode = status
  response.setHeader('Content-Type', 'application/json; charset=utf-8')
  response.setHeader('Cache-Control', 'no-store')
  response.end(JSON.stringify(value))
}
function body(request: IncomingMessage) { return new Promise<any>((resolve) => { let raw = ''; request.on('data', (chunk) => raw += chunk); request.on('end', () => { try { resolve(raw ? JSON.parse(raw) : {}) } catch { resolve({}) } }) }) }
function escapeHTML(value: unknown) { return String(value ?? '').replace(/[&<>"']/g, (character) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[character] || character)) }

export function demoApi(): Plugin {
  return { name: 'vohivex-local-demo-api', configureServer(server) {
    server.middlewares.use(async (request, response, next) => {
      if (request.url?.startsWith('/healthz')) return json(response, 200, { status: 'ok', version: '2.1.6', uptime_seconds: 3600 })
      if (!request.url?.startsWith('/api/')) return next()
      const url = new URL(request.url, 'http://demo.local'); const path = url.pathname; const method = request.method || 'GET'
      if (path === '/api/auth/login') return json(response, 200, { token: 'vohivex-local-demo-token' })
      if (path === '/api/settings/agreement') {
        if (method === 'POST') {
          const data = await body(request)
          if (data.user_id !== 'demo-admin') return json(response, 409, { error: '账号已变化，请刷新后重试' })
          if (!demoAgreementAcceptedAt) {
            const previous = Number(data.previous_accepted_at)
            if (data.confirmation === '我同意并确认') demoAgreementAcceptedAt = Date.now()
            else if (Number.isSafeInteger(previous) && previous >= 946684800000 && previous <= Date.now()) demoAgreementAcceptedAt = previous
            else return json(response, 400, { error: '请输入“我同意并确认”' })
          }
        }
        return json(response, 200, { user_id: 'demo-admin', accepted: demoAgreementAcceptedAt > 0, accepted_at: demoAgreementAcceptedAt })
      }
      if (path === '/api/sms/devices') return json(response, 200, { devices })
      if (path === '/api/network-debug/devices' && method === 'GET') return json(response, 200, { devices: devices.filter((row) => row.running && row.healthy && row.network_enabled && row.network_connected).map((row) => ({ id: row.id, name: row.name, interface: row.interface, operator: row.operator, network_mode: row.network_mode, local_phone: row.local_phone })) })
      if (path === '/api/network-debug/sessions' && method === 'POST') {
        const data = await body(request)
        const device = devices.find((row) => row.id === data.device_id && row.running && row.healthy && row.network_enabled && row.network_connected)
        if (!device) return json(response, 400, { error: '所选设备的蜂窝网络未启用或尚未连接' })
        demoNetworkSession = { id: `demo-network-${Date.now()}`, device_id: device.id, interface: device.interface, started_at: now() }
        demoNetworkLoadedAt = 0
        demoNetworkTarget = ''
        return json(response, 201, { ...demoNetworkSession, entry_url: `/api/network-debug/view/${demoNetworkSession.id}`, expires_in: 1800 })
      }
      if (/^\/api\/network-debug\/sessions\/[^/]+\/stats$/.test(path) && method === 'GET') {
        if (!demoNetworkSession || !path.includes(demoNetworkSession.id)) return json(response, 404, { error: '浏览会话不存在或已过期' })
        const elapsed = demoNetworkLoadedAt ? Math.max(1, Math.floor((Date.now() - demoNetworkLoadedAt) / 1000)) : 0
        const received = demoNetworkLoadedAt ? 182_400 + elapsed * 3_840 : 0
        const sent = demoNetworkLoadedAt ? 4_620 + elapsed * 96 : 0
        return json(response, 200, { ...demoNetworkSession, received_bytes: received, sent_bytes: sent, total_bytes: received + sent, current_speed_bps: demoNetworkLoadedAt ? (elapsed < 6 ? 26_400 : 3_936) : 0, average_speed_bps: elapsed ? (received + sent) / elapsed : 0, request_count: demoNetworkLoadedAt ? 14 : 0, active_requests: elapsed > 0 && elapsed < 2 ? 3 : 0, last_url: demoNetworkTarget, last_status: demoNetworkLoadedAt ? 200 : 0, last_error: '' })
      }
      if (/^\/api\/network-debug\/sessions\/[^/]+$/.test(path) && method === 'DELETE') { demoNetworkSession = null; demoNetworkLoadedAt = 0; demoNetworkTarget = ''; return json(response, 200, { closed: true }) }
      if (/^\/api\/network-debug\/view\/[^/]+/.test(path) && method === 'GET') {
        if (!demoNetworkSession || !path.includes(demoNetworkSession.id)) { response.statusCode = 410; response.setHeader('Content-Type', 'text/html; charset=utf-8'); return response.end('<h1>浏览会话已结束</h1>') }
        const target = url.searchParams.get('url')
        if (!target) { response.statusCode = 400; response.setHeader('Content-Type', 'text/html; charset=utf-8'); return response.end('<h1>请输入网页地址</h1>') }
        demoNetworkLoadedAt ||= Date.now()
        demoNetworkTarget = target
        response.statusCode = 200
        response.setHeader('Content-Type', 'text/html; charset=utf-8')
        response.setHeader('Cache-Control', 'no-store')
        return response.end(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>VoHiveX 网络调试演示</title><style>*{box-sizing:border-box}body{margin:0;background:#fff;color:#242722;font:15px/1.6 system-ui,sans-serif}header{padding:18px 24px;background:#9fe870;color:#163300;font-weight:800}main{max-width:820px;margin:0 auto;padding:56px 24px}.hero{padding:34px;border-radius:24px;background:#edefec}h1{margin:0 0 8px;color:#163300;font-size:32px}p{margin:8px 0}.url{overflow:hidden;margin-top:22px;padding:12px 16px;border-radius:999px;background:#fff;text-overflow:ellipsis;white-space:nowrap}.grid{display:grid;grid-template-columns:repeat(3,1fr);gap:16px;margin-top:18px}.grid div{padding:18px;border-radius:18px;background:#f1f1ee}.grid strong{display:block;color:#163300}@media(max-width:620px){.grid{grid-template-columns:1fr}}</style><header>VoHiveX · Cellular Network Debug</header><main><section class="hero"><h1>蜂窝网页访问成功</h1><p>这是本地演示模式中的虚拟网页，用于检查浏览窗口、全网页模式和流量统计。</p><div class="url">${escapeHTML(target)}</div><div class="grid"><div><strong>设备</strong>DJI 4G Module · Cellular Demo</div><div><strong>接口</strong>wwan0</div><div><strong>状态</strong>HTTP 200</div></div></section></main></html>`)
      }
      if (path === '/api/dashboard/devices') return json(response, 200, devices)
      if (path === '/api/traffic/analysis') {
        const range = url.searchParams.get('range') || 'day'
        if (range === 'month') return json(response, 200, { buckets: [], chart: null })
        const count = range === 'day' ? 24 : range === 'week' ? 7 : 30
        const step = range === 'day' ? 3600_000 : 86400_000
        const buckets = Array.from({ length: count }, (_, index) => {
          const stamp = new Date(Date.now() - (count - index - 1) * step).toISOString()
          const rx = range === 'week' ? 0 : (index % 6 + 1) * 7_800_000
          const tx = range === 'week' ? 0 : (index % 4 + 1) * 2_200_000
          return { period_start: stamp, rx_bytes: rx, tx_bytes: tx, total_bytes: rx + tx }
        })
        return json(response, 200, { buckets, chart: null })
      }
      if (path === '/api/devices' && method === 'GET') return json(response, 200, { devices, device_limit: 16 })
      if (path === '/api/devices' && method === 'POST') { const data = await body(request); if (!data?.config?.id) return json(response, 400, { error: '必须填写 id' }); return json(response, 200, { started: true }) }
      if (path === '/api/devices/discovered') return json(response, 200, { devices: [
        { discovery_key: '1-2.4|/dev/ttyUSB9', configured: false, net_interface: 'wwan4', driver_name: 'qmi_wwan', mode: 'qmi', control_path: '/dev/cdc-wdm1', at_port: '/dev/ttyUSB9', usb_path: '1-2.4', imei: '860000000000009' },
        { discovery_key: '1-2.3|/dev/ttyUSB2', configured: true, net_interface: 'wwan0', driver_name: 'qmi_wwan', mode: 'qmi', control_path: '/dev/cdc-wdm0', at_port: '/dev/ttyUSB2', usb_path: '1-2.3', imei: devices[0].imei },
      ] })
      if (/^\/api\/devices\/[^/]+\/overview$/.test(path)) return json(response, 200, devices.find((row) => path.includes(row.id)) || devices[0])
      if (/^\/api\/devices\/[^/]+\/config$/.test(path)) return json(response, 200, { config: { id: devices[0].id, name: devices[0].name, modem_imei: devices[0].imei, usb_path: '1-2.3', at_port: '/dev/ttyUSB2', control_device: '/dev/cdc-wdm0', interface: 'wwan0', device_backend: 'auto', sms_enabled: true } })
      if (/^\/api\/devices\/[^/]+\/esim\/notifications$/.test(path)) return json(response, 200, { items: [{ id: 'demo-note', type: 'Profile installation', message: 'A fictional notification used only for the local demonstration.' }] })
      if (/^\/api\/devices\/[^/]+\/esim\/profiles\/[^/]+$/.test(path) && method === 'PATCH') { const data = await body(request); demoEsimProfileName = String(data.name || demoEsimProfileName); return json(response, 200, { ok: true }) }
      if (/^\/api\/devices\/[^/]+\/esim\/profiles\/[^/]+$/.test(path) && method === 'DELETE') { demoEsimProfileDeleted = true; return json(response, 200, { ok: true }) }
      if (/^\/api\/devices\/[^/]+\/esim$/.test(path)) return json(response, 200, { chip_info: { sku_name: 'Demo eUICC', firmware: '1.0.0', eids: [{ eid: '89049032000000000000000000000001', aid: 'A0000005591010FFFFFFFF8900000100', manufacturer: 'Demo Semiconductor', free_nvram: '1.8 MB', free_nvram_bytes: 1_887_436 }] }, profiles: [{ name: 'Demo eUICC', eid: '89049032000000000000000000000001', aid_hex: 'A0000005591010FFFFFFFF8900000100', profiles: demoEsimProfileDeleted ? [] : [{ iccid: '8986000000000001001', name: demoEsimProfileName, phone_number: '+1 202-555-0100', provider_name: 'Example Mobile', state: 1, state_text: '已启用' }] }] })
      if (/^\/api\/cards\/.+\/policy$/.test(path)) return json(response, 200, { source: 'local', ip_version: 'v4v6', apn: 'internet', network_enabled: true, vowifi_enabled: true, airplane_enabled: false })
      if (path === '/api/managed-proxy' && method === 'GET') return json(response, 200, managed)
      if (/^\/api\/managed-proxy\//.test(path) && path !== '/api/managed-proxy/public-ip') { const data = await body(request); if (path.endsWith('/pause')) managed = { ...managed, enabled: false, running: false, version: managed.version + 1 }; if (path.endsWith('/select')) managed = { ...managed, enabled: true, running: true, selected: data.node_id || managed.selected, version: managed.version + 1 }; if (path.endsWith('/test')) return json(response, 200, { delay_ms: 42 + Number(String(data.node_id || '').replace(/\D/g, '') || 0) * 17 }); return json(response, 200, managed) }
      if (path === '/api/managed-proxy/public-ip') return json(response, 200, { nas: { ip: '2001:db8:100::18' }, proxy: { ip: '198.51.100.42' }, devices: { MODEM_DEMO_001: { label: 'IP 地址', ip: '203.0.113.18' }, 'RM520N-DEMO': { label: '代理 IP', ip: '198.51.100.42' } } })
      if (path === '/api/upstream-proxies') return json(response, 200, [{ id: 'vohive-mihomo', name: '订阅节点 · 内置代理', mode: 'socks5', host: '127.0.0.1', port: 17890, enabled: true }, { id: 'demo-roaming', name: 'Demo roaming proxy', mode: 'socks5', host: '192.0.2.10', port: 1080, enabled: true }])
      if (path === '/api/upstream-proxy-countries') return json(response, 200, [
        { country_code: 'JP', country_name: 'Japan', mccs: ['440', '441'] },
        { country_code: 'SG', country_name: 'Singapore', mccs: ['525'] },
        { country_code: 'US', country_name: 'United States', mccs: ['310', '311', '312', '313', '314', '315', '316'] },
        { country_code: 'UA', country_name: 'Ukraine', mccs: ['255'] },
      ])
      if (path === '/api/upstream-proxy-country-rules') return json(response, 200, [{ country_code: 'JP', upstream_proxy_id: 'vohive-mihomo' }])
      if (path === '/api/proxy-instances/overview') return json(response, 200, { instances: [{ id: 'outbound-demo', name: 'DJI outbound', device_id: 'MODEM_DEMO_001', listen_addr: '0.0.0.0', listen_port: 1080, auth_enabled: false, username: '', mode: 'socks5', enabled: true }], devices, status: [{ id: 'outbound-demo', running: true }] })
      if (path === '/api/calls/devices') return json(response, 200, { devices, audio: { available: true, detail: '本地演示 ALSA 捕获设备已就绪', device: 'demo:0', active: demoRecordingActive } })
      if (path === '/api/calls/status') {
        const deviceID = url.searchParams.get('device_id')
        const device = devices.find((row) => row.id === deviceID)
        const active = demoCall?.device_id === deviceID ? demoCall : null
        const mode = device?.vowifi_enabled || device?.vowifi_active ? 'vowifi' : 'cellular'
        const transport = active ? 'cellular' : mode
        return json(response, 200, { device_id: deviceID, calls: active ? [active] : [], transport, device_transport: mode, call_control_available: transport === 'cellular', call_control_detail: transport === 'vowifi' ? '设备使用 VoWiFi；当前网关尚未接入 VoWiFi 电话控制，不会改走蜂窝通道' : '跟随设备模式，当前使用蜂窝电话通道', audio: { available: true, detail: '本地演示 ALSA 捕获设备已就绪', device: 'demo:0', active: demoRecordingActive, call_id: active?.id }, updated_at: now() })
      }
      if (path === '/api/calls/dial' && method === 'POST') { const data = await body(request); const device = devices.find((row) => row.id === data.device_id); if (device?.vowifi_enabled || device?.vowifi_active) return json(response, 501, { error: 'VoWiFi 电话接口尚未接入，不会改走蜂窝通道' }); demoCall = { id: `call-demo-${Date.now()}`, device_id: data.device_id, device_name: devices.find((row) => row.id === data.device_id)?.name || data.device_id, index: 1, direction: 'outgoing', state: 'active', number: data.number, contact_name: callContacts.find((row) => row.number === data.number)?.name || '', started_at: now(), connected_at: now(), duration_seconds: 0, missed: false }; callHistory = [demoCall, ...callHistory]; return json(response, 200, { dialing: true, call: demoCall }) }
      if (path === '/api/calls/answer' && method === 'POST') { if (demoCall) { demoCall.state = 'active'; demoCall.connected_at = now() } return json(response, 200, { answered: true }) }
      if ((path === '/api/calls/hangup' || path === '/api/calls/reject') && method === 'POST') { if (demoCall) { demoCall.state = 'ended'; demoCall.ended_at = now(); demoCall.duration_seconds = Math.max(0, now() - Number(demoCall.connected_at || now())); demoCall = null }; demoRecordingActive = false; return json(response, 200, { hung_up: true }) }
      if (path === '/api/calls/dtmf' && method === 'POST') return json(response, 200, { sent: true })
      if (path === '/api/calls/contacts' && method === 'GET') return json(response, 200, { items: callContacts })
      if (path === '/api/calls/contacts' && method === 'POST') { const data = await body(request); const item = { id: `contact-${Date.now()}`, ...data, created_at: now(), updated_at: now() }; callContacts = [item, ...callContacts]; return json(response, 201, item) }
      if (/^\/api\/calls\/contacts\/[^/]+$/.test(path) && method === 'PUT') { const data = await body(request); const id = path.split('/').pop(); callContacts = callContacts.map((row) => row.id === id ? { ...row, ...data, updated_at: now() } : row); return json(response, 200, callContacts.find((row) => row.id === id)) }
      if (/^\/api\/calls\/contacts\/[^/]+$/.test(path) && method === 'DELETE') { const id = path.split('/').pop(); callContacts = callContacts.filter((row) => row.id !== id); return json(response, 200, { deleted: true }) }
      if (path === '/api/calls/history' && method === 'GET') return json(response, 200, { items: callHistory })
      if (/^\/api\/calls\/history\/[^/]+$/.test(path) && method === 'DELETE') { const id = path.split('/').pop(); callHistory = callHistory.filter((row) => row.id !== id); return json(response, 200, { deleted: true }) }
      if (path === '/api/calls/recordings' && method === 'GET') return json(response, 200, { items: [], state: { available: true, detail: '本地演示 ALSA 捕获设备已就绪', device: 'demo:0', active: demoRecordingActive, call_id: demoCall?.id } })
      if (path === '/api/calls/recordings/start' && method === 'POST') { demoRecordingActive = true; return json(response, 201, { active: true, id: 'recording-demo', started_at: now() }) }
      if (path === '/api/calls/recordings/stop' && method === 'POST') { demoRecordingActive = false; return json(response, 200, { id: 'recording-demo', saved: true }) }
      if (path === '/api/sms/contacts') return json(response, 200, contacts.filter((row) => !url.searchParams.get('device_id') || url.searchParams.get('device_id') === 'all' || row.device_id === url.searchParams.get('device_id')))
      if (path === '/api/sms/thread' && method === 'GET') return json(response, 200, sms.map((row) => ({ ...row, peer: url.searchParams.get('peer') || row.peer })))
      if (path === '/api/schedules') return json(response, 200, method === 'GET' ? { tasks: schedules, server_time: now() } : { ok: true })
      if (path === '/api/schedules/devices') return json(response, 200, { devices })
      if (/^\/api\/schedules\/\d+\/history$/.test(path)) return json(response, 200, { runs: [{ scheduled_for: now() - 3600, status: 'success', detail: 'Demo message submitted successfully.' }] })
      if (path === '/api/settings/username') return json(response, 200, method === 'GET' ? { username: 'admin' } : { ok: true })
      if (path === '/api/settings/system') return json(response, 200, { system_time: now(), build_time: new Date().toISOString(), driver_version: 'kernel built-in · demo', proxy_version: 'Mihomo v1.19.31', config_path: '/opt/vohivex/config/config.yaml' })
      if (path === '/api/settings/notifications') return json(response, 200, method === 'GET' ? { telegram: { enabled: false }, feishu: { enabled: false }, qq: { enabled: false }, bark: { enabled: false, urls: [] }, email: { enabled: false }, pushplus: { enabled: false }, webhook: { enabled: false, urls: [] } } : { applied: true })
      if (path === '/api/logs/history' && method === 'DELETE') { demoLogs = []; return json(response, 200, { cleared: true }) }
      if (path === '/api/logs/history') return json(response, 200, { data: { history: demoLogs } })
      if (method !== 'GET') return json(response, 200, { ok: true, inserted: 3, skipped: 0 })
      return json(response, 200, {})
    })
  } }
}
