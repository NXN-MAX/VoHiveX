<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Modal, message } from 'antdv-next'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
import RiIcon from '@/components/RiIcon.vue'
import { apiError, http, request } from '@/api/http'
import { getCountries, getCountryCallingCode, type CountryCode } from 'libphonenumber-js/min'
import { useCallAudio } from '@/utils/callAudio'

interface CallDevice { id: string; name: string; running?: boolean; healthy?: boolean; local_phone?: string; msisdn?: string; modem?: { local_phone?: string; msisdn?: string } }
interface CallItem { id: string; device_id: string; device_name: string; index: number; direction: 'incoming' | 'outgoing'; state: string; number: string; contact_name?: string; started_at: number; connected_at?: number; ended_at?: number; duration_seconds: number; missed?: boolean; recording_id?: string }
interface Contact { id: string; name: string; number: string; note?: string; favorite: boolean; created_at: number; updated_at: number }
interface Recording { id: string; call_id: string; device_id: string; number: string; direction: string; started_at: number; ended_at: number; duration_seconds: number; mime_type: string; size: number; audio_path: string }
interface AudioState { available: boolean; detail: string; device?: string; active: boolean; id?: string; call_id?: string; started_at?: number; browser_supported?: boolean; ready?: boolean; restore_required?: boolean; preparing?: boolean }
type CallTransport = 'cellular' | 'vowifi' | 'unknown'
interface CallRoute { transport: CallTransport; device_transport: CallTransport; call_control_available: boolean; call_control_detail: string }
const emptyRoute = (): CallRoute => ({ transport: 'unknown', device_transport: 'unknown', call_control_available: false, call_control_detail: '正在确认设备电话模式' })

const devices = ref<CallDevice[]>([])
const selectedDevice = ref('')
const calls = ref<CallItem[]>([])
const contacts = ref<Contact[]>([])
const history = ref<CallItem[]>([])
const recordings = ref<Recording[]>([])
const audio = ref<AudioState>({ available: false, detail: '正在检查录音设备', active: false })
const browserAudio = useCallAudio()
const listenOnly = ref(false)
const audioSetupBusy = ref(false)
async function prepareAudio(restore = false) {
  audioSetupBusy.value = true
  try {
    if (restore) browserAudio.disconnect()
    await request({ method: 'POST', url: `/calls/audio/${restore ? 'restore' : 'prepare'}`, data: { device_id: selectedDevice.value }, timeout: 190_000 })
    await loadStatus(true)
  } catch (reason) { message.error(apiError(reason).message) } finally { audioSetupBusy.value = false }
}
async function connectAudio() {
  if (browserAudio.connected.value) { browserAudio.disconnect(); return }
  try { await browserAudio.connect(selectedDevice.value, listenOnly.value); await loadStatus(true) }
  catch (reason) { message.error(apiError(reason).message) }
}
const number = ref('')
const callingCountry = ref<CountryCode>('CN')
const loading = ref(false)
const statusBusy = ref(false)
const controlBusy = ref(false)
const statusError = ref('')
const statusReady = ref(false)
const route = ref<CallRoute>(emptyRoute())
const transportLabel = computed(() => ({ cellular: '蜂窝', vowifi: 'VoWiFi', unknown: '未知通道' })[route.value.transport])
const activeTab = ref('contacts')
const contactOpen = ref(false)
const editingContact = ref<Contact | null>(null)
const contactForm = reactive({ name: '', number: '', note: '', favorite: false })
const contactBusy = ref(false)
const playingID = ref('')
const playbackURL = ref('')
let pollTimer = 0
let clockTimer = 0
let callFingerprint = 'uninitialized'
const clock = ref(Date.now())

const regionNames = new Intl.DisplayNames(['zh-CN'], { type: 'region' })
function regionFlag(country: CountryCode) {
  return [...country].map((letter) => String.fromCodePoint(127397 + letter.charCodeAt(0))).join('')
}
function regionName(country: CountryCode) {
  if (country === 'CN') return '中国大陆'
  return regionNames.of(country) || country
}
const callingCodes = getCountries()
  .map((country) => {
    const dialCode = `+${getCountryCallingCode(country)}`
    const flag = regionFlag(country)
    const countryName = regionName(country)
    return { value: country, country: countryName, dialCode, flag, label: `${flag} ${dialCode} ${countryName}` }
  })
  .sort((left, right) => {
    const priority = (country: CountryCode) => country === 'CN' ? 0 : country === 'US' ? 1 : 2
    return priority(left.value) - priority(right.value) || left.country.localeCompare(right.country, 'zh-CN')
  })

