<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Modal, message } from 'antdv-next'
import jsQR from 'jsqr'
import PageHeader from '@/components/PageHeader.vue'
import PasswordInput from '@/components/PasswordInput.vue'
import EmptyState from '@/components/EmptyState.vue'
import EgressIps from '@/components/EgressIps.vue'
import RiIcon from '@/components/RiIcon.vue'
import { apiError, request } from '@/api/http'
import type { ManagedProxyNode, ManagedProxySource } from '@/types/domain'
import { mergeProxyCountries } from '@/utils/proxyCountries'

const tab = ref('managed')
const state = ref<any>({ sources: [], nodes: [], version: 0, enabled: false, running: false, selected: '' })
const loading = ref(false)
const busy = ref(false)
const collapsed = reactive<Record<string, boolean>>({})
const delays = reactive<Record<string, number | null>>({})
const testBusy = ref('')
let timer = 0

const addOpen = ref(false)
const addForm = reactive({ name: '新订阅', kind: 'subscription', content: '' })
const qrBusy = ref(false)
const qrInput = ref<HTMLInputElement>()

const roamRows = ref<any[]>([])
const countries = ref<any[]>([])
const countryRules = ref<any[]>([])
const roamingLoading = ref(false)
const roamingOpen = ref(false)
const roamingForm = reactive<any>({ id: '', name: '', addr: '', username: '', password: '', enabled: true })
const countryRulesOpen = ref(false)
const countryRuleProxy = ref<any | null>(null)
const selectedCountry = ref('')
const countryRuleBusy = ref(false)

const outbound = ref<any>({ instances: [], devices: [], status: [] })
const outboundLoading = ref(false)
const outboundOpen = ref(false)
const outboundForm = reactive<any>({ id: '', name: '', device_id: '', enabled: false, mode: 'socks5', listen_addr: '0.0.0.0', listen_port: 1080, auth_enabled: false, username: '', password: '' })

const selectedNode = computed(() => state.value.nodes.find((node: any) => node.id === state.value.selected))
const latencyBadge = computed(() => state.value.enabled && selectedNode.value && Number.isFinite(delays[selectedNode.value.id]) ? `${Math.min(999, Number(delays[selectedNode.value.id]))}ms` : '')
const isBuiltInSource = (source: ManagedProxySource) => Boolean(source.built_in) || source.id === 'builtin' || source.id === 'built-in' || source.kind === 'built_in' || source.kind === 'builtin'
const selectedProxyRules = computed(() => {
  const proxyId = countryRuleProxy.value?.id
  if (!proxyId) return []
  return countryRules.value.filter((rule) => rule.upstream_proxy_id === proxyId).map((rule) => {
    const country = countries.value.find((item) => countryCode(item) === String(rule.country_code)) || {}
    return { ...country, ...rule, country_code: String(rule.country_code), country_name: country.country_name || country.name || rule.country_name || rule.country_code, mccs: country.mccs || rule.mccs || [] }
  })
})
const countryOptions = computed(() => {
  const proxyId = countryRuleProxy.value?.id
  return countries.value.flatMap((country) => {
    const code = countryCode(country)
    if (!code) return []
    const rule = countryRules.value.find((item) => String(item.country_code) === code)
    if (rule && rule.upstream_proxy_id !== proxyId) return []
    return [{ label: `${countryLabel(country)}${rule ? ' · 已配置' : ''}`, value: code, configured: Boolean(rule) }]
  })
})

function countryCode(country: any) { return String(country?.country_code || country?.code || '') }
function countryLabel(country: any) {
  const code = countryCode(country)
  const name = country?.country_name || country?.name || code
  const mcc = Array.isArray(country?.mccs) && country.mccs.length ? ` · MCC ${country.mccs.join('/')}` : ''
  return `${code} · ${name}${mcc}`
}

async function managedRequest(action = '', body?: Record<string, any>) {
  return request<any>({ method: body ? 'POST' : 'GET', url: `/managed-proxy${action ? `/${action}` : ''}`, data: body })
}

async function refreshManaged(silent = false) {
  if (!silent) loading.value = true
  try {
    state.value = await managedRequest()
    for (const source of state.value.sources || []) if (!(source.id in collapsed)) collapsed[source.id] = true
  } catch (reason) {
    if (!silent) message.error(apiError(reason).message)
  } finally {
    loading.value = false
  }
}

