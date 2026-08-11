<template>
  <header class="mp-nav">
    <div class="mx-auto flex max-w-7xl items-center justify-between gap-4 px-4 py-3.5 sm:px-6">
      <!-- Left: site logo + name -->
      <div class="flex min-w-0 items-center gap-3">
        <template v-if="settings">
          <span
            class="flex h-9 w-9 flex-shrink-0 items-center justify-center overflow-hidden rounded-xl bg-white shadow-sm ring-1 ring-primary-200/70 dark:bg-dark-800 dark:ring-primary-800/40"
          >
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </span>
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span class="truncate font-display text-base font-semibold tracking-tight text-gray-950 dark:text-white">
                {{ siteName }}
              </span>
              <span class="protocol-pill shrink-0"><i></i>PLAZA</span>
            </div>
            <p class="mt-0.5 hidden text-[11px] text-gray-500 dark:text-dark-400 sm:block">
              {{ t('modelPlaza.nav.subtitle') }}
            </p>
          </div>
        </template>
        <template v-else>
          <span class="h-9 w-9 flex-shrink-0 animate-pulse rounded-xl bg-gray-200 dark:bg-dark-700" aria-hidden="true"></span>
          <span class="h-5 w-28 animate-pulse rounded bg-gray-200 dark:bg-dark-700" aria-hidden="true"></span>
        </template>
      </div>

      <!-- Right: login / back -->
      <RouterLink
        v-if="isAuthenticated"
        :to="backTarget"
        class="btn btn-primary btn-sm"
      >
        {{ t('modelPlaza.nav.backToDashboard') }}
      </RouterLink>
      <RouterLink
        v-else
        :to="{ path: '/login', query: { redirect: '/model-plaza' } }"
        class="btn btn-primary btn-sm"
      >
        {{ t('modelPlaza.nav.login') }}
      </RouterLink>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { sanitizeUrl } from '@/utils/url'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const settings = computed(() => appStore.cachedPublicSettings)
const siteName = computed(() => settings.value?.site_name || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(settings.value?.site_logo || '', { allowRelative: true, allowDataUrl: true })
)
const isAuthenticated = computed(() => authStore.isAuthenticated)
const backTarget = computed(() => (authStore.isAdmin ? '/admin/dashboard' : '/dashboard'))
</script>

<style scoped>
.mp-nav {
  position: sticky;
  top: 0;
  z-index: 30;
  border-bottom: 1px solid rgba(15, 23, 32, 0.06);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.9), rgba(244, 247, 246, 0.78));
  backdrop-filter: blur(16px) saturate(1.15);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.65);
}

.dark .mp-nav {
  border-bottom-color: rgba(51, 65, 85, 0.55);
  background: linear-gradient(180deg, rgba(15, 23, 32, 0.92), rgba(8, 13, 19, 0.82));
  box-shadow: none;
}
</style>
