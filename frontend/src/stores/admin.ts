import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/composables/useApi'
import type { User } from './auth'

export interface AdminStats {
  users: {
    total: number
    active: number
  }
  torrents: {
    db_total: number
    live: {
      total?: number
      downloading?: number
      seeding?: number
      paused?: number
    }
  }
  storage: {
    used_bytes: number
    quota_bytes: number
  }
}

export const useAdminStore = defineStore('admin', () => {
  const users = ref<User[]>([])
  const stats = ref<AdminStats | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchStats() {
    loading.value = true
    error.value = null
    try {
      const { data } = await api.get('/admin/stats')
      stats.value = data
    } catch (e: any) {
      error.value = e?.response?.data?.error ?? 'Failed to load stats'
    } finally {
      loading.value = false
    }
  }

  async function fetchUsers() {
    loading.value = true
    error.value = null
    try {
      const { data } = await api.get('/admin/users')
      users.value = data
    } catch (e: any) {
      error.value = e?.response?.data?.error ?? 'Failed to load users'
    } finally {
      loading.value = false
    }
  }

  async function updateUser(
    id: string,
    patch: {
      storage_quota?: number
      download_limit?: number
      upload_limit?: number
      is_admin?: boolean
      is_active?: boolean
    }
  ): Promise<User> {
    const { data } = await api.patch(`/admin/users/${id}`, patch)
    const idx = users.value.findIndex((u) => u.id === id)
    if (idx !== -1) users.value[idx] = data
    return data
  }

  async function deleteUser(id: string) {
    await api.delete(`/admin/users/${id}`)
    users.value = users.value.filter((u) => u.id !== id)
  }

  return { users, stats, loading, error, fetchStats, fetchUsers, updateUser, deleteUser }
})
