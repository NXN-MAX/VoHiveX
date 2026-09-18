<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
import EgressIps from '@/components/EgressIps.vue'
import RiIcon from '@/components/RiIcon.vue'
import TrafficAnalysisPanel from '@/components/TrafficAnalysisPanel.vue'
import { request } from '@/api/http'
import type { Device } from '@/types/domain'

type TrafficRange = 'day' | 'week' | 'month'
const router = useRouter()
const devices = ref<Device[]>([])
const loading = ref(false)
const trafficLoading = ref(false)
const trafficRange = ref<TrafficRange>('day')
const traffic = ref<{ buckets?: any[] }>({ buckets: [] })
const lastRefresh = ref<number>()
const error = ref('')
const trafficError = ref('')
let deviceTimer = 0
let trafficTimer = 0

const online = computed(() => devices.value.filter((device) => device.healthy).length)
const offline = computed(() => Math.max(0, devices.value.length - online.value))
const hasNetworkDevice = computed(() => devices.value.some((device) => Boolean(device.healthy && device.network_connected)))

function modemOf(device: Device) {
  return device.modem || {}
}

function cellularRegistered(device: Device) {
  const modem = modemOf(device)
  const status = Number(modem.reg_status ?? modem.registration_status)
  if ([1, 5].includes(status)) return true
  const text = String(modem.reg_status_text || '').trim()
  return text.includes('已注册') || /\b(registered|home|roaming)\b/i.test(text)
}

function signalValue(device: Device) {
  const value = Number(modemOf(device).signal_dbm ?? device.signal_dbm ?? -999)
  return Number.isFinite(value) && value !== 0 && value !== -999 ? value : undefined
}

function cellularAvailable(device: Device) {
  return Boolean(device.healthy && (device.network_connected || cellularRegistered(device) || signalValue(device) !== undefined))
}

function mergeDevice(base: Device, detail: any): Device {
  const row = detail?.devices?.[0] || detail
  if (!row || typeof row !== 'object' || Array.isArray(row)) return base
  return {
    ...base,
    ...row,
    modem: { ...(base.modem || {}), ...(row.modem || {}) },
    vowifi_runtime: { ...(base.vowifi_runtime || {}), ...(row.vowifi_runtime || {}) },
  }
}

function networkLabel(device: Device) {
  if (!device.healthy) return '设备离线'
  if (device.vowifi_active) return 'Wi-Fi Calling'
  if (!cellularAvailable(device)) return '未驻网'
  const modem = modemOf(device)
  return [modem.network_duplex || device.network_duplex, modem.network_mode || device.network_mode].filter(Boolean).join(' ') || '蜂窝网络'
}

function networkName(device: Device) {
  if (!device.healthy) return '设备离线'
  if (device.vowifi_active) return 'Wi-Fi Calling'
  if (!cellularAvailable(device)) return '未驻网'
  return String(modemOf(device).operator || device.operator || '蜂窝网络')
}

function signalIcon(value?: number) {
  if (!value || value === -999) return 'signal-cellular-off-line'
  if (value > -70) return 'signal-cellular-3-fill'
  if (value > -90) return 'signal-cellular-2-fill'
  return 'signal-cellular-1-fill'
}

function networkIcon(device: Device) {
  if (device.vowifi_active) return 'wifi-line'
  if (!cellularAvailable(device)) return 'signal-cellular-off-line'
  return signalIcon(signalValue(device))
}

async function loadDevices() {
  loading.value = true
  error.value = ''
  try {
    const data = await request<Device[] | { devices?: Device[] }>({ url: '/dashboard/devices' })
    const rows = Array.isArray(data) ? data : data.devices || []
    const details = await Promise.allSettled(rows.map((device) => request<any>({ url: `/devices/${encodeURIComponent(device.id)}/overview` })))
    devices.value = rows.map((device, index) => details[index]?.status === 'fulfilled' ? mergeDevice(device, details[index].value) : device)
    lastRefresh.value = Date.now()
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '设备加载失败'
  } finally {
    loading.value = false
  }
}

async function loadTraffic() {
  trafficLoading.value = true
  trafficError.value = ''
  try {
    traffic.value = await request({ url: '/traffic/analysis', params: { range: trafficRange.value } })
  } catch (reason) {
    traffic.value = { buckets: [] }
    trafficError.value = reason instanceof Error ? reason.message : '流量分析加载失败'
  } finally {
    trafficLoading.value = false
  }
}

function changeTrafficRange(value: TrafficRange) {
  trafficRange.value = value
  loadTraffic()
}

function refreshAll() { void Promise.allSettled([loadDevices(), loadTraffic()]) }

onMounted(() => {
  refreshAll()
  deviceTimer = window.setInterval(loadDevices, 15_000)
  trafficTimer = window.setInterval(loadTraffic, 60_000)
})
onBeforeUnmount(() => { window.clearInterval(deviceTimer); window.clearInterval(trafficTimer) })
</script>

