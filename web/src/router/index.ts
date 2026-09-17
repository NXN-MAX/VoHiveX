import { createRouter, createWebHashHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('@/pages/LoginPage.vue') },
    {
      path: '/',
      component: () => import('@/layouts/AppShell.vue'),
      meta: { requiresAuth: true },
      children: [
        { path: '', name: 'dashboard', component: () => import('@/pages/DashboardPage.vue') },
        { path: 'devices', name: 'devices', component: () => import('@/pages/DevicesPage.vue') },
        { path: 'proxy', name: 'proxy', component: () => import('@/pages/ProxyPage.vue') },
        { path: 'sms', name: 'sms', component: () => import('@/pages/SmsPage.vue') },
        { path: 'tasks', name: 'tasks', component: () => import('@/pages/TasksPage.vue') },
        { path: 'push', name: 'push', component: () => import('@/pages/PushPage.vue') },
        { path: 'logs', name: 'logs', component: () => import('@/pages/LogsPage.vue') },
        { path: 'settings', name: 'settings', component: () => import('@/pages/SettingsPage.vue') },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (to.meta.requiresAuth && !auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && auth.isAuthenticated) return { name: 'dashboard' }
})

export default router
