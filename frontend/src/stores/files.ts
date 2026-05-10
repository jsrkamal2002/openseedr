import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/composables/useApi'

export interface FileItem {
  name: string
  size: number
  is_dir: boolean
  mod_time: number
  path: string
}

export const useFilesStore = defineStore('files', () => {
  const files = ref<FileItem[]>([])
  const currentPath = ref('/')
  const loading = ref(false)
  const error = ref<string | null>(null)
  const storageUsed = ref(0)

  async function listFiles(path = '/') {
    loading.value = true
    error.value = null
    try {
      const { data } = await api.get('/files', { params: { path } })
      files.value = data.files ?? []
      currentPath.value = path
    } catch (e: any) {
      error.value = e.response?.data?.error ?? 'Failed to list files'
    } finally {
      loading.value = false
    }
  }

  async function deleteFile(path: string) {
    try {
      await api.delete('/files', { params: { path } })
      files.value = files.value.filter((f) => f.path !== path)
    } catch (e: any) {
      error.value = e.response?.data?.error ?? 'Failed to delete file'
      throw e
    }
  }

  async function downloadFile(path: string) {
    // Build a direct download URL with the JWT as a query parameter.
    // This lets the browser stream the file natively — no fetch+blob, no
    // memory buffering, and no user-gesture-context loss after await calls.
    const token = localStorage.getItem('token') ?? ''
    const url =
      `/api/v1/files/download` +
      `?path=${encodeURIComponent(path)}` +
      `&token=${encodeURIComponent(token)}`

    const a = document.createElement('a')
    a.href = url
    a.download = path.split('/').pop() ?? 'download'
    a.style.display = 'none'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
  }

  async function fetchStorageInfo() {
    const { data } = await api.get('/files/storage')
    storageUsed.value = data.used_bytes
  }

  return { files, currentPath, loading, error, storageUsed, listFiles, deleteFile, downloadFile, fetchStorageInfo }
})