async function mutate(action: string, payload: Record<string, any> = {}) {
  busy.value = true
  try {
    state.value = await managedRequest(action, { version: state.value.version, ...payload })
    if (action === 'select') await testNode(state.value.nodes.find((node: any) => node.id === state.value.selected), true)
  } catch (reason) {
    message.error(apiError(reason).message)
    await refreshManaged(true)
  } finally {
    busy.value = false
  }
}

async function toggleConnection() {
  if (state.value.enabled) {
    await mutate('pause')
  } else if (state.value.selected) {
    await mutate('select', { node_id: state.value.selected })
  } else {
    message.warning('请先选择一个节点')
  }
}

async function testNode(node?: ManagedProxyNode, quiet = false) {
  if (!node) return
  testBusy.value = node.id
  try {
    const result = await managedRequest('test', { node_id: node.id })
    delays[node.id] = Number(result.delay_ms)
  } catch {
    delays[node.id] = null
    if (!quiet) message.error(`${node.name} 连接失败或超时`)
  } finally {
    testBusy.value = ''
  }
}

async function testSource(source: ManagedProxySource) {
  const nodes = (state.value.nodes || []).filter((node: ManagedProxyNode) => node.source_id === source.id)
  for (const node of nodes) await testNode(node, true)
}

async function saveSource() {
  const values = addForm.kind === 'subscription' ? [...new Set(addForm.content.split(/\r?\n/).map((value) => value.trim()).filter(Boolean))] : [addForm.content.trim()]
  if (!values.length || !values[0]) return message.warning('请填写订阅地址或节点内容')
  if (addForm.kind === 'subscription' && values.some((value) => { try { return new URL(value).protocol !== 'https:' } catch { return true } })) return message.error('每行填写一个完整的 HTTPS 订阅链接')
  busy.value = true
  try {
    for (const [index, content] of values.entries()) {
      state.value = await managedRequest('import', { version: state.value.version, name: values.length > 1 ? `${addForm.name} ${index + 1}` : addForm.name, kind: addForm.kind, content })
    }
    addOpen.value = false
    addForm.content = ''
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    busy.value = false
  }
}

async function decodeQr(file?: File | null) {
  if (!file) return
  qrBusy.value = true
  try {
    const bitmap = await createImageBitmap(file)
    const canvas = document.createElement('canvas')
    canvas.width = bitmap.width
    canvas.height = bitmap.height
    const context = canvas.getContext('2d', { willReadFrequently: true })!
    context.drawImage(bitmap, 0, 0)
    const image = context.getImageData(0, 0, bitmap.width, bitmap.height)
    const code = jsQR(image.data, image.width, image.height)
    if (!code?.data) throw new Error('二维码识别失败')
    addForm.content = code.data
    addForm.kind = /^https:\/\//i.test(code.data) ? 'subscription' : 'nodes'
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    qrBusy.value = false
    if (qrInput.value) qrInput.value.value = ''
  }
}

async function deleteSource(source: ManagedProxySource) {
  Modal.confirm({ title: '删除来源', content: `删除“${source.name}”及其导入的节点？`, okText: '删除', okButtonProps: { danger: true }, cancelText: '取消', onOk: () => mutate('delete', { source_id: source.id }) })
}

async function refreshRoaming() {
  roamingLoading.value = true
  try {
    const [rows, countryList, rules] = await Promise.all([
      request<any[]>({ url: '/upstream-proxies' }),
      request<any[]>({ url: '/upstream-proxy-countries' }),
      request<any[]>({ url: '/upstream-proxy-country-rules' }),
    ])
    roamRows.value = rows || []
    countries.value = mergeProxyCountries(countryList)
    countryRules.value = rules || []
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    roamingLoading.value = false
  }
}

function editRoaming(row?: any) {
  const addr = row?.addr || [row?.host, row?.port].filter(Boolean).join(':')
  Object.assign(roamingForm, row ? { ...row, addr, password: row.password === '****' ? '' : row.password } : { id: '', name: '', addr: '', username: '', password: '', enabled: true })
  roamingOpen.value = true
}