const currentCall = computed(() => calls.value[0] || null)
const selected = computed(() => devices.value.find((item) => item.id === selectedDevice.value))
const deviceOptions = computed(() => devices.value.map((item) => ({
  value: item.id,
  label: `${item.name} · ${item.running ? '在线' : '离线'}`,
})))
const selectedPhone = computed(() => String(
  selected.value?.local_phone
  || selected.value?.msisdn
  || selected.value?.modem?.local_phone
  || selected.value?.modem?.msisdn
  || '',
).trim())
const selectedCallingCode = computed(() => callingCodes.find((item) => item.value === callingCountry.value)?.dialCode || '+86')
const isIncoming = computed(() => ['incoming', 'waiting'].includes(currentCall.value?.state || ''))
const isConnected = computed(() => currentCall.value?.state === 'active')
const currentTitle = computed(() => currentCall.value?.contact_name || currentCall.value?.number || '未知号码')
const elapsed = computed(() => {
  const call = currentCall.value
  if (!call) return 0
  const start = Number(call.connected_at || call.started_at || 0) * 1000
  return Math.max(0, Math.floor((clock.value - start) / 1000))
})

const keypad = [
  ['1', ''], ['2', 'ABC'], ['3', 'DEF'],
  ['4', 'GHI'], ['5', 'JKL'], ['6', 'MNO'],
  ['7', 'PQRS'], ['8', 'TUV'], ['9', 'WXYZ'],
  ['*', ''], ['0', '+'], ['#', ''],
]

function formatTime(value?: number) {
  if (!value) return '—'
  return new Date(value * 1000).toLocaleString('zh-CN', { hour12: false })
}
function formatDuration(value: number) {
  const seconds = Math.max(0, Math.floor(value || 0))
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const rest = seconds % 60
  return hours ? `${hours}:${String(minutes).padStart(2, '0')}:${String(rest).padStart(2, '0')}` : `${String(minutes).padStart(2, '0')}:${String(rest).padStart(2, '0')}`
}
function stateText(state: string) {
  return ({ active: '通话中', held: '已保持', dialing: '正在拨号', alerting: '网络回铃，尚未接通', incoming: '来电', waiting: '呼叫等待', ended: '已结束', failed: '未接通' } as Record<string, string>)[state] || state
}
function directionText(item: CallItem) {
  if (item.missed) return '未接来电'
  return item.direction === 'incoming' ? '呼入' : '呼出'
}
function appendDigit(digit: string) {
  if (currentCall.value) {
    if (isConnected.value) void sendDTMF(digit)
    return
  }
  if (number.value.length >= 24) return
  if (digit === '+' && number.value) return
  number.value += digit
}
function backspace() { number.value = number.value.slice(0, -1) }
function callingCodeLabel(value: unknown) {
  const option = callingCodes.find((item) => item.value === String(value || ''))
  return option ? `${option.flag} ${option.dialCode}` : String(value || '')
}
function setDialNumber(value: string) {
  const compact = String(value || '').trim().replace(/[\s().-]+/g, '')
  const match = callingCodes
    .slice()
    .sort((left, right) => right.dialCode.length - left.dialCode.length)
    .find((item) => compact.startsWith(item.dialCode))
  if (match) {
    callingCountry.value = match.value
    number.value = compact.slice(match.dialCode.length)
    return
  }
  number.value = compact
}

