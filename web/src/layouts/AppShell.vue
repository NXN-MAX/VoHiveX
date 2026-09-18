<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import RiIcon from '@/components/RiIcon.vue'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const ui = useUiStore()
const backendConnected = ref<boolean | null>(null)
let heartbeatTimer = 0
let heartbeatController: AbortController | null = null

const items = [
  { to: '/', label: '仪表盘', icon: 'dashboard-line' },
  { to: '/devices', label: '设备管理', icon: 'smartphone-line' },
  { to: '/proxy', label: '代理管理', icon: 'global-line' },
  { to: '/sms', label: '短信中心', icon: 'message-2-line' },
  { to: '/tasks', label: '定时任务', icon: 'calendar-schedule-line' },
  { to: '/push', label: '消息推送', icon: 'notification-3-line' },
  { to: '/logs', label: '实时日志', icon: 'file-list-3-line' },
  { to: '/settings', label: '系统设置', icon: 'settings-3-line' },
]

const title = computed(() => items.find((item) => item.to === route.path)?.label || 'VoHiveX')

function logout() {
  auth.logout()
  router.replace('/login')
}

function closeMobile() {
  ui.mobileOpen = false
}

async function checkHeartbeat() {
  if (heartbeatController) return
  if (!navigator.onLine) {
    backendConnected.value = false
    return
  }
  const controller = new AbortController()
  heartbeatController = controller
  const timeout = window.setTimeout(() => controller.abort(), 4_000)
  try {
    const response = await fetch(`/healthz?_=${Date.now()}`, {
      cache: 'no-store',
      headers: { Accept: 'application/json' },
      signal: controller.signal,
    })
    backendConnected.value = response.ok
  } catch {
    backendConnected.value = false
  } finally {
    window.clearTimeout(timeout)
    if (heartbeatController === controller) heartbeatController = null
  }
}

onMounted(() => {
  ui.applyTheme()
  void checkHeartbeat()
  heartbeatTimer = window.setInterval(checkHeartbeat, 10_000)
})
onBeforeUnmount(() => {
  window.clearInterval(heartbeatTimer)
  heartbeatController?.abort()
  heartbeatController = null
})
</script>

<template>
  <div class="app-shell" :class="{ 'is-collapsed': ui.collapsed }">
    <div v-if="ui.mobileOpen" class="mobile-mask" @click="closeMobile" />
    <aside class="sidebar" :class="{ 'mobile-open': ui.mobileOpen }">
      <div class="sidebar-brand">
        <span class="brand-word">VoHive<span>X</span></span>
        <a-tooltip :title="ui.collapsed ? '展开导航' : '收起导航'" placement="right"><button class="icon-button desktop-collapse" :aria-label="ui.collapsed ? '展开导航' : '收起导航'" @click="ui.toggleCollapsed"><RiIcon :name="ui.collapsed ? 'menu-unfold-line' : 'menu-fold-line'" /></button></a-tooltip>
        <a-tooltip title="关闭导航" placement="right"><button class="icon-button mobile-close" aria-label="关闭导航" @click="closeMobile"><RiIcon name="close-line" /></button></a-tooltip>
      </div>

      <nav class="sidebar-nav" aria-label="主导航">
        <a-tooltip v-for="item in items" :key="item.to" :title="ui.collapsed ? item.label : ''" placement="right">
          <RouterLink :to="item.to" class="nav-link" @click="closeMobile">
            <RiIcon :name="item.icon" :size="19" />
            <span>{{ item.label }}</span>
          </RouterLink>
        </a-tooltip>
      </nav>

      <div class="sidebar-user">
        <RiIcon name="ghost-line" :size="21" />
        <div class="user-copy">
          <strong>{{ auth.user?.name || 'Admin' }}</strong>
          <small>Administrator</small>
        </div>
        <a-tooltip title="退出登录" placement="top">
          <button class="icon-button logout-button" aria-label="退出登录" @click="logout">
            <RiIcon name="logout-box-r-line" />
          </button>
        </a-tooltip>
      </div>
    </aside>

    <section class="workspace">
      <header class="mobile-header">
        <a-tooltip title="打开导航" placement="bottom"><button class="icon-button" aria-label="打开导航" @click="ui.mobileOpen = true"><RiIcon name="menu-unfold-line" /></button></a-tooltip>
        <span class="mobile-brand">VoHive<span>X</span></span>
        <div class="mobile-topbar-tools">
          <a-tooltip :title="ui.dark ? '切换为浅色模式' : '切换为深色模式'" placement="bottom"><button class="appearance-switch" :aria-label="ui.dark ? '切换为浅色模式' : '切换为深色模式'" @click="ui.toggleTheme"><span class="appearance-track"><span class="appearance-thumb"><RiIcon :name="ui.dark ? 'moon-line' : 'sun-line'" :size="13" /></span></span></button></a-tooltip>
          <span
            class="heartbeat-dot"
            :class="{ connected: backendConnected === true, disconnected: backendConnected === false }"
            role="status"
            :aria-label="backendConnected === null ? '正在检查后台连接' : backendConnected ? '后台连接正常' : '后台连接已断开'"
          />
        </div>
      </header>
      <header class="topbar">
        <span>{{ title }}</span>
        <div class="topbar-tools">
          <a-tooltip :title="ui.dark ? '切换为浅色模式' : '切换为深色模式'" placement="bottom"><button class="appearance-switch" :aria-label="ui.dark ? '切换为浅色模式' : '切换为深色模式'" @click="ui.toggleTheme"><span class="appearance-track"><span class="appearance-thumb"><RiIcon :name="ui.dark ? 'moon-line' : 'sun-line'" :size="13" /></span></span></button></a-tooltip>
          <span
            class="heartbeat-dot"
            :class="{ connected: backendConnected === true, disconnected: backendConnected === false }"
            role="status"
            :aria-label="backendConnected === null ? '正在检查后台连接' : backendConnected ? '后台连接正常' : '后台连接已断开'"
          />
        </div>
      </header>
      <main class="content-grid"><RouterView /></main>
    </section>
  </div>
</template>