async function saveRoaming() {
  const id = String(roamingForm.id || '').trim()
  const addr = String(roamingForm.addr || '').trim()
  if (!id || !roamingForm.name?.trim() || !addr) return message.warning('请填写代理 ID、名称和 Socks5 地址')
  if (!addr.startsWith('[') && (addr.match(/:/g) || []).length > 1) return message.warning('IPv6 地址请使用 [IPv6]:端口，例如 [2001:db8::1]:1080')
  try {
    const editing = roamRows.value.some((row) => row.id === id)
    const data = { id, name: roamingForm.name.trim(), addr, username: String(roamingForm.username || '').trim(), password: roamingForm.password || '', enabled: Boolean(roamingForm.enabled) }
    await request({ method: editing ? 'PUT' : 'POST', url: editing ? `/upstream-proxies/${id}` : '/upstream-proxies', data })
    roamingOpen.value = false
    await refreshRoaming()
  } catch (reason) {
    message.error(apiError(reason).message)
  }
}

async function toggleRoaming(row: any, enabled: boolean) {
  try {
    await request({ method: 'PUT', url: `/upstream-proxies/${row.id}`, data: row.id === 'vohive-mihomo' ? { enabled } : { ...row, enabled, password: row.password === '****' ? '' : row.password } })
    await refreshRoaming()
  } catch (reason) {
    message.error(apiError(reason).message)
  }
}

async function removeRoaming(row: any) {
  Modal.confirm({ title: '删除漫游前置代理', content: `确定删除“${row.name || row.id}”？`, okText: '删除', okButtonProps: { danger: true }, cancelText: '取消', onOk: async () => { await request({ method: 'DELETE', url: `/upstream-proxies/${row.id}` }); await refreshRoaming() } })
}

function openCountryRules(row: any) {
  countryRuleProxy.value = row
  selectedCountry.value = ''
  countryRulesOpen.value = true
}

async function saveCountryRule() {
  if (!countryRuleProxy.value || !selectedCountry.value) return message.warning('请选择国家')
  countryRuleBusy.value = true
  try {
    await request({ method: 'PUT', url: `/upstream-proxy-country-rules/${encodeURIComponent(selectedCountry.value)}`, data: { upstream_proxy_id: countryRuleProxy.value.id, enabled: true } })
    selectedCountry.value = ''
    await refreshRoaming()
    message.success('国家规则已保存')
  } catch (reason) { message.error(apiError(reason).message) } finally { countryRuleBusy.value = false }
}

async function deleteCountryRule(code: string) {
  countryRuleBusy.value = true
  try {
    await request({ method: 'DELETE', url: `/upstream-proxy-country-rules/${encodeURIComponent(code)}` })
    await refreshRoaming()
    message.success('国家规则已删除，该国家将默认直连')
  } catch (reason) { message.error(apiError(reason).message) } finally { countryRuleBusy.value = false }
}

async function refreshOutbound() {
  outboundLoading.value = true
  try {
    outbound.value = await request({ url: '/proxy-instances/overview' })
  } catch (reason) {
    message.error(apiError(reason).message)
  } finally {
    outboundLoading.value = false
  }
}

async function editOutbound(row?: any) {
  if (!row) {
    Object.assign(outboundForm, { id: `proxy-${Date.now()}`, name: '', device_id: outbound.value.devices?.[0]?.id || '', enabled: false, mode: 'socks5', listen_addr: '0.0.0.0', listen_port: 10800 + (outbound.value.instances || []).length, auth_enabled: false, username: '', password: '' })
  } else {
    let listenAddr = row.listen_addr || '0.0.0.0'
    let listenPort = Number(row.listen_port || 1080)
    if (row.listen && !row.listen_addr) {
      const match = String(row.listen).match(/^\[([^\]]+)]:(\d+)$/) || String(row.listen).match(/^([^:]+):(\d+)$/)
      if (match) { listenAddr = match[1]; listenPort = Number(match[2]) }
    }
    Object.assign(outboundForm, { ...row, mode: row.mode || 'socks5', listen_addr: listenAddr, listen_port: listenPort, auth_enabled: Boolean(row.auth_enabled), username: row.username || '', password: row.password === '****' ? '' : row.password || '' })
  }
  outboundOpen.value = true
  if (row?.id) {
    try {
      const detail = await request<any>({ url: `/proxy-instances/${encodeURIComponent(row.id)}` })
      Object.assign(outboundForm, { ...outboundForm, ...detail, mode: detail.mode || outboundForm.mode })
    } catch {
      message.warning('读取完整实例配置失败，已使用概览数据')
    }
  }
}

