<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Modal, message } from 'antdv-next'
import dayjs, { Dayjs } from 'dayjs'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
import { apiError, request } from '@/api/http'
import type { Device, ScheduleTask } from '@/types/domain'

const tasks = ref<ScheduleTask[]>([])
const devices = ref<Device[]>([])
const loading = ref(false)
const busy = ref(false)
const editOpen = ref(false)
const editing = ref<ScheduleTask | null>(null)
const historyOpen = ref(false)
const runs = ref<any[]>([])
const historyTitle = ref('')
const firstRun = ref<Dayjs>(dayjs().add(5, 'minute'))
const form = reactive({ name: '定时短信', device_id: '', phone: '', message: '', mode: 'once' as 'once' | 'interval', days: 0, hours: 0, minutes: 0, seconds: 0 })
let timer = 0

const activeCount = computed(() => tasks.value.filter((row) => row.state === 'active' || row.status === 'active').length)
const pausedCount = computed(() => tasks.value.filter((row) => row.state === 'paused' || row.status === 'paused').length)
const nextTask = computed(() => tasks.value.filter((row) => row.next_run).sort((a, b) => Number(a.next_run) - Number(b.next_run))[0])
function formatTime(value: unknown) { return value ? new Date(Number(value) * (Number(value) < 1e12 ? 1000 : 1)).toLocaleString('zh-CN', { hour12: false }) : '—' }
function intervalText(value: number) { let rest = value; const out: string[] = []; for (const [size, label] of [[86400, '天'], [3600, '小时'], [60, '分钟'], [1, '秒']] as const) { const count = Math.floor(rest / size); if (count) out.push(`${count} ${label}`); rest %= size } return `每隔 ${out.join(' ') || '0 秒'}` }
function deviceName(id: string) { return devices.value.find((row) => row.id === id)?.name || id }
function taskState(row: ScheduleTask) { return String(row.state || row.status || 'paused') }

async function load(silent = false) {
  if (!silent) loading.value = true
  try {
    const [taskData, deviceData] = await Promise.all([request<any>({ url: '/schedules' }), request<any>({ url: '/schedules/devices' })])
    tasks.value = taskData.tasks || []
    devices.value = deviceData.devices || []
  } catch (reason) { if (!silent) message.error(apiError(reason).message) } finally { loading.value = false }
}
function openEdit(row?: ScheduleTask) {
  editing.value = row || null
  let interval = Number(row?.interval_seconds || 86400)
  form.days = Math.floor(interval / 86400); interval %= 86400
  form.hours = Math.floor(interval / 3600); interval %= 3600
  form.minutes = Math.floor(interval / 60); form.seconds = interval % 60
  Object.assign(form, { name: row?.name || '定时短信', device_id: row?.device_id || devices.value[0]?.id || '', phone: row?.phone || '', message: row?.message || '', mode: row?.mode || 'once' })
  firstRun.value = dayjs(Math.max(Number(row?.first_run || 0) * 1000, Date.now() + 300_000))
  editOpen.value = true
}
async function save() {
  const interval = form.days * 86400 + form.hours * 3600 + form.minutes * 60 + form.seconds
  if (form.mode === 'interval' && interval <= 0) return message.warning('重复任务的间隔必须大于 0 秒')
  busy.value = true
  try {
    const payload = { name: form.name, device_id: form.device_id, phone: form.phone, message: form.message, mode: form.mode, first_run: Math.floor(firstRun.value.valueOf() / 1000), interval_seconds: form.mode === 'interval' ? interval : 0 }
    await request({ method: editing.value ? 'PUT' : 'POST', url: editing.value ? `/schedules/${editing.value.id}` : '/schedules', data: editing.value ? { ...payload, version: editing.value.version } : payload })
    editOpen.value = false; message.success(editing.value ? '任务已修改并暂停' : '任务已创建并暂停'); await load(true)
  } catch (reason) { message.error(apiError(reason).message) } finally { busy.value = false }
}
async function action(row: ScheduleTask, name: 'start' | 'pause') {
  try { await request({ method: 'POST', url: `/schedules/${row.id}/${name}`, data: { version: row.version } }); await load(true) } catch (reason) { message.error(apiError(reason).message) }
}
function remove(row: ScheduleTask) {
  Modal.confirm({ title: '删除任务', content: `确定删除“${row.name}”及其执行记录吗？`, okText: '删除', cancelText: '取消', okButtonProps: { danger: true }, onOk: async () => { await request({ method: 'DELETE', url: `/schedules/${row.id}`, data: { version: row.version } }); await load(true) } })
}
async function history(row: ScheduleTask) {
  historyTitle.value = `执行记录 · ${row.name}`; historyOpen.value = true; runs.value = []
  try { runs.value = (await request<any>({ url: `/schedules/${row.id}/history` })).runs || [] } catch (reason) { message.error(apiError(reason).message) }
}
onMounted(() => { load(); timer = window.setInterval(() => { if (!document.hidden) load(true) }, 5000) })
onBeforeUnmount(() => window.clearInterval(timer))
</script>

