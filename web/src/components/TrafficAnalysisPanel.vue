<script setup lang="ts">
import { computed } from 'vue'

type Range = 'day' | 'week' | 'month'
type Bucket = { period_start?: string; bucket?: string; rx_bytes?: number; tx_bytes?: number; total_bytes?: number }

const props = withDefaults(defineProps<{
  analysis?: { buckets?: Bucket[] }
  range: Range
  loading?: boolean
  error?: string
  title?: string
  disabled?: boolean
  disabledText?: string
}>(), {
  analysis: () => ({ buckets: [] }),
  loading: false,
  error: '',
  title: '流量分析',
  disabled: false,
  disabledText: '网络已禁用，暂无流量分析',
})

const emit = defineEmits<{ 'update:range': [value: Range]; refresh: [] }>()
const buckets = computed(() => props.analysis?.buckets || [])
const totals = computed(() => buckets.value.reduce((result, row) => {
  result.rx += Number(row.rx_bytes || 0)
  result.tx += Number(row.tx_bytes || 0)
  return result
}, { rx: 0, tx: 0 }))
const hasTraffic = computed(() => totals.value.rx + totals.value.tx > 0)
const maximum = computed(() => Math.max(1, ...buckets.value.map((row) => Number(row.total_bytes || Number(row.rx_bytes || 0) + Number(row.tx_bytes || 0)))))
const periodName = computed(() => ({ day: '本日', week: '本周', month: '本月' }[props.range]))

function formatBytes(value: unknown) {
  let amount = Math.max(0, Number(value || 0))
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let unit = 0
  while (amount >= 1024 && unit < units.length - 1) { amount /= 1024; unit++ }
  return `${amount.toFixed(unit ? 2 : 0)} ${units[unit]}`
}