<template>
  <div class="dashboard-page">
    <div class="dashboard-heading">
      <PageHeader title="设备监控" subtitle="实时查看设备状态与网络连接" />
      <dl class="dashboard-summary">
        <div><dt>设备总数</dt><dd>{{ devices.length }}</dd></div>
        <div><dt>在线设备</dt><dd>{{ online }}</dd></div>
        <div><dt>离线设备</dt><dd>{{ offline }}</dd></div>
        <div><dt>最后刷新</dt><dd>{{ lastRefresh ? new Date(lastRefresh).toLocaleTimeString() : '--:--:--' }}</dd></div>
      </dl>
      <a-button class="outline-button dashboard-refresh" :loading="loading || trafficLoading" @click="refreshAll">刷新</a-button>
    </div>
    <a-alert v-if="error" type="error" :message="error" show-icon closable class="page-alert" />
    <a-spin :spinning="loading && !devices.length">
      <EmptyState v-if="!loading && !devices.length" title="暂无设备接入" description="请先在设备管理中添加或接管设备" />
      <div v-else class="device-card-grid">
        <button v-for="device in devices" :key="device.id" class="device-card" @click="router.push({ path: '/devices', query: { device: device.id, tab: 'overview' } })">
          <span class="device-card-heading">
            <strong :title="device.name || device.id">{{ device.name || device.id }}</strong>
            <span class="device-state" :class="device.healthy ? 'online' : 'offline'"><i />{{ device.healthy ? '在线' : '离线' }}</span>
          </span>
          <span class="device-card-row">
            <span class="network-name"><RiIcon :name="networkIcon(device)" :size="16" />{{ networkName(device) }}</span>
            <small>{{ networkLabel(device) }}</small>
          </span>
          <span v-if="!device.vowifi_active" class="device-signal">{{ signalValue(device) !== undefined ? `${signalValue(device)} dBm` : (cellularAvailable(device) ? '-- dBm' : '无蜂窝信号') }}</span>
          <EgressIps v-if="device.vowifi_active" compact :device-id="device.id" :vowifi-enabled="true" />
        </button>
      </div>
    </a-spin>
    <TrafficAnalysisPanel :analysis="traffic" :loading="trafficLoading" :error="trafficError" :range="trafficRange" :disabled="devices.length > 0 && !hasNetworkDevice" disabled-text="当前没有已连接网络的设备，暂无流量分析" @update:range="changeTrafficRange" @refresh="loadTraffic" />
  </div>
</template>

<style scoped>
.dashboard-heading{display:grid;grid-template-columns:minmax(260px,1fr) auto auto;align-items:flex-start;gap:32px}.dashboard-heading :deep(.page-header){min-width:0}.dashboard-summary{display:grid;grid-template-columns:repeat(2,minmax(140px,1fr));gap:5px 26px;margin:5px 0 28px;font-size:13px;font-variant-numeric:tabular-nums}.dashboard-summary div{display:grid;grid-template-columns:5em minmax(4em,1fr);gap:8px}.dashboard-summary dt{color:var(--vx-muted)}.dashboard-summary dd{margin:0;color:var(--vx-ink);font-weight:700;white-space:nowrap}.dashboard-refresh{justify-self:end}.page-alert{margin-bottom:20px}.device-card-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(250px,288px));gap:20px}.device-card{display:flex;min-height:174px;flex-direction:column;gap:13px;padding:20px;border:0;border-radius:20px;background:var(--vx-panel);color:var(--vx-ink);text-align:left;cursor:pointer;transition:transform .15s ease,background .15s ease}.device-card:hover{transform:translateY(-2px);background:color-mix(in srgb,var(--vx-panel) 90%,var(--vx-accent))}.device-card-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:14px;padding-bottom:13px;border-bottom:1px solid color-mix(in srgb,var(--vx-line) 70%,transparent)}.device-card-heading strong{min-width:0;overflow:hidden;font-size:16px;text-overflow:ellipsis;white-space:nowrap}.device-state{display:inline-flex;align-items:center;gap:6px;flex:none;color:var(--vx-muted);font-size:12px;white-space:nowrap}.device-state i{width:7px;height:7px;border-radius:50%;background:#a4a69f}.device-state.online{color:#3e771b}.device-state.online i{background:#61bd32}.device-state.offline{color:var(--vx-danger)}.device-state.offline i{background:var(--vx-danger)}.device-card-row{display:flex;align-items:center;justify-content:space-between;gap:12px}.network-name{display:flex;min-width:0;align-items:center;gap:7px;overflow:hidden;font-size:13px;text-overflow:ellipsis;white-space:nowrap}.device-card-row small,.device-signal{color:var(--vx-muted);font-size:11px}.device-card :deep(.compact-ip){margin-top:auto;padding-top:12px;border-top:1px solid color-mix(in srgb,var(--vx-line) 70%,transparent)}
@media(max-width:1050px){.dashboard-heading{grid-template-columns:minmax(0,1fr) auto}.dashboard-summary{grid-column:1/-1;grid-row:2;margin:-10px 0 24px}.dashboard-refresh{grid-column:2;grid-row:1}.device-card-grid{grid-template-columns:repeat(auto-fill,minmax(250px,1fr))}}
@media(max-width:620px){.dashboard-summary{grid-template-columns:1fr 1fr;gap:6px 12px}.dashboard-summary div{display:block}.dashboard-summary dd{margin-top:2px}.device-card-grid{grid-template-columns:1fr}.device-card{max-width:none}}
</style>
