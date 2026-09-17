<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { message } from 'antdv-next'
import { request } from '@/api/http'

const props = withDefaults(defineProps<{ compact?: boolean; moduleIp?: string; deviceId?: string; vowifiEnabled?: boolean; unavailableText?: string }>(), {
  compact: false,
  moduleIp: '',
  deviceId: '',
  vowifiEnabled: false,
  unavailableText: '',
})

const data = ref<Record<string, any> | null>(null)
const loading = ref(false)
const error = ref('')
let timer = 0

async function refresh(force = false) {
  if (props.compact && (props.unavailableText || !props.vowifiEnabled)) {
    data.value = null
    error.value = ''
    loading.value = false
    return
  }
  loading.value = true
  error.value = ''
  try {
    data.value = await request({ url: '/managed-proxy/public-ip', params: force ? { refresh: 1 } : undefined })
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '查询失败'
  } finally {
    loading.value = false
  }
}

async function copy(value?: string) {
  if (!value) return
  await navigator.clipboard.writeText(value)
  message.success('IP 已复制')
}

watch(() => [props.deviceId, props.vowifiEnabled, props.moduleIp, props.unavailableText], () => refresh())
onMounted(() => {
  refresh()
  timer = window.setInterval(() => refresh(), 60_000)
})
onBeforeUnmount(() => window.clearInterval(timer))
</script>

<template>
  <div v-if="compact" class="compact-ip">
    <span>{{ unavailableText ? 'IP 地址' : (data?.devices?.[deviceId]?.label === '代理 IP' ? '代理 IP' : 'IP 地址') }}</span>
    <strong :title="unavailableText || data?.devices?.[deviceId]?.ip || moduleIp || error || '尚未获取到 SIM 卡 IP'">
      {{ unavailableText || (vowifiEnabled ? (data?.devices?.[deviceId]?.ip || (loading ? '查询中…' : '暂不可用')) : (moduleIp || '暂不可用')) }}
    </strong>
  </div>
  <div v-else class="egress-row">
    <div class="egress-item">
      <span>设备公网IP</span>
      <button v-if="data?.nas?.ip" :title="data.nas.ip" @click="copy(data.nas.ip)">{{ data.nas.ip }}</button>
      <small v-else>{{ loading ? '查询中…' : (data?.nas?.error || error || '未查询') }}</small>
    </div>
    <div class="egress-item">
      <span>订阅代理出口 IP</span>
      <button v-if="data?.proxy?.ip" :title="data.proxy.ip" @click="copy(data.proxy.ip)">{{ data.proxy.ip }}</button>
      <small v-else>{{ loading ? '查询中…' : (data?.proxy?.error || error || '未查询') }}</small>
    </div>
    <a-button class="ghost-button" :loading="loading" @click="refresh(true)">刷新</a-button>
  </div>
</template>

<style scoped>
.compact-ip{display:flex;align-items:center;justify-content:space-between;gap:12px;font-size:12px}.compact-ip span{color:var(--vx-muted)}.compact-ip strong{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-family:ui-monospace,monospace;font-size:12px}
.egress-row{display:flex;align-items:center;gap:16px;flex-wrap:wrap}.egress-item{display:flex;align-items:center;gap:8px;min-width:0}.egress-item span{color:var(--vx-muted);font-size:13px}.egress-item button{max-width:280px;overflow:hidden;padding:0;border:0;background:transparent;color:var(--vx-ink);font-weight:700;text-overflow:ellipsis;white-space:nowrap;cursor:pointer}.egress-item small{max-width:260px;overflow:hidden;color:var(--vx-muted);text-overflow:ellipsis;white-space:nowrap}
</style>