async function saveOutbound() {
  if (!String(outboundForm.id || '').trim()) return message.warning('实例 ID 不能为空')
  if (!outboundForm.device_id) return message.warning('必须绑定设备')
  if (!['socks5', 'http'].includes(outboundForm.mode)) return message.warning('代理模式仅支持 SOCKS5 或 HTTP')
  if (Number(outboundForm.listen_port) < 1 || Number(outboundForm.listen_port) > 65535) return message.warning('监听端口无效')
  if (outboundForm.auth_enabled && (!String(outboundForm.username || '').trim() || !String(outboundForm.password || '').trim())) return message.warning('启用认证时必须填写用户名和密码')
  const saved = { ...outboundForm, id: String(outboundForm.id).trim(), name: String(outboundForm.name || '').trim(), listen_addr: String(outboundForm.listen_addr || '0.0.0.0').trim(), listen_port: Number(outboundForm.listen_port), username: outboundForm.auth_enabled ? String(outboundForm.username).trim() : '', password: outboundForm.auth_enabled ? String(outboundForm.password).trim() : '' }
  const list = [...(outbound.value.instances || [])]
  const index = list.findIndex((row: any) => row.id === outboundForm.id)
  if (index >= 0) list[index] = saved
  else list.push(saved)
  try {
    await saveOutboundList(list)
    outboundOpen.value = false
  } catch (reason) {
    message.error(apiError(reason).message)
  }
}

async function saveOutboundList(list: any[]) {
  await request({ method: 'PUT', url: '/proxy-instances/config', data: { instances: list } })
  await refreshOutbound()
}

async function toggleOutbound(row: any, enabled: boolean) {
  const list = (outbound.value.instances || []).map((item: any) => item.id === row.id ? { ...item, enabled } : item)
  try { await saveOutboundList(list) } catch (reason) { message.error(apiError(reason).message); await refreshOutbound() }
}

function removeOutbound(row: any) {
  Modal.confirm({ title: '删除本地出站实例', content: `确定删除“${row.name || row.id}”？`, okText: '删除', cancelText: '取消', okButtonProps: { danger: true }, onOk: () => saveOutboundList((outbound.value.instances || []).filter((item: any) => item.id !== row.id)) })
}

async function instanceAction(row: any, action: string) {
  await request({ method: 'POST', url: `/proxy-instances/${row.id}/actions/${action}` })
  await refreshOutbound()
}

function loadTab(key: string) {
  if (key === 'roaming' && !roamRows.value.length) refreshRoaming()
  if (key === 'outbound' && !(outbound.value.instances || []).length) refreshOutbound()
}

onMounted(() => {
  refreshManaged()
  timer = window.setInterval(() => refreshManaged(true), 30_000)
})
onBeforeUnmount(() => window.clearInterval(timer))
</script>

