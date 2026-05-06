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
</script>

<template>
  <div class="bg-gray-900 border border-gray-800 rounded-xl px-5 py-4 hover:border-gray-700 transition-colors">
    <div class="flex items-start justify-between gap-4">
      <div class="flex-1 min-w-0">
        <p class="text-white font-medium truncate">{{ torrent.name }}</p>
        <div class="flex items-center gap-3 mt-1 text-xs">
          <span :class="statusColors[torrent.status] ?? 'text-gray-400'" class="capitalize font-medium">
            {{ torrent.status }}
          </span>
          <span class="text-gray-500">{{ formatBytes(torrent.downloaded) }} / {{ formatBytes(torrent.size) }}</span>
          <span v-if="torrent.status === 'downloading'" class="text-gray-500">
            {{ (torrent.progress * 100).toFixed(1) }}%
          </span>
        </div>
      </div>

      <!-- Actions -->
      <div class="flex items-center gap-2 shrink-0">
        <button v-if="torrent.status === 'downloading'" @click="emit('pause')"
          title="Pause"
          class="p-1.5 rounded-md text-gray-400 hover:text-yellow-400 hover:bg-gray-800 transition-colors">
          <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
            <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/>
          </svg>
        </button>
        <button v-if="torrent.status === 'paused'" @click="emit('resume')"
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
