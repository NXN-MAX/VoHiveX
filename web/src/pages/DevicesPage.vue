<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Modal, message } from 'antdv-next'
import jsQR from 'jsqr'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
import RiIcon from '@/components/RiIcon.vue'
import TrafficAnalysisPanel from '@/components/TrafficAnalysisPanel.vue'
import { apiError, http, request } from '@/api/http'
import type { Device } from '@/types/domain'
import { formatSimOperator } from '@/utils/simOperator'

const route = useRoute()
const router = useRouter()
const devices = ref<Device[]>([])
const limit = ref(0)
const selectedId = ref('')
const selected = computed(() => devices.value.find((item) => item.id === selectedId.value) || null)
const overview = ref<Record<string, any> | null>(null)
const detailHealthy = computed(() => Boolean(overview.value?.healthy ?? selected.value?.healthy))
const detailNetworkConnected = computed(() => Boolean(overview.value?.network_connected ?? selected.value?.network_connected))
const detailVowifiActive = computed(() => Boolean(overview.value?.vowifi_active ?? selected.value?.vowifi_active))
const detailOperator = computed(() => detailHealthy.value && (detailNetworkConnected.value || detailVowifiActive.value) ? String(overview.value?.operator || selected.value?.operator || '—') : '—')
const detailNetworkMode = computed(() => detailNetworkConnected.value ? String(overview.value?.network_mode || selected.value?.network_mode || '蜂窝网络') : detailVowifiActive.value ? 'Wi-Fi Calling' : '未驻网')
const detailSignal = computed(() => {
  const value = Number(overview.value?.signal_dbm ?? selected.value?.signal_dbm ?? -999)
  return detailNetworkConnected.value && Number.isFinite(value) && value !== -999 ? `${value} dBm` : '—'
})
const simOperator = computed(() => formatSimOperator([selected.value, overview.value], detailOperator.value === '—' ? '' : detailOperator.value))
const detailTrafficDisabledText = computed(() => !detailHealthy.value ? '设备离线，暂无流量分析' : '设备未连接网络，暂无流量分析')
const loading = ref(false)
const detailLoading = ref(false)
const search = ref('')
const status = ref<'all' | 'online' | 'offline'>('all')
const sortBy = ref<'name' | 'signal'>('name')
const ascending = ref(true)
const activeTab = ref(String(route.query.tab || 'overview'))
let timer = 0

const addOpen = ref(false)
const discovered = ref<Record<string, any>[]>([])
const addForm = reactive({ selected: '', name: '' })
const addBusy = ref(false)

const atCommand = ref('AT+CSQ')
const atTemplate = ref('')
const atTimeout = ref(10000)
const atHistory = ref<{ command: string; response: string; ok: boolean }[]>([])
const atBusy = ref(false)
const ussdCode = ref('')
const ussdTimeout = ref(45000)
const ussdHistory = ref<{ command: string; response: string }[]>([])
const ussdBusy = ref(false)
const ussdSession = ref('')

const deviceConfig = ref<Record<string, any>>({})
const configBusy = ref(false)
const policy = ref<Record<string, any> | null>(null)
const policyBusy = ref(false)

const esim = ref<{ chip_info?: Record<string, any>; profiles?: any[] }>({})
const esimBusy = ref(false)
const sensitive = ref(false)
const notificationsOpen = ref(false)
const notifications = ref<any[]>([])
const notificationsBusy = ref(false)
const lpaOpen = ref(false)
const lpaText = ref('')
const lpaError = ref('')
const lpaBusy = ref(false)
const fileInput = ref<HTMLInputElement>()
const downloadBusy = ref(false)
const downloadProgress = ref({ pct: 0, message: '', error: '' })
const esimForm = reactive({ smdp: '', matchingId: '', confirmationCode: '', aidHex: '', imei: '' })
const deviceTraffic = ref<{ buckets?: any[] }>({ buckets: [] })
const deviceTrafficRange = ref<'day' | 'week' | 'month'>('day')
const deviceTrafficBusy = ref(false)
const deviceTrafficError = ref('')

const atTemplates = [
  { label: '基础', options: [
    { label: '连通性 (AT)', value: 'AT' },
    { label: '模块信息 (ATI)', value: 'ATI' },
    { label: 'IMEI (AT+CGSN)', value: 'AT+CGSN' },
    { label: 'ICCID (AT+QCCID)', value: 'AT+QCCID' },
    { label: 'IMSI (AT+CIMI)', value: 'AT+CIMI' },
    { label: '信号 (AT+CSQ)', value: 'AT+CSQ' },
    { label: '运营商 (AT+COPS?)', value: 'AT+COPS?' },
    { label: '网络模式 (AT+QNWINFO)', value: 'AT+QNWINFO' },
    { label: '注册状态 (AT+CREG?)', value: 'AT+CREG?' },
    { label: '固件版本 (AT+CGMR)', value: 'AT+CGMR' },
    { label: 'APN (AT+CGDCONT?)', value: 'AT+CGDCONT?' },
    { label: '所有信号 (AT+QENG="servingcell")', value: 'AT+QENG="servingcell"' },
  ] },
  { label: '网络控制', options: [
    { label: '飞行模式 ON (AT+CFUN=0)', value: 'AT+CFUN=0' },
    { label: '飞行模式 OFF (AT+CFUN=1)', value: 'AT+CFUN=1' },
    { label: '重启模组 (AT+CFUN=1,1)', value: 'AT+CFUN=1,1' },
    { label: '附着状态 (AT+CGATT?)', value: 'AT+CGATT?' },
    { label: '脱附 (AT+CGATT=0)', value: 'AT+CGATT=0' },
    { label: '附着 (AT+CGATT=1)', value: 'AT+CGATT=1' },
  ] },
  { label: '漫游服务', options: [
    { label: '关闭漫游 (AT+QCFG="roamservice",1,1)', value: 'AT+QCFG="roamservice",1,1' },
    { label: '恢复自动 (AT+QCFG="roamservice",255,1)', value: 'AT+QCFG="roamservice",255,1' },
  ] },
  { label: 'USBNET / 模式', options: [
    { label: '查询模式 (AT+QCFG="usbnet"?)', value: 'AT+QCFG="usbnet"?' },
    { label: '设为 QMI (AT+QCFG="usbnet",0)', value: 'AT+QCFG="usbnet",0' },
    { label: '设为 ECM (AT+QCFG="usbnet",1)', value: 'AT+QCFG="usbnet",1' },
  ] },
  { label: '短信 / USSD', options: [
    { label: '列出短信 (AT+CMGL=4)', value: 'AT+CMGL=4' },
    { label: '读取短信示例 (AT+CMGR=1)', value: 'AT+CMGR=1' },
    { label: '删除所有短信 (AT+CMGD=1,4)', value: 'AT+CMGD=1,4' },
    { label: 'USSD 示例 (AT+CUSD=1,"*100#",15)', value: 'AT+CUSD=1,"*100#",15' },
  ] },
]

