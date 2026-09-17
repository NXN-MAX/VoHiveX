<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Modal, message } from 'antdv-next'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
import RiIcon from '@/components/RiIcon.vue'
import { apiError, request } from '@/api/http'
import type { Device, SmsContact, SmsMessage } from '@/types/domain'
import { parseArchive, serializeMessages } from '@/utils/smsArchive'

type TransferMode = 'import' | 'export'
const route = useRoute()
const devices = ref<Device[]>([])
const deviceId = ref('all')
const contacts = ref<(SmsContact & { key: string })[]>([])
const activeKey = ref('')
const messages = ref<SmsMessage[]>([])
const search = ref('')
const loading = ref(false)
const threadLoading = ref(false)
const sending = ref(false)
const draft = ref('')
const composer = ref<HTMLTextAreaElement>()
const newSmsOpen = ref(false)
const newSmsSending = ref(false)
const newSmsForm = ref({ device_id: '', phone: '', message: '' })
const selecting = ref(false)
const selected = ref<string[]>([])
const transferOpen = ref(false)
const transferMode = ref<TransferMode>('export')
const transferDevice = ref('')
const transferFormat = ref('csv')
const transferContacts = ref<(SmsContact & { key: string })[]>([])
const transferSelected = ref<string[]>([])
const transferBusy = ref(false)
const importRows = ref<Record<string, any>[]>([])
const mobileChatOpen = ref(false)
const unreadVersion = ref(0)

const current = computed(() => contacts.value.find((row) => row.key === activeKey.value))
const filtered = computed(() => {
  const value = search.value.trim().toLowerCase()
  return contacts.value.filter((row) => !value || `${row.peer} ${row.last_content || ''}`.toLowerCase().includes(value))
})
const transferSource = computed(() => transferMode.value === 'import'
  ? groupImportRows(importRows.value)
  : transferContacts.value)
const transferDeviceInfo = computed(() => devices.value.find((row) => row.id === transferDevice.value))
const transferDeviceOptions = computed(() => devices.value.map((row) => ({ label: row.name || row.id, value: row.id })))
const smsMetrics = computed(() => measureSms(draft.value))
const newSmsMetrics = computed(() => measureSms(newSmsForm.value.message))

const gsm7Basic = new Set(Array.from("@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞ !\"#¤%&'()*+,-./0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"))
const gsm7Extended = new Set(Array.from('^{}\\[~]|€'))

function contactKey(row: SmsContact) { return `${row.imsi || ''}|${row.peer}` }
function displayTime(value: unknown) {
  if (!value) return ''
  const number = Number(value)
  return new Date(Number.isFinite(number) && number < 1e12 ? number * 1000 : String(value)).toLocaleString('zh-CN', { hour12: false })
}
function dateParts(value: unknown) {
  if (!value) return { date: '—', time: '' }
  const number = Number(value)
  const date = new Date(Number.isFinite(number) && number < 1e12 ? number * 1000 : String(value))
  return {
    date: date.toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' }),
    time: date.toLocaleTimeString('zh-CN', { hour12: false }),
  }
}
function measureSms(value: string) {
  const chars = Array.from(value)
  let septets = 0
  let gsm7 = true
  for (const char of chars) {
    if (gsm7Basic.has(char)) septets += 1
    else if (gsm7Extended.has(char)) septets += 2
    else { gsm7 = false; break }
  }
  const units = gsm7 ? septets : value.length
  const single = gsm7 ? 160 : 70
  const multipart = gsm7 ? 153 : 67
  return { encoding: gsm7 ? 'GSM7' : 'UCS-2', segments: Math.max(1, Math.ceil(units / (units <= single ? single : multipart))), chars: chars.length }
}
function messageContent(row: SmsMessage) { return String(row.content ?? row.text ?? '') }
function isOutgoing(row: SmsMessage) { return Boolean(row.outgoing || row.direction === 'out' || row.direction === 'sent' || row.type === 'sent') }
function phoneOf(device?: Device) { return String(device?.msisdn || device?.phone_number || device?.local_phone || '') }
function seenKey(row: SmsContact & { key: string }) { return `sms_thread_last_seen:${deviceId.value}:${row.key}` }
function toEpoch(value: unknown) {
  const number = Number(value)
  if (Number.isFinite(number)) return number < 1e12 ? number * 1000 : number
  const parsed = new Date(String(value || '')).getTime()
  return Number.isFinite(parsed) ? parsed : 0
}
function isUnread(row: SmsContact & { key: string }) {
  void unreadVersion.value
  const stored = localStorage.getItem(seenKey(row))
  if (stored === 'unread') return true
  const last = toEpoch(stored)
  const latest = toEpoch(row.last_ts)
  if (last && latest && last >= latest) return false
  return Number(row.unread || (row as any).unread_count || 0) > 0
}
function messageStatus(row: SmsMessage) {
  const status = String(row.status || '').toLowerCase()
  if (['sent', 'success', 'delivered', 'ok'].includes(status)) return { label: status === 'delivered' ? '已送达' : '发送成功', class: 'success' }
  if (['failed', 'error', 'undelivered'].includes(status)) return { label: '发送失败', class: 'failed' }
  if (['queued', 'pending', 'sending', 'submitted'].includes(status)) return { label: '发送中', class: 'pending' }
  return { label: '状态未知', class: 'unknown' }
}