async function loadStatus(silent = false) {
  if (!selectedDevice.value || statusBusy.value) return
  const deviceID = selectedDevice.value
  statusBusy.value = true
  try {
    const data = await request<any>({ url: '/calls/status', params: { device_id: deviceID }, timeout: 15_000 })
    if (selectedDevice.value !== deviceID) return
    const nextCalls = data.calls || []
    const nextFingerprint = nextCalls.map((item: CallItem) => `${item.id}:${item.state}:${item.recording_id || ''}`).join('|') || 'idle'
    const shouldRefreshLists = callFingerprint !== 'uninitialized' && callFingerprint !== nextFingerprint
    callFingerprint = nextFingerprint
    calls.value = nextCalls
    audio.value = data.audio || audio.value
    route.value = {
      transport: ['cellular', 'vowifi'].includes(data.transport) ? data.transport : 'unknown',
      device_transport: ['cellular', 'vowifi'].includes(data.device_transport) ? data.device_transport : 'unknown',
      call_control_available: data.call_control_available === true,
      call_control_detail: data.call_control_detail || '无法确认设备电话模式',
    }
    statusError.value = ''
    statusReady.value = route.value.call_control_available
    if (shouldRefreshLists) void loadLists().catch(() => undefined)
  } catch (reason) {
    if (selectedDevice.value !== deviceID) return
    const error = apiError(reason)
    statusError.value = error.message
    statusReady.value = false
    if (!silent) message.error(error.message)
  } finally {
    statusBusy.value = false
    if (selectedDevice.value !== deviceID) void loadStatus(true)
  }
}

async function loadLists() {
  const [contactData, historyData, recordingData] = await Promise.all([
    request<any>({ url: '/calls/contacts' }),
    request<any>({ url: '/calls/history', params: { device_id: 'all', limit: 200 } }),
    request<any>({ url: '/calls/recordings' }),
  ])
  contacts.value = contactData.items || []
  history.value = historyData.items || []
  recordings.value = recordingData.items || []
  // The selected call route determines whether audio is available. A global
  // ALSA inventory must not overwrite the VoWiFi media capability.
  if (!selectedDevice.value) audio.value = recordingData.state || audio.value
}

async function load() {
  loading.value = true
  try {
    const data = await request<any>({ url: '/calls/devices' })
    devices.value = data.devices || []
    audio.value = data.audio || audio.value
    if (!devices.value.some((item) => item.id === selectedDevice.value)) selectedDevice.value = devices.value.find((item) => item.running)?.id || devices.value[0]?.id || ''
    await Promise.all([loadLists(), loadStatus(true)])
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    loading.value = false
  }
}

async function control(action: 'dial' | 'answer' | 'hangup' | 'reject', extra: Record<string, unknown> = {}) {
  if (!selectedDevice.value || controlBusy.value || audioSetupBusy.value || audio.value.preparing) return
  const deviceID = selectedDevice.value
  controlBusy.value = true
  try {
    const result = await request<{ call?: CallItem; pending_confirmation?: boolean }>({ method: 'POST', url: `/calls/${action}`, data: { ...extra, device_id: deviceID, call_id: currentCall.value?.id || '', transport: action === 'dial' ? 'auto' : route.value.transport }, timeout: 20_000 })
    if (selectedDevice.value !== deviceID) return
    if (action === 'dial') {
      number.value = ''
      if (result.call) calls.value = [result.call]
      if (result.pending_confirmation) message.info('拨号结果正在确认，请勿重复拨号')
    }
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    controlBusy.value = false
    void Promise.all([loadStatus(true), loadLists()]).catch(() => undefined)
  }
}
function dial(value = number.value) {
  if (!selected.value?.running || !statusReady.value || currentCall.value) return
  const compact = String(value || '').trim().replace(/[\s().-]+/g, '')
  if (!compact) return message.warning('请输入电话号码')
  const normalized = compact.startsWith('+') ? compact : `${selectedCallingCode.value}${compact}`
  if (!/^\+?\d{3,24}$/.test(normalized)) return message.warning('请输入 3–24 位有效电话号码')
  void control('dial', { number: normalized })
}
async function sendDTMF(digits: string) {
  if (!isConnected.value || controlBusy.value) return
  try {
    await request({ method: 'POST', url: '/calls/dtmf', data: { device_id: selectedDevice.value, call_id: currentCall.value?.id, transport: route.value.transport, digits } })
  } catch (reason) { message.error(apiError(reason).message) }
}

