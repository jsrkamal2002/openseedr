<script setup lang="ts">
import { onMounted, computed } from 'vue'
import { useFilesStore } from '@/stores/files'
import AppLayout from '@/components/AppLayout.vue'
import FileBrowser from '@/components/FileBrowser.vue'

const filesStore = useFilesStore()

const breadcrumbs = computed(() => {
  const parts = filesStore.currentPath.split('/').filter(Boolean)
  const crumbs = [{ label: 'Home', path: '/' }]
  let acc = ''
  for (const p of parts) {
    acc += '/' + p
    crumbs.push({ label: p, path: acc })
  }
  return crumbs
})

onMounted(() => filesStore.listFiles('/'))
</script>

<template>
  <AppLayout>
    <div class="flex items-center justify-between mb-6">
      <div>
        <h2 class="text-xl font-semibold text-white">Files</h2>
        <!-- Breadcrumbs -->
        <nav class="flex items-center gap-1 mt-1 text-sm text-gray-400">
          <template v-for="(crumb, i) in breadcrumbs" :key="crumb.path">
            <span v-if="i > 0" class="text-gray-600">/</span>
            <button
              @click="filesStore.listFiles(crumb.path)"
              class="hover:text-indigo-400 transition-colors"
              :class="i === breadcrumbs.length - 1 ? 'text-white font-medium' : ''"
            >
              {{ crumb.label }}
            </button>
          </template>
        </nav>
      </div>
    </div>

    <FileBrowser />
  </AppLayout>
</template>
