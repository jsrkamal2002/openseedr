<script setup lang="ts">
import { ref, computed } from 'vue'
import { useTorrentStore } from '@/stores/torrents'
import api from '@/composables/useApi'

const emit = defineEmits<{ close: [] }>()
const torrentStore = useTorrentStore()

const tab = ref<'magnet' | 'file'>('magnet')
const magnet = ref('')
const files = ref<File[]>([])
const loading = ref(false)

interface ItemResult {
  label: string
  status: 'ok' | 'wishlisted' | 'error'
  message?: string
}
const results = ref<ItemResult[]>([])
const done = ref(false)

// ── magnet helpers ─────────────────────────────────────────────────────────
const magnetLines = computed(() =>
  magnet.value
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l.startsWith('magnet:')),
)

// ── file helpers ──────────────────────────────────────────────────────────
function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files?.length) files.value = Array.from(input.files)
}

// ── submit ─────────────────────────────────────────────────────────────────
async function submit() {
  results.value = []
  done.value = false
  loading.value = true

  try {
    if (tab.value === 'magnet') {
      if (!magnetLines.value.length) {
        results.value = [{ label: 'Input', status: 'error', message: 'No valid magnet links found. Each line must start with magnet:' }]
        done.value = true
        return
      }
      for (const url of magnetLines.value) {
        try {
          const { data, status } = await api.post('/torrents/magnet', { magnet_url: url })
          const name = data.torrent?.name ?? url.slice(0, 60)
          if (status === 202 || data.wishlisted) {
            results.value.push({ label: name, status: 'wishlisted', message: 'Quota full — added to wishlist' })
            // Wishlist items are not shown on dashboard; skip unshift
          } else {
            if (data.torrent) torrentStore.torrents.unshift(data.torrent)
            results.value.push({ label: name, status: 'ok' })
          }
        } catch (e: any) {
          results.value.push({
            label: url.slice(0, 60),
            status: 'error',
            message: e.response?.data?.error ?? 'Failed',
          })
        }
      }
    } else {
      if (!files.value.length) {
        results.value = [{ label: 'Input', status: 'error', message: 'Select at least one .torrent file' }]
        done.value = true
        return
      }
      for (const file of files.value) {
        try {
          const form = new FormData()
          form.append('torrent', file)
          const { data, status } = await api.post('/torrents/file', form, {
            headers: { 'Content-Type': 'multipart/form-data' },
          })
          if (status === 202 || data.wishlisted) {
            results.value.push({ label: file.name, status: 'wishlisted', message: 'Quota full — added to wishlist' })
          } else {
            if (data.torrent) torrentStore.torrents.unshift(data.torrent)
            results.value.push({ label: file.name, status: 'ok' })
          }
        } catch (e: any) {
          results.value.push({
            label: file.name,
            status: 'error',
            message: e.response?.data?.error ?? 'Failed',
          })
        }
      }
    }
  } finally {
    loading.value = false
    done.value = true
  }

  // Auto-close if everything succeeded
  const allOk = results.value.every((r) => r.status === 'ok' || r.status === 'wishlisted')
  if (allOk) emit('close')
}

const progressLabel = computed(() => {
  if (!loading.value) return ''
  const total = tab.value === 'magnet' ? magnetLines.value.length : files.value.length
  return `Adding ${results.value.length + 1} / ${total}…`
})
</script>