function openContact(item?: Contact) {
  editingContact.value = item || null
  Object.assign(contactForm, { name: item?.name || '', number: item?.number || number.value, note: item?.note || '', favorite: Boolean(item?.favorite) })
  contactOpen.value = true
}
async function saveContact() {
  contactBusy.value = true
  try {
    await request({ method: editingContact.value ? 'PUT' : 'POST', url: editingContact.value ? `/calls/contacts/${editingContact.value.id}` : '/calls/contacts', data: contactForm })
    contactOpen.value = false
    await loadLists()
  } catch (reason) { message.error(apiError(reason).message) } finally { contactBusy.value = false }
}
function deleteContact(item: Contact) {
  Modal.confirm({ title: '删除联系人', content: `确定从通讯录删除“${item.name}”吗？`, okText: '删除', cancelText: '取消', okButtonProps: { danger: true }, onOk: async () => { await request({ method: 'DELETE', url: `/calls/contacts/${item.id}` }); await loadLists() } })
}
function useContact(item: Contact) { setDialNumber(item.number); if (!currentCall.value) dial(item.number) }
function deleteHistory(item: CallItem) {
  Modal.confirm({ title: '删除通话记录', content: '确定删除这条通话记录吗？', okText: '删除', cancelText: '取消', okButtonProps: { danger: true }, onOk: async () => { await request({ method: 'DELETE', url: `/calls/history/${item.id}` }); await loadLists() } })
}

async function toggleRecording() {
  if (controlBusy.value) return
  controlBusy.value = true
  try {
    if (audio.value.active) {
      await request({ method: 'POST', url: '/calls/recordings/stop', data: {} })
      message.success('通话录音已保存')
    } else if (currentCall.value) {
      await request({ method: 'POST', url: '/calls/recordings/start', data: { call_id: currentCall.value.id } })
    }
    await Promise.all([loadStatus(true), loadLists()])
  } catch (reason) { message.error(apiError(reason).message) } finally { controlBusy.value = false }
}
async function playRecording(item: Recording) {
  try {
    if (playbackURL.value) URL.revokeObjectURL(playbackURL.value)
    const path = item.audio_path.replace(/^\/api/, '')
    const response = await http.get(path, { responseType: 'blob' })
    playbackURL.value = URL.createObjectURL(response.data)
    playingID.value = item.id
  } catch (reason) { message.error(apiError(reason).message) }
}
function deleteRecording(item: Recording) {
  Modal.confirm({ title: '删除录音', content: '删除后无法恢复，是否继续？', okText: '删除', cancelText: '取消', okButtonProps: { danger: true }, onOk: async () => { await request({ method: 'DELETE', url: `/calls/recordings/${item.id}` }); if (playingID.value === item.id) { playingID.value = ''; if (playbackURL.value) URL.revokeObjectURL(playbackURL.value); playbackURL.value = '' } await loadLists() } })
}

function onDeviceChange() { browserAudio.disconnect(); calls.value = []; route.value = emptyRoute(); statusError.value = ''; statusReady.value = false; callFingerprint = 'uninitialized'; void loadStatus() }
onMounted(() => {
  void load()
  pollTimer = window.setInterval(() => { if (!document.hidden) void loadStatus(true) }, 2500)
  clockTimer = window.setInterval(() => { clock.value = Date.now() }, 1000)
})
onBeforeUnmount(() => {
  browserAudio.disconnect()
  window.clearInterval(pollTimer); window.clearInterval(clockTimer)
  if (playbackURL.value) URL.revokeObjectURL(playbackURL.value)
})
</script>

