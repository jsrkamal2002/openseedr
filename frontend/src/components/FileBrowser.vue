<script setup lang="ts">
import { ref } from 'vue'
import { useFilesStore } from '@/stores/files'
import type { FileItem } from '@/stores/files'
import { formatBytes } from '@/composables/useFormat'

const store = useFilesStore()

// Inline delete confirmation state
const deletingPath = ref<string | null>(null)

function fmtDate(unix: number) {
  return new Date(unix * 1000).toLocaleDateString(undefined, {
    year: 'numeric', month: 'short', day: 'numeric',
  })
}

function open(item: FileItem) {
  if (item.is_dir) store.listFiles(item.path)
}

function requestDelete(item: FileItem) {
  deletingPath.value = item.path
}

function cancelDelete() {
  deletingPath.value = null
}

async function confirmDelete(item: FileItem) {
  deletingPath.value = null
  store.error = null
  try {
    await store.deleteFile(item.path)
  } catch {
    // error is already set in the store
  }
}
</script>

<template>
  <div>
    <div v-if="store.loading" class="text-center py-16 text-gray-500">Loading…</div>
    <div v-else-if="store.error" class="text-center py-16 text-red-500 dark:text-red-400">{{ store.error }}</div>
    <div v-else-if="!store.files.length" class="text-center py-16 text-gray-400 dark:text-gray-600">
      This folder is empty.
    </div>
    <div v-else class="bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-800 rounded-xl overflow-hidden transition-colors">
      <table class="w-full text-sm">
        <thead>
          <tr class="border-b border-gray-200 dark:border-gray-800 text-gray-500 dark:text-gray-500">
            <th class="text-left px-5 py-3 font-medium" scope="col">Name</th>
            <th class="text-right px-5 py-3 font-medium hidden md:table-cell" scope="col">Size</th>
            <th class="text-right px-5 py-3 font-medium hidden lg:table-cell" scope="col">Modified</th>
            <th class="px-5 py-3" scope="col"><span class="sr-only">Actions</span></th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="item in store.files"
            :key="item.path"
            class="border-b border-gray-100 dark:border-gray-800/50 hover:bg-gray-50 dark:hover:bg-gray-800/40 transition-colors last:border-0"
          >
            <!-- Name -->
            <td class="px-5 py-3">
              <button
                @click="open(item)"
                class="flex items-center gap-2 text-left max-w-xs"
                :class="item.is_dir
                  ? 'text-indigo-500 dark:text-indigo-400 hover:text-indigo-600 dark:hover:text-indigo-300 cursor-pointer'
                  : 'text-gray-700 dark:text-gray-200 cursor-default pointer-events-none'"
              >
                <svg v-if="item.is_dir" class="w-4 h-4 shrink-0 text-indigo-500 dark:text-indigo-400" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M10 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z"/>
                </svg>
                <svg v-else class="w-4 h-4 shrink-0 text-gray-400 dark:text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                    d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/>
                </svg>
                <span class="truncate">{{ item.name }}</span>
              </button>
            </td>

            <!-- Size -->
            <td class="px-5 py-3 text-right text-gray-500 hidden md:table-cell">
              {{ item.is_dir ? '—' : formatBytes(item.size) }}
            </td>

            <!-- Date -->
            <td class="px-5 py-3 text-right text-gray-500 hidden lg:table-cell">
              {{ fmtDate(item.mod_time) }}
            </td>

            <!-- Actions -->
            <td class="px-5 py-3 text-right">
              <div class="flex items-center justify-end gap-2">
                <template v-if="deletingPath === item.path">
                  <span class="text-xs text-gray-500 dark:text-gray-400">Delete?</span>
                  <button @click="confirmDelete(item)"
                    class="px-2 py-1 rounded text-xs bg-red-600 hover:bg-red-500 text-white transition-colors">Yes</button>
                  <button @click="cancelDelete"
                    class="px-2 py-1 rounded text-xs text-gray-500 hover:text-gray-700 dark:hover:text-gray-300 transition-colors">No</button>
                </template>
                <template v-else>
                  <button v-if="!item.is_dir" @click="store.downloadFile(item.path)"
                    :title="`Download ${item.name}`"
                    class="p-1.5 rounded-md text-gray-400 hover:text-indigo-500 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                        d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/>
                    </svg>
                    <span class="sr-only">Download {{ item.name }}</span>
                  </button>
                  <button @click="requestDelete(item)"
                    :title="`Delete ${item.name}`"
                    class="p-1.5 rounded-md text-gray-400 hover:text-red-500 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                        d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
                    </svg>
                    <span class="sr-only">Delete {{ item.name }}</span>
                  </button>
                </template>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
