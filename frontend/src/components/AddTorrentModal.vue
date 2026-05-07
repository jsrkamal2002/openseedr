<script setup lang="ts">
import { ref } from 'vue'
import { useTorrentStore } from '@/stores/torrents'

const emit = defineEmits<{ close: [] }>()
const torrentStore = useTorrentStore()

const tab = ref<'magnet' | 'file'>('magnet')
const magnet = ref('')
const file = ref<File | null>(null)
const loading = ref(false)
const error = ref('')

function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files?.length) file.value = input.files[0]
}

async function submit() {
  error.value = ''
  loading.value = true
  try {
    if (tab.value === 'magnet') {
      if (!magnet.value.startsWith('magnet:')) {
        error.value = 'Invalid magnet link'
        return
      }
      await torrentStore.addMagnet(magnet.value.trim())
    } else {
      if (!file.value) { error.value = 'Select a .torrent file'; return }
      await torrentStore.addFile(file.value)
    }
    emit('close')
  } catch (e: any) {
    error.value = e.response?.data?.error ?? 'Failed to add torrent'
  } finally {
    loading.value = false
  }
}
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
          @click="tab = t"
          class="flex-1 py-1.5 rounded-md text-sm font-medium transition-colors"
          :class="tab === t
            ? 'bg-white dark:bg-gray-700 text-gray-900 dark:text-white shadow-sm'
            : 'text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'">
          {{ t === 'magnet' ? '🧲 Magnet link' : '📄 .torrent file' }}
        </button>
      </div>

      <div v-if="error" class="bg-red-50 dark:bg-red-900/40 border border-red-300 dark:border-red-700 text-red-600 dark:text-red-300 rounded-lg px-4 py-2.5 mb-4 text-sm">
        {{ error }}
      </div>

      <!-- Magnet -->
      <div v-if="tab === 'magnet'">
        <label class="block text-sm text-gray-500 dark:text-gray-400 mb-2">Magnet URL</label>
        <textarea
          v-model="magnet"
          placeholder="magnet:?xt=urn:btih:…"
          rows="3"
          class="w-full bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg px-4 py-3 text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-600 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-500 resize-none transition-colors"
        />
      </div>

      <!-- File -->
      <div v-else>
        <label class="block text-sm text-gray-500 dark:text-gray-400 mb-2">.torrent file</label>
        <label class="flex flex-col items-center justify-center w-full h-28 border-2 border-dashed border-gray-300 dark:border-gray-700 rounded-lg cursor-pointer hover:border-indigo-500 transition-colors">
          <span class="text-gray-500 dark:text-gray-400 text-sm">{{ file ? file.name : 'Click to choose a .torrent file' }}</span>
          <input type="file" accept=".torrent" class="hidden" @change="onFileChange" />
        </label>
      </div>

      <div class="flex justify-end gap-3 mt-5">
        <button @click="emit('close')" class="px-4 py-2 text-sm text-gray-500 hover:text-gray-900 dark:hover:text-white transition-colors">
          Cancel
        </button>
        <button @click="submit" :disabled="loading"
          class="px-5 py-2 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition-colors">
          {{ loading ? 'Adding…' : 'Add torrent' }}
        </button>
      </div>
    </div>
  </div>
</template>