const configControlDevice = computed(() => String(deviceConfig.value.control_device || ''))
const fixedQmiBackend = computed(() => /^wwan\d+qmi\d+$/.test(configControlDevice.value.replace(/\\/g, '/').split('/').filter(Boolean).pop() || configControlDevice.value))
const fixedMbimBackend = computed(() => String(deviceConfig.value.device_backend || '').toLowerCase() === 'mbim')
const backendDescription = computed(() => fixedQmiBackend.value ? '此类设备固定 QMI，AT 口仅用于终端' : fixedMbimBackend.value ? '此类设备固定 MBIM，AT 口仅用于终端' : 'AT=传统串口 / QMI=纯 QMI')
const backendOptions = computed(() => fixedMbimBackend.value
  ? [{ label: 'MBIM', value: 'mbim' }]
  : [
      { label: 'AT', value: 'at', disabled: fixedQmiBackend.value },
      { label: 'QMI', value: 'qmi', disabled: !configControlDevice.value && deviceConfig.value.device_backend !== 'qmi' },
    ])

watch(fixedQmiBackend, (fixed) => { if (fixed) deviceConfig.value.device_backend = 'qmi' }, { immediate: true })

const filtered = computed(() => {
  const query = search.value.trim().toLowerCase()
  return devices.value
    .filter((device) => status.value === 'all' || (status.value === 'online' ? device.healthy : !device.healthy))
    .filter((device) => !query || [device.name, device.id, device.iccid, device.imei, device.interface, device.iface].some((value) => String(value || '').toLowerCase().includes(query)))
    .sort((a, b) => {
      const result = sortBy.value === 'signal'
        ? Number(a.signal_dbm || -999) - Number(b.signal_dbm || -999)
        : String(a.name || a.id).localeCompare(String(b.name || b.id), 'zh-CN')
      return ascending.value ? result : -result
    })
})

async function loadDevices(silent = false) {
  if (!silent) loading.value = true
  try {
    const data = await request<{ devices?: Device[]; device_limit?: number }>({ url: '/devices' })
    devices.value = data.devices || []
    limit.value = Number(data.device_limit || 0)
    const queryDevice = String(route.query.device || '')
    if (!selectedId.value && devices.value.length) selectedId.value = devices.value.some((item) => item.id === queryDevice) ? queryDevice : devices.value[0].id
  } catch (reason) {
    if (!silent) message.error(apiError(reason).message)
  } finally {
    loading.value = false
  }
}

async function loadOverview() {
  if (!selectedId.value) return
  detailLoading.value = true
  try {
    const data = await request<any>({ url: `/devices/${selectedId.value}/overview` })
    overview.value = data?.devices?.[0] || data || null
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    detailLoading.value = false
  }
}

async function runAction(label: string, config: Parameters<typeof request>[0], reload = true) {
  try {
    await request(config)
    message.success(`${label}请求已发送`)
    if (reload) await Promise.all([loadDevices(true), loadOverview()])
  } catch (reason) {
    message.error(apiError(reason).message)
  }
}

async function rescan() {
  await runAction('重新扫描', { method: 'POST', url: '/devices/actions/rescan' })
}

async function openAdd() {
  addOpen.value = true
  addBusy.value = true
  try {
    const data = await request<{ devices?: Record<string, any>[] }>({ url: '/devices/discovered', params: { with_imei: 1 } })
    discovered.value = data.devices || []
    addForm.selected = discovered.value[0]?.id || discovered.value[0]?.at_port || ''
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    addBusy.value = false
  }
}

async function addDevice() {
  const item = discovered.value.find((row) => (row.id || row.at_port) === addForm.selected)
  if (!item) return message.warning('请选择一个未配置设备')
  addBusy.value = true
  try {
    const config = { ...item, name: addForm.name.trim() || item.name || item.id }
    await request({ method: 'POST', url: '/devices', data: { config } })
    message.success('设备已添加并开始接管')
    addOpen.value = false
    await loadDevices()
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    addBusy.value = false
  }
}

async function sendAT() {
  if (!selectedId.value || !atCommand.value.trim()) return
  atBusy.value = true
  const command = atCommand.value.trim()
  try {
    const data = await request<any>({ method: 'POST', url: `/devices/${selectedId.value}/actions/at`, data: { command, cmd: command, timeout_ms: atTimeout.value } })
    atHistory.value.unshift({ command, response: String(data?.response ?? data?.result ?? JSON.stringify(data)), ok: data?.ok !== false })
  } catch (reason) {
    atHistory.value.unshift({ command, response: apiError(reason).message, ok: false })
  } finally {
    atBusy.value = false
  }
}

async function sendUSSD(continuing = false) {
  if (!selectedId.value || !ussdCode.value.trim()) return
  ussdBusy.value = true
  const command = ussdCode.value.trim()
  try {
    const data = await request<any>({ method: 'POST', url: `/devices/${selectedId.value}/actions/ussd${continuing ? '/continue' : ''}`, data: continuing ? { response: command, input: command, session_id: ussdSession.value, timeout_ms: ussdTimeout.value } : { code: command, command, timeout_ms: ussdTimeout.value }, timeout: ussdTimeout.value + 5000 })
    const result = data?.result || data
    ussdSession.value = String(result?.session_id || '')
    ussdHistory.value.unshift({ command, response: String(result?.text || result?.raw_text || JSON.stringify(result)) })
    ussdCode.value = ''
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    ussdBusy.value = false
  }
}

async function loadConfig() {
  if (!selectedId.value) return
  configBusy.value = true
  try {
    const data = await request<any>({ url: `/devices/${selectedId.value}/config` })
    deviceConfig.value = structuredClone(data?.config || data || {})
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    configBusy.value = false
  }
}

async function saveConfig() {
  try {
    await request({ method: 'PUT', url: `/devices/${selectedId.value}`, data: { config: deviceConfig.value } })
    message.success('配置已保存')
  } catch (reason) {
    message.error(apiError(reason).message)
  }
}

