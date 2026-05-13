<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useTorrentStore } from '@/stores/torrents'
import { useAuthStore } from '@/stores/auth'
import AppLayout from '@/components/AppLayout.vue'
import TorrentCard from '@/components/TorrentCard.vue'
import AddTorrentModal from '@/components/AddTorrentModal.vue'
import StorageBar from '@/components/StorageBar.vue'

const torrentStore = useTorrentStore()
const auth = useAuthStore()
const showModal = ref(false)

function openModal() { showModal.value = true }
function closeModal() { showModal.value = false }

let pollInterval: ReturnType<typeof setInterval>

onMounted(async () => {
  // Refresh user (includes real storage_used computed from disk) and torrents
  // concurrently so the storage bar shows an accurate balance on every visit.
  await Promise.all([torrentStore.fetchTorrents(), auth.fetchMe()])
  // Poll every 5 seconds to refresh torrent progress and storage usage
  pollInterval = setInterval(() => Promise.all([torrentStore.fetchTorrents(), auth.fetchMe()]), 5000)
})

onUnmounted(() => clearInterval(pollInterval))
</script>

<template>
  <AppLayout>
    <!-- Header bar -->
    <div class="flex items-center justify-between mb-6">
      <div>
        <h2 class="text-xl font-semibold text-gray-900 dark:text-white">My Torrents</h2>
        <p class="text-gray-500 dark:text-gray-400 text-sm mt-0.5">
          {{ torrentStore.torrents.length }} torrent{{ torrentStore.torrents.length !== 1 ? 's' : '' }}
        </p>
      </div>
      <button
        @click="openModal"
        class="flex items-center gap-2 bg-indigo-600 hover:bg-indigo-500 text-white px-4 py-2 rounded-lg text-sm font-medium transition-colors"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
        Add torrent
      </button>
    </div>

    <!-- Storage bar -->
    <StorageBar
      :used="auth.user?.storage_used ?? 0"
      :quota="auth.user?.storage_quota ?? 10737418240"
      class="mb-6"
    />

    <!-- Error banner -->
    <div v-if="torrentStore.error" class="mb-4 px-4 py-3 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-600 dark:text-red-400">
      {{ torrentStore.error }}
    </div>

    <!-- Loading state -->
    <div v-if="torrentStore.loading && !torrentStore.torrents.length" class="text-center py-20 text-gray-500">
      Loading torrents…
    </div>

    <!-- Empty state -->
    <div v-else-if="!torrentStore.torrents.length" class="text-center py-20">
      <div class="text-5xl mb-4">🌱</div>
      <p class="text-gray-500 dark:text-gray-400 text-lg">No torrents yet.</p>
      <p class="text-gray-400 dark:text-gray-600 text-sm mt-1">Add a magnet link or .torrent file to get started.</p>
      <button
        @click="openModal"
        class="mt-5 inline-flex items-center gap-2 bg-indigo-600 hover:bg-indigo-500 text-white px-4 py-2 rounded-lg text-sm font-medium transition-colors"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
        Add your first torrent
      </button>
    </div>

    <!-- Torrent list -->
    <div v-else class="space-y-3">
      <TorrentCard
        v-for="t in torrentStore.torrents"
        :key="t.id"
        :torrent="t"
        @pause="torrentStore.pauseTorrent(t.id)"
        @resume="torrentStore.resumeTorrent(t.id)"
        @delete="torrentStore.deleteTorrent(t.id, true)"
      />
    </div>

    <!-- Add torrent modal -->
    <AddTorrentModal v-if="showModal" @close="closeModal" />
  </AppLayout>
</template>
