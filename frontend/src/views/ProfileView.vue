<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import AppLayout from '@/components/AppLayout.vue'
import api from '@/composables/useApi'
import { formatBytesZero, formatSpeedLimit } from '@/composables/useFormat'

const auth = useAuthStore()

// ── Avatar upload ─────────────────────────────────────────────────────────────
const avatarInput = ref<HTMLInputElement | null>(null)
const avatarPreview = ref<string | null>(null)
const avatarFile = ref<File | null>(null)
const avatarError = ref<string | null>(null)
const avatarSuccess = ref(false)
const avatarUploading = ref(false)

function openAvatarPicker() {
  avatarInput.value?.click()
}

function onAvatarSelected(e: Event) {
  avatarError.value = null
  avatarSuccess.value = false
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return

  if (file.size > 5 * 1024 * 1024) {
    avatarError.value = 'Image must be under 5 MB.'
    return
  }
  if (!file.type.startsWith('image/')) {
    avatarError.value = 'Only image files are supported.'
    return
  }

  avatarFile.value = file
  avatarPreview.value = URL.createObjectURL(file)
}

function cancelAvatarPreview() {
  avatarFile.value = null
  if (avatarPreview.value) URL.revokeObjectURL(avatarPreview.value)
  avatarPreview.value = null
  avatarError.value = null
  if (avatarInput.value) avatarInput.value.value = ''
}

async function uploadAvatar() {
  if (!avatarFile.value) return
  avatarUploading.value = true
  avatarError.value = null
  avatarSuccess.value = false
  try {
    const form = new FormData()
    form.append('avatar', avatarFile.value)
    const { data } = await api.post('/auth/avatar', form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    // Bust the browser cache by appending a timestamp to the URL
    auth.user = { ...data, avatar_url: data.avatar_url + '?t=' + Date.now() }
    avatarSuccess.value = true
    cancelAvatarPreview()
  } catch (e: any) {
    avatarError.value = e?.response?.data?.error ?? 'Upload failed.'
  } finally {
    avatarUploading.value = false
  }
}

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

        <div class="flex items-center gap-5">
          <!-- Avatar with click-to-change overlay -->
          <div class="relative group cursor-pointer shrink-0" @click="openAvatarPicker" title="Change profile picture">
            <img
              v-if="avatarPreview || auth.user?.avatar_url"
              :src="avatarPreview ?? auth.user?.avatar_url"
              class="w-16 h-16 rounded-full object-cover ring-2 ring-indigo-500/30 group-hover:ring-indigo-500 transition"
            />
            <div
              v-else
              class="w-16 h-16 rounded-full bg-indigo-700 flex items-center justify-center text-white text-xl font-bold ring-2 ring-transparent group-hover:ring-indigo-500 transition"
            >
              {{ auth.user?.username?.charAt(0).toUpperCase() ?? '?' }}
            </div>
            <!-- Hover overlay -->
            <div class="absolute inset-0 rounded-full bg-black/40 flex items-center justify-center opacity-0 group-hover:opacity-100 transition">
              <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M3 9a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 0110.07 4h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0018.07 7H19a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V9z" />
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 13a3 3 0 11-6 0 3 3 0 016 0z" />
              </svg>
            </div>
          </div>

          <div class="flex-1 min-w-0">
            <p class="font-semibold text-gray-900 dark:text-white truncate">{{ auth.user?.username }}</p>
            <p class="text-sm text-gray-500 dark:text-gray-400 truncate">{{ auth.user?.email }}</p>
            <p class="text-xs text-gray-400 mt-0.5 capitalize">{{ auth.user?.provider }} account</p>
            <button
              @click="openAvatarPicker"
              class="mt-1.5 text-xs text-indigo-600 dark:text-indigo-400 hover:underline"
            >Change picture</button>
          </div>
        </div>

        <!-- Hidden file input -->
        <input
          ref="avatarInput"
          type="file"
          accept="image/jpeg,image/png,image/gif,image/webp"
          class="hidden"
          @change="onAvatarSelected"
        />

        <!-- Avatar preview action bar -->
        <div v-if="avatarPreview" class="flex items-center gap-3 p-3 bg-indigo-50 dark:bg-indigo-900/20 rounded-lg border border-indigo-200 dark:border-indigo-800">
          <p class="text-sm text-indigo-700 dark:text-indigo-300 flex-1">New picture selected — save to apply.</p>
          <button
            @click="cancelAvatarPreview"
            class="text-xs text-gray-500 hover:text-gray-700 dark:hover:text-gray-300 transition"
          >Cancel</button>
          <button
            @click="uploadAvatar"
            :disabled="avatarUploading"
            class="px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-xs font-medium transition"
          >{{ avatarUploading ? 'Uploading…' : 'Save picture' }}</button>
        </div>

        <!-- Avatar feedback -->
        <div v-if="avatarError" class="px-4 py-2 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-600 dark:text-red-400">
          {{ avatarError }}
        </div>
        <div v-if="avatarSuccess && !avatarPreview" class="px-4 py-2 bg-green-50 dark:bg-green-900/30 border border-green-200 dark:border-green-800 rounded-lg text-sm text-green-700 dark:text-green-400">
          Profile picture updated.
        </div>

        <!-- Stats grid -->
        <div class="grid grid-cols-2 gap-3 pt-2 border-t border-gray-100 dark:border-gray-700 text-sm">
          <div>
            <p class="text-xs text-gray-500 dark:text-gray-400">Storage used</p>
            <p class="font-medium text-gray-900 dark:text-white mt-0.5">
              {{ formatBytesZero(auth.user?.storage_used ?? 0) }}
              <span class="text-gray-400">/ {{ formatBytesZero(auth.user?.storage_quota ?? 0) }}</span>
            </p>
          </div>
          <div>
            <p class="text-xs text-gray-500 dark:text-gray-400">Speed limits</p>
            <p class="font-medium text-gray-900 dark:text-white mt-0.5 text-xs">
              Down: {{ formatSpeedLimit(auth.user?.download_limit ?? 0) }} &nbsp;·&nbsp;
              Up: {{ formatSpeedLimit(auth.user?.upload_limit ?? 0) }}
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
          <div v-if="pwSuccess" class="px-4 py-3 bg-green-50 dark:bg-green-900/30 border border-green-200 dark:border-green-800 rounded-lg text-sm text-green-700 dark:text-green-400">
            Password updated successfully.
          </div>
          <div v-if="pwError" class="px-4 py-3 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-600 dark:text-red-400">
            {{ pwError }}
          </div>

          <div class="space-y-3">
            <label class="block">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Current password</span>
              <input v-model="currentPassword" type="password" autocomplete="current-password"
                class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500" />
            </label>
            <label class="block">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">New password</span>
              <input v-model="newPassword" type="password" autocomplete="new-password"
                class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500" />
            </label>
            <label class="block">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Confirm new password</span>
              <input v-model="confirmPassword" type="password" autocomplete="new-password"
                class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500" />
            </label>
          </div>

          <div class="flex justify-end pt-1">
            <button
              @click="changePassword"
              :disabled="pwSaving || !currentPassword || !newPassword || !confirmPassword"
              class="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm font-medium transition-colors"
            >{{ pwSaving ? 'Saving…' : 'Update password' }}</button>
          </div>
        </template>
      </div>

    </div>
  </AppLayout>
</template>