<template>
  <div class="proxy-page">
    <PageHeader title="代理管理" subtitle="管理本地出站代理和 VoWiFi 漫游前置代理">
      <template #actions>
        <a-button v-if="tab === 'managed'" class="ghost-button" :loading="loading" @click="refreshManaged()">刷新</a-button>
        <a-button v-if="tab === 'managed'" type="primary" @click="addOpen = true">添加订阅 / 节点</a-button>
        <a-button v-if="tab === 'roaming'" class="ghost-button" :loading="roamingLoading" @click="refreshRoaming">刷新</a-button>
        <a-button v-if="tab === 'roaming'" type="primary" @click="editRoaming()">新增代理</a-button>
        <a-button v-if="tab === 'outbound'" class="ghost-button" :loading="outboundLoading" @click="refreshOutbound">刷新</a-button>
        <a-button v-if="tab === 'outbound'" type="primary" @click="editOutbound()">新增实例</a-button>
      </template>
    </PageHeader>
    <a-tabs v-model:active-key="tab" class="proxy-tabs" @change="loadTab">
      <a-tab-pane key="managed">
        <template #tab><span>订阅与节点 <b v-if="latencyBadge" class="latency-badge">{{ latencyBadge }}</b></span></template>
        <section class="managed-panel">
          <div class="current-outlet"><div><span>当前出口</span><strong>{{ selectedNode?.name || '未选择节点' }}</strong><small>{{ state.enabled ? '已连接' : '未连接' }}</small></div><a-button :danger="state.enabled" :type="state.enabled ? 'primary' : 'primary'" :loading="busy" :disabled="!state.enabled && !selectedNode" @click="toggleConnection">{{ state.enabled ? '断开' : '连接' }}</a-button></div>
          <EgressIps />
          <a-spin :spinning="loading">
            <EmptyState v-if="!loading && !(state.sources || []).length" title="还没有导入节点" description="可一次添加多个订阅链接，每行一个" icon="global-line" />
            <div v-else class="source-list">
              <section v-for="source in state.sources as ManagedProxySource[]" :key="source.id" class="source-group">
                <header class="source-header"><button class="source-title" @click="collapsed[source.id] = !collapsed[source.id]"><RiIcon :name="collapsed[source.id] ? 'arrow-right-s-line' : 'arrow-down-s-line'" /><span><strong>{{ source.name }}</strong><small>{{ source.count ?? (state.nodes || []).filter((node:any) => node.source_id === source.id).length }} 个节点 · 更新时间 {{ source.updated ? new Date(Number(source.updated) * 1000).toLocaleString('zh-CN', { hour12: false }) : '—' }}</small></span></button><div class="row-actions"><a-button v-if="source.kind === 'subscription'" class="ghost-button" @click="mutate('refresh', { source_id: source.id })">更新</a-button><a-button class="ghost-button" @click="testSource(source)">一键测试</a-button><a-button v-if="!isBuiltInSource(source)" class="ghost-button danger" @click="deleteSource(source)">删除</a-button></div></header>
                <div v-if="!collapsed[source.id]" class="node-table"><div class="node-head"><span>节点名称</span><span>协议</span><span>延迟</span><span>操作</span></div><div class="node-scroll"><article v-for="node in (state.nodes || []).filter((item:any) => item.source_id === source.id)" :key="node.id" class="node-row" :class="{ selected: state.enabled && state.selected === node.id }"><div class="node-name"><strong>{{ node.name }}</strong><small v-if="state.enabled && state.selected === node.id">使用中</small></div><span>{{ String(node.type || '').toUpperCase() }}</span><span>{{ delays[node.id] == null ? '—' : `${delays[node.id]} ms` }}</span><div class="row-actions"><a-button class="ghost-button" :loading="testBusy === node.id" @click="testNode(node)">测试</a-button><a-button class="ghost-button" :disabled="state.enabled && state.selected === node.id" @click="mutate('select', { node_id: node.id })">{{ state.enabled && state.selected === node.id ? '使用中' : '使用' }}</a-button></div></article></div></div>
              </section>
            </div>
          </a-spin>
        </section>
      </a-tab-pane>

      <a-tab-pane key="roaming"><template #tab><span><RiIcon name="earth-line" :size="17" /> 漫游前置代理 <b v-if="roamRows.filter(row => row.enabled).length" class="count-badge">{{ roamRows.filter(row => row.enabled).length }}</b></span></template><section class="section-panel"><div class="section-title"><div><h2>VoWiFi 漫游前置代理</h2><p>VoWiFi 通过 Socks5 代理穿透连接海外运营商。注意 Socks5 端必须支持 UDP Associate</p></div></div><a-spin :spinning="roamingLoading"><EmptyState v-if="!roamRows.length" title="暂无漫游前置代理" icon="earth-line" /><div v-for="row in roamRows" :key="row.id" class="proxy-row"><div><strong>{{ row.name || row.id }}</strong><small>Socks5 · {{ row.addr || ([row.host || row.server || '127.0.0.1', row.port].filter(Boolean).join(':')) }}</small></div><a-switch :checked="Boolean(row.enabled)" :title="row.enabled ? '' : '禁用后绑定到该代理的国家规则会回退为直连'" @change="(value:boolean) => toggleRoaming(row,value)" /><span class="rule-count">{{ countryRules.filter(rule => rule.upstream_proxy_id === row.id).length }} 个国家规则</span><div class="row-actions"><a-button class="ghost-button" @click="openCountryRules(row)">国家规则</a-button><a-button v-if="row.id !== 'vohive-mihomo'" class="ghost-button" @click="editRoaming(row)">编辑</a-button><a-button v-if="row.id !== 'vohive-mihomo'" class="ghost-button danger" @click="removeRoaming(row)">删除</a-button></div></div></a-spin></section></a-tab-pane>

      <a-tab-pane key="outbound"><template #tab><span><RiIcon name="router-line" :size="17" /> 本地出站代理 <b v-if="(outbound.instances || []).filter((row:any) => row.enabled).length" class="count-badge">{{ (outbound.instances || []).filter((row:any) => row.enabled).length }}</b></span></template><section class="section-panel"><div class="section-title"><div><h2>本地出站代理</h2><p>为设备创建本地 SOCKS5 或 HTTP 出口</p></div></div><a-spin :spinning="outboundLoading"><EmptyState v-if="!(outbound.instances || []).length" title="暂无本地出站实例" icon="router-line" /><div v-for="row in outbound.instances || []" :key="row.id" class="proxy-row"><div><strong>{{ row.name || row.id }}</strong><small>{{ row.device_id || '未关联设备' }} · {{ String(row.mode || 'socks5').toUpperCase() }} · {{ row.listen || row.address || `${row.listen_addr || '0.0.0.0'}:${row.listen_port || '—'}` }}</small></div><a-switch :checked="Boolean(row.enabled)" @change="(value:boolean) => toggleOutbound(row, value)" /><span class="pill">{{ outbound.status?.find((item:any) => item.id === row.id)?.running ? '运行中' : '已停止' }}</span><div class="row-actions"><a-button class="ghost-button" @click="instanceAction(row, outbound.status?.find((item:any) => item.id === row.id)?.running ? 'stop' : 'start')">{{ outbound.status?.find((item:any) => item.id === row.id)?.running ? '停止' : '启动' }}</a-button><a-button class="ghost-button" @click="editOutbound(row)">编辑</a-button><a-button class="ghost-button danger" @click="removeOutbound(row)">删除</a-button></div></div></a-spin></section></a-tab-pane>
    </a-tabs>

    <a-modal v-model:open="addOpen" title="添加订阅 / 节点" :footer="null" @cancel="addForm.content = ''"><div class="form-stack"><label>名称<a-input v-model:value="addForm.name" /></label><label>导入方式<a-select v-model:value="addForm.kind" style="width:100%" :options="[{ label: '订阅链接', value: 'subscription' }, { label: '节点链接 / Clash YAML', value: 'nodes' }]" /></label><label>{{ addForm.kind === 'subscription' ? 'HTTPS 订阅地址（每行一个）' : '节点内容' }}<a-textarea v-model:value="addForm.content" :auto-size="{ minRows: 5 }" /></label><input ref="qrInput" type="file" accept="image/png,image/jpeg,image/webp" hidden @change="decodeQr(($event.target as HTMLInputElement).files?.[0])" /><a-button class="outline-button" :loading="qrBusy" @click="qrInput?.click()"><RiIcon name="image-fill" :size="17" /> 上传二维码图片</a-button><p class="muted">图片仅在浏览器内识别，完成后立即清空，不上传、不缓存。</p><div class="dialog-actions"><a-button @click="addOpen = false">取消</a-button><a-button type="primary" :loading="busy" @click="saveSource">确认导入</a-button></div></div></a-modal>
    <a-modal v-model:open="roamingOpen" :title="roamRows.some(row => row.id === roamingForm.id) ? '编辑漫游前置代理' : '新增漫游前置代理'" ok-text="保存" cancel-text="取消" @ok="saveRoaming"><div class="form-stack"><div class="form-grid"><label>代理 ID<a-input v-model:value="roamingForm.id" :disabled="roamRows.some(row => row.id === roamingForm.id)" placeholder="唯一标识，如 jp-proxy-01" /></label><label>名称<a-input v-model:value="roamingForm.name" placeholder="例如：日本代理" /></label></div><label>Socks5 地址<a-input v-model:value="roamingForm.addr" placeholder="host:port，例如 1.2.3.4:1080" /><small>IPv6 请使用 [IPv6]:端口；保存时服务端会检查 Socks5 握手与 UDP Associate。</small></label><div class="form-switch"><div><strong>启用代理</strong><small>禁用后绑定到该代理的国家规则会回退为直连</small></div><a-switch v-model:checked="roamingForm.enabled" /></div><div class="form-grid"><label>用户名（可选）<a-input v-model:value="roamingForm.username" placeholder="留空则免鉴权" /></label><label>密码（可选）<PasswordInput v-model:value="roamingForm.password" /></label></div></div></a-modal>

    <a-modal v-model:open="countryRulesOpen" :title="`国家规则 — ${countryRuleProxy?.name || countryRuleProxy?.id || ''}`" :footer="null">
      <div class="country-rule-form">
        <section><h3>已路由到该代理的国家</h3><EmptyState v-if="!selectedProxyRules.length" title="暂无国家规则" icon="earth-line" /><div v-else class="country-rule-list"><div v-for="rule in selectedProxyRules" :key="rule.country_code"><span><strong>{{ rule.country_code }} · {{ rule.country_name }}</strong><small v-if="rule.mccs?.length">MCC {{ rule.mccs.join('/') }}</small></span><a-button class="ghost-button danger" :loading="countryRuleBusy" @click="deleteCountryRule(rule.country_code)">删除规则</a-button></div></div></section>
        <section><h3 class="country-rule-heading"><span>添加国家规则</span><small>{{ countryOptions.length }} 个国家/地区可选</small></h3><a-select v-model:value="selectedCountry" show-search allow-clear placeholder="选择国家或输入国家代码、名称、MCC 搜索" :options="countryOptions" :filter-option="(input:string, option:any) => String(option.label).toLowerCase().includes(input.toLowerCase())"><template #option="option"><span class="country-option"><span>{{ option.label }}</span><small v-if="option.configured">已配置</small></span></template></a-select><p>规则按 SIM 归属 MCC 解析国家。例如 US 会覆盖 MCC 310/311/312/313/314/315/316 等表内分组；没有配置规则的国家默认直连。需要重启 VoWiFi 生效。</p><div class="dialog-actions"><a-button type="primary" :loading="countryRuleBusy" :disabled="!selectedCountry" @click="saveCountryRule">保存规则</a-button></div></section>
      </div>
    </a-modal>

    <a-modal v-model:open="outboundOpen" :title="(outbound.instances || []).some((row:any) => row.id === outboundForm.id) ? '编辑代理实例' : '新增代理实例'" ok-text="保存" cancel-text="取消" @ok="saveOutbound"><div class="form-stack"><div class="form-grid"><label>实例 ID<a-input v-model:value="outboundForm.id" :disabled="(outbound.instances || []).some((row:any) => row.id === outboundForm.id)" placeholder="唯一标识" /></label><label>名称<a-input v-model:value="outboundForm.name" placeholder="显示名称" /></label></div><label>绑定设备（必填）<a-select v-model:value="outboundForm.device_id" placeholder="选择设备" :options="(outbound.devices || []).map((row:any) => ({ label: `${row.name || row.id} (${row.interface || '—'})`, value: row.id }))" /></label><label>代理模式<a-select v-model:value="outboundForm.mode" placeholder="选择代理模式" :options="[{label:'SOCKS5',value:'socks5'},{label:'HTTP',value:'http'}]" /></label><div class="form-grid"><label>监听地址<a-input v-model:value="outboundForm.listen_addr" placeholder="0.0.0.0" /></label><label>监听端口<a-input-number v-model:value="outboundForm.listen_port" :min="1" :max="65535" style="width:100%" /></label></div><div class="form-switch"><div><strong>启用账号认证</strong><small>关闭后将允许免认证连接</small></div><a-switch v-model:checked="outboundForm.auth_enabled" /></div><div v-if="outboundForm.auth_enabled" class="form-grid"><label>用户名<a-input v-model:value="outboundForm.username" placeholder="例如 user01" /></label><label>密码<PasswordInput v-model:value="outboundForm.password" /></label></div></div></a-modal>
  </div>
