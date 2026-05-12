<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useAdminStore } from '@/stores/admin'
import { useAuthStore } from '@/stores/auth'
import type { User } from '@/stores/auth'
import AppLayout from '@/components/AppLayout.vue'

const adminStore = useAdminStore()
const auth = useAuthStore()

// ── Edit modal state ──────────────────────────────────────────────────────────
const editingUser = ref<User | null>(null)
const editForm = ref({
  storage_quota_gb: 0,
  download_limit_mbps: 0,
  upload_limit_mbps: 0,
  is_admin: false,
  is_active: true,
})
const saveError = ref<string | null>(null)
const saving = ref(false)

function openEdit(user: User) {
  editingUser.value = user
  editForm.value = {
    storage_quota_gb: Math.round(user.storage_quota / 1073741824),
    download_limit_mbps: user.download_limit ? Math.round(user.download_limit / 131072) : 0,
    upload_limit_mbps: user.upload_limit ? Math.round(user.upload_limit / 131072) : 0,
    is_admin: user.is_admin,
    is_active: user.is_active,
  }
  saveError.value = null
}

function closeEdit() {
  editingUser.value = null
}

async function saveEdit() {
  if (!editingUser.value) return
  saving.value = true
  saveError.value = null
  try {
    await adminStore.updateUser(editingUser.value.id, {
      storage_quota: editForm.value.storage_quota_gb * 1073741824,
      download_limit: editForm.value.download_limit_mbps * 131072,
      upload_limit: editForm.value.upload_limit_mbps * 131072,
      is_admin: editForm.value.is_admin,
      is_active: editForm.value.is_active,
    })
    closeEdit()
  } catch (e: any) {
    saveError.value = e?.response?.data?.error ?? 'Failed to save'
  } finally {
    saving.value = false
  }
}

// ── Delete confirmation ───────────────────────────────────────────────────────
const deletingUserId = ref<string | null>(null)

function confirmDelete(id: string) {
  deletingUserId.value = id
}

async function executeDelete() {
  if (!deletingUserId.value) return
  try {
    await adminStore.deleteUser(deletingUserId.value)
  } finally {
    deletingUserId.value = null
  }
}