function deleteDevice() {
  if (!selected.value) return
  Modal.confirm({ title: '删除设备', content: `确定删除“${selected.value.name || selected.value.id}”的配置？删除后设备将停止接管。`, okText: '删除', cancelText: '取消', okButtonProps: { danger: true }, onOk: async () => { await request({ method: 'DELETE', url: `/devices/${selected.value?.id}` }); selectedId.value = ''; await loadDevices() } })
}

async function loadPolicy() {
  const iccid = String(overview.value?.iccid || selected.value?.iccid || '')
  if (!iccid) return
  policyBusy.value = true
  try {
    policy.value = await request({ url: `/cards/${encodeURIComponent(iccid)}/policy` })
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    policyBusy.value = false
  }
}

async function savePolicy() {
  const iccid = String(overview.value?.iccid || selected.value?.iccid || '')
  if (!iccid || !policy.value) return
  await runAction('卡策略保存', { method: 'PUT', url: `/cards/${encodeURIComponent(iccid)}/policy`, data: policy.value }, false)
}

async function loadEsim(refresh = false) {
  if (!selectedId.value) return
  esimBusy.value = true
  try {
    esim.value = await request({ url: `/devices/${selectedId.value}/esim`, params: refresh ? { refresh: 1 } : undefined })
    esimForm.imei = String(selected.value?.imei || '')
    esimForm.aidHex = String(esim.value.chip_info?.eids?.[0]?.aid || '')
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    esimBusy.value = false
  }
}

async function loadNotifications() {
  if (!selectedId.value) return
  notificationsOpen.value = true
  notificationsBusy.value = true
  try {
    const data = await request<any>({ url: `/devices/${selectedId.value}/esim/notifications` })
    const rows = data?.items ?? data?.notifications ?? data
    notifications.value = Array.isArray(rows) ? rows : []
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    notificationsBusy.value = false
  }
}

async function loadDeviceTraffic() {
  if (!selectedId.value) return
  if (!(overview.value?.network_connected ?? selected.value?.network_connected)) {
    deviceTraffic.value = { buckets: [] }
    return
  }
  deviceTrafficBusy.value = true
  deviceTrafficError.value = ''
  try {
    deviceTraffic.value = await request({ url: '/traffic/analysis', params: { range: deviceTrafficRange.value, device_id: selectedId.value } })
  } catch (reason) {
    deviceTraffic.value = { buckets: [] }
    deviceTrafficError.value = apiError(reason).message
  } finally {
    deviceTrafficBusy.value = false
  }
}

function changeDeviceTrafficRange(value: 'day' | 'week' | 'month') { deviceTrafficRange.value = value; loadDeviceTraffic() }

function parseLpa(value: string) {
  const clean = value.trim()
  const parts = clean.split('$')
  if (!/^LPA:1$/i.test(parts[0] || '') || !parts[1]?.trim() || parts.length > 4) throw new Error('eSIM 激活链接格式不正确')
  return { smdp: parts[1].trim(), matchingId: (parts[2] || '').trim(), confirmationCode: (parts[3] || '').trim() }
}

async function decodeImage(file?: File | null) {
  if (!file) return
  if (!['image/jpeg', 'image/png', 'image/webp'].includes(file.type) || file.size > 8 * 1024 * 1024) {
    lpaError.value = '仅支持 JPG、JPEG、PNG、WebP，最大 8 MB'
    return
  }
  lpaBusy.value = true
  lpaError.value = ''
  try {
    const bitmap = await createImageBitmap(file)
    const canvas = document.createElement('canvas')
    canvas.width = bitmap.width
    canvas.height = bitmap.height
    const context = canvas.getContext('2d', { willReadFrequently: true })!
    context.drawImage(bitmap, 0, 0)
    const image = context.getImageData(0, 0, canvas.width, canvas.height)
    const code = jsQR(image.data, image.width, image.height)
    if (!code?.data) throw new Error('二维码识别失败')
    lpaText.value = code.data
  } catch (reason) {
    lpaError.value = reason instanceof Error ? reason.message : '二维码识别失败'
  } finally {
    lpaBusy.value = false
  }
}

async function handlePaste(event: ClipboardEvent) {
  const item = Array.from(event.clipboardData?.items || []).find((entry) => entry.type.startsWith('image/'))
  if (!item) return
  event.preventDefault()
  await decodeImage(item.getAsFile())
}

function applyLpa() {
  try {
    const parsed = parseLpa(lpaText.value)
    Object.assign(esimForm, parsed)
    lpaOpen.value = false
    lpaText.value = ''
    lpaError.value = ''
    message.success('已识别并填入下载信息，请确认后点击“开始下载”')
    nextTick(() => document.getElementById('esim-smdp')?.focus())
  } catch (reason) {
    lpaError.value = reason instanceof Error ? reason.message : 'eSIM 激活链接格式不正确'
  }
}

