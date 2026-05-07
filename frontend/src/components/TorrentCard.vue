<script setup lang="ts">
import type { Torrent } from '@/stores/torrents'

const props = defineProps<{ torrent: Torrent }>()
const emit = defineEmits<{
  pause: []
  resume: []
  delete: []
}>()

const statusColors: Record<string, string> = {
  queued: 'text-gray-400',
  downloading: 'text-blue-400',
  seeding: 'text-green-400',
  paused: 'text-yellow-400',
  completed: 'text-emerald-400',
  error: 'text-red-400',
}

const barColors: Record<string, string> = {
  queued: 'bg-gray-600',
  downloading: 'bg-blue-500',
  seeding: 'bg-green-500',
  paused: 'bg-yellow-500',
  completed: 'bg-emerald-500',
  error: 'bg-red-500',
}

function formatBytes(bytes: number): string {
  if (!bytes) return '—'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`
}

function formatSpeed(bytesPerSec: number): string {
  if (!bytesPerSec) return ''
  const k = 1024
  const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s']
  const i = Math.floor(Math.log(bytesPerSec) / Math.log(k))
  return `${(bytesPerSec / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`
}

function formatEta(seconds: number): string {
  if (!seconds || seconds <= 0 || seconds >= 8640000) return ''
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return m > 0 ? `${h}h ${m}m` : `${h}h`
}

const canPause = (s: Torrent['status']) => s === 'downloading' || s === 'seeding'
const canResume = (s: Torrent['status']) => s === 'paused'
const showSpeeds = (t: Torrent) => t.status === 'downloading' || t.status === 'seeding'
</script>

<template>
  <div class="bg-gray-900 border border-gray-800 rounded-xl px-5 py-4 hover:border-gray-700 transition-colors">
    <!-- Row 1: name + action buttons -->
    <div class="flex items-start justify-between gap-4">
      <p class="text-white font-medium truncate" :title="torrent.name">{{ torrent.name }}</p>

      <!-- Actions -->
      <div class="flex items-center gap-2 shrink-0">
        <button v-if="canPause(torrent.status)" @click="emit('pause')"
          title="Pause"
          class="p-1.5 rounded-md text-gray-400 hover:text-yellow-400 hover:bg-gray-800 transition-colors">
          <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
            <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/>
          </svg>
        </button>
        <button v-if="canResume(torrent.status)" @click="emit('resume')"
          title="Resume"
          class="p-1.5 rounded-md text-gray-400 hover:text-green-400 hover:bg-gray-800 transition-colors">
          <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
            <path d="M8 5v14l11-7z"/>
          </svg>
        </button>
        <button @click="emit('delete')"
          title="Delete"
          class="p-1.5 rounded-md text-gray-400 hover:text-red-400 hover:bg-gray-800 transition-colors">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
          </svg>
        </button>
      </div>
    </div>

    <!-- Row 2: status + size + progress % -->
    <div class="flex items-center gap-3 mt-1.5 text-xs flex-wrap">
      <span :class="statusColors[torrent.status] ?? 'text-gray-400'" class="capitalize font-medium">
        {{ torrent.status }}
      </span>
      <span class="text-gray-500">{{ formatBytes(torrent.downloaded) }} / {{ formatBytes(torrent.size) }}</span>
      <span v-if="torrent.status === 'downloading'" class="text-gray-500">
        {{ (torrent.progress * 100).toFixed(1) }}%
      </span>
    </div>

    <!-- Row 3: live stats (speeds / ETA / peers) — only when active -->
    <div v-if="showSpeeds(torrent)" class="flex items-center gap-3 mt-1 text-xs flex-wrap">
      <!-- Download speed -->
      <span v-if="torrent.download_speed > 0" class="flex items-center gap-1 text-blue-400">
        <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 24 24">
          <path d="M19 9l-7 7-7-7"/>
        </svg>
        {{ formatSpeed(torrent.download_speed) }}
      </span>
      <!-- Upload speed -->
      <span v-if="torrent.upload_speed > 0" class="flex items-center gap-1 text-green-400">
        <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 24 24">
          <path d="M5 15l7-7 7 7"/>
        </svg>
        {{ formatSpeed(torrent.upload_speed) }}
      </span>
      <!-- ETA -->
      <span v-if="torrent.status === 'downloading' && formatEta(torrent.eta)" class="text-gray-500">
        ETA {{ formatEta(torrent.eta) }}
      </span>
      <!-- Seeds / Peers -->
      <span v-if="torrent.num_seeds > 0 || torrent.num_leechs > 0" class="text-gray-500">
        {{ torrent.num_seeds }}S {{ torrent.num_leechs }}P
      </span>
    </div>

    <!-- Progress bar -->
    <div class="mt-3 h-1.5 bg-gray-800 rounded-full overflow-hidden">
      <div
        :class="barColors[torrent.status] ?? 'bg-gray-600'"
        class="h-full rounded-full transition-all duration-500"
        :style="{ width: `${(torrent.progress * 100).toFixed(1)}%` }"
      />
    </div>
  </div>
</template>
