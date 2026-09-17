import type { Plugin } from 'vite'
import type { IncomingMessage, ServerResponse } from 'node:http'

const now = () => Math.floor(Date.now() / 1000)
const devices = [
  { id: 'DJI26123', name: 'DJI 4G Module · Cellular Demo', running: true, healthy: true, operator: 'Demo Mobile', network_mode: 'LTE', network_duplex: 'FDD', signal_dbm: -72, vowifi_active: false, vowifi_enabled: false, network_connected: true, public_ip: '203.0.113.18', private_ip: '10.10.0.8', private_ipv6: '2001:db8:10::8', interface: 'wwan0', imei: '860000000000001', imsi: '460001234567890', iccid: '8986000000000000001', msisdn: '+86 138 0000 1001', modem: { native_spn: 'Demo Mobile', native_mcc: '310', native_mnc: '260' } },
  { id: 'RM520N-DEMO', name: 'Quectel RM520N · VoWiFi Demo', running: true, healthy: true, operator: 'Example Telecom', network_mode: '5G NSA', network_duplex: 'TDD', signal_dbm: -88, vowifi_active: true, vowifi_enabled: true, network_connected: false, interface: 'wwan1', imei: '860000000000002', imsi: '460031234567890', iccid: '8986030000000000002', msisdn: '+86 139 0000 1002', modem: { native_mcc: '440', native_mnc: '10', opl: [{ plmn: '44010', pnn_record: 1 }], pnn: [{ record: 1, full_name: 'Example Telecom' }] } },
  { id: 'NO-NETWORK-DEMO', name: 'Online Modem · No Network or Data', running: true, healthy: true, operator: 'Stale Carrier', network_mode: 'LTE', network_duplex: 'FDD', signal_dbm: -999, vowifi_active: false, vowifi_enabled: false, network_connected: false, interface: 'wwan2', imei: '860000000000003', imsi: '460011234567890', iccid: '8986010000000000003', msisdn: '+86 137 0000 1003', modem: { native_spn: 'Example MVNO', native_mcc: '460', native_mnc: '01' } },
  { id: 'OFFLINE-DEMO', name: 'Offline Modem with a Very Long Demonstration Name', running: false, healthy: false, operator: 'Stale Carrier', network_mode: 'LTE', signal_dbm: -83, vowifi_active: false, vowifi_enabled: false, network_connected: false, public_ip: '198.51.100.199', interface: 'wwan3', imei: '860000000000004', iccid: '8986010000000000004' },
]
const contacts = [
  { peer: '+86 188 0000 2001', device_id: 'DJI26123', device_name: devices[0].name, imsi: devices[0].imsi, local_phone: devices[0].msisdn, last_content: 'Your scheduled test message was received.', last_ts: now() - 90, unread: 1 },
  { peer: '10086', device_id: 'DJI26123', device_name: devices[0].name, imsi: devices[0].imsi, local_phone: devices[0].msisdn, last_content: 'Demo account balance notification', last_ts: now() - 3600, unread: 0 },
  ...Array.from({ length: 10 }, (_, index) => ({ peer: `+86 166 0000 ${String(3000 + index)}`, device_id: 'DJI26123', device_name: devices[0].name, imsi: devices[0].imsi, local_phone: devices[0].msisdn, last_content: `Example conversation ${index + 1}`, last_ts: now() - (index + 2) * 7200, unread: index % 3 === 0 ? 1 : 0 })),
]
const sms = [
  { id: 1, peer: contacts[0].peer, content: 'This is a fictional message used only for local UI testing.', timestamp: new Date(Date.now() - 600_000).toISOString(), direction: 'in', outgoing: false, status: 'received' },
  { id: 2, peer: contacts[0].peer, content: 'VoHiveX local demo message with a longer line that verifies wrapping and the maximum bubble width on desktop and mobile screens.', timestamp: new Date(Date.now() - 300_000).toISOString(), direction: 'out', outgoing: true, status: 'sent' },
  { id: 3, peer: contacts[0].peer, content: 'Received, thank you.', timestamp: new Date(Date.now() - 90_000).toISOString(), direction: 'in', outgoing: false, status: 'received' },
]
const schedules = [
  { id: 1, version: 2, name: 'Daily demo reminder', device_id: 'DJI26123', phone: '+86 188 0000 2001', message: 'This is a fictional scheduled SMS.', mode: 'interval', first_run: now() - 86400, interval_seconds: 86400, state: 'active', next_run: now() + 3600, last_run: now() - 82800, run_count: 3, last_result: 'Submitted' },
  { id: 2, version: 1, name: 'One-time test', device_id: 'RM520N-DEMO', phone: '+86 166 0000 3001', message: 'Paused sample task.', mode: 'once', first_run: now() + 7200, interval_seconds: 0, state: 'paused', next_run: now() + 7200, run_count: 0, last_result: 'Waiting' },
]
const sources = [{ id: 'builtin', name: '订阅节点 · 内置代理', kind: 'built_in', count: 1, updated: now() - 300 }, { id: 'demo-sub', name: 'Demo Subscription', kind: 'subscription', count: 4, updated: now() - 1800 }]
const nodes = [{ id: 'builtin-socks', source_id: 'builtin', name: 'Socks5 · 127.0.0.1:17890', type: 'socks5' }, ...['Tokyo Demo', 'Singapore Demo', 'Frankfurt Demo', 'Los Angeles Demo'].map((name, index) => ({ id: `node-${index}`, source_id: 'demo-sub', name, type: index % 2 ? 'vmess' : 'trojan' }))]
let managed = { sources, nodes, version: 1, enabled: true, running: true, selected: 'node-0' }