function label(row: Bucket) {
  const value = row.period_start || row.bucket || ''
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return value
  return props.range === 'day'
    ? `${String(date.getHours()).padStart(2, '0')}:00`
    : `${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

function height(row: Bucket) {
  const total = Number(row.total_bytes || Number(row.rx_bytes || 0) + Number(row.tx_bytes || 0))
  return `${Math.max(total ? 6 : 1, Math.round(total / maximum.value * 100))}%`
}
</script>

<template>
  <section class="traffic-panel section-panel">
    <header class="traffic-heading">
      <div><h2>{{ title }}</h2><p>数据每分钟采样一次，按日/周/月聚合</p></div>
      <div class="traffic-actions">
        <a-segmented :value="range" :disabled="disabled" :options="[{label:'日',value:'day'},{label:'周',value:'week'},{label:'月',value:'month'}]" @change="emit('update:range', $event as Range)" />
        <a-button class="ghost-button" :loading="loading" :disabled="disabled" @click="emit('refresh')">刷新</a-button>
      </div>
    </header>
    <div v-if="disabled" class="traffic-empty">{{ disabledText }}</div>
    <template v-else>
      <div class="traffic-stats">
        <div><span>{{ periodName }}下载</span><strong>{{ formatBytes(totals.rx) }}</strong></div>
        <div><span>{{ periodName }}上传</span><strong>{{ formatBytes(totals.tx) }}</strong></div>
        <div><span>{{ periodName }}合计</span><strong>{{ formatBytes(totals.rx + totals.tx) }}</strong></div>
      </div>
      <div v-if="error" class="traffic-state error-state"><strong>流量数据读取失败</strong><span>{{ error }}</span><a-button class="ghost-button" @click="emit('refresh')">重试</a-button></div>
      <div v-else-if="loading && !buckets.length" class="traffic-state">正在读取流量数据…</div>
      <div v-else-if="buckets.length && !hasTraffic" class="traffic-state zero-state"><strong>所选时段未产生流量</strong><span>采样正常，下载和上传均为 0 B</span></div>
      <div v-else-if="buckets.length" class="traffic-chart" aria-label="流量趋势图">
        <div v-for="(row,index) in buckets" :key="`${row.period_start || row.bucket}-${index}`" class="traffic-bar-column" :title="`${label(row)} · ${formatBytes(Number(row.rx_bytes || 0) + Number(row.tx_bytes || 0))}`">
          <span class="traffic-bar" :style="{ height: height(row) }" />
          <small>{{ label(row) }}</small>
        </div>
      </div>
      <div v-else class="traffic-state"><strong>暂无流量图表数据</strong><span>当前时段还没有采样记录</span></div>
      <div v-if="buckets.length && hasTraffic && !error" class="traffic-table-wrap">
        <table><thead><tr><th>时间</th><th>下载</th><th>上传</th><th>合计</th></tr></thead><tbody><tr v-for="(row,index) in buckets" :key="index"><td>{{ label(row) }}</td><td>{{ formatBytes(row.rx_bytes) }}</td><td>{{ formatBytes(row.tx_bytes) }}</td><td>{{ formatBytes(Number(row.rx_bytes || 0) + Number(row.tx_bytes || 0)) }}</td></tr></tbody></table>
      </div>
    </template>
  </section>
</template>

<style scoped>
.traffic-panel{margin-top:28px}.traffic-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:20px}.traffic-heading h2{margin:0;color:var(--vx-ink);font-size:20px}.traffic-heading p{margin:5px 0 0;color:var(--vx-muted);font-size:13px}.traffic-actions{display:flex;align-items:center;gap:12px}.traffic-stats{display:grid;grid-template-columns:repeat(3,1fr);gap:14px;margin:22px 0}.traffic-stats>div{display:flex;flex-direction:column;gap:6px;padding:16px;border-radius:16px;background:color-mix(in srgb,var(--vx-canvas) 60%,transparent)}.traffic-stats span{color:var(--vx-muted);font-size:12px}.traffic-stats strong{color:var(--vx-ink);font-size:20px;font-variant-numeric:tabular-nums}.traffic-chart{display:flex;height:210px;align-items:flex-end;gap:clamp(4px,1vw,12px);padding:22px 14px 0;border-radius:16px;background:color-mix(in srgb,var(--vx-canvas) 56%,transparent)}.traffic-bar-column{display:flex;min-width:0;max-width:52px;height:100%;flex:1;align-items:center;justify-content:flex-end;flex-direction:column;gap:8px}.traffic-bar{width:min(72%,24px);min-height:2px;border-radius:999px 999px 3px 3px;background:var(--vx-accent-hover)}.traffic-bar-column small{width:100%;overflow:hidden;color:var(--vx-muted);font-size:10px;text-align:center;text-overflow:ellipsis;white-space:nowrap}.traffic-empty,.traffic-state{display:grid;min-height:180px;place-content:center;justify-items:center;gap:6px;padding:24px;border-radius:16px;background:color-mix(in srgb,var(--vx-canvas) 56%,transparent);color:var(--vx-muted);font-size:13px;text-align:center}.traffic-state strong{color:var(--vx-ink);font-size:15px}.traffic-state span{max-width:560px;line-height:1.55}.traffic-state.error-state strong{color:var(--vx-danger)}.traffic-state.zero-state:before{width:52px;height:4px;border-radius:999px;background:var(--vx-line);content:''}.traffic-table-wrap{max-height:240px;margin-top:18px;overflow:auto}.traffic-table-wrap table{width:100%;border-collapse:collapse;font-size:12px}.traffic-table-wrap th,.traffic-table-wrap td{padding:10px 12px;border-bottom:1px solid color-mix(in srgb,var(--vx-line) 65%,transparent);text-align:left}.traffic-table-wrap th{color:var(--vx-muted);font-weight:650}.traffic-table-wrap td{color:var(--vx-text);font-variant-numeric:tabular-nums}
@media(max-width:700px){.traffic-heading{flex-direction:column}.traffic-actions{width:100%;justify-content:space-between}.traffic-stats{grid-template-columns:1fr}.traffic-chart{height:170px}.traffic-table-wrap{overflow-x:auto}.traffic-table-wrap table{min-width:520px}}
</style>
