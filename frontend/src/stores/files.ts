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
    await api.delete('/files', { params: { path } })
    files.value = files.value.filter((f) => f.path !== path)
  }

  function downloadFile(path: string) {
    const token = localStorage.getItem('token')
    const url = `/api/v1/files/download?path=${encodeURIComponent(path)}`
    // Use anchor trick to trigger download with auth header via fetch+blob
    fetch(url, { headers: { Authorization: `Bearer ${token}` } })
      .then((res) => res.blob())
      .then((blob) => {
        const a = document.createElement('a')
        a.href = URL.createObjectURL(blob)
        a.download = path.split('/').pop() ?? 'download'
        a.click()
        URL.revokeObjectURL(a.href)
      })
  }

  async function fetchStorageInfo() {
    const { data } = await api.get('/files/storage')
    storageUsed.value = data.used_bytes
  }

  return { files, currentPath, loading, error, storageUsed, listFiles, deleteFile, downloadFile, fetchStorageInfo }
})
