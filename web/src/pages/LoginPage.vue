<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import RiIcon from '@/components/RiIcon.vue'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const form = reactive({ username: '', password: '' })
const errors = reactive({ username: '', password: '', form: '' })
const loading = ref(false)
const showPassword = ref(false)

async function submit() {
  errors.username = form.username.trim() ? '' : '请填写用户名'
  errors.password = form.password ? '' : '请填写密码'
  errors.form = ''
  if (errors.username || errors.password || loading.value) return
  loading.value = true
  const ok = await auth.login(form.username.trim(), form.password)
  loading.value = false
  if (!ok) {
    errors.form = '登录失败，请检查用户名和密码，或稍后重试。'
    return
  }
  const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : sessionStorage.getItem('post_login_redirect') || '/'
  sessionStorage.removeItem('post_login_redirect')
  router.replace(redirect)
}
</script>

<template>
  <div class="login-page">
    <header class="login-brand">VoHive<span>X</span></header>
    <main class="login-main">
      <form class="login-form" novalidate @submit.prevent="submit">
        <h1>欢迎回来</h1>
        <div v-if="errors.form" class="login-error" role="alert">{{ errors.form }}</div>
        <label for="username">用户名</label>
        <a-input
          id="username"
          v-model:value="form.username"
          size="large"
          autocomplete="username"
          :status="errors.username ? 'error' : undefined"
          @input="errors.username = ''; errors.form = ''"
        />
        <p v-if="errors.username" class="field-error">{{ errors.username }}</p>
        <label for="password">密码</label>
        <div class="login-password">
          <a-input
            id="password"
            v-model:value="form.password"
            size="large"
            :type="showPassword ? 'text' : 'password'"
            autocomplete="current-password"
            :status="errors.password ? 'error' : undefined"
            @input="errors.password = ''; errors.form = ''"
          />
          <button type="button" class="password-toggle" :aria-label="showPassword ? '隐藏密码' : '显示密码'" @click="showPassword = !showPassword">
            <RiIcon :name="showPassword ? 'eye-off-line' : 'eye-line'" />
          </button>
        </div>
        <p v-if="errors.password" class="field-error">{{ errors.password }}</p>
        <a-button type="primary" html-type="submit" size="large" block :loading="loading">登录</a-button>
      </form>
    </main>
  </div>
</template>
