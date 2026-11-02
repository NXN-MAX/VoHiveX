<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { message } from 'antdv-next'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
import RiIcon from '@/components/RiIcon.vue'
import { apiError, request } from '@/api/http'

interface DebugDevice {
  id: string
  name: string
  interface: string
  operator?: string
  network_mode?: string
  local_phone?: string
}

interface DebugSession {
  id: string
  device_id: string
  interface: string
  entry_url: string
}

interface DebugStats {
  received_bytes: number
  sent_bytes: number
  total_bytes: number
  current_speed_bps: number
  average_speed_bps: number
  request_count: number
  active_requests: number
  last_url: string
  last_status: number
  last_error: string
}

const devices = ref<DebugDevice[]>([])
const selectedDevice = ref('')
const address = ref('')
const frameSource = ref('')
const session = ref<DebugSession | null>(null)
const loadingDevices = ref(true)
const opening = ref(false)
const frameLoading = ref(false)
const fullPage = ref(false)
const loadError = ref('')
const stats = ref<DebugStats>({
  received_bytes: 0,
  sent_bytes: 0,
  total_bytes: 0,
  current_speed_bps: 0,
  average_speed_bps: 0,
  request_count: 0,
  active_requests: 0,
  last_url: '',
  last_status: 0,
  last_error: '',
})
let statsTimer = 0
let statsBusy = false

const deviceOptions = computed(() => devices.value.map((device) => ({
  value: device.id,
  label: [device.name || device.id, device.local_phone, device.operator, device.network_mode, device.interface].filter(Boolean).join(' · '),
})))
const activeDevice = computed(() => devices.value.find((device) => device.id === selectedDevice.value))
const currentHost = computed(() => {
  const value = stats.value.last_url || address.value
  try { return new URL(value).host } catch { return value || '尚未打开网页' }
})

function emptyStats() {
  stats.value = {
    received_bytes: 0,
    sent_bytes: 0,
    total_bytes: 0,
    current_speed_bps: 0,
    average_speed_bps: 0,
    request_count: 0,
    active_requests: 0,
    last_url: '',
    last_status: 0,
    last_error: '',
  }
}

function formatBytes(value: number) {
  const number = Math.max(0, Number(value) || 0)
  if (number < 1024) return `${Math.round(number)} B`
  if (number < 1024 ** 2) return `${(number / 1024).toFixed(number < 10 * 1024 ? 1 : 0)} KB`
  if (number < 1024 ** 3) return `${(number / 1024 ** 2).toFixed(number < 10 * 1024 ** 2 ? 2 : 1)} MB`
  return `${(number / 1024 ** 3).toFixed(2)} GB`
}

function formatRate(value: number) {
  return `${formatBytes(value)}/s`
}

function normalizedAddress() {
  let value = address.value.trim()
  if (!value) throw new Error('请输入要访问的网址')
  if (!/^[a-z][a-z\d+.-]*:\/\//i.test(value)) value = `https://${value}`
  const parsed = new URL(value)
  if (!['http:', 'https:'].includes(parsed.protocol)) throw new Error('仅支持 HTTP 或 HTTPS 网页地址')
  return parsed.toString()
}

async function loadDevices() {
  loadingDevices.value = true
  try {
    const result = await request<{ devices: DebugDevice[] }>({ url: '/network-debug/devices' })
    devices.value = result.devices || []
    if (!devices.value.some((device) => device.id === selectedDevice.value)) {
      selectedDevice.value = devices.value[0]?.id || ''
    }
  } catch (reason) {
    loadError.value = apiError(reason).message
  } finally {
    loadingDevices.value = false
  }
}

async function pollStats() {
  if (!session.value || statsBusy) return
  statsBusy = true
  try {
    stats.value = await request<DebugStats>({ url: `/network-debug/sessions/${encodeURIComponent(session.value.id)}/stats` })
    if (stats.value.last_error) loadError.value = stats.value.last_error
  } catch (reason) {
    const error = apiError(reason)
    if (error.status === 404) {
      session.value = null
      frameSource.value = ''
      window.clearInterval(statsTimer)
    }
  } finally {
    statsBusy = false
  }
}

function startPolling() {
  window.clearInterval(statsTimer)
  void pollStats()
  statsTimer = window.setInterval(pollStats, 1000)
}

async function closeSession(clearAddress = false) {
  window.clearInterval(statsTimer)
  statsTimer = 0
  const current = session.value
  session.value = null
  frameSource.value = ''
  frameLoading.value = false
  fullPage.value = false
  document.body.style.overflow = ''
  if (clearAddress) {
    address.value = ''
    loadError.value = ''
    emptyStats()
  }
  if (current) {
    try { await request({ method: 'DELETE', url: `/network-debug/sessions/${encodeURIComponent(current.id)}` }) } catch { /* expired sessions need no retry */ }
  }
}

async function changeDevice(value: string) {
  if (session.value && session.value.device_id !== value) {
    await closeSession()
    emptyStats()
    loadError.value = ''
  }
}

async function browse() {
  if (!selectedDevice.value) {
    message.warning('没有可用的蜂窝网络设备')
    return
  }
  let target = ''
  try { target = normalizedAddress() } catch (reason) {
    message.warning(reason instanceof Error ? reason.message : '网页地址无效')
    return
  }
  opening.value = true
  loadError.value = ''
  try {
    if (!session.value || session.value.device_id !== selectedDevice.value) {
      await closeSession()
      emptyStats()
      session.value = await request<DebugSession>({
        method: 'POST',
        url: '/network-debug/sessions',
        data: { device_id: selectedDevice.value },
      })
    }
    address.value = target
    frameLoading.value = true
    frameSource.value = `${session.value.entry_url}?url=${encodeURIComponent(target)}`
    startPolling()
  } catch (reason) {
    loadError.value = apiError(reason).message
    message.error(loadError.value)
  } finally {
    opening.value = false
  }
}

function toggleFullPage() {
  if (!frameSource.value) return
  fullPage.value = !fullPage.value
  document.body.style.overflow = fullPage.value ? 'hidden' : ''
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && fullPage.value) toggleFullPage()
}

