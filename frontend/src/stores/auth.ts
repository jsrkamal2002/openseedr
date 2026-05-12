import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import api from '@/composables/useApi'

export interface User {
  id: string
  email: string
  username: string
  avatar_url?: string
  is_admin: boolean
  is_active: boolean
  storage_quota: number
  storage_used: number
  download_limit: number
  upload_limit: number
  provider: string
  created_at: string
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(localStorage.getItem('token'))
  const user = ref<User | null>(null)

  const isAuthenticated = computed(() => !!token.value)

  async function login(email: string, password: string) {
    const { data } = await api.post('/auth/login', { email, password })
    token.value = data.token
    user.value = data.user
    localStorage.setItem('token', data.token)
  }

  async function register(email: string, username: string, password: string) {
    const { data } = await api.post('/auth/register', { email, username, password })
    token.value = data.token
    user.value = data.user
    localStorage.setItem('token', data.token)
  }

  async function fetchMe() {
    try {
      const { data } = await api.get('/auth/me')
      user.value = data
    } catch {
      logout()
    }
  }

  function logout() {
    token.value = null
    user.value = null
    localStorage.removeItem('token')
  }

  function loginWithOAuth(provider: 'google' | 'github') {
    window.location.href = `/api/v1/auth/${provider}`
  }

  return { token, user, isAuthenticated, login, register, fetchMe, logout, loginWithOAuth }
})
