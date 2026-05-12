<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import AppLayout from '@/components/AppLayout.vue'
import api from '@/composables/useApi'

const auth = useAuthStore()

// ── Change password ───────────────────────────────────────────────────────────
const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const pwError = ref<string | null>(null)
const pwSuccess = ref(false)
const pwSaving = ref(false)

async function changePassword() {
  pwError.value = null
  pwSuccess.value = false

  if (newPassword.value !== confirmPassword.value) {
    pwError.value = 'New passwords do not match.'
    return
  }
  if (newPassword.value.length < 8) {
    pwError.value = 'New password must be at least 8 characters.'
    return
  }

  pwSaving.value = true
  try {
    await api.post('/auth/change-password', {
      current_password: currentPassword.value,
      new_password: newPassword.value,
    })
    pwSuccess.value = true
    currentPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
  } catch (e: any) {
    pwError.value = e?.response?.data?.error ?? 'Failed to change password.'
  } finally {
    pwSaving.value = false
  }
}

function fmtBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`
}

function fmtSpeed(bytesPerSec: number): string {
  if (!bytesPerSec) return 'Unlimited'
  return `${(bytesPerSec / 131072).toFixed(1)} Mbps`
}
</script>

<template>
  <AppLayout>
    <div class="max-w-lg space-y-8">

      <!-- Page header -->
      <div>
        <h2 class="text-xl font-semibold text-gray-900 dark:text-white">Profile</h2>
        <p class="text-sm text-gray-500 dark:text-gray-400 mt-0.5">Account details and security settings.</p>
      </div>

      <!-- Account info card -->
      <div class="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 p-6 space-y-4">
        <h3 class="font-medium text-gray-900 dark:text-white">Account</h3>
        <div class="flex items-center gap-4">
          <img v-if="auth.user?.avatar_url" :src="auth.user.avatar_url" class="w-14 h-14 rounded-full" />
          <div v-else class="w-14 h-14 rounded-full bg-indigo-700 flex items-center justify-center text-white text-xl font-bold">
            {{ auth.user?.username?.charAt(0).toUpperCase() ?? '?' }}
          </div>
          <div>
            <p class="font-semibold text-gray-900 dark:text-white">{{ auth.user?.username }}</p>
            <p class="text-sm text-gray-500 dark:text-gray-400">{{ auth.user?.email }}</p>
            <p class="text-xs text-gray-400 mt-0.5 capitalize">{{ auth.user?.provider }} account</p>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-3 pt-2 border-t border-gray-100 dark:border-gray-700 text-sm">
          <div>
            <p class="text-xs text-gray-500 dark:text-gray-400">Storage used</p>
            <p class="font-medium text-gray-900 dark:text-white mt-0.5">
              {{ fmtBytes(auth.user?.storage_used ?? 0) }}
              <span class="text-gray-400">/ {{ fmtBytes(auth.user?.storage_quota ?? 0) }}</span>
            </p>
          </div>
          <div>
            <p class="text-xs text-gray-500 dark:text-gray-400">Speed limits</p>
            <p class="font-medium text-gray-900 dark:text-white mt-0.5 text-xs">
              Down: {{ fmtSpeed(auth.user?.download_limit ?? 0) }} &nbsp;·&nbsp;
              Up: {{ fmtSpeed(auth.user?.upload_limit ?? 0) }}
            </p>
          </div>
          <div>
            <p class="text-xs text-gray-500 dark:text-gray-400">Role</p>
            <p class="font-medium text-gray-900 dark:text-white mt-0.5">
              {{ auth.user?.is_admin ? 'Administrator' : 'User' }}
            </p>
          </div>
          <div>
            <p class="text-xs text-gray-500 dark:text-gray-400">Status</p>
            <p class="font-medium mt-0.5" :class="auth.user?.is_active ? 'text-green-600 dark:text-green-400' : 'text-red-500'">
              {{ auth.user?.is_active ? 'Active' : 'Disabled' }}
            </p>
          </div>
        </div>
      </div>

      <!-- Change password card -->
      <div class="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 p-6 space-y-4">
        <div>
          <h3 class="font-medium text-gray-900 dark:text-white">Change password</h3>
          <p v-if="auth.user?.provider !== 'local'" class="text-sm text-gray-400 mt-1">
            Password changes are not available for {{ auth.user?.provider }} accounts.
          </p>
        </div>

        <template v-if="auth.user?.provider === 'local'">
          <!-- Success banner -->
          <div
            v-if="pwSuccess"
            class="px-4 py-3 bg-green-50 dark:bg-green-900/30 border border-green-200 dark:border-green-800 rounded-lg text-sm text-green-700 dark:text-green-400"
          >
            Password updated successfully.
          </div>

          <!-- Error banner -->
          <div
            v-if="pwError"
            class="px-4 py-3 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-600 dark:text-red-400"
          >
            {{ pwError }}
          </div>

          <div class="space-y-3">
            <label class="block">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Current password</span>
              <input
                v-model="currentPassword"
                type="password"
                autocomplete="current-password"
                class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </label>
            <label class="block">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">New password</span>
              <input
                v-model="newPassword"
                type="password"
                autocomplete="new-password"
                class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </label>
            <label class="block">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Confirm new password</span>
              <input
                v-model="confirmPassword"
                type="password"
                autocomplete="new-password"
                class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </label>
          </div>

          <div class="flex justify-end pt-1">
            <button
              @click="changePassword"
              :disabled="pwSaving || !currentPassword || !newPassword || !confirmPassword"
              class="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm font-medium transition-colors"
            >
              {{ pwSaving ? 'Saving…' : 'Update password' }}
            </button>
          </div>
        </template>
      </div>

    </div>
  </AppLayout>
</template>