onMounted(() => {
  void loadDevices()
  window.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  window.clearInterval(statsTimer)
  document.body.style.overflow = ''
  void closeSession()
})
</script>

<template>
  <div class="network-debug-page">
    <PageHeader title="网络调试" subtitle="使用已连接的蜂窝网络访问网页并查看本次调试流量" />

    <a-alert v-if="loadError && !frameSource" class="page-alert" type="error" :message="loadError" show-icon closable @close="loadError = ''" />

    <section class="network-browser" :class="{ 'is-full-page': fullPage }">
      <header class="browser-toolbar">
        <div class="browser-controls">
          <div class="device-field">
            <a-select
              v-model:value="selectedDevice"
              :loading="loadingDevices"
              :disabled="loadingDevices || !devices.length"
              :options="deviceOptions"
              :placeholder="loadingDevices ? '正在读取蜂窝网络设备' : '没有已连接的蜂窝网络设备'"
              @change="changeDevice"
            />
          </div>
          <div class="address-field">
            <a-input
              v-model:value="address"
              placeholder="输入网址，例如 https://example.com"
              :disabled="!selectedDevice"
              @press-enter="browse"
            />
          </div>
          <a-button type="primary" :loading="opening" :disabled="!selectedDevice || !address.trim()" @click="browse">访问</a-button>
        </div>

        <div class="browser-session-row">
          <div class="traffic-metrics" aria-label="蜂窝流量统计">
            <div><span>当前速度</span><strong>{{ formatRate(stats.current_speed_bps) }}</strong></div>
            <div><span>本次浏览流量</span><strong>{{ formatBytes(stats.total_bytes) }}</strong></div>
            <small>下载 {{ formatBytes(stats.received_bytes) }} · 上传 {{ formatBytes(stats.sent_bytes) }}</small>
          </div>
          <div class="browser-actions">
            <a-button class="ghost-button" :disabled="!frameSource" @click="toggleFullPage">
              <RiIcon :name="fullPage ? 'aspect-ratio-line' : 'fullscreen-line'" :size="17" />
              {{ fullPage ? '退出全网页' : '全网页' }}
            </a-button>
            <a-button class="ghost-button danger" :disabled="!session" @click="closeSession(true)">
              <RiIcon name="close-line" :size="17" />
              退出
            </a-button>
          </div>
        </div>
      </header>

      <div class="browser-stage">
        <div v-if="frameSource" class="browser-window">
          <div class="browser-window-heading">
            <span class="browser-live-dot" :class="{ active: stats.active_requests > 0 }" />
            <strong :title="stats.last_url || address">{{ currentHost }}</strong>
            <span>{{ activeDevice?.name }} · {{ activeDevice?.interface }}</span>
            <span v-if="stats.last_status" class="http-status">HTTP {{ stats.last_status }}</span>
          </div>
          <a-spin :spinning="frameLoading" tip="正在通过蜂窝网络加载网页" class="frame-spinner">
            <iframe
              :key="frameSource"
              :src="frameSource"
              title="蜂窝网络调试浏览窗口"
              sandbox="allow-forms allow-scripts allow-downloads"
              referrerpolicy="no-referrer"
              @load="frameLoading = false"
            />
          </a-spin>
        </div>
        <EmptyState
          v-else
          class="browser-empty"
          icon="earth-line"
          title="尚未打开网页"
          :description="devices.length ? '选择已连接蜂窝网络的设备，输入网址后开始访问' : '请先在设备管理中启用并连接蜂窝网络'"
        />
      </div>

      <p class="browser-note">网页请求固定从所选蜂窝接口发出；流量按本次网页请求与响应正文统计，不含链路协议开销。部分限制嵌入或依赖复杂跨域脚本的网站可能无法完整显示。</p>
    </section>
  </div>
