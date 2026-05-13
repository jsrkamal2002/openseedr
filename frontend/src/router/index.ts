import { createRouter, createWebHistory } from 'vue-router'
import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import api from '@/composables/useApi'

export const setupNeeded = ref(false)

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      redirect: '/dashboard',
    },
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/views/LoginView.vue'),
      meta: { public: true },
    },
    {
      path: '/register',
      name: 'Register',
      component: () => import('@/views/RegisterView.vue'),
      meta: { public: true },
    },
    {
      path: '/setup',
      name: 'Setup',
      component: () => import('@/views/SetupView.vue'),
      meta: { public: true, setup: true },
    },
    {
      path: '/dashboard',
      name: 'Dashboard',
      component: () => import('@/views/DashboardView.vue'),
    },
    {
      path: '/files',
      name: 'Files',
      component: () => import('@/views/FilesView.vue'),
    },
    {
      path: '/profile',
      name: 'Profile',
      component: () => import('@/views/ProfileView.vue'),
    },
    {
      path: '/admin',
      name: 'Admin',
      component: () => import('@/views/AdminView.vue'),
      meta: { requiresAdmin: true },
    },
  ],
})

let setupChecked = false

router.beforeEach(async (to) => {
  // Check setup status once per app load (not on every navigation)
  if (!setupChecked) {
    setupChecked = true
    try {
      const { data } = await api.get('/setup/status')
      setupNeeded.value = data.needed
    } catch {
      // If the check fails, don't block the app — assume setup is done
      setupNeeded.value = false
    }
  }

  // If setup is needed, only allow the /setup route
  if (setupNeeded.value && to.name !== 'Setup') {
    return { name: 'Setup' }
  }

  // If setup is done, prevent navigating back to /setup
  if (!setupNeeded.value && to.meta.setup) {
    return { name: 'Dashboard' }
  }

  // Existing auth guards
  const auth = useAuthStore()
  if (!to.meta.public && !auth.isAuthenticated) {
    return { name: 'Login' }
  }
  // Restore user data after a page refresh (Pinia state is not persisted).
  if (auth.isAuthenticated && !auth.user) {
    await auth.fetchMe()
  }
  if (to.meta.public && auth.isAuthenticated) {
    return { name: 'Dashboard' }
  }
  // Guard admin routes
  if (to.meta.requiresAdmin && !auth.user?.is_admin) {
    return { name: 'Dashboard' }
  }
})

export default router
