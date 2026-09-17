<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { message } from 'antdv-next'
import PageHeader from '@/components/PageHeader.vue'
import { apiError, request } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useRouter } from 'vue-router'
import PasswordInput from '@/components/PasswordInput.vue'

const router = useRouter()
const auth = useAuthStore()
const username = reactive({ current: '', value: '', password: '' })
const password = reactive({ old_password: '', new_password: '', confirm_password: '' })
const info = ref<any>(null)
const serverNow = ref(0)
const received = ref(0)
const loading = ref(false)
let clock = 0
let sync = 0
function timestamp(value: unknown) {
  if (value instanceof Date) return value.getTime()
  const number = Number(value)
  if (Number.isFinite(number) && number > 0) return number < 1e12 ? number * 1000 : number
  const parsed = Date.parse(String(value || ''))
  return Number.isFinite(parsed) ? parsed : 0
}
function format(value: unknown) {
  const valueMs = timestamp(value)
  return valueMs ? new Date(valueMs).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false }) : '未记录'
}
async function load() {
  loading.value = true
  try {
    const [user, system] = await Promise.all([request<any>({ url: '/settings/username' }), request<any>({ url: '/settings/system' })])
    username.current = user.username || ''; username.value = username.current; info.value = system; serverNow.value = timestamp(system.system_time) || Date.now(); received.value = performance.now()
  } catch (reason) { message.error(apiError(reason).message) } finally { loading.value = false }
}
async function changeUsername() {
  try { await request({ method: 'POST', url: '/settings/username', data: { username: username.value, current_password: username.password } }); message.success('用户名已修改，请重新登录'); auth.logout(); router.replace('/login') } catch (reason) { message.error(apiError(reason).message) }
}
async function changePassword() {
  if (password.new_password !== password.confirm_password) return message.error('两次输入的新密码不一致')
  try { await request({ method: 'POST', url: '/settings/password', data: password }); message.success('密码已修改'); Object.assign(password, { old_password: '', new_password: '', confirm_password: '' }) } catch (reason) { message.error(apiError(reason).message) }
}
async function copyPath() { try { await navigator.clipboard.writeText(String(info.value?.config_path || '')); message.success('配置文件路径已复制') } catch { message.error('复制失败，请手动选择路径') } }
onMounted(() => { load(); clock = window.setInterval(() => serverNow.value = (timestamp(info.value?.system_time) || Date.now()) + performance.now() - received.value, 1000); sync = window.setInterval(load, 60_000) })
onBeforeUnmount(() => { window.clearInterval(clock); window.clearInterval(sync) })
</script>

<template><div class="settings-page"><PageHeader title="系统设置" subtitle="管理访问凭证与查看运行信息" /><a-spin :spinning="loading"><section class="settings-grid"><article class="setting-card"><h2>用户名修改</h2><p>当前用户名：{{ username.current || '—' }}</p><a-form layout="vertical"><a-form-item label="新用户名"><a-input v-model:value="username.value" /></a-form-item><a-form-item label="当前密码"><PasswordInput v-model:value="username.password" /></a-form-item><a-button type="primary" @click="changeUsername">修改用户名</a-button></a-form></article><article class="setting-card"><h2>密码修改</h2><p>更新后台管理登录密码</p><a-form layout="vertical"><a-form-item label="当前密码"><PasswordInput v-model:value="password.old_password" /></a-form-item><a-form-item label="新密码"><PasswordInput v-model:value="password.new_password" /></a-form-item><a-form-item label="确认新密码"><PasswordInput v-model:value="password.confirm_password" /></a-form-item><a-button type="primary" @click="changePassword">修改密码</a-button></a-form></article><article class="setting-card version-card"><h2>版本信息</h2><dl><div><dt>原作者</dt><dd><a href="https://github.com/iniwex5" target="_blank" rel="noopener">iniwex5</a></dd></div><div><dt>VoHive<span>X</span> 作者</dt><dd><a href="https://github.com/NXN-MAX" target="_blank" rel="noopener">NXN-MAX</a></dd></div><div><dt>版本号</dt><dd>2.1.1</dd></div></dl></article></section><section class="system-card"><h2>系统信息</h2><dl><div><dt>系统时间</dt><dd>{{ format(serverNow) }}</dd></div><div><dt>构建时间</dt><dd>{{ format(info?.build_time) }}</dd></div><div><dt>驱动版本号</dt><dd>{{ info?.driver_version || '暂不可用' }}</dd></div><div><dt>代理模块版本号</dt><dd>{{ info?.proxy_version || '暂不可用' }}</dd></div><div><dt>配置文件</dt><dd><button class="copy-path" :title="info?.config_path" @click="copyPath">{{ info?.config_path || '—' }}</button></dd></div><div><dt>API 文档</dt><dd><a href="/api/docs" target="_blank" rel="noopener noreferrer">查看本机接口说明</a></dd></div></dl></section></a-spin></div></template>

<style scoped>.settings-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px}.setting-card,.system-card{padding:24px;border-radius:22px;background:var(--vx-panel)}.setting-card h2,.system-card h2{margin:0;color:var(--vx-ink);font-size:20px}.setting-card>p{margin:6px 0 20px;color:var(--vx-muted);font-size:13px}.version-card dl,.system-card dl{margin:20px 0 0}.version-card dl div,.system-card dl div{display:grid;grid-template-columns:120px minmax(0,1fr);gap:16px;padding:11px 0}.version-card dt,.system-card dt{color:var(--vx-muted)}.version-card dd,.system-card dd{min-width:0;margin:0;font-weight:650}.version-card h2 span,.version-card dt span{color:#50a41d}.version-card a,.system-card a{text-decoration:underline;text-underline-offset:3px}.system-card{margin-top:16px}.system-card dl{display:grid;grid-template-columns:1fr 1fr;column-gap:40px}.copy-path{display:block;max-width:100%;padding:0;border:0;background:transparent;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;cursor:pointer;text-decoration:underline;text-underline-offset:3px}@media(max-width:900px){.settings-grid{grid-template-columns:1fr}.system-card dl{grid-template-columns:1fr}}@media(max-width:500px){.system-card dl div{grid-template-columns:1fr;gap:4px}}</style>