</template>

<style scoped>
.network-browser{display:flex;min-height:610px;flex-direction:column;gap:16px;padding:16px;border-radius:24px;background:var(--vx-panel)}
.browser-toolbar{display:grid;gap:14px}.browser-controls{display:grid;grid-template-columns:minmax(220px,360px) minmax(280px,1fr) auto;align-items:center;gap:12px}.device-field,.address-field{width:100%;min-width:0}.device-field :deep(.ant-select),.address-field :deep(.ant-input){width:100%}
.browser-session-row{display:flex;min-width:0;align-items:center;justify-content:space-between;gap:16px}.traffic-metrics{display:flex;min-width:0;align-items:center;gap:24px}.traffic-metrics>div{display:flex;align-items:baseline;gap:8px}.traffic-metrics span,.traffic-metrics small{color:var(--vx-muted);font-size:11px}.traffic-metrics strong{color:var(--vx-ink);font-size:15px;white-space:nowrap}.browser-actions{display:flex;align-items:center;gap:8px}.browser-actions :deep(.ant-btn){display:inline-flex;align-items:center;gap:6px}
.browser-stage{min-height:0;flex:1}.browser-window{display:flex;height:min(680px,calc(100vh - 290px));min-height:460px;overflow:hidden;flex-direction:column;border:1px solid var(--vx-line);border-radius:18px;background:var(--vx-canvas)}.browser-window-heading{display:flex;height:42px;min-height:42px;align-items:center;gap:8px;padding:0 14px;background:var(--vx-neutral);font-size:11px}.browser-window-heading strong{max-width:240px;overflow:hidden;color:var(--vx-ink);font-size:12px;text-overflow:ellipsis;white-space:nowrap}.browser-window-heading>span:not(.browser-live-dot):not(.http-status){overflow:hidden;color:var(--vx-muted);text-overflow:ellipsis;white-space:nowrap}.browser-live-dot{width:7px;height:7px;flex:none;border-radius:50%;background:#50a41d}.browser-live-dot.active{box-shadow:0 0 0 4px rgba(80,164,29,.15)}.http-status{margin-left:auto;color:var(--vx-muted);white-space:nowrap}.frame-spinner{min-height:0;flex:1}.frame-spinner :deep(.ant-spin-container){height:100%}.frame-spinner :deep(.ant-spin-nested-loading){height:100%}.browser-window iframe{display:block;width:100%;height:100%;border:0;background:#fff}.browser-empty{height:min(680px,calc(100vh - 290px));min-height:460px;border:1px solid var(--vx-line);border-radius:18px;background:var(--vx-canvas)}.browser-note{margin:0;color:var(--vx-muted);font-size:11px;line-height:1.6}
.network-browser.is-full-page{position:fixed;inset:0;z-index:1000;min-height:100vh;padding:16px;border-radius:0;background:var(--vx-canvas)}.is-full-page .browser-toolbar{flex:none}.is-full-page .browser-window,.is-full-page .browser-empty{height:100%;min-height:0}.is-full-page .browser-note{display:none}
@media(max-width:900px){.network-browser{min-height:560px;border-radius:18px}.browser-controls{grid-template-columns:1fr auto}.device-field{grid-column:1/-1}.browser-window,.browser-empty{height:calc(100vh - 330px);min-height:420px}.browser-session-row{align-items:flex-start;flex-direction:column}.browser-actions{align-self:flex-end}.network-browser.is-full-page{padding:12px}.is-full-page .browser-controls{grid-template-columns:1fr auto}.is-full-page .device-field{display:none}.is-full-page .browser-session-row{align-items:center;flex-direction:row}.is-full-page .traffic-metrics small{display:none}}
@media(max-width:620px){.browser-controls{grid-template-columns:minmax(0,1fr) auto}.traffic-metrics{width:100%;justify-content:space-between;gap:12px}.traffic-metrics>div{align-items:flex-start;flex-direction:column;gap:2px}.traffic-metrics small{display:none}.browser-actions{width:100%;justify-content:flex-end}.browser-window-heading>span:not(.browser-live-dot):not(.http-status){display:none}.network-browser.is-full-page{padding:8px}.is-full-page .browser-session-row{align-items:flex-end;flex-direction:column}}
</style>