<template>
  <!-- Backdrop -->
  <div class="fixed inset-0 bg-black/60 flex items-center justify-center z-50 px-4" @click.self="emit('close')">
    <div class="w-full max-w-lg bg-white dark:bg-gray-900 rounded-2xl shadow-2xl border border-gray-200 dark:border-gray-800 p-6 transition-colors">
      <div class="flex items-center justify-between mb-5">
        <h3 class="text-gray-900 dark:text-white font-semibold text-lg">Add Torrent</h3>
        <button @click="emit('close')" class="text-gray-400 hover:text-gray-700 dark:hover:text-white transition-colors">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <!-- Tabs -->
      <div class="flex gap-1 bg-gray-100 dark:bg-gray-800 rounded-lg p-1 mb-5">
        <button v-for="t in ['magnet', 'file'] as const" :key="t"
          @click="tab = t; results = []; done = false"
          class="flex-1 py-1.5 rounded-md text-sm font-medium transition-colors"
          :class="tab === t
            ? 'bg-white dark:bg-gray-700 text-gray-900 dark:text-white shadow-sm'
            : 'text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'">
          {{ t === 'magnet' ? '🧲 Magnet link' : '📄 .torrent file' }}
        </button>
      </div>

      <!-- Results list (shown after submission or during) -->
      <div v-if="results.length" class="mb-4 space-y-1.5 max-h-40 overflow-y-auto">
        <div v-for="r in results" :key="r.label"
          class="flex items-start gap-2 text-sm px-3 py-2 rounded-lg"
          :class="{
            'bg-green-50 dark:bg-green-900/30 text-green-700 dark:text-green-300': r.status === 'ok',
            'bg-amber-50 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300': r.status === 'wishlisted',
            'bg-red-50 dark:bg-red-900/30 text-red-600 dark:text-red-300': r.status === 'error',
          }">
          <span class="shrink-0 mt-px">
            {{ r.status === 'ok' ? '✓' : r.status === 'wishlisted' ? '⏳' : '✗' }}
          </span>
          <span class="truncate flex-1" :title="r.label">{{ r.label }}</span>
          <span v-if="r.message" class="shrink-0 text-xs opacity-75">{{ r.message }}</span>
        </div>
      </div>

      <!-- Magnet -->
      <div v-if="tab === 'magnet'">
        <label class="block text-sm text-gray-500 dark:text-gray-400 mb-2">
          Magnet URL(s) — one per line
        </label>
        <textarea
          v-model="magnet"
          placeholder="magnet:?xt=urn:btih:…&#10;magnet:?xt=urn:btih:…"
          rows="4"
          class="w-full bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg px-4 py-3 text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-600 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-500 resize-none transition-colors"
        />
        <p v-if="magnetLines.length > 0" class="mt-1 text-xs text-gray-400 dark:text-gray-500">
          {{ magnetLines.length }} valid magnet link{{ magnetLines.length !== 1 ? 's' : '' }} detected
        </p>
      </div>

      <!-- File -->
      <div v-else>
        <label class="block text-sm text-gray-500 dark:text-gray-400 mb-2">.torrent file(s)</label>
        <label class="flex flex-col items-center justify-center w-full h-28 border-2 border-dashed border-gray-300 dark:border-gray-700 rounded-lg cursor-pointer hover:border-indigo-500 transition-colors">
          <span class="text-gray-500 dark:text-gray-400 text-sm text-center px-4">
            <template v-if="files.length === 0">Click to choose .torrent file(s)</template>
            <template v-else>{{ files.length }} file{{ files.length !== 1 ? 's' : '' }} selected: {{ files.map(f => f.name).join(', ') }}</template>
          </span>
          <input type="file" accept=".torrent" multiple class="hidden" @change="onFileChange" />
        </label>
      </div>

      <div class="flex items-center justify-end gap-3 mt-5">
        <span v-if="progressLabel" class="text-sm text-gray-500 dark:text-gray-400 mr-auto">{{ progressLabel }}</span>
        <button @click="emit('close')" class="px-4 py-2 text-sm text-gray-500 hover:text-gray-900 dark:hover:text-white transition-colors">
          {{ done ? 'Close' : 'Cancel' }}
        </button>
        <button @click="submit" :disabled="loading"
          class="px-5 py-2 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors">
          {{ loading ? 'Adding…' : (magnetLines.length > 1 || files.length > 1) ? `Add ${tab === 'magnet' ? magnetLines.length : files.length} torrents` : 'Add torrent' }}
        </button>
      </div>
    </div>
  </div>
</template>