<template>
  <div class="calls-page">
    <PageHeader title="电话" subtitle="拨号、接听、通讯录、通话记录与录音">
      <template #actions>
        <a-select v-model:value="selectedDevice" class="device-select" placeholder="选择电话设备" :options="deviceOptions" @change="onDeviceChange" />
        <a-button class="ghost-button" :loading="loading || statusBusy" @click="load">刷新</a-button>
      </template>
    </PageHeader>

    <section v-if="audio.browser_supported" class="browser-audio-panel white-panel">
      <div><strong>网页通话音频</strong><p>{{ browserAudio.error.value || audio.detail }}</p></div>
      <div class="browser-audio-controls">
        <a-checkbox v-model:checked="listenOnly" :disabled="browserAudio.connected.value || browserAudio.busy.value">仅收听</a-checkbox>
        <a-button v-if="!audio.ready" class="outline-button" :loading="audioSetupBusy" :disabled="Boolean(currentCall)" @click="prepareAudio()">准备音频</a-button>
        <a-button :type="browserAudio.connected.value ? 'default' : 'primary'" :loading="browserAudio.busy.value" :disabled="!audio.ready || audioSetupBusy" @click="connectAudio">{{ browserAudio.connected.value ? '断开网页音频' : '连接网页音频' }}</a-button>
        <a-button v-if="browserAudio.connected.value && !listenOnly" class="outline-button" @click="browserAudio.setMuted(!browserAudio.muted.value)">{{ browserAudio.muted.value ? '取消静音' : '麦克风静音' }}</a-button>
        <a-button v-if="audio.restore_required" class="ghost-button" :disabled="Boolean(currentCall) || browserAudio.connected.value" :loading="audioSetupBusy" @click="prepareAudio(true)">恢复接口</a-button>
      </div>
      <div v-if="browserAudio.connected.value" class="browser-audio-meters"><label>麦克风 <meter min="0" max="1" :value="browserAudio.inputLevel.value" /></label><label>对方声音 <meter min="0" max="1" :value="browserAudio.outputLevel.value" /></label><span>断开网页音频不会挂断电话</span></div>
    </section>

    <a-spin :spinning="loading">
      <section class="phone-grid">
        <article class="dialer-panel white-panel">
          <div class="section-title"><div><h2>拨号盘</h2><p>{{ selectedPhone ? `本机号码 ${selectedPhone}` : selected?.name || '请选择设备' }}</p></div></div>
          <div class="number-entry">
            <div class="number-entry-control">
              <a-select v-model:value="callingCountry" class="calling-code-select" :options="callingCodes" :disabled="Boolean(currentCall)" show-search option-filter-prop="label" :popup-match-select-width="340" aria-label="国家或地区区号">
                <template #labelRender="{ value }">{{ callingCodeLabel(value) }}</template>
              </a-select>
              <a-input v-model:value="number" inputmode="tel" :maxlength="24" placeholder="输入电话号码" :disabled="Boolean(currentCall)" @press-enter="dial()" />
            </div>
            <button v-if="number && !currentCall" type="button" class="erase-button" aria-label="删除一位" @click="backspace"><RiIcon name="arrow-left-line" :size="18" /></button>
          </div>
          <div class="keypad" aria-label="电话拨号盘">
            <button v-for="key in keypad" :key="key[0]" type="button" :disabled="Boolean(currentCall) && !isConnected" @click="appendDigit(key[0])"><strong>{{ key[0] }}</strong><small>{{ key[1] }}</small></button>
          </div>
          <a-button v-if="!currentCall" type="primary" size="large" block :loading="controlBusy" :disabled="!selected?.running || !statusReady || audioSetupBusy || audio.preparing || !number.trim()" @click="dial()"><RiIcon name="phone-line" :size="18" />拨打</a-button>
          <div v-else class="call-actions">
            <a-button v-if="isIncoming" type="primary" size="large" :loading="controlBusy" @click="control('answer')">接听</a-button>
            <a-button danger type="primary" size="large" :loading="controlBusy" @click="control(isIncoming ? 'reject' : 'hangup')">{{ isIncoming ? '拒接' : '挂断' }}</a-button>
          </div>
        </article>

        <article class="call-status-panel section-panel" :class="{ active: currentCall }">
          <div v-if="currentCall" class="live-call">
            <span class="call-direction">{{ transportLabel }} · {{ currentCall.direction === 'incoming' ? '来电' : '呼出' }}</span>
            <h2>{{ currentTitle }}</h2>
            <p>{{ stateText(currentCall.state) }}<template v-if="isConnected"> · {{ formatDuration(elapsed) }}</template></p>
            <div class="live-call-actions">
              <a-button class="outline-button" :disabled="!isConnected || (!audio.available && !audio.active)" :loading="controlBusy" @click="toggleRecording">
                <span class="record-dot" :class="{ recording: audio.active }" />{{ audio.active ? '停止并保存录音' : '开始录音' }}
              </a-button>
              <a-button v-if="!isIncoming" danger type="primary" @click="control('hangup')">挂断</a-button>
            </div>
          </div>
          <div v-else class="idle-call">
            <RiIcon name="phone-line" :size="38" />
            <h2>{{ route.call_control_available ? '等待拨号或来电' : `${transportLabel}电话暂不可用` }}</h2>
            <p>跟随设备 · {{ transportLabel }}通道 · 每 2.5 秒自动刷新</p>
          </div>
          <div class="capability-list">
            <div><span class="status-dot" :class="{ online: selected?.running && statusReady }" /><span>电话控制</span><strong>{{ !selected?.running ? '设备离线' : statusReady ? `${transportLabel}通道` : '暂不可用' }}</strong></div>
            <div><span class="status-dot" :class="{ online: audio.available }" /><span>通话录音</span><strong>{{ audio.available ? (audio.browser_supported ? '网页音频已就绪' : 'ALSA 已就绪') : '暂不可用' }}</strong></div>
          </div>
          <p class="audio-detail">{{ route.call_control_detail }}</p>
          <p class="audio-detail">{{ audio.detail }}</p>
          <p v-if="statusError" class="status-error">{{ statusError }}</p>
        </article>
      </section>

      <section class="call-library">
        <a-tabs v-model:active-key="activeTab">
          <a-tab-pane key="contacts" :tab="`通讯录 ${contacts.length}`">
            <div class="library-toolbar"><div><h2>通讯录</h2><p>保存常用号码并快速拨打</p></div><a-button type="primary" @click="openContact()">新增联系人</a-button></div>
            <EmptyState v-if="!contacts.length" title="通讯录为空" description="新增联系人后可在这里快速拨号。" icon="user-line" />
            <div v-else class="contact-grid">
              <article v-for="item in contacts" :key="item.id" class="contact-card">
                <button class="contact-main" type="button" @click="useContact(item)"><span class="contact-avatar">{{ item.name.slice(0, 1).toUpperCase() }}</span><span><strong>{{ item.name }}</strong><small>{{ item.number }}</small></span></button>
                <div class="row-actions"><a-button class="ghost-button" @click="openContact(item)">编辑</a-button><a-button class="ghost-button danger" @click="deleteContact(item)">删除</a-button></div>
              </article>
            </div>
          </a-tab-pane>
          <a-tab-pane key="history" :tab="`通话记录 ${history.length}`">
            <div class="library-toolbar"><div><h2>通话记录</h2><p>呼入、呼出、未接与通话时长</p></div></div>
            <EmptyState v-if="!history.length" title="暂无通话记录" icon="phone-line" />
            <div v-else class="history-list">
              <article v-for="item in history" :key="item.id">
                <button class="history-main" type="button" @click="setDialNumber(item.number); activeTab = 'contacts'">
                  <span class="history-icon" :class="{ missed: item.missed || item.state === 'failed' }"><RiIcon :name="item.direction === 'incoming' ? 'arrow-left-down-line' : 'arrow-right-up-line'" :size="17" /></span>
                  <span><strong>{{ item.contact_name || item.number || '未知号码' }}</strong><small>{{ directionText(item) }} · {{ stateText(item.state) }}<template v-if="item.duration_seconds"> · {{ formatDuration(item.duration_seconds) }}</template></small></span>
                </button>
                <time>{{ formatTime(item.started_at) }}</time>
                <a-button v-if="item.ended_at && !item.recording_id" class="ghost-button danger" @click="deleteHistory(item)">删除</a-button>
              </article>
            </div>
          </a-tab-pane>
          <a-tab-pane key="recordings" :tab="`录音 ${recordings.length}`">
            <div class="library-toolbar"><div><h2>通话录音</h2><p>录音存储在 VoHiveX 数据目录中</p></div></div>
            <EmptyState v-if="!recordings.length" title="暂无通话录音" :description="audio.available ? '电话接通后可手动开始录音。' : audio.detail" icon="chat-voice-line" />
            <div v-else class="recording-list">
              <article v-for="item in recordings" :key="item.id">
                <div><strong>{{ item.number || '未知号码' }}</strong><small>{{ item.direction === 'incoming' ? '呼入' : '呼出' }} · {{ formatTime(item.started_at) }} · {{ formatDuration(item.duration_seconds) }}</small></div>
                <div class="row-actions"><a-button class="ghost-button" :loading="playingID === item.id && !playbackURL" @click="playRecording(item)">播放</a-button><a-button class="ghost-button danger" @click="deleteRecording(item)">删除</a-button></div>
                <audio v-if="playingID === item.id && playbackURL" :src="playbackURL" controls autoplay />
              </article>
            </div>
          </a-tab-pane>
        </a-tabs>
      </section>
    </a-spin>

    <a-modal v-model:open="contactOpen" :title="editingContact ? '编辑联系人' : '新增联系人'" :confirm-loading="contactBusy" ok-text="保存" cancel-text="取消" @ok="saveContact">
      <a-form layout="vertical">
        <a-form-item label="姓名" required><a-input v-model:value="contactForm.name" :maxlength="80" placeholder="联系人姓名" /></a-form-item>
        <a-form-item label="电话号码" required><a-input v-model:value="contactForm.number" :maxlength="24" inputmode="tel" placeholder="例如 +12025550123" /></a-form-item>
        <a-form-item label="备注"><a-textarea v-model:value="contactForm.note" :maxlength="240" :rows="3" placeholder="可选" /></a-form-item>
        <a-checkbox v-model:checked="contactForm.favorite">设为常用联系人</a-checkbox>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.browser-audio-panel{padding:16px;margin-bottom:16px;display:flex;align-items:center;justify-content:space-between;gap:16px;flex-wrap:wrap}.browser-audio-panel p{margin:6px 0 0;color:var(--vx-muted);font-size:13px}.browser-audio-controls,.browser-audio-meters{display:flex;align-items:center;gap:12px;flex-wrap:wrap}.browser-audio-meters{width:100%;font-size:12px}.browser-audio-meters label{display:flex;align-items:center;gap:8px}.browser-audio-meters meter{width:100px;height:12px}.browser-audio-meters span{color:var(--vx-muted)}