<template>
  <div class="tasks-page"><PageHeader title="定时任务" subtitle="按指定时间或固定间隔发送短信"><template #actions><a-button type="primary" :disabled="!devices.length" @click="openEdit()">新增任务</a-button></template></PageHeader>
    <section class="task-summary"><article><span>进行中的任务</span><strong>{{ activeCount }}</strong></article><article><span>已暂停的任务</span><strong>{{ pausedCount }}</strong></article><article><span>最近执行时间 · 北京时间</span><strong>{{ nextTask ? formatTime(nextTask.next_run) : '暂无安排' }}</strong></article></section>
    <section class="task-section"><div class="section-title"><h2>全部任务（{{ tasks.length }}）</h2><span class="muted">北京时间（UTC+8）</span></div><p class="task-footnote">新建或修改任务后默认暂停。错过的执行时间不集中补发；发送失败或结果不确定时自动暂停，详情可查看执行记录。</p><a-spin :spinning="loading"><EmptyState v-if="!tasks.length" title="还没有定时任务" :description="devices.length ? '创建任务后点击开始即可启用。' : '请先在设备管理中添加发信设备。'" icon="calendar-schedule-line" /><div v-else class="task-list"><article v-for="row in tasks" :key="row.id" class="task-card"><header><div><h3>{{ row.name }}</h3><span class="pill" :class="{ success: taskState(row) === 'active' }">{{ taskState(row) === 'active' ? '进行中' : taskState(row) === 'completed' ? '已完成' : '已暂停' }}</span></div><div class="row-actions"><a-button class="ghost-button" @click="action(row, taskState(row) === 'active' ? 'pause' : 'start')">{{ taskState(row) === 'active' ? '暂停' : '开始' }}</a-button><a-button class="ghost-button" @click="openEdit(row)">修改</a-button><a-button class="ghost-button" @click="history(row)">记录</a-button><a-button class="ghost-button danger" @click="remove(row)">删除</a-button></div></header><div class="task-grid"><div><span>发信设备</span><strong>{{ deviceName(row.device_id) }}</strong></div><div><span>收信号码</span><strong>{{ row.phone }}</strong></div><div><span>执行规则</span><strong>{{ row.mode === 'once' ? '指定时间 · 仅一次' : intervalText(Number(row.interval_seconds || 0)) }}</strong></div><div><span>下次执行时间</span><strong>{{ formatTime(row.next_run) }}</strong></div></div><p>{{ row.message }}</p><small>{{ row.last_result || row.last_status || '等待开始' }}<template v-if="row.last_run"> · 最近执行 {{ formatTime(row.last_run) }} · 已提交 {{ row.run_count || 0 }} 次</template></small></article></div></a-spin></section>

    <a-modal v-model:open="editOpen" wrap-class-name="task-editor-modal" :width="720" :title="editing ? '修改任务' : '新增任务'" :confirm-loading="busy" ok-text="保存任务" cancel-text="取消" @ok="save">
      <a-form class="task-editor" layout="vertical">
        <div class="task-editor-grid">
          <a-form-item class="task-name-field" label="任务名称" required><a-input v-model:value="form.name" :maxlength="80" placeholder="例如：每日提醒" /></a-form-item>
          <a-form-item label="发信设备" required><a-select v-model:value="form.device_id" placeholder="请选择设备"><a-select-option v-for="device in devices" :key="device.id" :value="device.id">{{ device.name || device.id }} · {{ device.running ? '在线' : '离线' }}</a-select-option></a-select></a-form-item>
          <a-form-item label="收信号码" required><a-input v-model:value="form.phone" :maxlength="24" placeholder="例如：+8613800138000" /></a-form-item>
        </div>

        <a-form-item class="message-field" required>
          <template #label><span class="field-label-row"><span>短信内容</span><small>{{ form.message.length }} / 2000 字符</small></span></template>
          <a-textarea v-model:value="form.message" :rows="3" :maxlength="2000" placeholder="输入要发送的短信内容" />
          <p class="field-help">长短信可能被拆分为多条发送并计费</p>
        </a-form-item>

        <section class="schedule-editor">
          <header><div><strong>执行安排</strong><small>北京时间（UTC+8），可精确到秒</small></div><div class="mode-switch" role="radiogroup" aria-label="执行方式"><button type="button" role="radio" :aria-checked="form.mode === 'once'" :class="{ active: form.mode === 'once' }" @click="form.mode = 'once'">指定时间</button><button type="button" role="radio" :aria-checked="form.mode === 'interval'" :class="{ active: form.mode === 'interval' }" @click="form.mode = 'interval'">按间隔</button></div></header>
          <div class="schedule-fields" :class="{ once: form.mode === 'once' }">
            <div v-if="form.mode === 'interval'" class="interval-fieldset"><span class="compact-label">重复间隔</span><div class="interval-grid"><a-form-item label="天"><a-input-number v-model:value="form.days" :min="0" :max="3659" /></a-form-item><a-form-item label="小时"><a-input-number v-model:value="form.hours" :min="0" :max="23" /></a-form-item><a-form-item label="分钟"><a-input-number v-model:value="form.minutes" :min="0" :max="59" /></a-form-item><a-form-item label="秒"><a-input-number v-model:value="form.seconds" :min="0" :max="59" /></a-form-item></div></div>
            <a-form-item class="first-run-field" :label="form.mode === 'once' ? '执行日期和时间' : '首次执行日期和时间'" required><a-date-picker v-model:value="firstRun" show-time format="YYYY-MM-DD HH:mm:ss" /></a-form-item>
          </div>
        </section>

        <p class="task-editor-note">保存后任务为暂停状态，需在任务列表中点击“开始”才会执行。</p>
      </a-form>
    </a-modal>
    <a-modal v-model:open="historyOpen" :title="historyTitle" :footer="null"><EmptyState v-if="!runs.length" title="暂无执行记录" icon="file-list-3-line" /><div v-else class="history-list"><article v-for="(run,index) in runs" :key="index"><strong>{{ formatTime(run.scheduled_for) }}</strong><span class="pill">{{ run.status }}</span><p>{{ run.detail }}</p></article></div></a-modal>
  </div>
