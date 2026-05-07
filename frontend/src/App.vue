<script setup lang="ts">
import { onMounted } from 'vue'
import { RouterView } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'

const auth = useAuthStore()
const theme = useThemeStore()

onMounted(() => {
  if (auth.isAuthenticated && !auth.user) auth.fetchMe()
  theme.init()
})
</script>

<template>
  <RouterView />
</template>