async function loadDevices() {
  const data = await request<{ devices?: Device[] }>({ url: '/devices' })
  devices.value = data.devices || []
  transferDevice.value ||= devices.value[0]?.id || ''
}
async function loadContacts(keep = true) {
  loading.value = true
  try {
    const rows = await request<SmsContact[]>({ url: '/sms/contacts', params: { device_id: deviceId.value, limit: 500 } })
    contacts.value = (rows || []).map((row) => ({ ...row, key: contactKey(row) }))
    if (!keep || !contacts.value.some((row) => row.key === activeKey.value)) activeKey.value = contacts.value[0]?.key || ''
    if (activeKey.value) await loadThread()
  } catch (reason) { message.error(apiError(reason).message) } finally { loading.value = false }
}
async function loadThread() {
  const row = current.value
  if (!row) return void (messages.value = [])
  threadLoading.value = true
  try {
    const data = await request<SmsMessage[]>({ url: '/sms/thread', params: { device_id: row.device_id || deviceId.value, imsi: row.imsi || '', peer: row.peer, limit: 500 } })
    messages.value = [...(data || [])].sort((a, b) => new Date(String(a.timestamp || 0)).getTime() - new Date(String(b.timestamp || 0)).getTime())
    localStorage.setItem(seenKey(row), String(row.last_ts || Date.now()))
    unreadVersion.value++
  } catch (reason) { message.error(apiError(reason).message) } finally { threadLoading.value = false }
}
async function sendSms() {
  if (!current.value || !draft.value.trim()) return
  sending.value = true
  try {
    await request({ method: 'POST', url: '/sms/send', data: { device_id: current.value.device_id || deviceId.value, phone: current.value.peer, message: draft.value.trim() } })
    draft.value = ''
    await nextTick(); resizeComposer(); await loadThread()
  } catch (reason) { message.error(apiError(reason).message) } finally { sending.value = false }
}
function openNewSms() {
  const preferred = deviceId.value !== 'all' && devices.value.some((row) => row.id === deviceId.value)
    ? deviceId.value
    : devices.value[0]?.id || ''
  newSmsForm.value = { device_id: preferred, phone: '', message: '' }
  newSmsOpen.value = true
}
async function sendNewSms() {
  const form = newSmsForm.value
  if (!form.device_id) return message.warning('请选择发送设备')
  if (!form.phone.trim()) return message.warning('请输入目标号码')
  if (!form.message.trim()) return message.warning('请输入短信内容')
  newSmsSending.value = true
  try {
    await request({ method: 'POST', url: '/sms/send', data: { device_id: form.device_id, phone: form.phone.trim(), message: form.message.trim() } })
    newSmsOpen.value = false
    message.success('短信已提交发送')
    deviceId.value = form.device_id
    await loadContacts(false)
    const match = contacts.value.find((row) => row.peer === form.phone.trim() && (!row.device_id || row.device_id === form.device_id))
    if (match) openThread(match.key)
  } catch (reason) { message.error(apiError(reason).message) } finally { newSmsSending.value = false }
}
function resizeComposer() {
  if (!composer.value) return
  composer.value.style.height = '44px'
  composer.value.style.height = `${Math.min(104, composer.value.scrollHeight)}px`
}
function openThread(key: string) { activeKey.value = key; mobileChatOpen.value = true }
function selectDevice(id: string) { deviceId.value = id; mobileChatOpen.value = false; loadContacts(false) }
function toggleSelection(key: string) {
  selected.value = selected.value.includes(key) ? selected.value.filter((item) => item !== key) : [...selected.value, key]
}
function leaveSelection() { selecting.value = false; selected.value = [] }
function confirmBatch(action: 'read' | 'unread' | 'delete') {
  if (!selected.value.length) return
  const label = { read: '标记已读', unread: '标记未读', delete: '删除' }[action]
  Modal.confirm({ title: `${label}所选会话？`, content: action === 'delete' ? `将永久删除选中的 ${selected.value.length} 个会话，无法恢复。` : `将把选中的 ${selected.value.length} 个会话${label}。`, okText: label, cancelText: '取消', okButtonProps: { danger: action === 'delete' }, onOk: () => runBatch(action) })
}
function confirmDeleteThread(row: SmsContact & { key: string }) {
  Modal.confirm({ title: '删除会话？', content: `将永久删除与“${row.peer}”的短信会话，无法恢复。`, okText: '删除', cancelText: '取消', okButtonProps: { danger: true }, onOk: async () => {
    await request({ method: 'DELETE', url: '/sms/thread', params: { device_id: row.device_id || deviceId.value, imsi: row.imsi || '', peer: row.peer } })
    if (activeKey.value === row.key) activeKey.value = ''
    await loadContacts(false)
  } })
}
async function runBatch(action: 'read' | 'unread' | 'delete') {
  const chosen = contacts.value.filter((row) => selected.value.includes(row.key))
  try {
    if (action === 'delete') {
      for (const row of chosen) await request({ method: 'DELETE', url: '/sms/thread', params: { device_id: row.device_id || deviceId.value, imsi: row.imsi || '', peer: row.peer } })
    } else {
      for (const row of chosen) {
        action === 'read' ? localStorage.setItem(seenKey(row), String(row.last_ts || Date.now())) : localStorage.setItem(seenKey(row), 'unread')
      }
    }
    unreadVersion.value++
    leaveSelection(); await loadContacts(false)
  } catch (reason) { message.error(apiError(reason).message) }
}

