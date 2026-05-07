<script setup lang="ts">
import { useAuthStore } from '@/stores/auth'
import { useRouter, RouterLink } from 'vue-router'
import { useThemeStore } from '@/stores/theme'

const auth = useAuthStore()
const router = useRouter()
const theme = useThemeStore()

function logout() {
  auth.logout()
  router.push('/login')
}
</script>

<template>
  <div class="min-h-screen bg-gray-100 dark:bg-gray-950 flex transition-colors duration-200">
    <!-- Sidebar -->
    <aside class="w-56 bg-white dark:bg-gray-900 border-r border-gray-200 dark:border-gray-800 flex flex-col transition-colors duration-200">
      <div class="px-5 py-5 border-b border-gray-200 dark:border-gray-800 flex items-center justify-between">
        <h1 class="text-lg font-bold text-gray-900 dark:text-white tracking-tight">☁ OpenSeedr</h1>
        <button
          @click="theme.toggle()"
          :title="theme.isDark ? 'Switch to light mode' : 'Switch to dark mode'"
          class="p-1.5 rounded-md text-gray-400 hover:text-gray-700 dark:hover:text-white hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
        >
          <svg v-if="theme.isDark" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364-6.364l-.707.707M6.343 17.657l-.707.707M17.657 17.657l-.707-.707M6.343 6.343l-.707-.707M12 8a4 4 0 100 8 4 4 0 000-8z" />
          </svg>
          <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
          </svg>
        </button>
      </div>

      <nav class="flex-1 px-3 py-4 space-y-1">
        <RouterLink to="/dashboard"
          class="flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800 hover:text-gray-900 dark:hover:text-white transition-colors"
          active-class="bg-gray-100 dark:bg-gray-800 text-gray-900 dark:text-white">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
          </svg>
          Torrents
        </RouterLink>
        <RouterLink to="/files"
          class="flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800 hover:text-gray-900 dark:hover:text-white transition-colors"
          active-class="bg-gray-100 dark:bg-gray-800 text-gray-900 dark:text-white">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
          </svg>
          Files
        </RouterLink>
      </nav>

      <!-- User + theme toggle section -->
      <div class="px-4 py-4 border-t border-gray-200 dark:border-gray-800">
        <div class="flex items-center gap-3 mb-3">
          <img v-if="auth.user?.avatar_url" :src="auth.user.avatar_url" class="w-8 h-8 rounded-full" />
          <div v-else class="w-8 h-8 rounded-full bg-indigo-700 flex items-center justify-center text-white text-xs font-bold">
            {{ auth.user?.username?.charAt(0).toUpperCase() ?? '?' }}
          </div>
          <div class="flex-1 min-w-0">
            <p class="text-sm font-medium text-gray-900 dark:text-white truncate">{{ auth.user?.username }}</p>
            <p class="text-xs text-gray-500 dark:text-gray-500 truncate">{{ auth.user?.email }}</p>
          </div>
        </div>

        <!-- Theme toggle -->
        <button @click="logout"
          class="w-full text-left text-sm text-gray-500 hover:text-red-500 dark:hover:text-red-400 transition-colors px-1">
          Sign out
        </button>
      </div>
    </aside>

    <!-- Main content -->
    <main class="flex-1 overflow-auto p-8">
      <slot />
    </main>
  </div>
</template>
