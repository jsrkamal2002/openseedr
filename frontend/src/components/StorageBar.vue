<script setup lang="ts">
import { computed } from 'vue'
import { formatBytesZero } from '@/composables/useFormat'

const props = defineProps<{ used: number; quota: number }>()

const pct = computed(() => props.quota > 0 ? Math.min((props.used / props.quota) * 100, 100) : 0)
</script>

<template>
  <div class="bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-800 rounded-xl px-5 py-4 transition-colors">
    <div class="flex justify-between text-sm mb-2">
      <span class="text-gray-500 dark:text-gray-400">Storage used</span>
      <span class="text-gray-700 dark:text-gray-300">{{ formatBytesZero(used) }} / {{ formatBytesZero(quota) }}</span>
    </div>
    <div class="h-2 bg-gray-200 dark:bg-gray-800 rounded-full overflow-hidden">
      <div
        class="h-full rounded-full transition-all duration-500"
        :class="pct > 90 ? 'bg-red-500' : pct > 70 ? 'bg-yellow-500' : 'bg-indigo-500'"
        :style="{ width: pct + '%' }"
      />
    </div>
  </div>
</template>