async function openTransfer(mode: TransferMode) {
  transferMode.value = mode; transferOpen.value = true; importRows.value = []
  transferDevice.value = deviceId.value !== 'all' ? deviceId.value : devices.value[0]?.id || ''
  await loadTransferContacts(mode === 'export')
}
async function loadTransferContacts(selectAll = false) {
  if (!transferDevice.value || transferMode.value === 'import') return
  transferBusy.value = true
  try {
    const rows = await request<SmsContact[]>({ url: '/sms/contacts', params: { device_id: transferDevice.value, limit: 500 } })
    transferContacts.value = (rows || []).map((row) => ({ ...row, key: contactKey(row) }))
    transferSelected.value = selectAll ? transferContacts.value.map((row) => row.key) : []
  } catch (reason) { message.error(apiError(reason).message) } finally { transferBusy.value = false }
}
function groupImportRows(rows: Record<string, any>[]) {
  const found = new Map<string, any>()
  for (const row of rows) { const key = `${row.imsi || ''}|${row.peer || row.phone || ''}`; const value = found.get(key) || { key, peer: row.peer || row.phone, count: 0 }; value.count++; found.set(key, value) }
  return [...found.values()]
}
async function fetchAll(row: SmsContact) { return request<SmsMessage[]>({ url: '/sms/thread', params: { device_id: transferDevice.value, imsi: row.imsi || '', peer: row.peer, limit: 5000 } }) }
async function executeTransfer() {
  if (!transferSelected.value.length) return message.warning('请至少选择一个会话')
  transferBusy.value = true
  try {
    if (transferMode.value === 'import') {
      const keys = new Set(transferSelected.value)
      await request({ method: 'POST', url: '/sms/archive/import', data: { device_id: transferDevice.value, messages: importRows.value.filter((row) => keys.has(`${row.imsi || ''}|${row.peer || row.phone || ''}`)) } })
      message.success('短信会话已导入'); transferOpen.value = false; await loadContacts(false)
    } else {
      const chosen = transferContacts.value.filter((row) => transferSelected.value.includes(row.key)); const rows = (await Promise.all(chosen.map(fetchAll))).flat()
      const body = serializeMessages(rows, transferFormat.value); const url = URL.createObjectURL(new Blob([body], { type: 'text/plain;charset=utf-8' })); const link = document.createElement('a')
      link.href = url; link.download = `VoHiveX-SMS-${new Date().toISOString().slice(0, 10)}.${transferFormat.value}`; link.click(); URL.revokeObjectURL(url); transferOpen.value = false
    }
  } catch (reason) { message.error(apiError(reason).message) } finally { transferBusy.value = false }
}
async function readArchive(file?: File) {
  if (!file) return
  if (file.size > 16 * 1024 * 1024 || !/\.(csv|txt|html?|xml)$/i.test(file.name)) return message.error('仅支持 CSV、TXT、HTML、XML，最大 16 MB')
  try {
    const text = await file.text(); const raw = file.name.split('.').pop()?.toLowerCase(); const ext = raw === 'htm' ? 'html' : raw || ''
    importRows.value = parseArchive(text, ext)
    transferSelected.value = groupImportRows(importRows.value).map((row) => row.key)
  } catch (reason) { message.error(apiError(reason).message) }
}