// ── Helpers ───────────────────────────────────────────────────────────────────
function fmtBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`
}

function fmtSpeed(bytesPerSec: number): string {
  if (bytesPerSec === 0) return 'Unlimited'
  const mbps = bytesPerSec / 131072
  return `${mbps.toFixed(1)} Mbps`
}

const storagePercent = computed(() => {
  if (!adminStore.stats) return 0
  const { used_bytes, quota_bytes } = adminStore.stats.storage
  if (quota_bytes === 0) return 0
  return Math.min(100, Math.round((used_bytes / quota_bytes) * 100))
})

onMounted(async () => {
  await Promise.all([adminStore.fetchStats(), adminStore.fetchUsers()])
})
</script>

<template>
  <AppLayout>
    <div class="space-y-8">

      <!-- Page header -->
      <div>
        <h2 class="text-xl font-semibold text-gray-900 dark:text-white">Admin Console</h2>
        <p class="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
          Manage users, quotas, speed limits and view system health.
        </p>
      </div>

      <!-- Error banner -->
      <div
        v-if="adminStore.error"
        class="px-4 py-3 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-600 dark:text-red-400"
      >
        {{ adminStore.error }}
      </div>

      <!-- Stats cards -->
      <div v-if="adminStore.stats" class="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div class="bg-white dark:bg-gray-800 rounded-xl p-4 border border-gray-200 dark:border-gray-700">
          <p class="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wide">Total Users</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{{ adminStore.stats.users.total }}</p>
          <p class="text-xs text-gray-400 mt-0.5">{{ adminStore.stats.users.active }} active</p>
        </div>
        <div class="bg-white dark:bg-gray-800 rounded-xl p-4 border border-gray-200 dark:border-gray-700">
          <p class="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wide">Torrents</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{{ adminStore.stats.torrents.db_total }}</p>
          <p class="text-xs text-gray-400 mt-0.5">
            {{ adminStore.stats.torrents.live?.downloading ?? 0 }} downloading ·
            {{ adminStore.stats.torrents.live?.seeding ?? 0 }} seeding
          </p>
        </div>
        <div class="bg-white dark:bg-gray-800 rounded-xl p-4 border border-gray-200 dark:border-gray-700">
          <p class="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wide">Storage Used</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{{ fmtBytes(adminStore.stats.storage.used_bytes) }}</p>
          <p class="text-xs text-gray-400 mt-0.5">of {{ fmtBytes(adminStore.stats.storage.quota_bytes) }} total quota</p>
        </div>
        <div class="bg-white dark:bg-gray-800 rounded-xl p-4 border border-gray-200 dark:border-gray-700">
          <p class="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wide">Storage Usage</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{{ storagePercent }}%</p>
          <div class="mt-2 h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
            <div
              class="h-full rounded-full transition-all"
              :class="storagePercent > 80 ? 'bg-red-500' : storagePercent > 60 ? 'bg-yellow-500' : 'bg-indigo-500'"
              :style="{ width: storagePercent + '%' }"
            />
          </div>
        </div>
      </div>

      <!-- Loading stats -->
      <div v-else-if="adminStore.loading" class="text-sm text-gray-500 dark:text-gray-400">Loading stats…</div>

      <!-- Users table -->
      <div class="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 overflow-hidden">
        <div class="px-5 py-4 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
          <h3 class="font-medium text-gray-900 dark:text-white">Users</h3>
          <span class="text-xs text-gray-400">{{ adminStore.users.length }} total</span>
        </div>

        <div v-if="adminStore.loading && !adminStore.users.length" class="py-10 text-center text-sm text-gray-500">
          Loading users…
        </div>

        <div v-else class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="text-left text-xs text-gray-500 dark:text-gray-400 border-b border-gray-200 dark:border-gray-700">
                <th class="px-5 py-3 font-medium">User</th>
                <th class="px-5 py-3 font-medium">Role</th>
                <th class="px-5 py-3 font-medium">Status</th>
                <th class="px-5 py-3 font-medium">Storage</th>
                <th class="px-5 py-3 font-medium">Down limit</th>
                <th class="px-5 py-3 font-medium">Up limit</th>
                <th class="px-5 py-3 font-medium">Joined</th>
                <th class="px-5 py-3 font-medium"></th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-gray-700">
              <tr
                v-for="u in adminStore.users"
                :key="u.id"
                class="hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors"
                :class="{ 'opacity-50': !u.is_active }"
              >
                <td class="px-5 py-3">
                  <div class="font-medium text-gray-900 dark:text-white">{{ u.username }}</div>
                  <div class="text-xs text-gray-400">{{ u.email }}</div>
                </td>
                <td class="px-5 py-3">
                  <span
                    class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium"
                    :class="u.is_admin
                      ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300'
                      : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'"
                  >
                    {{ u.is_admin ? 'Admin' : 'User' }}
                  </span>
                </td>
                <td class="px-5 py-3">
                  <span
                    class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium"
                    :class="u.is_active
                      ? 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'
                      : 'bg-red-100 text-red-600 dark:bg-red-900/40 dark:text-red-300'"
                  >
                    {{ u.is_active ? 'Active' : 'Disabled' }}
                  </span>
                </td>
                <td class="px-5 py-3 text-gray-600 dark:text-gray-300">
                  {{ fmtBytes(u.storage_used) }} / {{ fmtBytes(u.storage_quota) }}
                </td>
                <td class="px-5 py-3 text-gray-600 dark:text-gray-300">{{ fmtSpeed(u.download_limit) }}</td>
                <td class="px-5 py-3 text-gray-600 dark:text-gray-300">{{ fmtSpeed(u.upload_limit) }}</td>
                <td class="px-5 py-3 text-gray-400 text-xs">{{ new Date(u.created_at).toLocaleDateString() }}</td>
                <td class="px-5 py-3">
                  <div class="flex gap-2 justify-end">
                    <!-- Don't let admin delete themselves -->
                    <button
                      @click="openEdit(u)"
                      class="text-indigo-600 dark:text-indigo-400 hover:underline text-xs"
                    >Edit</button>
                    <button
                      v-if="u.id !== auth.user?.id"
                      @click="confirmDelete(u.id)"
                      class="text-red-500 hover:underline text-xs"
                    >Delete</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

    </div>

    <!-- Edit modal -->
    <div
      v-if="editingUser"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm px-4"
      @click.self="closeEdit"
    >
      <div class="bg-white dark:bg-gray-800 rounded-xl shadow-xl w-full max-w-md p-6 space-y-4">
        <h3 class="font-semibold text-gray-900 dark:text-white">
          Edit {{ editingUser.username }}
        </h3>

        <div class="space-y-3">
          <!-- Storage quota -->
          <label class="block">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Storage quota (GB)</span>
            <input
              v-model.number="editForm.storage_quota_gb"
              type="number" min="1" step="1"
              class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </label>

          <!-- Download limit -->
          <label class="block">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Download limit (Mbps, 0 = unlimited)</span>
            <input
              v-model.number="editForm.download_limit_mbps"
              type="number" min="0" step="1"
              class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </label>

          <!-- Upload limit -->
          <label class="block">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Upload limit (Mbps, 0 = unlimited)</span>
            <input
              v-model.number="editForm.upload_limit_mbps"
              type="number" min="0" step="1"
              class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </label>

          <!-- Toggles -->
          <div class="flex gap-6 pt-1">
            <label class="flex items-center gap-2 cursor-pointer">
              <input v-model="editForm.is_admin" type="checkbox" class="rounded border-gray-300 text-indigo-600 focus:ring-indigo-500" />
              <span class="text-sm text-gray-700 dark:text-gray-300">Admin</span>
            </label>
            <label class="flex items-center gap-2 cursor-pointer">
              <input v-model="editForm.is_active" type="checkbox" class="rounded border-gray-300 text-indigo-600 focus:ring-indigo-500" />
              <span class="text-sm text-gray-700 dark:text-gray-300">Active</span>
            </label>
          </div>
        </div>

        <div v-if="saveError" class="text-xs text-red-500">{{ saveError }}</div>

        <div class="flex gap-3 justify-end pt-2">
          <button
            @click="closeEdit"
            class="px-4 py-2 rounded-lg border border-gray-300 dark:border-gray-600 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors"
          >Cancel</button>
          <button
            @click="saveEdit"
            :disabled="saving"
            class="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm font-medium transition-colors"
          >{{ saving ? 'Saving…' : 'Save' }}</button>
        </div>
      </div>
    </div>

    <!-- Delete confirmation -->
    <div
      v-if="deletingUserId"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm px-4"
      @click.self="deletingUserId = null"
    >
      <div class="bg-white dark:bg-gray-800 rounded-xl shadow-xl w-full max-w-sm p-6 space-y-4">
        <h3 class="font-semibold text-gray-900 dark:text-white">Delete user?</h3>
        <p class="text-sm text-gray-500 dark:text-gray-400">
          This will soft-delete the account. The action can be reversed from the database.
        </p>
        <div class="flex gap-3 justify-end">
          <button
            @click="deletingUserId = null"
            class="px-4 py-2 rounded-lg border border-gray-300 dark:border-gray-600 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors"
          >Cancel</button>
          <button
            @click="executeDelete"
            class="px-4 py-2 rounded-lg bg-red-600 hover:bg-red-500 text-white text-sm font-medium transition-colors"
          >Delete</button>
        </div>
      </div>
    </div>

  </AppLayout>
</template>
