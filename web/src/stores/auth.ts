import { defineStore } from 'pinia'
import { http, setToken } from '@/api/http'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('token') || '',
    user: null as null | { name?: string },
  }),
  getters: {
    isAuthenticated: (state) => Boolean(state.token),
  },
  actions: {
    async login(username: string, password: string) {
      try {
        const { data } = await http.post('/auth/login', { username, password })
        const token = String(data?.token || '')
        if (!token) return false
        this.token = token
        this.user = { name: username }
        setToken(token)
        return true
      } catch {
        return false
      }
    },
    logout() {
      this.token = ''
      this.user = null
      setToken('')
    },
  },
})