watch(activeKey, () => loadThread())
onMounted(async () => {
  try {
    await loadDevices()
    const requested = String(route.query.device || '')
    if (requested && devices.value.some((row) => row.id === requested)) deviceId.value = requested
    await loadContacts(false)
  } catch (reason) { message.error(apiError(reason).message) }
})
</script>

<template>
  <div class="sms-page">
    <PageHeader title="短信中心" subtitle="收发与管理设备短信">
      <template #actions><a-button class="ghost-button" @click="openTransfer('import')">导入</a-button><a-button class="ghost-button" @click="openTransfer('export')">导出</a-button><a-button class="ghost-button" @click="loadContacts()">刷新</a-button><a-button type="primary" @click="openNewSms">新建短信</a-button></template>
    </PageHeader>
    <section class="sms-layout white-panel" :class="{ 'mobile-chat-open': mobileChatOpen }">
      <aside class="device-pane">
        <strong class="pane-label">设备</strong>
        <button class="device-row" :class="{ active: deviceId === 'all' }" @click="selectDevice('all')"><span><strong>全部设备</strong><small>汇总所有设备短信</small></span></button>
        <button v-for="device in devices" :key="device.id" class="device-row" :class="{ active: deviceId === device.id }" @click="selectDevice(device.id)"><span><strong>{{ device.name || device.id }}</strong><small>{{ phoneOf(device) || device.id }}</small></span><i :class="{ online: device.healthy }" /></button>
      </aside>
      <aside class="thread-pane" :class="{ selecting }">
        <div class="thread-tools"><button class="select-toggle" :class="{ cancel: selecting }" @click="selecting ? leaveSelection() : selecting = true">{{ selecting ? '取消选择' : '选择' }}</button><a-input v-model:value="search" class="pill-search" placeholder="搜索会话" allow-clear /></div>
        <a-spin :spinning="loading"><EmptyState v-if="!filtered.length" title="暂无短信会话" icon="message-2-line" /><div v-else class="thread-list"><div v-for="row in filtered" :key="row.key" class="thread-row" :class="{ active: activeKey === row.key }" role="button" tabindex="0" @click="selecting ? toggleSelection(row.key) : openThread(row.key)" @keydown.enter="selecting ? toggleSelection(row.key) : openThread(row.key)"><span class="thread-leading" :class="{ 'selection-visible': selecting }"><span v-if="selecting" class="select-dot" :class="{ checked: selected.includes(row.key) }"><RiIcon v-if="selected.includes(row.key)" name="check-line" :size="12" /></span></span><span class="thread-copy"><strong>{{ row.peer }}</strong><small>{{ row.last_content || '暂无内容' }}</small></span><time class="thread-time"><span>{{ dateParts(row.last_ts).date }}</span><span>{{ dateParts(row.last_ts).time }}</span></time><span v-if="!selecting && isUnread(row)" class="unread-dot" title="未读消息" /><button v-if="!selecting" class="thread-delete" :aria-label="`删除与 ${row.peer} 的会话`" title="删除会话" @click.stop="confirmDeleteThread(row)"><RiIcon name="delete-bin-line" :size="17" /></button></div></div></a-spin>
        <div v-if="selecting" class="selection-dock"><button :disabled="!selected.length" @click="confirmBatch('read')">标记已读</button><button :disabled="!selected.length" @click="confirmBatch('unread')">标记未读</button><button class="danger" :disabled="!selected.length" @click="confirmBatch('delete')">删除</button></div>
      </aside>
      <main class="chat-pane"><template v-if="current"><header class="chat-header"><button class="mobile-back" aria-label="返回会话" @click="mobileChatOpen = false"><RiIcon name="arrow-left-line" /></button><div><strong>{{ current.peer }}</strong><small>本机：{{ current.local_phone || phoneOf(devices.find(row => row.id === current?.device_id)) || '号码未知' }}</small></div><span>最新</span></header><div class="message-list"><a-spin :spinning="threadLoading"><div v-for="row in messages" :key="row.id" class="message-line" :class="{ outgoing: isOutgoing(row) }"><div class="message-bubble"><span>{{ messageContent(row) }}</span><small class="message-meta"><time>{{ displayTime(row.timestamp) }}</time><em v-if="isOutgoing(row)" :class="messageStatus(row).class">{{ messageStatus(row).label }}</em></small></div></div></a-spin></div><form class="composer" @submit.prevent="sendSms"><small class="sms-metrics">{{ smsMetrics.encoding }} · 预计 {{ smsMetrics.segments }} 段 · {{ smsMetrics.chars }} 字</small><div class="composer-row"><textarea ref="composer" v-model="draft" rows="1" maxlength="2000" placeholder="回复（Enter 发送）" @input="resizeComposer" /><a-button type="primary" html-type="submit" :loading="sending" :disabled="!draft.trim()">发送</a-button></div></form></template><EmptyState v-else title="选择一个会话" description="从左侧会话列表查看或发送短信" icon="chat-voice-line" /></main>
    </section>

    <a-modal v-model:open="newSmsOpen" title="发送短信" width="min(520px, 92vw)" :confirm-loading="newSmsSending" ok-text="发送" cancel-text="取消" @ok="sendNewSms">
      <div class="new-sms-form">
        <label>发送设备<a-select v-model:value="newSmsForm.device_id" placeholder="选择设备" :options="transferDeviceOptions" /></label>
        <label>目标号码<a-input v-model:value="newSmsForm.phone" placeholder="+86138..." :maxlength="32" /></label>
        <label>短信内容<a-textarea v-model:value="newSmsForm.message" placeholder="输入短信内容..." :auto-size="{ minRows: 4, maxRows: 10 }" :maxlength="2000" /></label>
        <small>{{ newSmsMetrics.encoding }} · 预计 {{ newSmsMetrics.segments }} 段 · {{ newSmsMetrics.chars }} 字</small>
      </div>
    </a-modal>

    <a-modal v-model:open="transferOpen" :title="transferMode === 'import' ? '导入短信会话' : '导出短信会话'" :confirm-loading="transferBusy" :ok-text="transferMode === 'import' ? '导入' : '导出'" cancel-text="取消" @ok="executeTransfer">
      <div class="transfer-form"><label>设备<a-select v-model:value="transferDevice" :options="transferDeviceOptions" @change="loadTransferContacts(transferMode === 'export')" /><small>设备手机号码：{{ phoneOf(transferDeviceInfo) || '未读取到' }}</small></label><label v-if="transferMode === 'export'">格式<a-select v-model:value="transferFormat" :options="['csv','txt','html','xml'].map(format => ({ label: format.toUpperCase(), value: format }))" /></label><label v-else class="archive-file">归档文件<input type="file" accept=".csv,.txt,.html,.htm,.xml" @change="readArchive(($event.target as HTMLInputElement).files?.[0])" /><small>支持 CSV、TXT、HTML、XML，最多 16 MB</small></label><label v-if="transferSource.length" class="select-all"><a-checkbox :checked="transferSelected.length === transferSource.length" @change="transferSelected = $event.target.checked ? transferSource.map(row => row.key) : []" />全选会话（{{ transferSource.length }}）</label><div class="transfer-list"><label v-for="row in transferSource" :key="row.key"><a-checkbox :checked="transferSelected.includes(row.key)" @change="$event.target.checked ? transferSelected.push(row.key) : transferSelected = transferSelected.filter(key => key !== row.key)" /><span><strong>{{ row.peer }}</strong><small>{{ row.count ? `${row.count} 条短信` : row.last_content || '暂无内容' }}</small></span></label></div></div>
    </a-modal>
  </div>
