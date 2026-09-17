<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { message } from 'antdv-next'
import PageHeader from '@/components/PageHeader.vue'
import { apiError, request } from '@/api/http'
import { normalizeLogResponse } from '@/utils/logs'

const lines = ref<string[]>([])
const loading = ref(false)
const paused = ref(false)
const wrap = ref(localStorage.getItem('vohivex:logs-wrap') !== '0')
const tail = ref(localStorage.getItem('vohivex:logs-tail') !== '0')
const query = ref('')
const level = ref('all')
const consoleEl = ref<HTMLDivElement>()
let timer = 0
const parsedLines = computed(() => lines.value.map((line) => parseLine(line)).filter((line) => {
  const levelMatch = level.value === 'all' || line.level.toLowerCase() === level.value
  const queryMatch = !query.value || line.raw.toLowerCase().includes(query.value.toLowerCase())
  return levelMatch && queryMatch
}))

function parseLine(line: string) {
  const match = line.match(/^(\S+)\s+(TRACE|DEBUG|INFO|WARN|WARNING|ERROR|FATAL)\s+(\S+)\s*(.*)$/i)
  if (!match) return { raw: line, time: '', level: '', device: '', text: line }
  return { raw: line, time: match[1], level: match[2].toUpperCase(), device: match[3], text: match[4] }
}

async function scrollToEnd() {
  if (!tail.value) return
  await nextTick()
  if (consoleEl.value) consoleEl.value.scrollTop = consoleEl.value.scrollHeight
}

async function load() {
  if (paused.value) return
  loading.value = true
  try {
    const data = await request<any>({ url: '/logs/history', params: { lines: 500 } })
    lines.value = normalizeLogResponse(data)
    await scrollToEnd()
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    loading.value = false
  }
}

function exportLog() {
  const url = URL.createObjectURL(new Blob([lines.value.join('\n')], { type: 'text/plain;charset=utf-8' }))
  const link = document.createElement('a')
  link.href = url
  link.download = `VoHiveX-${new Date().toISOString()}.log`
  link.click()
  URL.revokeObjectURL(url)
}

function persistWrap() { localStorage.setItem('vohivex:logs-wrap', wrap.value ? '1' : '0') }
function persistTail() { localStorage.setItem('vohivex:logs-tail', tail.value ? '1' : '0'); scrollToEnd() }
function togglePaused() {
  paused.value = !paused.value
  if (!paused.value) void load()
}
onMounted(() => { load(); timer = window.setInterval(load, 5000) })
onBeforeUnmount(() => window.clearInterval(timer))
</script>

<template>
  <div class="logs-page">
    <PageHeader title="实时日志" subtitle="查看系统运行日志，支持过滤和搜索">
      <template #actions>
        <a-button class="ghost-button danger" @click="lines = []">清空</a-button>
        <a-button class="ghost-button" @click="exportLog">导出</a-button>
        <a-button class="log-toggle" :class="paused ? 'resume' : 'pause'" @click="togglePaused">{{ paused ? '继续' : '暂停' }}</a-button>
      </template>
    </PageHeader>
    <section class="log-toolbar">
      <div class="log-filter-controls">
        <a-select v-model:value="level" placeholder="日志级别" :options="[{label:'全部',value:'all'},{label:'DEBUG',value:'debug'},{label:'INFO',value:'info'},{label:'WARN',value:'warn'},{label:'ERROR',value:'error'}]" aria-label="日志级别" />
        <a-input v-model:value="query" allow-clear placeholder="搜索日志内容..." />
      </div>
      <div class="log-meta">
        <span class="connection-state"><span class="status-dot" :class="paused ? 'paused' : 'online'" />{{ paused ? '已暂停' : '已连接' }}</span>
        <span>{{ lines.length }} 条日志</span>
        <label><input v-model="tail" type="checkbox" @change="persistTail" />自动追尾</label>
        <label><input v-model="wrap" type="checkbox" @change="persistWrap" />自动换行</label>
      </div>
    </section>
    <a-spin :spinning="loading">
      <div ref="consoleEl" class="log-console" :class="{ wrap }">
        <div v-if="!parsedLines.length" class="log-empty">暂无日志</div>
        <template v-else><div v-for="(line,index) in parsedLines" :key="`${index}-${line.raw}`" class="log-line">
            <time v-if="line.time" class="log-time">{{ line.time }}</time>
            <strong v-if="line.level" class="log-level" :class="`level-${line.level.toLowerCase()}`">{{ line.level }}</strong>
            <span v-if="line.device" class="log-device">{{ line.device }}</span>
            <span class="log-message">{{ line.text }}</span>
        </div></template>
      </div>
    </a-spin>
  </div>
</template>

<style scoped>
.log-toggle{height:36px!important;padding:0 18px!important;border:0!important;border-radius:999px!important;box-shadow:none!important;font-weight:750}.log-toggle.pause{background:#fff0dc!important;color:#9a4a00!important}.log-toggle.pause:hover{background:#ffe2b9!important;color:#7b3b00!important}.log-toggle.resume{background:var(--vx-accent)!important;color:var(--vx-accent-ink)!important}.log-toggle.resume:hover{background:var(--vx-accent-hover)!important}.status-dot.paused{background:#f59e0b}.log-toolbar{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:16px;flex-wrap:wrap}.log-filter-controls{display:flex;min-width:360px;flex:1 1 360px;align-items:center;gap:16px}.log-filter-controls :deep(.ant-select){width:126px;flex:none}.log-filter-controls :deep(.ant-input-affix-wrapper){max-width:360px;flex:1;background:var(--vx-canvas)!important}.log-meta{display:flex;flex:none;align-items:center;justify-content:flex-end;gap:16px;color:var(--vx-muted);font-size:12px}.connection-state{display:inline-flex;align-items:center;gap:4px;white-space:nowrap}.log-meta label{display:flex;align-items:center;gap:4px;color:var(--vx-ink);font-weight:650;white-space:nowrap;cursor:pointer}.log-meta input{width:15px;height:15px;margin:0;accent-color:#50a41d}.log-console{height:calc(100vh - 350px);min-height:440px;margin:0;overflow:auto;padding:18px;border-radius:20px;background:#111827!important;color:#e5e7eb!important;font:12px/1.65 ui-monospace,SFMono-Regular,Menlo,monospace;caret-color:#9fe870}.log-line{display:grid;min-width:max-content;grid-template-columns:24ch 7ch 18ch minmax(max-content,1fr);gap:10px;white-space:pre}.log-time{color:#93c5fd}.log-level{font-weight:800}.level-trace,.level-debug{color:#c4b5fd}.level-info{color:#9fe870}.level-warn,.level-warning{color:#fbbf24}.level-error,.level-fatal{color:#fb7185}.log-device{color:#67e8f9}.log-message{color:#e5e7eb}.log-console.wrap .log-line{min-width:0;grid-template-columns:24ch 7ch minmax(110px,18ch) minmax(0,1fr);white-space:normal}.log-console.wrap .log-message{white-space:pre-wrap;overflow-wrap:anywhere}.log-empty{color:#9ca3af}
.dark .log-toggle.pause{background:#4a2b12!important;color:#ffc47d!important}.dark .log-toggle.pause:hover{background:#593514!important}
@media(max-width:760px){.log-toolbar{align-items:flex-start}.log-filter-controls{min-width:0;flex-basis:100%}.log-meta{justify-content:flex-start;flex-wrap:wrap}.log-console{height:520px}.log-console.wrap .log-line{grid-template-columns:1fr 7ch}.log-console.wrap .log-device,.log-console.wrap .log-message{grid-column:1/-1}}
</style>