function json(response: ServerResponse, status: number, value: unknown) {
  response.statusCode = status
  response.setHeader('Content-Type', 'application/json; charset=utf-8')
  response.setHeader('Cache-Control', 'no-store')
  response.end(JSON.stringify(value))
}
function body(request: IncomingMessage) { return new Promise<any>((resolve) => { let raw = ''; request.on('data', (chunk) => raw += chunk); request.on('end', () => { try { resolve(raw ? JSON.parse(raw) : {}) } catch { resolve({}) } }) }) }

export function demoApi(): Plugin {
  return { name: 'vohivex-local-demo-api', configureServer(server) {
    server.middlewares.use(async (request, response, next) => {
      if (request.url?.startsWith('/healthz')) return json(response, 200, { status: 'ok', version: '2.1.1', uptime_seconds: 3600 })
      if (!request.url?.startsWith('/api/')) return next()
      const url = new URL(request.url, 'http://demo.local'); const path = url.pathname; const method = request.method || 'GET'
      if (path === '/api/auth/login') return json(response, 200, { token: 'vohivex-local-demo-token' })
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
      if (path === '/api/devices/discovered') return json(response, 200, { devices: [{ id: 'UNMANAGED-DEMO', name: 'Unmanaged demo modem', at_port: '/dev/ttyUSB9', imei: '860000000000009' }] })
      if (/^\/api\/devices\/[^/]+\/overview$/.test(path)) return json(response, 200, devices.find((row) => path.includes(row.id)) || devices[0])
      if (/^\/api\/devices\/[^/]+\/config$/.test(path)) return json(response, 200, { config: { id: devices[0].id, name: devices[0].name, modem_imei: devices[0].imei, usb_path: '1-2.3', at_port: '/dev/ttyUSB2', control_device: '/dev/cdc-wdm0', interface: 'wwan0', device_backend: 'auto', sms_enabled: true } })
      if (/^\/api\/devices\/[^/]+\/esim\/notifications$/.test(path)) return json(response, 200, { items: [{ id: 'demo-note', type: 'Profile installation', message: 'A fictional notification used only for the local demonstration.' }] })
      if (/^\/api\/devices\/[^/]+\/esim$/.test(path)) return json(response, 200, { chip_info: { sku_name: 'Demo eUICC', firmware: '1.0.0', manufacturer: 'Demo Semiconductor', free_space: '1.8 MB', eids: [{ eid: '89049032000000000000000000000001', aid: 'A0000005591010FFFFFFFF8900000100' }] }, profiles: [{ name: 'Demo eUICC', aid_hex: 'A0000005591010FFFFFFFF8900000100', free_space: '1.8 MB', profiles: [{ iccid: '8986000000000001001', name: 'Demo Profile', provider_name: 'Example Mobile', state: 1, state_text: '已启用' }] }] })
      if (/^\/api\/cards\/.+\/policy$/.test(path)) return json(response, 200, { source: 'local', ip_version: 'v4v6', apn: 'internet', network_enabled: true, vowifi_enabled: true, airplane_enabled: false })
      if (path === '/api/managed-proxy' && method === 'GET') return json(response, 200, managed)
      if (/^\/api\/managed-proxy\//.test(path) && path !== '/api/managed-proxy/public-ip') { const data = await body(request); if (path.endsWith('/pause')) managed = { ...managed, enabled: false, running: false, version: managed.version + 1 }; if (path.endsWith('/select')) managed = { ...managed, enabled: true, running: true, selected: data.node_id || managed.selected, version: managed.version + 1 }; if (path.endsWith('/test')) return json(response, 200, { delay_ms: 42 + Number(String(data.node_id || '').replace(/\D/g, '') || 0) * 17 }); return json(response, 200, managed) }
      if (path === '/api/managed-proxy/public-ip') return json(response, 200, { nas: { ip: '2001:db8:100::18' }, proxy: { ip: '198.51.100.42' }, devices: { DJI26123: { label: 'IP 地址', ip: '203.0.113.18' }, 'RM520N-DEMO': { label: '代理 IP', ip: '198.51.100.42' } } })
      if (path === '/api/upstream-proxies') return json(response, 200, [{ id: 'vohive-mihomo', name: '订阅节点 · 内置代理', mode: 'socks5', host: '127.0.0.1', port: 17890, enabled: true }, { id: 'demo-roaming', name: 'Demo roaming proxy', mode: 'socks5', host: '192.0.2.10', port: 1080, enabled: true }])
      if (path === '/api/upstream-proxy-countries') return json(response, 200, [
        { country_code: 'JP', country_name: 'Japan', mccs: ['440', '441'] },
        { country_code: 'SG', country_name: 'Singapore', mccs: ['525'] },
        { country_code: 'US', country_name: 'United States', mccs: ['310', '311', '312', '313', '314', '315', '316'] },
        { country_code: 'UA', country_name: 'Ukraine', mccs: ['255'] },
      ])
      if (path === '/api/upstream-proxy-country-rules') return json(response, 200, [{ country_code: 'JP', upstream_proxy_id: 'vohive-mihomo' }])
      if (path === '/api/proxy-instances/overview') return json(response, 200, { instances: [{ id: 'outbound-demo', name: 'DJI outbound', device_id: 'DJI26123', listen_addr: '0.0.0.0', listen_port: 1080, auth_enabled: false, username: '', mode: 'socks5', enabled: true }], devices, status: [{ id: 'outbound-demo', running: true }] })
      if (path === '/api/sms/contacts') return json(response, 200, contacts.filter((row) => !url.searchParams.get('device_id') || url.searchParams.get('device_id') === 'all' || row.device_id === url.searchParams.get('device_id')))
      if (path === '/api/sms/thread' && method === 'GET') return json(response, 200, sms.map((row) => ({ ...row, peer: url.searchParams.get('peer') || row.peer })))
      if (path === '/api/schedules') return json(response, 200, method === 'GET' ? { tasks: schedules, server_time: now() } : { ok: true })
      if (path === '/api/schedules/devices') return json(response, 200, { devices })
      if (/^\/api\/schedules\/\d+\/history$/.test(path)) return json(response, 200, { runs: [{ scheduled_for: now() - 3600, status: 'success', detail: 'Demo message submitted successfully.' }] })
      if (path === '/api/settings/username') return json(response, 200, method === 'GET' ? { username: 'admin' } : { ok: true })
      if (path === '/api/settings/system') return json(response, 200, { system_time: now(), build_time: new Date().toISOString(), driver_version: 'kernel built-in · demo', proxy_version: 'Mihomo v1.19.31', config_path: '/opt/vohivex/config/config.yaml' })
      if (path === '/api/settings/notifications') return json(response, 200, method === 'GET' ? { telegram: { enabled: false }, feishu: { enabled: false }, qq: { enabled: false }, bark: { enabled: false, urls: [] }, email: { enabled: false }, pushplus: { enabled: false }, webhook: { enabled: false, urls: [] } } : { applied: true })
      if (path === '/api/logs/history') return json(response, 200, { lines: Array.from({ length: 80 }, (_, index) => `${new Date(Date.now() - (80 - index) * 1000).toISOString()} INFO demo/device-${index % 3} ${index % 5 === 0 ? 'A deliberately long fictional log line used to verify automatic wrapping across the available content width.' : 'health check passed'}`) })
      if (method !== 'GET') return json(response, 200, { ok: true, inserted: 3, skipped: 0 })
      return json(response, 200, {})
    })
  } }
}
