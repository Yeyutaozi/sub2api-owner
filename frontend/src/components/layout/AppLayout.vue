<template>
  <div class="app-shell text-gray-900 dark:text-gray-100">
    <div class="app-shell-bg" aria-hidden="true"></div>
    <AppSidebar />
    <div
      class="app-main-panel relative min-h-screen transition-all duration-300"
      :class="[sidebarCollapsed ? 'lg:ml-[76px]' : 'lg:ml-64']"
    >
      <AppHeader />
      <main class="relative px-3 py-3 md:px-5 md:py-4 lg:px-6 lg:py-5 desk-main-shell">
        <div class="relative relay-stage">
          <div class="relay-stage__inner">
            <slot />
          </div>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>
