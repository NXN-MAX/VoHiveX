import axios, { AxiosError, type AxiosRequestConfig } from 'axios'

export interface ApiError {
  message: string
  status?: number
  method?: string
  url?: string
}

export const http = axios.create({
  baseURL: '/api',
  timeout: 120_000,
  headers: { 'Content-Type': 'application/json' },
})

const token = localStorage.getItem('token') || ''
if (token) http.defaults.headers.common.Authorization = `Bearer ${token}`

http.interceptors.response.use(
  (response) => response,
  (error: AxiosError) => {
    if (error.response?.status === 401) {
      const current = String(window.location.hash || '').replace(/^#/, '') || '/'
      if (!current.startsWith('/login')) sessionStorage.setItem('post_login_redirect', current)
      localStorage.removeItem('token')
      delete http.defaults.headers.common.Authorization
      window.location.hash = `#/login?redirect=${encodeURIComponent(current)}`
    }
    return Promise.reject(error)
  },
)

export function setToken(tokenValue: string) {
  if (tokenValue) {
    localStorage.setItem('token', tokenValue)
    http.defaults.headers.common.Authorization = `Bearer ${tokenValue}`
  } else {
    localStorage.removeItem('token')
    delete http.defaults.headers.common.Authorization
  }
}

export function apiError(error: unknown): ApiError {
  if (axios.isAxiosError(error)) {
    const body = error.response?.data as Record<string, unknown> | undefined
    return {
      message: String(body?.error || body?.message || error.message || '请求失败'),
      status: error.response?.status,
      method: error.config?.method?.toUpperCase(),
      url: error.config?.url,
    }
  }
  return { message: error instanceof Error ? error.message : '请求失败' }
}

export async function request<T>(config: AxiosRequestConfig): Promise<T> {
  const response = await http.request<T>(config)
  return response.data
}
