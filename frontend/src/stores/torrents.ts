import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/composables/useApi'
import { useFilesStore } from '@/stores/files'

export interface Torrent {
  id: string
  user_id: string
  hash: string
  name: string
  size: number
  downloaded: number
  progress: number
  status: 'queued' | 'downloading' | 'seeding' | 'paused' | 'completed' | 'error'
  save_path: string
  added_at: string
  created_at: string
  // Live-only fields from qBittorrent (not stored in DB)
  download_speed: number
  upload_speed: number
  eta: number
  num_seeds: number
  num_leechs: number
}

export const useTorrentStore = defineStore('torrents', () => {
  const torrents = ref<Torrent[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchTorrents() {
    loading.value = true
    error.value = null
    try {
      const { data } = await api.get('/torrents')
      torrents.value = data.torrents ?? []
    } catch (e: any) {
      error.value = e.response?.data?.error ?? 'Failed to fetch torrents'
    } finally {
      loading.value = false
    }
  }

  async function addMagnet(magnetUrl: string) {
    const { data } = await api.post('/torrents/magnet', { magnet_url: magnetUrl })
    torrents.value.unshift(data.torrent)
    return data.torrent
  }

  async function addFile(file: File) {
    const form = new FormData()
    form.append('torrent', file)
    const { data } = await api.post('/torrents/file', form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    torrents.value.unshift(data.torrent)
    return data.torrent
  }

  async function deleteTorrent(id: string, deleteFiles = false) {
    await api.delete(`/torrents/${id}?delete_files=${deleteFiles}`)
    torrents.value = torrents.value.filter((t) => t.id !== id)
    // Refresh storage usage immediately so the bar updates without a page reload
    const filesStore = useFilesStore()
    await filesStore.fetchStorageInfo()
  }

  async function pauseTorrent(id: string) {
    await api.post(`/torrents/${id}/pause`)
    const t = torrents.value.find((t) => t.id === id)
    if (t) t.status = 'paused'
  }

  async function resumeTorrent(id: string) {
    await api.post(`/torrents/${id}/resume`)
    const t = torrents.value.find((t) => t.id === id)
    if (t) t.status = 'downloading'
  }

  return { torrents, loading, error, fetchTorrents, addMagnet, addFile, deleteTorrent, pauseTorrent, resumeTorrent }
})