</template>

<style scoped>
.sms-layout{display:grid;grid-template-columns:220px 320px minmax(320px,1fr);height:calc(100vh - 206px);min-height:560px;padding:0;overflow:hidden}.device-pane,.thread-pane{min-height:0;background:var(--vx-canvas)}.device-pane{display:flex;flex-direction:column;gap:5px;padding:16px;border-right:1px solid var(--vx-line)}.pane-label{margin-bottom:6px;color:var(--vx-muted);font-size:12px}.device-row{display:flex;width:100%;align-items:center;justify-content:space-between;gap:10px;padding:12px;border:0;border-radius:16px;background:transparent;text-align:left;cursor:pointer}.device-row:hover{background:var(--vx-neutral)}.device-row.active{background:var(--vx-soft)}.device-row>span{display:flex;min-width:0;flex:1;flex-direction:column;gap:3px}.device-row strong,.device-row small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.device-row small{color:var(--vx-muted);font-size:11px}.device-row i{width:7px;height:7px;min-width:7px;flex:0 0 7px;aspect-ratio:1;border-radius:50%;background:#a4a69f}.device-row i.online{background:#61bd32}.thread-pane{position:relative;display:flex;flex-direction:column;padding:16px;border-right:1px solid var(--vx-line)}.thread-tools{display:grid;grid-template-columns:auto 1fr;align-items:center;gap:10px}.select-toggle{height:36px;padding:0 14px;border:0;border-radius:999px;background:transparent;color:var(--vx-ink);font-size:13px;font-weight:700;cursor:pointer}.select-toggle:hover{background:var(--vx-neutral)}.select-toggle.cancel{color:var(--vx-danger)}.thread-list{margin:12px -4px 0;overflow:auto}.thread-row{display:grid;width:100%;grid-template-columns:auto minmax(0,1fr) auto auto auto;align-items:center;padding:12px;border:0;border-radius:18px;background:transparent;text-align:left;cursor:pointer}.thread-row:hover{background:var(--vx-neutral)}.thread-row.active{background:var(--vx-soft)}.thread-leading{display:grid;width:0;min-width:0;margin-right:0;overflow:hidden;place-items:center;transition:width .18s ease,margin-right .18s ease}.thread-leading.selection-visible{width:18px;margin-right:9px}.unread-dot{width:8px;height:8px;margin-left:9px;border-radius:50%;background:#50a41d;box-shadow:0 0 0 3px color-mix(in srgb,#50a41d 14%,transparent)}.thread-copy{display:flex;min-width:0;flex-direction:column;gap:4px}.thread-copy strong,.thread-copy small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.thread-copy small,.thread-row time{color:var(--vx-muted);font-size:11px}.thread-time{display:flex;align-items:flex-end;flex-direction:column;margin-left:9px;line-height:1.35;text-align:right;white-space:nowrap;transition:transform .18s ease}.thread-delete{display:grid;width:0;height:32px;margin-left:0;overflow:hidden;place-items:center;padding:0;border:0;border-radius:50%;background:transparent;color:var(--vx-danger);opacity:0;transform:translateX(8px);cursor:pointer;transition:width .18s ease,margin-left .18s ease,opacity .15s ease,transform .18s ease,background .15s ease}.thread-row:hover .thread-delete,.thread-delete:focus-visible{width:32px;margin-left:9px;opacity:1;transform:translateX(0)}.thread-delete:hover{background:var(--vx-danger-bg)}.select-dot{display:grid;width:18px;height:18px;min-width:18px;place-items:center;border:1px solid var(--vx-muted);border-radius:50%}.select-dot.checked{border-color:#50a41d;background:#50a41d;color:#fff}.selection-dock{position:absolute;right:0;bottom:0;left:0;display:flex;justify-content:center;gap:16px;padding:44px 12px 16px;background:linear-gradient(transparent,var(--vx-canvas) 42%)}.selection-dock button{padding:8px 12px;border:0;border-radius:999px;background:transparent;font-size:12px;font-weight:700;cursor:pointer;transition:background .15s ease}.selection-dock button:hover:not(:disabled){background:var(--vx-neutral)}.selection-dock button.danger:hover:not(:disabled){background:var(--vx-danger-bg)}.selection-dock button:disabled{opacity:.35}.chat-pane{display:flex;min-width:0;min-height:0;flex-direction:column;background:var(--vx-neutral)}.chat-header{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:16px 20px;background:var(--vx-canvas)}.chat-header div{display:flex;min-width:0;flex:1;flex-direction:column}.chat-header small,.chat-header>span{color:var(--vx-muted);font-size:12px}.mobile-back{display:none;width:34px;height:34px;place-items:center;border:0;border-radius:50%;background:transparent}.message-list{flex:1;padding:24px;overflow:auto}.message-line{display:flex;margin:8px 0;justify-content:flex-start}.message-line.outgoing{justify-content:flex-end}.message-bubble{display:flex;max-width:min(70%,620px);flex-direction:column;gap:5px;padding:10px 14px;border-radius:18px 18px 18px 5px;background:var(--vx-canvas);white-space:pre-wrap;overflow-wrap:anywhere}.outgoing .message-bubble{border-radius:18px 18px 5px;background:var(--vx-accent)}.message-meta{display:flex;align-items:center;justify-content:flex-end;gap:8px;color:var(--vx-muted);font-size:10px}.message-meta em{font-style:normal}.message-meta .success{color:#3e771b}.message-meta .failed{color:var(--vx-danger)}.message-meta .pending,.message-meta .unknown{color:var(--vx-muted)}.composer{display:flex;align-items:stretch;flex-direction:column;gap:6px;padding:14px;background:var(--vx-canvas)}.composer-row{display:flex;align-items:flex-end;gap:12px}.composer textarea{height:44px;min-height:44px;max-height:104px;flex:1;overflow:hidden;resize:none;padding:10px 16px;border:2px solid var(--vx-line);border-radius:22px;background:var(--vx-canvas);outline:0}.composer textarea:focus{border-color:var(--vx-ink)}.composer .ant-btn{height:44px;border-radius:999px}.sms-metrics{padding-left:2px;color:var(--vx-muted);font-size:10px;text-align:left}.transfer-form{display:grid;gap:16px}.transfer-form>label{display:grid;gap:7px;color:var(--vx-ink);font-weight:700}.transfer-form small{color:var(--vx-muted);font-weight:400}.archive-file input{padding:12px;border-radius:999px;background:var(--vx-neutral)}.select-all{display:flex!important;align-items:center}.transfer-list{max-height:260px;overflow:auto}.transfer-list label{display:flex;align-items:center;gap:10px;padding:10px;border-radius:12px}.transfer-list label:hover{background:var(--vx-neutral)}.transfer-list span{display:flex;min-width:0;flex-direction:column}.transfer-list strong,.transfer-list small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.new-sms-form{display:grid;gap:16px}.new-sms-form label{display:grid;gap:7px;color:var(--vx-muted);font-size:13px;font-weight:650}.new-sms-form>small{color:var(--vx-muted);font-size:12px}.new-sms-form :deep(textarea){resize:none}
@media(max-width:1200px){.sms-layout{grid-template-columns:170px 240px minmax(280px,1fr)}}
@media(max-width:1024px){.sms-layout{display:block;height:calc(100vh - 190px);min-height:540px}.device-pane{display:none}.thread-pane{height:100%;border:0}.chat-pane{display:none;height:100%;min-height:0}.sms-layout.mobile-chat-open .thread-pane{display:none}.sms-layout.mobile-chat-open .chat-pane{display:flex}.mobile-back{display:grid}.message-bubble{max-width:86%}}
</style>
