<template>
  <!-- 后台内嵌形态: ?embedded=1 且已登录,套完整后台布局 -->
  <AppLayout v-if="isEmbedded">
    <ModelPlazaContent
      :response="data"
      :loading="loading"
      :error="loadFailed"
      :refreshing="refreshing"
      embedded
      @refresh="reload"
    />
  </AppLayout>

  <!-- 独立形态: 自带导航条 logo/站名 + 登录/回后台 -->
  <div v-else class="mp-page">
    <PlazaNavBar />
    <main class="mp-page__main">
      <ModelPlazaContent
        :response="data"
        :loading="loading"
        :error="loadFailed"
        :refreshing="refreshing"
        @refresh="reload"
      />
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import PlazaNavBar from '@/components/modelPlaza/PlazaNavBar.vue'
import ModelPlazaContent from '@/components/modelPlaza/ModelPlazaContent.vue'
import { getModelPlaza, type ModelPlazaResponse } from '@/api/modelPlaza'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const appStore = useAppStore()
const authStore = useAuthStore()

// Logged-in users always get the AppLayout embed; public visitors stay standalone.
const isEmbedded = computed(() => authStore.isAuthenticated)

const data = ref<ModelPlazaResponse | null>(null)
const loading = ref(false)
const refreshing = ref(false)
const loadFailed = ref(false)

async function reload() {
  if (loading.value || refreshing.value) return
  const first = data.value == null
  if (first) loading.value = true
  else refreshing.value = true
  loadFailed.value = false
  try {
    data.value = await getModelPlaza()
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

onMounted(async () => {
  // 独立形态导航条需要站点名/Logo;有 __APP_CONFIG__ 注入时同步命中缓存。
  void appStore.fetchPublicSettings()
  await reload()
})
</script>

<style scoped>
.mp-page {
  min-height: 100vh;
  background:
    radial-gradient(58rem 28rem at 8% -8%, rgb(45 212 191 / 0.14), transparent 58%),
    radial-gradient(42rem 24rem at 96% 0%, rgb(251 191 36 / 0.12), transparent 52%),
    radial-gradient(36rem 22rem at 70% 100%, rgb(244 63 94 / 0.07), transparent 55%),
    linear-gradient(180deg, #f2f7f6 0%, #f7f4ee 48%, #f8f7fb 100%);
}

.mp-page__main {
  margin: 0 auto;
  max-width: 1240px;
  padding: 24px 16px 56px;
}

@media (min-width: 640px) {
  .mp-page__main {
    padding: 28px 24px 64px;
  }
}

:global(.dark) .mp-page {
  background:
    radial-gradient(58rem 28rem at 8% -8%, rgb(45 212 191 / 0.08), transparent 58%),
    radial-gradient(42rem 24rem at 96% 0%, rgb(34 211 238 / 0.06), transparent 52%),
    linear-gradient(180deg, #070b14 0%, #0a101b 46%, #0b1220 100%);
}
</style>