.calls-page{width:100%;min-width:0}.device-select{width:min(340px,42vw)}.phone-grid{display:grid;grid-template-columns:minmax(320px,420px) minmax(0,1fr);gap:16px;margin-bottom:24px}.dialer-panel{padding:24px}.number-entry{position:relative;margin-bottom:18px}.number-entry-control{display:grid;grid-template-columns:100px minmax(0,1fr);overflow:hidden;border:2px solid var(--vx-line);border-radius:999px;background:var(--vx-canvas);transition:border-color .15s ease,box-shadow .15s ease}.number-entry-control:focus-within{border-color:var(--vx-ink);box-shadow:0 0 0 2px color-mix(in srgb,var(--vx-ink) 10%,transparent)}.number-entry-control :deep(.ant-select-selector),.number-entry-control :deep(.ant-input){height:48px!important;border:0!important;border-radius:0!important;box-shadow:none!important}.number-entry-control :deep(.ant-select-selector){padding-inline:12px 8px!important;background:var(--vx-neutral)!important}.number-entry-control :deep(.ant-select-selection-item){font-size:13px;font-weight:750}.number-entry-control :deep(.ant-input){padding-inline:18px 52px;font-size:22px;font-weight:700;letter-spacing:.03em}.erase-button{position:absolute;top:9px;right:9px;display:grid;width:34px;height:34px;place-items:center;border:0;border-radius:50%;background:transparent;color:var(--vx-muted);cursor:pointer}.erase-button:hover{background:var(--vx-neutral);color:var(--vx-ink)}.keypad{display:grid;grid-template-columns:repeat(3,1fr);gap:10px;margin-bottom:18px}.keypad button{display:flex;min-height:64px;align-items:center;justify-content:center;flex-direction:column;border:0;border-radius:999px;background:var(--vx-neutral);color:var(--vx-ink);cursor:pointer;transition:background .15s ease,transform .1s ease}.keypad button:hover{background:var(--vx-soft)}.keypad button:active{transform:scale(.96)}.keypad button:disabled{opacity:.4;cursor:not-allowed}.keypad strong{font-size:23px;line-height:1}.keypad small{min-height:11px;margin-top:4px;color:var(--vx-muted);font-size:9px;letter-spacing:.12em}.dialer-panel :deep(.ant-btn .ri-icon){margin-right:7px}.call-actions{display:grid;grid-template-columns:1fr 1fr;gap:12px}.call-status-panel{display:flex;min-height:480px;flex-direction:column;justify-content:center;padding:36px}.idle-call,.live-call{text-align:center}.idle-call>.ri-icon{margin-bottom:12px;color:var(--vx-muted)}.idle-call h2,.live-call h2{margin:0;color:var(--vx-ink);font-size:30px}.idle-call p,.live-call p{margin:8px 0 0;color:var(--vx-muted)}.call-direction{display:inline-flex;margin-bottom:18px;padding:6px 12px;border-radius:999px;background:var(--vx-canvas);color:var(--vx-muted);font-size:12px;font-weight:700}.live-call-actions{display:flex;align-items:center;justify-content:center;gap:12px;margin-top:28px;flex-wrap:wrap}.record-dot{display:inline-block;width:8px;height:8px;margin-right:7px;border-radius:50%;background:var(--vx-danger)}.record-dot.recording{animation:record-pulse 1.2s ease-out infinite}@keyframes record-pulse{0%{box-shadow:0 0 0 0 color-mix(in srgb,var(--vx-danger) 50%,transparent)}70%,100%{box-shadow:0 0 0 6px transparent}}.capability-list{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-top:38px}.capability-list>div{display:grid;grid-template-columns:auto 1fr auto;align-items:center;gap:7px;padding:13px 14px;border-radius:14px;background:var(--vx-canvas);font-size:12px}.capability-list strong{color:var(--vx-ink)}.audio-detail,.status-error{margin:12px 0 0;text-align:center;color:var(--vx-muted);font-size:11px}.status-error{color:var(--vx-danger)}.call-library{min-width:0}.library-toolbar{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:16px}.library-toolbar h2{margin:0;color:var(--vx-ink);font-size:20px}.library-toolbar p{margin:4px 0 0;color:var(--vx-muted);font-size:12px}.contact-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.contact-card{display:flex;min-width:0;align-items:center;justify-content:space-between;gap:12px;padding:13px;border-radius:17px;background:var(--vx-panel)}.contact-main,.history-main{display:flex;min-width:0;flex:1;align-items:center;gap:12px;padding:0;border:0;background:transparent;text-align:left;cursor:pointer}.contact-main>span:last-child,.history-main>span:last-child,.recording-list article>div:first-child{display:flex;min-width:0;flex-direction:column;gap:3px}.contact-main strong,.contact-main small,.history-main strong,.history-main small,.recording-list strong,.recording-list small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.contact-main small,.history-main small,.recording-list small{color:var(--vx-muted);font-size:11px}.contact-avatar,.history-icon{display:grid;width:40px;height:40px;place-items:center;flex:none;border-radius:50%;background:var(--vx-accent);color:#163300;font-weight:800}.history-icon{background:var(--vx-neutral);color:var(--vx-ink)}.history-icon.missed{background:var(--vx-danger-bg);color:var(--vx-danger)}.history-list,.recording-list{display:grid;gap:8px}.history-list article,.recording-list article{display:flex;min-width:0;align-items:center;gap:12px;padding:13px 14px;border-radius:17px;background:var(--vx-panel)}.history-list time{margin-left:auto;color:var(--vx-muted);font-size:11px;white-space:nowrap}.recording-list article{flex-wrap:wrap}.recording-list article>div:first-child{flex:1}.recording-list audio{width:100%;height:38px}.dark .capability-list>div,.dark .call-direction{background:var(--vx-neutral)}
@media(max-width:900px){.phone-grid{grid-template-columns:minmax(280px,360px) minmax(0,1fr)}.call-status-panel{padding:24px}.capability-list{grid-template-columns:1fr}.contact-grid{grid-template-columns:1fr}}
@media(max-width:760px){.device-select{width:min(100%,280px)}.phone-grid{grid-template-columns:1fr}.call-status-panel{min-height:330px;grid-row:1}.dialer-panel{grid-row:2}.contact-card{align-items:flex-start}.history-list article{align-items:flex-start;flex-wrap:wrap}.history-list time{order:3;width:100%;margin-left:52px}.capability-list{margin-top:26px}}
@media(max-width:520px){.device-select{width:100%}.call-status-panel,.dialer-panel{padding:18px}.keypad button{min-height:58px}.capability-list>div{grid-template-columns:auto 1fr}.capability-list strong{grid-column:2}.contact-card{flex-direction:column}.contact-card .row-actions{align-self:flex-end}.history-list .ghost-button{margin-left:auto}.recording-list article{align-items:flex-start}}
</style>