</template>

<style scoped>
.tasks-page,.task-summary,.task-section{width:100%;min-width:0}.task-summary{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px;margin-bottom:20px}.task-summary article{display:flex;min-height:108px;flex-direction:column;justify-content:space-between;padding:22px;border-radius:20px;background:var(--vx-panel)}.task-summary span{color:var(--vx-muted);font-size:13px}.task-summary strong{color:var(--vx-ink);font-size:24px}.task-section{padding:0;background:transparent}.task-list{display:grid;gap:16px}.task-card{padding:20px;border-radius:20px;background:var(--vx-panel)}.task-card header{display:flex;align-items:center;justify-content:space-between;gap:16px}.task-card header>div:first-child{display:flex;align-items:center;gap:10px}.task-card h3{margin:0;color:var(--vx-ink);font-size:18px}.task-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:18px;margin:18px 0}.task-grid div{display:flex;min-width:0;flex-direction:column;gap:4px}.task-grid span,.task-card small{color:var(--vx-muted);font-size:12px}.task-grid strong{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.task-card>p{margin:0 0 12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.task-footnote{margin:-8px 0 16px;color:var(--vx-muted);font-size:12px;line-height:1.6}.task-editor{max-height:min(64vh,560px);overflow-y:auto;padding:0 4px 2px 0}.task-editor :deep(.ant-form-item){margin-bottom:14px}.task-editor-grid{display:grid;grid-template-columns:1fr 1fr;gap:0 16px}.task-name-field{grid-column:1/-1}.field-label-row{display:flex;width:100%;align-items:center;justify-content:space-between;gap:16px}.field-label-row small{color:var(--vx-muted);font-size:11px;font-weight:500}.message-field :deep(textarea){min-height:82px;max-height:120px;resize:vertical}.schedule-editor{padding:16px;border-radius:18px;background:var(--vx-neutral)}.schedule-editor>header{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:14px}.schedule-editor>header>div:first-child{display:flex;min-width:0;flex-direction:column;gap:3px}.schedule-editor>header strong{color:var(--vx-ink);font-size:15px}.schedule-editor>header small{color:var(--vx-muted);font-size:11px}.mode-switch{display:inline-flex;flex:none;align-items:center;gap:4px;padding:3px;border-radius:999px;background:var(--vx-canvas)}.mode-switch button{min-height:34px;padding:0 16px;border:0;border-radius:999px;background:transparent;color:var(--vx-muted);font-family:inherit;font-size:13px;font-weight:700;line-height:1;cursor:pointer;transition:background .18s ease,color .18s ease}.mode-switch button:hover{background:var(--vx-soft);color:var(--vx-ink)}.mode-switch button.active{background:var(--vx-accent);color:#163300}.schedule-fields{display:grid;grid-template-columns:minmax(0,1.2fr) minmax(220px,.8fr);align-items:end;gap:16px}.schedule-fields.once{grid-template-columns:1fr}.interval-fieldset{min-width:0}.compact-label{display:block;margin-bottom:8px;color:var(--vx-muted);font-size:12px;font-weight:650}.interval-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px}.interval-grid :deep(.ant-form-item){margin:0}.interval-grid :deep(.ant-form-item-label){padding-bottom:4px}.interval-grid :deep(.ant-input-number){width:100%}.first-run-field{margin:0!important}.first-run-field :deep(.ant-picker){width:100%}.task-editor-note{margin:12px 2px 0;color:var(--vx-muted);font-size:12px;line-height:1.5}.history-list{display:grid;gap:10px;max-height:360px;overflow:auto}.history-list article{padding:14px;border-radius:14px;background:var(--vx-neutral)}.history-list span{margin-left:10px}.history-list p{margin:8px 0 0;color:var(--vx-muted)}
:global(.task-editor-modal .ant-modal){top:clamp(16px,4vh,40px);max-width:calc(100vw - 32px);padding-bottom:0}:global(.task-editor-modal .ant-modal-content){max-height:calc(100vh - 32px)}
@media(max-width:760px){.task-summary,.task-grid{grid-template-columns:1fr}.task-card header{align-items:flex-start;flex-direction:column}.task-editor{max-height:calc(100vh - 180px)}.task-editor-grid,.schedule-fields{grid-template-columns:1fr}.schedule-editor>header{align-items:flex-start;flex-direction:column}.mode-switch{width:100%}.mode-switch button{flex:1}.interval-grid{grid-template-columns:1fr 1fr}.task-name-field{grid-column:auto}:global(.task-editor-modal .ant-modal){top:16px;margin:0 auto}}
</style>