</template>

<style scoped>
.proxy-tabs :deep(.ant-tabs-tab-btn>span){display:inline-flex;align-items:center;gap:7px}.latency-badge,.count-badge{display:inline-flex;min-width:20px;height:20px;align-items:center;justify-content:center;padding:0 6px;border-radius:999px;background:#50a41d;color:#fff;font-size:11px}.managed-panel{position:relative}.current-outlet{display:flex;align-items:center;justify-content:space-between;gap:20px;margin-bottom:6px;padding:8px 0}.current-outlet>div{display:flex;align-items:baseline;gap:18px}.current-outlet span,.current-outlet small{color:var(--vx-muted);font-size:13px}.current-outlet strong{font-size:18px}.source-list{margin-top:14px;padding:4px 24px;border-radius:22px;background:var(--vx-panel)}.source-group+.source-group{border-top:1px solid var(--vx-line)}.source-header{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:12px 0}.source-title{display:flex;min-width:0;align-items:center;gap:10px;padding:0;border:0;background:transparent;text-align:left;cursor:pointer}.source-title>span{display:flex;min-width:0;flex-direction:column}.source-title strong{overflow:hidden;font-size:18px;text-overflow:ellipsis;white-space:nowrap}.source-title small{margin-top:4px;color:var(--vx-muted);font-size:12px}.node-table{padding-bottom:10px}.node-head,.node-row{display:grid;grid-template-columns:minmax(180px,2.2fr) minmax(70px,1fr) minmax(90px,1fr) 150px;align-items:center;gap:16px}.node-head{padding:10px 12px;color:var(--vx-muted);font-size:12px}.node-scroll{max-height:min(420px,48vh);overflow-y:auto;overscroll-behavior:auto}.node-row{min-height:58px;padding:10px 12px;border-radius:12px;font-size:13px}.node-row:hover,.node-row.selected{background:var(--vx-soft)}.node-name{display:flex;min-width:0;align-items:center;gap:7px}.node-name strong{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.node-name small{color:#50a41d}.proxy-row{display:grid;grid-template-columns:minmax(220px,1fr) auto auto auto;align-items:center;gap:18px;padding:16px 0}.proxy-row+.proxy-row{border-top:1px solid var(--vx-line)}.proxy-row>div:first-child{display:flex;min-width:0;flex-direction:column}.proxy-row small{margin-top:4px;color:var(--vx-muted)}.rule-count{color:var(--vx-muted);font-size:12px}.form-stack,.form-grid{display:grid;gap:16px}.form-stack label,.form-grid label{display:grid;gap:7px;color:var(--vx-muted);font-size:13px;font-weight:650}.form-stack label small{font-weight:400;line-height:1.5}.form-grid{grid-template-columns:1fr 1fr}.form-switch{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:14px;border-radius:16px;background:var(--vx-neutral)}.form-switch>div{display:flex;flex-direction:column;gap:4px}.form-switch small{color:var(--vx-muted);font-size:12px}.country-rule-form{display:grid;gap:24px}.country-rule-form section{display:grid;gap:12px}.country-rule-form h3{margin:0;font-size:15px}.country-rule-heading{display:flex;align-items:baseline;justify-content:space-between;gap:16px}.country-rule-heading small{color:var(--vx-muted);font-size:11px;font-weight:500}.country-rule-form p{margin:0;color:var(--vx-muted);font-size:12px;line-height:1.65}.country-rule-list{display:grid;gap:8px}.country-rule-list>div{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:10px 12px;border-radius:14px;background:var(--vx-neutral)}.country-rule-list span{display:flex;min-width:0;flex-direction:column;gap:3px}.country-rule-list small{color:var(--vx-muted);font-size:11px}.country-option{display:flex;align-items:center;justify-content:space-between;gap:12px}.country-option small{color:#50a41d;font-size:11px}
@media(max-width:800px){.current-outlet,.source-header{align-items:flex-start;flex-direction:column}.node-head{display:none}.node-row{grid-template-columns:1fr auto}.node-name{grid-column:1/-1}.node-row>.row-actions{grid-column:2;grid-row:2/4}.proxy-row{grid-template-columns:1fr auto}.proxy-row>.row-actions{grid-column:1/-1;justify-content:flex-start}.form-grid{grid-template-columns:1fr}}
</style>