async function downloadProfile() {
  if (!esimForm.smdp.trim()) return message.warning('请输入 SM-DP+ 地址')
  downloadBusy.value = true
  downloadProgress.value = { pct: 0, message: '正在连接…', error: '' }
  const params = new URLSearchParams({ smdp: esimForm.smdp.trim() })
  if (esimForm.matchingId.trim()) params.set('matching_id', esimForm.matchingId.trim())
  if (esimForm.confirmationCode.trim()) params.set('confirmation_code', esimForm.confirmationCode.trim())
  if (esimForm.aidHex.trim()) params.set('aid_hex', esimForm.aidHex.trim())
  if (esimForm.imei.trim()) params.set('imei', esimForm.imei.trim())
  try {
    const response = await fetch(`/api/devices/${selectedId.value}/esim/actions/download?${params}`, { headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}`, Accept: 'text/event-stream' } })
    if (!response.ok || !response.body) throw new Error((await response.text()) || `HTTP ${response.status}`)
    const reader = response.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    while (true) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split(/\r?\n/)
      buffer = lines.pop() || ''
      for (const line of lines) {
        if (!line.startsWith('data:')) continue
        const event = JSON.parse(line.slice(5).trim())
        downloadProgress.value = { pct: Number(event.pct || 0), message: String(event.msg || ''), error: event.step === 'error' ? String(event.msg || '下载失败') : '' }
        if (event.step === 'done') {
          message.success('Profile 下载成功')
          Object.assign(esimForm, { smdp: '', matchingId: '', confirmationCode: '' })
          await loadEsim(true)
        }
      }
    }
  } catch (reason) {
    downloadProgress.value.error = apiError(reason).message
  } finally {
    downloadBusy.value = false
  }
}

async function switchProfile(profile: any, group: any) {
  await runAction('Profile 切换', { method: 'POST', url: `/devices/${selectedId.value}/esim/actions/switch`, data: { iccid: profile.iccid, aid_hex: group.aid_hex || group.aid } }, false)
  await loadEsim(true)
}

watch(selectedId, async (value) => {
  if (!value) return
  router.replace({ query: { ...route.query, device: value, tab: activeTab.value } })
  overview.value = null
  await loadOverview()
  loadDeviceTraffic()
  if (activeTab.value === 'esim') loadEsim()
})

watch(activeTab, (tab) => {
  router.replace({ query: { ...route.query, device: selectedId.value, tab } })
  if (tab === 'esim') loadEsim()
  if (tab === 'config') loadConfig()
  if (tab === 'policy') loadPolicy()
  if (tab === 'overview') loadDeviceTraffic()
})

watch(() => route.query.tab, (tab) => {
  const next = String(tab || 'overview')
  if (['overview', 'esim', 'at', 'ussd', 'policy', 'config'].includes(next) && next !== activeTab.value) activeTab.value = next
})

onMounted(async () => {
  await loadDevices()
  if (selectedId.value) await loadOverview()
  timer = window.setInterval(() => loadDevices(true), 15_000)
})
onBeforeUnmount(() => window.clearInterval(timer))
</script>

<template>
  <div class="devices-page">
    <PageHeader title="设备管理" subtitle="查看设备信息、编辑配置、执行 AT 指令">
      <a-button class="ghost-button" :loading="loading" @click="loadDevices()">刷新</a-button>
      <a-button class="ghost-button" @click="rescan">重新扫描</a-button>
      <a-button type="primary" @click="openAdd"><RiIcon name="add-line" :size="17" /> 添加设备</a-button>
    </PageHeader>

    <div class="device-toolbar">
      <a-input v-model:value="search" placeholder="搜索设备 / ICCID / IMEI / 网卡" allow-clear class="device-search" />
      <div class="device-filter-controls">
        <div class="status-filter" role="radiogroup" aria-label="设备状态">
          <button v-for="item in [{ label: '全部', value: 'all' }, { label: '在线', value: 'online' }, { label: '离线', value: 'offline' }]" :key="item.value" type="button" :class="{ active: status === item.value }" @click="status = item.value as typeof status">{{ item.label }}</button>
        </div>
        <button class="text-sort" @click="sortBy = sortBy === 'name' ? 'signal' : 'name'">{{ sortBy === 'name' ? '名称' : '信号' }}</button>
        <button class="icon-button" :aria-label="ascending ? '升序，点击切换为降序' : '降序，点击切换为升序'" @click="ascending = !ascending"><RiIcon :name="ascending ? 'sort-asc' : 'sort-desc'" /></button>
        <span class="quota">配额 {{ devices.length }} / {{ limit || '—' }}</span>
      </div>
    </div>

    <div class="device-layout">
      <aside class="device-list">
        <EmptyState v-if="!filtered.length" title="暂无设备" icon="smartphone-line" />
        <button v-for="device in filtered" :key="device.id" class="device-option" :class="{ active: device.id === selectedId }" @click="selectedId = device.id">
          <span class="device-option-copy">
            <strong :title="device.name || device.id">{{ device.name || device.id }}</strong>
            <small>{{ device.id }} · {{ device.interface || device.iface || '未识别网卡' }}</small>
            <small>{{ device.operator || '未知网络' }} · {{ device.network_mode || '未驻网' }}</small>
          </span>
          <span class="pill" :class="device.healthy ? 'success' : 'danger'">{{ device.healthy ? '在线' : '离线' }}</span>
        </button>
      </aside>

      <section v-if="selected" class="device-details">
        <header class="device-details-header">
          <div>
            <h2 :title="selected.name || selected.id">{{ selected.name || selected.id }}</h2>
            <p>{{ selected.id }} · {{ selected.interface || selected.iface || '未识别网卡' }}</p>
          </div>
          <div class="row-actions">
            <a-button class="outline-button" :disabled="!selected.healthy" @click="runAction('重连 VoWiFi', { method: 'POST', url: `/devices/${selected.id}/vowifi/actions/reconnect` })">重连 VoWiFi</a-button>
            <a-button class="outline-button" @click="runAction('重启模组', { method: 'POST', url: `/devices/${selected.id}/actions/reboot` })">重启模组</a-button>
            <a-button class="outline-button" @click="router.push({ path: '/sms', query: { device: selected.id } })">短信</a-button>
          </div>
        </header>

        <a-spin :spinning="detailLoading">
          <a-tabs v-model:active-key="activeTab">
            <a-tab-pane key="overview" tab="概览">
              <div class="overview-grid overview-restored">
                <section class="white-panel info-card runtime-card">
                  <div class="card-heading"><div><h3>运行状态</h3><p>设备链路与服务状态</p></div><span class="plain-status" :class="selected.healthy ? 'online' : 'offline'"><i />{{ selected.healthy ? '在线' : '离线' }}</span></div>
                  <div class="service-summary"><span class="signal-dot" :class="detailHealthy ? 'online' : ''" /> <strong>{{ !detailHealthy ? '设备离线' : detailVowifiActive ? 'WiFi Calling · 已就绪' : detailNetworkConnected ? '蜂窝网络 · 已连接' : '设备在线 · 等待驻网' }}</strong></div>
                  <div class="stage-list">
                    <div v-for="step in [{label:'SIM',ok:Boolean(overview?.iccid || selected.iccid)},{label:'接入',ok:Boolean(overview?.network_connected ?? selected.network_connected)},{label:'隧道',ok:Boolean(overview?.tunnel_ready || overview?.vowifi_active || selected.vowifi_active)},{label:'IMS',ok:Boolean(overview?.ims_registered || overview?.vowifi_active || selected.vowifi_active)},{label:'短信',ok:Boolean(overview?.sms_ready ?? selected.healthy)}]" :key="step.label" :class="{ ready: step.ok }"><span>{{ step.ok ? '✓' : '·' }}</span><small>{{ step.label }}</small></div>
                  </div>
                  <dl><dt>数据通道</dt><dd>{{ overview?.data_plane || overview?.vowifi_status?.data_plane || (overview?.network_connected ? '蜂窝网络' : '—') }}</dd><dt>最后状态</dt><dd>{{ overview?.last_status || overview?.last_reason || '—' }}</dd><dt>错误分类</dt><dd>{{ overview?.error_category || overview?.last_error || '无' }}</dd></dl>
                </section>

                <section class="white-panel info-card">
                  <div class="card-heading"><div><h3>SIM / 设备信息</h3><p>识别信息与当前 Profile</p></div></div>
                  <dl><template v-for="(value, key) in { IMEI: overview?.modem?.imei || overview?.imei || selected.imei, ICCID: overview?.modem?.iccid || overview?.iccid || selected.iccid, IMSI: overview?.modem?.imsi || overview?.imsi || selected.imsi, 本机号码: overview?.modem?.msisdn || overview?.msisdn || selected.msisdn, 当前eSIM: overview?.active_profile_name || overview?.profile_name }" :key="key"><dt>{{ key }}</dt><dd :title="String(value || '')">{{ value || '—' }}</dd></template><dt>原运营商</dt><dd class="sim-operator" :title="simOperator.display"><span v-if="simOperator.flag" class="country-flag" aria-hidden="true">{{ simOperator.flag }}</span><span>{{ simOperator.display }}</span></dd><dt>固件</dt><dd :title="String(overview?.firmware || overview?.modem?.firmware || '')">{{ overview?.firmware || overview?.modem?.firmware || '—' }}</dd></dl>
                </section>

                <section class="white-panel info-card network-card">
                  <div class="card-heading"><div><h3>网络</h3><p>蜂窝网络、VoWiFi 与 IP 状态</p></div><a-button class="ghost-button compact" @click="runAction('切换 IP', { method: 'POST', url: '/rotateip', data: { device_id: selected?.id } })">切换 IP</a-button></div>
                  <dl><dt>运营商</dt><dd>{{ detailOperator }}</dd><dt>网络模式</dt><dd>{{ detailNetworkMode }}</dd><dt>信号</dt><dd>{{ detailSignal }}</dd><dt>内网 IPv4</dt><dd :title="detailNetworkConnected ? String(overview?.private_ip || selected.private_ip || '') : ''">{{ detailNetworkConnected ? (overview?.private_ip || selected.private_ip || '—') : '—' }}</dd><dt>内网 IPv6</dt><dd :title="detailNetworkConnected ? String(overview?.private_ipv6 || selected.private_ipv6 || '') : ''">{{ detailNetworkConnected ? (overview?.private_ipv6 || selected.private_ipv6 || '—') : '—' }}</dd><dt>公网 IP</dt><dd :title="detailNetworkConnected ? String(overview?.public_ip || overview?.public_ipv6 || selected.public_ip || selected.public_ipv6 || '') : ''">{{ detailNetworkConnected ? (overview?.public_ip || overview?.public_ipv6 || selected.public_ip || selected.public_ipv6 || '—') : '—' }}</dd></dl>
                  <div class="control-list inline-controls"><label><span>蜂窝网络</span><a-switch :checked="Boolean(overview?.network_connected ?? selected.network_connected)" @change="(checked:boolean) => runAction(checked ? '开启网络' : '关闭网络', { method: 'PATCH', url: `/devices/${selected?.id}/network`, data: { enabled: checked } })" /></label><label><span>VoWiFi</span><a-switch :checked="Boolean(overview?.vowifi_enabled ?? selected.vowifi_enabled)" @change="(checked:boolean) => runAction(checked ? '启用 VoWiFi' : '禁用 VoWiFi', { method: 'PATCH', url: `/devices/${selected?.id}/vowifi`, data: { enabled: checked } })" /></label><label><span>飞行模式</span><a-switch :checked="Boolean(overview?.flight_mode ?? selected.flight_mode)" @change="(checked:boolean) => runAction(checked ? '开启飞行模式' : '关闭飞行模式', { method: 'PATCH', url: `/devices/${selected?.id}/flight-mode`, data: { enabled: checked } })" /></label></div>
                </section>
              </div>
              <TrafficAnalysisPanel :analysis="deviceTraffic" :range="deviceTrafficRange" :loading="deviceTrafficBusy" :error="deviceTrafficError" title="当前设备流量分析" :disabled="!detailNetworkConnected" :disabled-text="detailTrafficDisabledText" @update:range="changeDeviceTrafficRange" @refresh="loadDeviceTraffic" />
            </a-tab-pane>

            <a-tab-pane key="esim" tab="eSIM">
              <div class="esim-toolbar"><div><h3>eUICC</h3><p>{{ esim.chip_info?.sku_name || (esimBusy ? '正在检测…' : '未检测到 eUICC') }} <span v-if="esim.chip_info?.firmware">· 固件 {{ esim.chip_info.firmware }}</span></p></div><div class="row-actions"><a-button class="ghost-button" :loading="esimBusy" @click="loadEsim(true)">刷新</a-button><a-button class="ghost-button" @click="loadNotifications">当前通知</a-button><a-button class="ghost-button" @click="sensitive = !sensitive">{{ sensitive ? '隐藏敏感信息' : '显示敏感信息' }}</a-button></div></div>
              <div v-if="esim.chip_info" class="euicc-meta"><div><span>EID</span><strong :class="{ blurred: !sensitive }">{{ esim.chip_info.eid || esim.chip_info.eids?.[0]?.eid || '—' }}</strong></div><div><span>制造商</span><strong>{{ esim.chip_info.manufacturer || esim.chip_info.vendor || '—' }}</strong></div><div><span>可用空间</span><strong>{{ esim.chip_info.free_space || '—' }}</strong></div></div>
              <div v-for="group in esim.profiles || []" :key="group.aid_hex || group.aid" class="euicc-group white-panel">
                <div class="euicc-heading"><strong>{{ group.name || 'eUICC' }}</strong><span>{{ group.free_space ? `可用 ${group.free_space}` : '' }}</span></div>
                <div v-for="profile in group.profiles || []" :key="profile.iccid" class="profile-row"><div><strong>{{ profile.name || profile.provider_name || 'Profile' }}</strong><small :class="{ blurred: !sensitive }">{{ profile.iccid }}</small></div><span class="plain-status" :class="profile.state === 1 ? 'online' : 'offline'"><i />{{ profile.state_text || (profile.state === 1 ? '已启用' : '已禁用') }}</span><div class="row-actions"><a-button class="ghost-button" @click="switchProfile(profile, group)">切换</a-button></div></div>
              </div>
              <section class="download-panel white-panel">
                <div class="section-title"><div><h2><RiIcon name="arrow-down-box-line" :size="20" /> 下载新 Profile</h2><p>确认激活信息后再写入 eUICC</p></div><a-button class="ghost-button" @click="lpaOpen = true">二维码/链接识别</a-button></div>
                <div class="form-grid two"><label>SM-DP+ 地址 *<a-input id="esim-smdp" v-model:value="esimForm.smdp" placeholder="例如 rsp.truphone.com" /></label><label>Matching ID<a-input v-model:value="esimForm.matchingId" placeholder="可选，部分运营商不要求" /></label><label>确认码<a-input v-model:value="esimForm.confirmationCode" placeholder="可选" /></label><label>IMEI<a-input v-model:value="esimForm.imei" placeholder="默认使用设备 IMEI，可修改" /></label></div>
                <a-progress v-if="downloadBusy || downloadProgress.message" :percent="downloadProgress.pct" :status="downloadProgress.error ? 'exception' : downloadProgress.pct >= 100 ? 'success' : 'active'" /><p v-if="downloadProgress.message" class="muted">{{ downloadProgress.error || downloadProgress.message }}</p>
                <div class="dialog-actions"><a-button type="primary" :loading="downloadBusy" @click="downloadProfile">开始下载</a-button></div>
              </section>
            </a-tab-pane>

            <a-tab-pane key="at" tab="AT 终端">
              <section class="terminal-section white-panel"><div class="section-title"><div><h2>AT 终端</h2><p>向当前模组发送 AT 指令并查看原始响应</p></div></div><div class="terminal-log"><EmptyState v-if="!atHistory.length" title="暂无 AT 会话记录" icon="terminal-box-line" /><div v-for="(item,index) in atHistory" :key="index" class="terminal-entry"><strong>&gt; {{ item.command }}</strong><pre :class="{ danger: !item.ok }">{{ item.response || '[空响应]' }}</pre></div></div><div class="terminal-composer"><label><span>快捷指令模板</span><a-select v-model:value="atTemplate" allow-clear show-search placeholder="选择常用命令（可选）" :options="atTemplates" @change="atCommand = String($event || '')" /></label><label><span>命令</span><a-input v-model:value="atCommand" placeholder="例如 AT+CSQ（可自由编辑）" :disabled="atBusy" @press-enter="sendAT" /></label><label><span>超时(ms)</span><a-input-number v-model:value="atTimeout" :min="1000" :max="120000" :step="1000" placeholder="10000" /></label><div class="terminal-actions"><a-button class="ghost-button" @click="atHistory = []">清空</a-button><a-button type="primary" :loading="atBusy" :disabled="!atCommand.trim()" @click="sendAT">发送</a-button></div></div></section>
            </a-tab-pane>

            <a-tab-pane key="ussd" tab="USSD 终端">
              <section class="terminal-section white-panel"><div class="section-title"><div><h2>USSD 终端</h2><p>发起 USSD 会话，并在会话未结束时继续回复</p></div></div><div class="terminal-log"><EmptyState v-if="!ussdHistory.length" title="暂无 USSD 会话记录" icon="chat-voice-line" /><div v-for="(item,index) in ussdHistory" :key="index" class="terminal-entry"><strong>&gt; {{ item.command }}</strong><pre>{{ item.response }}</pre></div></div><div class="terminal-composer ussd"><label><span>{{ ussdSession ? '菜单回复' : '命令 / 回复' }}</span><a-input v-model:value="ussdCode" :placeholder="ussdSession ? '输入菜单编号或回复内容' : '例如 *100#'" :disabled="ussdBusy" @press-enter="sendUSSD(Boolean(ussdSession))" /></label><label><span>超时(ms)</span><a-input-number v-model:value="ussdTimeout" :min="5000" :max="120000" :step="5000" placeholder="45000" /></label><div class="terminal-actions"><a-button class="ghost-button" @click="ussdHistory = []; ussdSession = ''">清空</a-button><a-button type="primary" :loading="ussdBusy" :disabled="!ussdCode.trim()" @click="sendUSSD(Boolean(ussdSession))">{{ ussdSession ? '回复' : '发送' }}</a-button></div></div></section>
            </a-tab-pane>

            <a-tab-pane key="policy" tab="卡策略">
              <section class="policy-section white-panel"><EmptyState v-if="!policyBusy && !policy" title="设备尚未识别到 SIM 卡 ICCID，策略不可操作" icon="sim-card-line" /><a-spin v-else :spinning="policyBusy"><template v-if="policy"><div class="section-title"><div><h2>卡策略</h2><p>ICCID {{ overview?.iccid || selected.iccid || '—' }} · 来源 {{ policy.source || '本地配置' }}</p></div></div><div class="form-grid two policy-fields"><label>IP 版本<a-select v-model:value="policy.ip_version" :options="[{label:'IPv4',value:'v4'},{label:'IPv6',value:'v6'},{label:'IPv4 + IPv6（双栈）',value:'v4v6'}]" /><small>下次开启网络时生效</small></label><label>APN（可选）<a-input v-model:value="policy.apn" placeholder="留空自动识别" /><small>下次开启网络时生效</small></label></div><div class="policy-switches"><label><div><strong>开启网络</strong><p>VoWiFi/飞行开启时不可用</p></div><a-switch v-model:checked="policy.network_enabled" /></label><label><div><strong>VoWiFi</strong><p>启用后进飞行模式，不支持国内运营商</p></div><a-switch v-model:checked="policy.vowifi_enabled" /></label><label><div><strong>飞行模式</strong><p>关闭蜂窝射频，保留可用的 WiFi Calling 链路</p></div><a-switch v-model:checked="policy.airplane_enabled" /></label></div><div class="dialog-actions"><a-button type="primary" @click="savePolicy">保存策略</a-button></div></template></a-spin></section>
            </a-tab-pane>

            <a-tab-pane key="config" tab="配置">
              <section class="config-section white-panel"><a-spin :spinning="configBusy"><div class="section-title"><div><h2>设备配置</h2><p>配置存储在数据库中，部分字段可能需要重启生效</p></div></div><div class="form-grid two"><label>设备 ID<a-input v-model:value="deviceConfig.id" disabled /></label><label>显示名称<a-input v-model:value="deviceConfig.name" placeholder="显示名称" /></label><label>IMEI 绑定<a-input v-model:value="deviceConfig.modem_imei" disabled placeholder="自动识别（添加时绑定）" /></label><label>设备路径<a-input v-model:value="deviceConfig.usb_path" disabled placeholder="由系统自动探测" /></label><label>网卡接口<a-input v-model:value="deviceConfig.interface" disabled placeholder="由系统自动探测" /></label><label>AT 端口<a-input v-model:value="deviceConfig.at_port" disabled placeholder="由系统自动探测" /></label><label>控制设备<a-input v-model:value="deviceConfig.control_device" disabled placeholder="由系统自动探测" /></label><div class="backend-mode"><div><strong>设备运行模式</strong><small>{{ backendDescription }}</small></div><a-select v-model:value="deviceConfig.device_backend" :disabled="fixedQmiBackend || fixedMbimBackend" :options="backendOptions" placeholder="AT" /></div></div><div class="dialog-actions spread"><a-button danger class="ghost-button" @click="deleteDevice">删除设备</a-button><a-button type="primary" @click="saveConfig">保存配置</a-button></div></a-spin></section>
            </a-tab-pane>
          </a-tabs>
        </a-spin>
      </section>
      <EmptyState v-else class="device-details" title="请选择设备" icon="smartphone-line" />
    </div>

    <a-modal v-model:open="addOpen" title="添加设备" :confirm-loading="addBusy" ok-text="添加设备" cancel-text="取消" @ok="addDevice">
      <a-spin :spinning="addBusy"><div class="form-stack"><label>未配置设备<a-select v-model:value="addForm.selected" style="width:100%" :options="discovered.map(item => ({ value: item.id || item.at_port, label: `${item.name || item.id || item.at_port} · ${item.imei || 'IMEI 未知'}` }))" /></label><label>显示名称（可选）<a-input v-model:value="addForm.name" placeholder="例如 ec20_3" /></label><p class="muted">系统将使用发现结果自动填充 AT 端口、USB 路径与网卡信息。</p></div></a-spin>
    </a-modal>

    <a-modal v-model:open="lpaOpen" title="二维码/链接识别" :footer="null" @cancel="lpaText = ''; lpaError = ''">
      <div class="form-stack" @paste="handlePaste"><p class="muted">可粘贴剪贴板中的二维码图片，也可以从手机相册或电脑选择图片。图片只在浏览器内识别，不上传、不缓存。</p><input ref="fileInput" type="file" accept="image/jpeg,image/png,image/webp" hidden @change="decodeImage(($event.target as HTMLInputElement).files?.[0])" /><a-button class="outline-button" :loading="lpaBusy" @click="fileInput?.click()"><RiIcon name="image-fill" :size="17" /> 选择二维码图片</a-button><label>eSIM 激活链接<a-textarea v-model:value="lpaText" :auto-size="{ minRows: 3 }" placeholder="LPA:1$SM-DP+地址$Matching ID（可空）$确认码（可选）" /></label><p v-if="lpaError" class="field-error">{{ lpaError }}</p><div class="dialog-actions"><a-button @click="lpaOpen = false">取消</a-button><a-button type="primary" :disabled="!lpaText.trim()" @click="applyLpa">识别</a-button></div></div>
    </a-modal>

    <a-modal v-model:open="notificationsOpen" title="当前通知" :footer="null">
      <a-spin :spinning="notificationsBusy">
        <EmptyState v-if="!notificationsBusy && !notifications.length" title="暂无 eSIM 通知" icon="notification-3-line" />
        <div v-else class="notification-list"><div v-for="(item,index) in notifications" :key="item.id || index"><strong>{{ item.title || item.type || `通知 ${index + 1}` }}</strong><p>{{ item.message || item.description || JSON.stringify(item) }}</p></div></div>
      </a-spin>
    </a-modal>
  </div>
</template>

<style scoped>
.devices-page{--device-list-width:280px}.device-toolbar{display:grid;grid-template-columns:var(--device-list-width) minmax(0,1fr);align-items:center;gap:16px;margin-bottom:18px}.device-search{width:100%;min-width:0}.device-filter-controls{display:flex;min-width:0;align-items:center;gap:16px;flex-wrap:wrap}.status-filter{display:flex;flex:none;align-items:center;gap:8px}.status-filter button{padding:7px 5px;border:0;background:transparent;color:var(--vx-muted);font-size:13px;font-weight:700;white-space:nowrap;cursor:pointer}.status-filter button:hover,.status-filter button.active{color:var(--vx-accent-ink)}.text-sort{flex:none;padding:6px 4px;border:0;background:transparent;color:var(--vx-ink);font-weight:650;cursor:pointer}.quota{flex:none;white-space:nowrap;color:var(--vx-muted);font-size:13px}.device-layout{display:grid;grid-template-columns:var(--device-list-width) minmax(0,1fr);gap:16px}.device-list{min-height:520px;padding:10px;border-radius:22px;background:#fff}.dark .device-list{background:var(--vx-panel)}.device-option{display:flex;width:100%;align-items:center;gap:10px;margin-bottom:6px;padding:14px;border:1px solid transparent;border-radius:15px;background:transparent;color:var(--vx-text);text-align:left;cursor:pointer}.device-option:hover{background:var(--vx-neutral)}.device-option.active{background:var(--vx-soft);border-color:transparent}.device-option-copy{display:flex;min-width:0;flex:1;flex-direction:column;gap:3px}.device-option-copy strong,.device-option-copy small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.device-option-copy strong{color:var(--vx-ink);font-size:14px}.device-option-copy small{color:var(--vx-muted);font-size:11px}.device-details{min-width:0}.device-details-header{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:16px}.device-details-header h2{max-width:520px;overflow:hidden;margin:0;color:var(--vx-ink);font-size:24px;text-overflow:ellipsis;white-space:nowrap}.device-details-header p{margin:4px 0 0;color:var(--vx-muted);font-size:12px}.overview-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px}.info-card{min-width:0}.card-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px;margin-bottom:16px}.info-card h3,.card-heading p{margin:0}.info-card h3{color:var(--vx-ink);font-size:17px}.card-heading p{margin-top:4px;color:var(--vx-muted);font-size:11px}.info-card dl{display:grid;grid-template-columns:92px minmax(0,1fr);gap:10px 16px;margin:0;font-size:12px}.info-card dt{color:var(--vx-muted)}.info-card dd{min-width:0;overflow:hidden;margin:0;color:var(--vx-ink);font-weight:650;text-overflow:ellipsis;white-space:nowrap}.sim-operator{display:flex;align-items:center;gap:6px}.sim-operator span:last-child{min-width:0;overflow:hidden;text-overflow:ellipsis}.country-flag{flex:none;font-size:15px;line-height:1}.plain-status{display:inline-flex;align-items:center;gap:6px;color:var(--vx-muted);font-size:12px;font-weight:700;white-space:nowrap}.plain-status i{width:7px;height:7px;border-radius:50%;background:#aaa}.plain-status.online{color:#237a3b}.plain-status.online i{background:#37b24d}.plain-status.offline i{background:#a6a6a2}.service-summary{display:flex;align-items:center;gap:8px;margin:2px 0 18px;padding:12px 14px;border-radius:14px;background:var(--vx-neutral);color:var(--vx-ink);font-size:13px}.signal-dot{width:8px;height:8px;border-radius:50%;background:#aaa}.signal-dot.online{background:#37b24d}.stage-list{display:grid;grid-template-columns:repeat(5,1fr);gap:5px;margin-bottom:18px}.stage-list div{display:flex;min-width:0;align-items:center;flex-direction:column;gap:4px;color:var(--vx-muted)}.stage-list span{display:grid;width:23px;height:23px;place-items:center;border-radius:50%;background:var(--vx-neutral);font-size:11px}.stage-list .ready span{background:var(--vx-accent);color:#123f0a}.stage-list small{font-size:10px}.control-list{display:grid;gap:12px}.control-list label{display:flex;align-items:center;justify-content:space-between;gap:8px}.inline-controls{grid-template-columns:repeat(3,minmax(0,1fr));margin-top:18px;padding-top:14px;border-top:1px solid color-mix(in srgb,var(--vx-line) 55%,transparent)}.inline-controls label{flex-direction:column;color:var(--vx-muted);font-size:11px}.compact{padding-inline:13px!important}.esim-toolbar,.euicc-heading,.profile-row{display:flex;align-items:center;justify-content:space-between;gap:16px}.esim-toolbar{margin-bottom:16px}.esim-toolbar h3,.esim-toolbar p{margin:0}.esim-toolbar h3{color:var(--vx-ink);font-size:20px}.esim-toolbar p,.euicc-heading span{color:var(--vx-muted);font-size:12px}.euicc-meta{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px;margin-bottom:16px}.euicc-meta>div{display:flex;min-width:0;flex-direction:column;gap:5px;padding:14px 16px;border-radius:15px;background:var(--vx-neutral)}.euicc-meta span{color:var(--vx-muted);font-size:11px}.euicc-meta strong{overflow:hidden;color:var(--vx-ink);font-size:13px;text-overflow:ellipsis;white-space:nowrap}.euicc-group{margin-bottom:16px}.profile-row{padding:14px 0;border-top:1px solid color-mix(in srgb,var(--vx-line) 55%,transparent)}.profile-row>div:first-child{display:flex;min-width:0;flex:1;flex-direction:column}.profile-row small{color:var(--vx-muted)}.blurred{filter:blur(5px);user-select:none}.download-panel{margin-top:16px}.section-title{display:flex;align-items:flex-start;justify-content:space-between;gap:16px;margin-bottom:18px}.section-title h2{display:flex;align-items:center;gap:8px;margin:0;color:var(--vx-ink);font-size:18px}.section-title p{margin:5px 0 0;color:var(--vx-muted);font-size:12px}.form-grid{display:grid;gap:16px}.form-grid.two{grid-template-columns:repeat(2,minmax(0,1fr))}.form-grid label,.form-stack label{display:grid;gap:7px;color:var(--vx-muted);font-size:13px;font-weight:650}.form-grid label>small{color:var(--vx-muted);font-size:11px;font-weight:400}.backend-mode{grid-column:1/-1;display:flex;align-items:center;justify-content:space-between;gap:16px;padding:14px 16px;border-radius:16px;background:var(--vx-neutral)}.backend-mode>div{display:flex;min-width:0;flex-direction:column;gap:4px}.backend-mode strong{color:var(--vx-ink);font-size:14px}.backend-mode small{color:var(--vx-muted);font-size:12px}.backend-mode :deep(.ant-select){width:120px;flex:none}.terminal-section,.policy-section,.config-section{padding:18px}.terminal-log{height:390px;overflow:auto;padding:14px;border-radius:14px;background:#161719;color:#e8e8e4}.terminal-log :deep(.empty-state),.terminal-log :deep(.empty-state strong),.terminal-log :deep(.empty-state p),.terminal-log :deep(.empty-state .ri-icon){color:#c8ccc5}.terminal-entry{padding:10px 0;border-bottom:1px solid #ffffff1c}.terminal-entry strong{color:#9fe870;font-family:ui-monospace,monospace;font-size:12px}.terminal-entry pre{margin:8px 0 0;overflow:auto;color:#eee;font:12px/1.55 ui-monospace,monospace;white-space:pre-wrap}.terminal-entry pre.danger{color:#ff9090}.terminal-composer{display:grid;grid-template-columns:210px minmax(0,1fr) 170px auto;align-items:end;gap:16px;margin-top:16px}.terminal-composer.ussd{grid-template-columns:minmax(0,1fr) 170px auto}.terminal-composer>label{display:grid;min-width:0;gap:7px;color:var(--vx-muted);font-size:11px;font-weight:700}.terminal-composer>label :deep(.ant-input-number){width:100%}.terminal-actions{display:flex;align-items:center;gap:8px}.policy-fields{margin-bottom:20px}.policy-switches{display:grid;gap:2px}.policy-switches label{display:flex;align-items:center;justify-content:space-between;gap:18px;padding:14px 4px}.policy-switches strong{color:var(--vx-ink);font-size:14px}.policy-switches p{margin:4px 0 0;color:var(--vx-muted);font-size:12px}.dialog-actions.spread{justify-content:space-between}.notification-list{display:grid;max-height:420px;gap:10px;overflow:auto}.notification-list>div{padding:14px;border-radius:14px;background:var(--vx-neutral)}.notification-list strong{color:var(--vx-ink)}.notification-list p{margin:5px 0 0;color:var(--vx-muted);font-size:12px;white-space:pre-wrap}.form-stack{display:grid;gap:16px}
@media(max-width:1220px){.device-details-header{align-items:flex-start;flex-direction:column}.device-details-header .row-actions{justify-content:flex-start}.overview-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.network-card{grid-column:1/-1}}
@media(max-width:1050px){.device-toolbar,.device-layout{grid-template-columns:1fr}.device-list{display:grid;grid-template-columns:repeat(auto-fill,minmax(230px,1fr));min-height:0}.terminal-composer,.terminal-composer.ussd{grid-template-columns:1fr 150px auto auto}.terminal-composer .ant-select{grid-column:1/-1}}
@media(max-width:700px){.device-filter-controls{align-items:flex-start}.device-details-header{align-items:flex-start;flex-direction:column}.overview-grid{grid-template-columns:1fr}.network-card{grid-column:auto}.form-grid.two,.euicc-meta{grid-template-columns:1fr}.profile-row,.esim-toolbar,.section-title{align-items:flex-start;flex-wrap:wrap}.terminal-composer,.terminal-composer.ussd{grid-template-columns:1fr}.terminal-composer>*{grid-column:auto!important}.inline-controls{grid-template-columns:repeat(3,1fr)}}
</style>
