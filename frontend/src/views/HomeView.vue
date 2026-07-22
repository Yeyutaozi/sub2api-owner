<template>
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <div v-else class="portal" :class="{ 'is-dark': isDark }">
    <header class="portal-header">
      <nav class="portal-nav" :aria-label="t('home.ui.primaryNavigation')">
        <router-link to="/home" class="wordmark" :aria-label="siteName">
          <span class="wordmark-logo"><img :src="siteLogo || '/logo.svg'" alt="" /></span>
          <span>{{ siteName }}</span>
        </router-link>

        <div class="header-status">
          <span><i></i> API</span>
          <code>/v1</code>
          <span>READY</span>
        </div>

        <div class="header-actions">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="square-action"
            :title="t('home.viewDocs')"
            :aria-label="t('home.viewDocs')"
          >
            <Icon name="book" size="sm" />
          </a>
          <button
            type="button"
            class="square-action"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon :name="isDark ? 'sun' : 'moon'" size="sm" />
          </button>
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="header-login">
            <span v-if="isAuthenticated" class="user-token">{{ userInitial }}</span>
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
            <Icon name="arrowRight" size="sm" />
          </router-link>
        </div>
      </nav>
    </header>

    <main>
      <section class="masthead">
        <TechBackground3D
          v-if="routeMotionAllowed"
          class="masthead-orbit"
          variant="home"
          :route-index="activeRouteIndex"
        />

        <div class="masthead-meta">
          <span>{{ t('home.ui.gatewayCapabilities') }}</span>
          <code>POST /v1/messages</code>
          <span class="meta-right">CLAUDE / OPENAI / GEMINI / GROK</span>
        </div>

        <h1>{{ siteName }}</h1>

        <div class="masthead-intro">
          <p class="value-line">{{ t('home.heroSubtitle') }}</p>
          <div class="intro-action">
            <p>{{ siteSubtitle }}</p>
            <div>
              <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="action-primary">
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                <Icon name="arrowRight" size="sm" />
              </router-link>
              <a
                v-if="docUrl"
                :href="docUrl"
                target="_blank"
                rel="noopener noreferrer"
                class="action-text"
              >
                {{ t('home.docs') }} <Icon name="externalLink" size="sm" />
              </a>
            </div>
          </div>
        </div>

        <div class="routing-loom" :aria-label="t('home.ui.routingPreview')">
          <div class="loom-labels">
            <span>01 / SAMPLE REQUEST</span>
            <span>02 / ROUTE</span>
            <span>03 / MODEL</span>
          </div>

          <div class="loom-flow">
            <div class="request-source">
              <span class="method">POST</span>
              <div>
                <strong>{{ t('home.ui.requestInspector') }}</strong>
                <code>/v1/messages</code>
              </div>
              <small>TRACE / SAMPLE</small>
            </div>

            <div class="incoming-track"><i></i></div>

            <div class="router-core">
              <Icon name="swap" size="md" />
              <span>ROUTER</span>
              <b>POLICY OK</b>
            </div>

            <div class="branch-track">
              <i
                v-for="(_, index) in routeModels"
                :key="index"
                :class="{ active: index === activeRouteIndex }"
              ></i>
            </div>

            <div class="model-targets">
              <div
                v-for="(route, index) in routeModels"
                :key="route.name"
                class="model-target"
                :class="{ active: index === activeRouteIndex }"
              >
                <span class="model-mark" :class="route.tone">
                  <PlatformIcon :platform="route.platform" size="md" />
                </span>
                <strong>{{ route.name }}</strong>
                <small>{{ index === activeRouteIndex ? t('home.ui.primaryRoute') : t('home.ui.ready') }}</small>
                <b>{{ index === activeRouteIndex ? 'SELECTED' : t('home.ui.standby') }}</b>
              </div>
            </div>
          </div>

          <div class="loom-response">
            <span><i></i> 200 OK</span>
            <code>ROUTE / {{ activeRoute.name.toUpperCase() }} / COMPLETE</code>
            <b>TRACE COMPLETE</b>
          </div>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'
import type { GroupPlatform } from '@/types'

const TechBackground3D = defineAsyncComponent({
  loader: () => import('@/components/visual/TechBackground3D.vue'),
  suspensible: false
})

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

interface RouteModel {
  name: string
  platform: GroupPlatform
  tone: string
}

const routeModels: RouteModel[] = [
  { name: 'Claude', platform: 'anthropic', tone: 'claude' },
  { name: 'OpenAI', platform: 'openai', tone: 'openai' },
  { name: 'Gemini', platform: 'gemini', tone: 'gemini' },
  { name: 'Grok', platform: 'grok', tone: 'grok' }
]
const activeRouteIndex = ref(0)
const activeRoute = computed(() => routeModels[activeRouteIndex.value] ?? routeModels[0])
const routeMotionAllowed = ref(!window.matchMedia('(prefers-reduced-motion: reduce)').matches)
const routeCycleMs = 5600
let routeCycleTimer: number | undefined
let routeMotionPreference: MediaQueryList | undefined

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isDark = ref(document.documentElement.classList.contains('dark'))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => {
  const user = authStore.user
  if (!user || !user.email) return ''
  return user.email.charAt(0).toUpperCase()
})

function stopRouteCycle() {
  if (routeCycleTimer !== undefined) {
    window.clearInterval(routeCycleTimer)
    routeCycleTimer = undefined
  }
}

function syncRouteCycle() {
  stopRouteCycle()
  if (routeMotionPreference?.matches) {
    activeRouteIndex.value = 0
    return
  }
  routeCycleTimer = window.setInterval(() => {
    activeRouteIndex.value = (activeRouteIndex.value + 1) % routeModels.length
  }, routeCycleMs)
}

function handleRouteMotionChange() {
  routeMotionAllowed.value = !routeMotionPreference?.matches
  syncRouteCycle()
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
  routeMotionPreference = window.matchMedia('(prefers-reduced-motion: reduce)')
  routeMotionAllowed.value = !routeMotionPreference.matches
  routeMotionPreference.addEventListener('change', handleRouteMotionChange)
  syncRouteCycle()
})

onBeforeUnmount(() => {
  stopRouteCycle()
  routeMotionPreference?.removeEventListener('change', handleRouteMotionChange)
})
</script>

<style scoped>
.portal {
  --canvas: #e8efed;
  --surface: #f8fbfa;
  --ink: #0a1213;
  --muted: #536562;
  --rule: #b7c5c2;
  --cobalt: #176bff;
  --coral: #ef6b45;
  --mint: #68dfbd;
  --tech-bg: #071012;
  --tech-panel: #0b1518;
  --tech-panel-raised: #101d20;
  --tech-ink: #eef7f5;
  --tech-muted: #8ba09d;
  --tech-rule: #29403f;
  --tech-electric: #6f91ff;
  --tech-signal: #42d5b5;
  --route-cycle: 5.6s;
  display: flex;
  height: 100svh;
  min-height: 100svh;
  flex-direction: column;
  overflow: hidden;
  background: var(--tech-bg);
  color: var(--ink);
  font-family: Aptos, Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
}

.portal.is-dark {
  --canvas: #070b0d;
  --surface: #101719;
  --ink: #eef5f3;
  --muted: #99aaa6;
  --rule: #2a3938;
  --cobalt: #6f91ff;
  --coral: #ff8768;
  --mint: #42d5b5;
  --tech-bg: #04090b;
  --tech-panel: #091114;
  --tech-panel-raised: #0e1a1d;
  --tech-rule: #243837;
}

.portal-header {
  position: relative;
  z-index: 2;
  flex: 0 0 auto;
  border-bottom: 1px solid var(--tech-rule);
  background: var(--tech-bg);
  color: var(--tech-ink);
}

.portal > main {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
  background: var(--tech-bg);
}

.portal-nav,
.masthead {
  width: min(1240px, calc(100% - 48px));
  margin: 0 auto;
}

.portal-nav {
  display: grid;
  min-height: 68px;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  gap: 24px;
}

.wordmark {
  display: inline-flex;
  align-items: center;
  gap: 11px;
  color: var(--ink);
  font-size: 14px;
  font-weight: 800;
  text-decoration: none;
}

.wordmark-logo {
  display: grid;
  width: 32px;
  height: 32px;
  place-items: center;
  overflow: hidden;
  border: 1px solid var(--rule);
  border-radius: 4px;
  background: var(--surface);
}

.wordmark-logo img { width: 100%; height: 100%; object-fit: contain; }

.portal-header .wordmark { color: var(--tech-ink); }
.portal-header .wordmark-logo {
  border-color: var(--tech-rule);
  background: var(--tech-panel-raised);
}

.header-status {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--tech-muted);
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 12px;
}

.header-status span:first-child { color: var(--tech-ink); }
.header-status i,
.loom-response i {
  display: inline-block;
  width: 6px;
  height: 6px;
  margin-right: 6px;
  border-radius: 1px;
  background: var(--tech-signal);
  box-shadow: none;
}

.header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
}

.square-action {
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  border: 1px solid transparent;
  border-radius: 4px;
  background: transparent;
  color: var(--muted);
}

.portal-header .square-action { color: var(--tech-muted); }

.portal-header .header-actions :deep(.relative > button:first-child) {
  color: var(--tech-muted);
}

.square-action:hover,
.square-action:focus-visible {
  border-color: var(--rule);
  background: var(--surface);
  color: var(--ink);
  outline: none;
}

.portal-header .square-action:hover,
.portal-header .square-action:focus-visible {
  border-color: var(--tech-rule);
  background: var(--tech-panel-raised);
  color: var(--tech-ink);
}

.header-login,
.action-primary {
  display: inline-flex;
  min-height: 38px;
  align-items: center;
  justify-content: center;
  gap: 9px;
  border: 1px solid var(--ink);
  border-radius: 4px;
  padding: 0 15px;
  background: var(--ink);
  color: var(--canvas);
  font-size: 14px;
  font-weight: 750;
  text-decoration: none;
  transition: transform 150ms ease, background-color 150ms ease, border-color 150ms ease;
}

.header-login:hover,
.action-primary:hover {
  transform: translateY(-2px);
  border-color: var(--cobalt);
  background: var(--cobalt);
  color: white;
}

.portal-header .header-login {
  border-color: var(--tech-electric);
  background: var(--tech-electric);
  color: #071012;
}

.portal-header .header-login:hover {
  border-color: var(--tech-signal);
  background: var(--tech-signal);
  color: #071012;
}

.header-login:focus-visible,
.action-primary:focus-visible,
.action-text:focus-visible {
  outline: 3px solid color-mix(in srgb, var(--cobalt) 28%, transparent);
  outline-offset: 3px;
}

.user-token {
  display: grid;
  width: 19px;
  height: 19px;
  place-items: center;
  border-radius: 50%;
  background: var(--coral);
  color: white;
  font-size: 11px;
}

.masthead {
  position: relative;
  z-index: 1;
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
  isolation: isolate;
  padding: 32px 0 0;
  color: var(--tech-ink);
}

.masthead::before {
  position: absolute;
  z-index: -2;
  top: 0;
  bottom: 0;
  left: calc(50% - 50vw);
  width: 100vw;
  background: var(--tech-bg);
  content: "";
  pointer-events: none;
}

.masthead-orbit {
  z-index: -1;
  right: auto;
  left: calc(50% - 50vw);
  width: 100vw;
}

.masthead-meta,
.loom-labels {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  color: var(--muted);
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 12px;
  font-weight: 700;
}

.masthead-meta {
  color: var(--tech-muted);
  animation: portal-module-enter 420ms 40ms ease-out both;
}

.masthead-meta code { text-align: center; }
.meta-right { text-align: right; }

.masthead h1 {
  margin: 22px 0 0;
  color: var(--tech-ink);
  font-family: Bahnschrift, "Arial Narrow", "Aptos Narrow", "PingFang SC", sans-serif;
  font-size: 84px;
  font-stretch: condensed;
  font-weight: 800;
  line-height: 0.9;
  overflow-wrap: anywhere;
  animation: portal-module-enter 520ms 100ms ease-out both;
}

.masthead h1::after {
  display: inline-block;
  width: 11px;
  height: 11px;
  margin-left: 18px;
  background: var(--tech-signal);
  box-shadow: 0 0 0 5px color-mix(in srgb, var(--tech-signal) 13%, transparent);
  content: "";
  vertical-align: 0.2em;
}

.masthead-intro {
  display: grid;
  max-width: 700px;
  grid-template-columns: 1fr;
  gap: 15px;
  align-items: end;
  margin-top: 24px;
  animation: portal-module-enter 520ms 180ms ease-out both;
}

.value-line {
  max-width: 650px;
  margin: 0;
  font-family: Bahnschrift, "Arial Narrow", "Aptos Narrow", "PingFang SC", sans-serif;
  font-size: 36px;
  font-weight: 650;
  line-height: 1.18;
}

.intro-action {
  display: grid;
  grid-template-columns: minmax(250px, 1fr) auto;
  align-items: end;
  gap: 30px;
}

.intro-action > p {
  max-width: 390px;
  margin: 0;
  color: var(--tech-muted);
  font-size: 14px;
  line-height: 1.75;
}

.intro-action > div {
  display: flex;
  align-items: center;
  gap: 22px;
  margin-top: 0;
}

.action-primary { min-height: 44px; padding: 0 19px; }

.action-text {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: var(--ink);
  font-size: 14px;
  font-weight: 700;
  text-decoration: none;
}

.action-text:hover { color: var(--cobalt); }

.masthead .action-primary {
  border-color: var(--tech-electric);
  background: var(--tech-electric);
  color: #071012;
}

.masthead .action-primary:hover {
  border-color: var(--tech-signal);
  background: var(--tech-signal);
  color: #071012;
}

.masthead .action-text { color: var(--tech-ink); }
.masthead .action-text:hover { color: var(--tech-signal); }

.routing-loom {
  --loom-gutter: max(24px, calc((100vw - 1240px) / 2));
  width: 100vw;
  display: flex;
  position: relative;
  flex: 1 1 0;
  min-height: 0;
  flex-direction: column;
  margin-top: 30px;
  margin-left: calc(50% - 50vw);
  border-top: 1px solid var(--tech-rule);
  border-bottom: 1px solid var(--tech-rule);
  background: var(--tech-panel);
  color: var(--tech-ink);
  box-shadow: inset 0 1px 0 color-mix(in srgb, var(--tech-ink) 4%, transparent);
  animation: portal-loom-enter 580ms 260ms ease-out both;
}

.routing-loom::before,
.routing-loom::after {
  position: absolute;
  z-index: 2;
  top: 0;
  bottom: 0;
  width: 1px;
  background: var(--tech-rule);
  content: "";
  pointer-events: none;
}

.routing-loom::before { left: var(--loom-gutter); }
.routing-loom::after { right: var(--loom-gutter); }

.loom-labels,
.loom-flow,
.loom-response {
  width: min(1240px, calc(100% - 48px));
  margin-right: auto;
  margin-left: auto;
}

.loom-labels {
  position: relative;
  flex: 0 0 auto;
  overflow: hidden;
  min-height: 42px;
  align-items: center;
  border-bottom: 1px solid var(--tech-rule);
  padding: 0 18px;
  color: var(--tech-muted);
}

.loom-labels::after {
  position: absolute;
  bottom: -1px;
  left: 0;
  width: 16%;
  height: 2px;
  background: var(--tech-signal);
  box-shadow: -10px 0 0 color-mix(in srgb, var(--tech-signal) 42%, transparent);
  content: "";
  opacity: 0;
  pointer-events: none;
  animation: route-stage-rail var(--route-cycle) linear infinite;
}

.loom-labels span:first-child { animation: route-stage-request var(--route-cycle) linear infinite; }
.loom-labels span:nth-child(2) { text-align: center; animation: route-stage-router var(--route-cycle) linear infinite; }
.loom-labels span:last-child { text-align: right; animation: route-stage-model var(--route-cycle) linear infinite; }

.loom-flow {
  display: grid;
  flex: 1 1 auto;
  min-height: 214px;
  grid-template-columns: minmax(230px, 1.15fr) minmax(50px, 0.65fr) 92px minmax(70px, 0.65fr) minmax(260px, 1.3fr);
  align-items: center;
  padding: 8px 18px;
}

.request-source {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 11px 13px;
  align-items: center;
}

.request-source .method { animation: route-ingress var(--route-cycle) linear infinite; }

.method {
  grid-row: span 2;
  border-radius: 3px;
  padding: 7px 8px;
  background: var(--coral);
  color: #fff;
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 11px;
  font-weight: 800;
}

.request-source strong,
.request-source code { display: block; }
.request-source strong { color: var(--tech-ink); font-size: 14px; }
.request-source code { margin-top: 4px; color: var(--tech-muted); font-size: 12px; }
.request-source small {
  grid-column: 2;
  color: var(--tech-muted);
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 11px;
}

.incoming-track {
  position: relative;
  height: 1px;
  margin: 0 12px;
  background: var(--tech-rule);
}

.incoming-track::before,
.incoming-track::after {
  position: absolute;
  top: -10px;
  display: grid;
  min-width: 38px;
  height: 20px;
  place-items: center;
  border: 1px solid var(--tech-rule);
  background: var(--tech-panel);
  color: var(--tech-muted);
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 11px;
  font-weight: 700;
}

.incoming-track::before { left: 18%; content: "KEY"; animation: route-key-check var(--route-cycle) linear infinite; }
.incoming-track::after { right: 10%; content: "POLICY"; animation: route-policy-check var(--route-cycle) linear infinite; }

.incoming-track i {
  position: absolute;
  top: -2px;
  left: 0;
  width: 12px;
  height: 4px;
  z-index: 2;
  border: 0;
  border-radius: 0;
  background: var(--tech-signal);
  box-shadow:
    -7px 0 0 color-mix(in srgb, var(--tech-signal) 68%, transparent),
    -14px 0 0 color-mix(in srgb, var(--tech-signal) 28%, transparent);
  transform-origin: right center;
  will-change: left, opacity, transform;
  animation: route-packet var(--route-cycle) linear infinite;
}

.router-core {
  position: relative;
  display: grid;
  width: 82px;
  height: 72px;
  place-items: center;
  align-content: center;
  border: 1px solid var(--tech-electric);
  transform: none;
  background: var(--tech-panel-raised);
  color: var(--tech-electric);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tech-electric) 8%, transparent);
  transform-origin: center;
  animation: route-core-lock var(--route-cycle) linear infinite;
}

.router-core::before,
.router-core::after {
  position: absolute;
  width: 11px;
  height: 11px;
  content: "";
  pointer-events: none;
  animation: route-core-brackets var(--route-cycle) linear infinite;
}

.router-core::before { top: -4px; left: -4px; border-top: 2px solid var(--tech-signal); border-left: 2px solid var(--tech-signal); transform-origin: top left; }
.router-core::after { right: -4px; bottom: -4px; border-right: 2px solid var(--tech-signal); border-bottom: 2px solid var(--tech-signal); transform-origin: bottom right; }
.router-core > * { transform: none; }
.router-core svg { transform-origin: center; animation: route-decision var(--route-cycle) linear infinite; }
.router-core span { margin-top: 4px; font-family: "SFMono-Regular", Consolas, monospace; font-size: 11px; }
.router-core b { margin-top: 2px; color: var(--tech-muted); font-family: "SFMono-Regular", Consolas, monospace; font-size: 11px; animation: route-policy-state var(--route-cycle) linear infinite; }

.branch-track {
  display: grid;
  height: 208px;
  grid-template-rows: repeat(4, 49px);
  align-items: center;
  gap: 4px;
  margin-left: 12px;
  border-left: 1px solid var(--tech-rule);
}

.branch-track i { display: block; position: relative; height: 1px; background: var(--tech-rule); transform-origin: left center; }
.branch-track i.active { animation: route-branch-draw var(--route-cycle) linear infinite; }
.branch-track i.active::after {
  position: absolute;
  top: -1px;
  left: 0;
  width: 9px;
  height: 3px;
  background: var(--tech-signal);
  box-shadow:
    -6px 0 0 color-mix(in srgb, var(--tech-signal) 58%, transparent),
    -12px 0 0 color-mix(in srgb, var(--tech-signal) 22%, transparent);
  content: "";
  opacity: 0;
  transform-origin: right center;
  animation: route-branch-packet var(--route-cycle) linear infinite;
}

.model-targets { display: grid; gap: 4px; }

.model-target {
  display: grid;
  position: relative;
  overflow: hidden;
  min-height: 44px;
  grid-template-columns: 32px 1fr auto;
  grid-template-rows: auto auto;
  align-items: center;
  gap: 1px 11px;
  border: 1px solid var(--tech-rule);
  border-radius: 4px;
  padding: 5px 10px;
  background: var(--tech-panel-raised);
  transition: border-color 180ms ease, background-color 180ms ease, transform 180ms ease;
}

.model-target.active {
  border-color: var(--tech-electric);
  background: color-mix(in srgb, var(--tech-electric) 12%, var(--tech-panel-raised));
  animation: route-target-enter 180ms ease-out both, route-target-lock var(--route-cycle) linear infinite;
}

.model-target.active::before {
  position: absolute;
  z-index: 1;
  top: 0;
  bottom: 0;
  left: 0;
  width: 2px;
  background: var(--tech-signal);
  box-shadow:
    -6px 0 0 color-mix(in srgb, var(--tech-signal) 48%, transparent),
    -13px 0 0 color-mix(in srgb, var(--tech-signal) 18%, transparent);
  content: "";
  opacity: 0;
  pointer-events: none;
  animation: route-target-scan var(--route-cycle) linear infinite;
}

.model-target.active::after {
  position: absolute;
  top: 7px;
  right: -1px;
  bottom: 7px;
  width: 3px;
  background: var(--tech-signal);
  content: "";
  animation: route-target-indicator var(--route-cycle) linear infinite;
}

.model-mark {
  display: grid;
  width: 30px;
  height: 30px;
  grid-row: span 2;
  place-items: center;
  border-radius: 3px;
  color: white;
  font-size: 12px;
  font-style: normal;
  font-weight: 800;
}

.claude { background: #bf6746; }
.openai { background: #24725b; }
.gemini { background: #3568a8; }
.grok { background: #dbe4e2; color: #071012; }

.model-target strong { font-size: 13px; }
.model-target small { color: var(--tech-muted); font-size: 11px; }
.model-target > b {
  grid-column: 3;
  grid-row: 1 / span 2;
  color: var(--tech-muted);
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 11px;
  font-weight: 500;
}

.model-target.active > b { color: var(--tech-electric); }
.model-target.active .model-mark { animation: route-mark-lock var(--route-cycle) linear infinite; }

.loom-response {
  display: grid;
  position: relative;
  flex: 0 0 auto;
  overflow: hidden;
  min-height: 42px;
  grid-template-columns: 1fr 2fr 1fr;
  align-items: center;
  border-top: 1px solid var(--tech-rule);
  padding: 0 18px;
  color: var(--tech-muted);
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 11px;
}

.loom-response::before {
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  width: 3px;
  background: var(--tech-signal);
  box-shadow:
    -8px 0 0 color-mix(in srgb, var(--tech-signal) 52%, transparent),
    -18px 0 0 color-mix(in srgb, var(--tech-signal) 18%, transparent);
  content: "";
  opacity: 0;
  animation: route-response-scan var(--route-cycle) linear infinite;
}

.loom-response::after {
  position: absolute;
  top: 0;
  right: 0;
  left: 0;
  height: 1px;
  background: var(--tech-signal);
  content: "";
  pointer-events: none;
  transform: scaleX(0);
  transform-origin: left center;
  animation: route-response-rail var(--route-cycle) linear infinite;
}

.loom-response > span { animation: route-response-ok var(--route-cycle) linear infinite; }
.loom-response > code { animation: route-response-code var(--route-cycle) linear infinite; }
.loom-response > b { animation: route-response-complete var(--route-cycle) linear infinite; }

.loom-response i { background: var(--tech-signal); transform-origin: center; animation: route-response-led var(--route-cycle) linear infinite; }
.loom-response b { color: var(--tech-ink); }

.loom-response code { text-align: center; }
.loom-response b { text-align: right; font-size: 11px; }

@keyframes portal-module-enter {
  from { opacity: 0; transform: translateY(10px); clip-path: inset(0 0 24% 0); }
  to { opacity: 1; transform: translateY(0); clip-path: inset(0); }
}

@keyframes portal-loom-enter {
  from { opacity: 0; clip-path: inset(0 0 100% 0); }
  to { opacity: 1; clip-path: inset(0); }
}

@keyframes route-stage-rail {
  0%, 4% { left: 0; opacity: 0; transform: scaleX(0.25); }
  7%, 24% { left: 0; opacity: 1; transform: scaleX(1); }
  31%, 42% { left: 42%; opacity: 1; transform: scaleX(0.72); }
  48%, 76% { left: 84%; opacity: 1; transform: scaleX(1); }
  82%, 100% { left: 84%; opacity: 0; transform: scaleX(0.25); }
}

@keyframes route-stage-request {
  0%, 4%, 30%, 100% { color: var(--tech-muted); }
  7%, 25% { color: var(--tech-signal); }
}

@keyframes route-stage-router {
  0%, 27%, 46%, 100% { color: var(--tech-muted); }
  31%, 42% { color: var(--tech-signal); }
}

@keyframes route-stage-model {
  0%, 43%, 82%, 100% { color: var(--tech-muted); }
  48%, 78% { color: var(--tech-signal); }
}

@keyframes route-ingress {
  0%, 5%, 88%, 100% { opacity: 0.66; }
  8%, 76% { opacity: 1; }
}

@keyframes route-packet {
  0%, 5% { left: 0; opacity: 0; transform: scaleX(0.25); }
  7% { left: 0; opacity: 1; transform: scaleX(1); }
  23% { opacity: 1; transform: scaleX(2.15); }
  30% { left: calc(100% - 12px); opacity: 1; transform: scaleX(0.72); }
  33%, 100% { left: calc(100% - 12px); opacity: 0; transform: scaleX(0.2); }
}

@keyframes route-key-check {
  0%, 10%, 82%, 100% { border-color: var(--tech-rule); background: var(--tech-panel); color: var(--tech-muted); }
  13%, 76% { border-color: var(--tech-signal); background: var(--tech-panel-raised); color: var(--tech-ink); }
}

@keyframes route-policy-check {
  0%, 19%, 82%, 100% { border-color: var(--tech-rule); background: var(--tech-panel); color: var(--tech-muted); }
  22%, 76% { border-color: var(--tech-signal); background: var(--tech-panel-raised); color: var(--tech-ink); }
}

@keyframes route-core-lock {
  0%, 27%, 84%, 100% {
    border-color: var(--tech-electric);
    background: var(--tech-panel-raised);
    color: var(--tech-electric);
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tech-electric) 8%, transparent);
    transform: scale(1);
  }
  29% { transform: scale(0.96); }
  32% {
    border-color: var(--tech-signal);
    background: color-mix(in srgb, var(--tech-signal) 13%, var(--tech-panel-raised));
    color: var(--tech-signal);
    box-shadow: inset 4px 0 0 var(--tech-signal), inset -4px 0 0 color-mix(in srgb, var(--tech-signal) 42%, transparent), 0 0 0 2px color-mix(in srgb, var(--tech-signal) 24%, transparent);
    transform: scale(1.045);
  }
  37%, 76% {
    border-color: var(--tech-signal);
    background: color-mix(in srgb, var(--tech-signal) 8%, var(--tech-panel-raised));
    color: var(--tech-signal);
    box-shadow: inset 3px 0 0 color-mix(in srgb, var(--tech-signal) 66%, transparent), inset -1px 0 0 color-mix(in srgb, var(--tech-signal) 24%, transparent);
    transform: scale(1);
  }
}

@keyframes route-core-brackets {
  0%, 27%, 84%, 100% { width: 11px; height: 11px; border-color: var(--tech-signal); opacity: 0.72; }
  30% { width: 19px; height: 19px; border-color: var(--tech-electric); opacity: 1; }
  34%, 76% { width: 14px; height: 14px; border-color: var(--tech-signal); opacity: 1; }
}

@keyframes route-decision {
  0%, 28%, 100% { transform: rotate(0deg); }
  30% { transform: rotate(45deg); }
  32% { transform: rotate(90deg); }
  34% { transform: rotate(135deg); }
  36%, 83% { transform: rotate(180deg); }
  86% { transform: rotate(0deg); }
}

@keyframes route-policy-state {
  0%, 27%, 84%, 100% { color: var(--tech-muted); }
  31%, 78% { color: var(--tech-signal); }
}

@keyframes route-branch-draw {
  0%, 35%, 87%, 100% { transform: scaleX(0); background: var(--tech-rule); }
  39% { transform: scaleX(0.42); background: var(--tech-signal); }
  44%, 79% { transform: scaleX(1); background: var(--tech-signal); }
}

@keyframes route-branch-packet {
  0%, 39%, 83%, 100% { left: 0; opacity: 0; transform: scaleX(0.3); }
  42% { left: 0; opacity: 1; transform: scaleX(1.8); }
  51% { left: calc(100% - 9px); opacity: 1; transform: scaleX(0.8); }
  54% { left: calc(100% - 9px); opacity: 0; transform: scaleX(0.2); }
}

@keyframes route-target-enter {
  from { opacity: 0.62; transform: translateX(-5px); }
  to { opacity: 1; transform: translateX(0); }
}

@keyframes route-target-lock {
  0%, 42%, 87%, 100% {
    border-color: var(--tech-electric);
    background: color-mix(in srgb, var(--tech-electric) 12%, var(--tech-panel-raised));
    box-shadow: inset 0 0 0 transparent;
  }
  46% {
    border-color: var(--tech-signal);
    background: color-mix(in srgb, var(--tech-signal) 17%, var(--tech-panel-raised));
    box-shadow: inset 5px 0 0 var(--tech-signal), inset -2px 0 0 color-mix(in srgb, var(--tech-signal) 42%, transparent);
  }
  51%, 80% {
    border-color: var(--tech-signal);
    background: color-mix(in srgb, var(--tech-signal) 10%, var(--tech-panel-raised));
    box-shadow: inset 3px 0 0 color-mix(in srgb, var(--tech-signal) 62%, transparent);
  }
}

@keyframes route-target-indicator {
  0%, 42%, 86%, 100% { opacity: 0; }
  47%, 80% { opacity: 1; }
}

@keyframes route-target-scan {
  0%, 43% { left: 0; opacity: 0; }
  46% { left: 0; opacity: 1; }
  55% { left: calc(100% - 2px); opacity: 1; }
  58%, 100% { left: calc(100% - 2px); opacity: 0; }
}

@keyframes route-mark-lock {
  0%, 43%, 84%, 100% { box-shadow: 0 0 0 transparent; transform: scale(1); }
  46% { box-shadow: 0 0 0 3px color-mix(in srgb, var(--tech-signal) 28%, transparent); transform: scale(0.88); }
  49% { box-shadow: 0 0 0 2px color-mix(in srgb, var(--tech-signal) 46%, transparent); transform: scale(1.12); }
  54%, 80% { box-shadow: 0 0 0 1px color-mix(in srgb, var(--tech-signal) 22%, transparent); transform: scale(1); }
}

@keyframes route-response-scan {
  0%, 49% { left: 0; opacity: 0; }
  52% { left: 0; opacity: 1; }
  66% { left: calc(100% - 3px); opacity: 1; }
  69%, 100% { left: calc(100% - 3px); opacity: 0; }
}

@keyframes route-response-rail {
  0%, 49%, 90%, 100% { transform: scaleX(0); transform-origin: left center; }
  53%, 78% { transform: scaleX(1); transform-origin: left center; }
  84% { transform: scaleX(0); transform-origin: right center; }
}

@keyframes route-response-ok {
  0%, 51%, 89%, 100% { color: var(--tech-muted); opacity: 0.58; }
  55%, 84% { color: var(--tech-signal); opacity: 1; }
}

@keyframes route-response-code {
  0%, 56%, 89%, 100% { color: var(--tech-muted); opacity: 0.58; }
  60%, 84% { color: var(--tech-ink); opacity: 1; }
}

@keyframes route-response-complete {
  0%, 61%, 89%, 100% { color: var(--tech-muted); opacity: 0.58; }
  65%, 84% { color: var(--tech-signal); opacity: 1; }
}

@keyframes route-response-led {
  0%, 51%, 89%, 100% { opacity: 0.42; transform: scale(0.72); }
  55% { opacity: 1; transform: scale(1.65); }
  59%, 84% { opacity: 1; transform: scale(1); }
}

@keyframes route-packet-mobile {
  0%, 6% { top: 0; left: -2px; opacity: 0; box-shadow: 0 -7px 0 color-mix(in srgb, var(--tech-signal) 52%, transparent), 0 -14px 0 color-mix(in srgb, var(--tech-signal) 20%, transparent); transform: scaleX(0.4); }
  9% { top: 0; left: -2px; opacity: 1; transform: scaleX(1); }
  20% { top: calc(100% - 4px); left: -2px; opacity: 1; box-shadow: 0 -7px 0 color-mix(in srgb, var(--tech-signal) 52%, transparent), 0 -14px 0 color-mix(in srgb, var(--tech-signal) 20%, transparent); transform: scaleX(1); }
  22% { box-shadow: -7px 0 0 color-mix(in srgb, var(--tech-signal) 62%, transparent), -14px 0 0 color-mix(in srgb, var(--tech-signal) 24%, transparent); transform: scaleX(1.7); }
  30% { top: calc(100% - 4px); left: calc(100% - 12px); opacity: 1; box-shadow: -7px 0 0 color-mix(in srgb, var(--tech-signal) 62%, transparent), -14px 0 0 color-mix(in srgb, var(--tech-signal) 24%, transparent); transform: scaleX(0.75); }
  33%, 100% { top: calc(100% - 4px); left: calc(100% - 12px); opacity: 0; transform: scaleX(0.2); }
}

@media (max-width: 1000px) {
  .masthead h1 { font-size: 72px; }
  .masthead-intro { gap: 15px; }
  .loom-flow { grid-template-columns: minmax(190px, 1fr) 45px 82px 50px minmax(220px, 1.1fr); }
  .incoming-track::before,
  .incoming-track::after { display: none; }
}

@media (max-width: 760px) {
  .portal-nav,
  .masthead { width: min(100% - 28px, 1240px); }

  .portal-nav { min-height: 62px; grid-template-columns: auto 1fr; }
  .wordmark > span:last-child,
  .header-status,
  .header-actions > a.square-action,
  .header-login svg,
  .user-token { display: none; }
  .header-actions { justify-self: end; }
  .header-login { padding: 0 13px; }

  .masthead { padding: 24px 0 0; }
  .masthead-meta { grid-template-columns: 1fr auto; }
  .masthead-meta code { text-align: right; }
  .meta-right { display: none; }
  .masthead h1 { margin-top: 26px; font-size: 54px; line-height: 0.92; }
  .masthead h1::after { width: 8px; height: 8px; margin-left: 11px; box-shadow: 0 0 0 4px color-mix(in srgb, var(--tech-signal) 13%, transparent); }
  .masthead-intro { max-width: none; grid-template-columns: 1fr; gap: 22px; margin-top: 32px; }
  .intro-action { display: block; }
  .value-line { font-size: 27px; }
  .intro-action > p { font-size: 13px; }
  .intro-action > div { justify-content: space-between; margin-top: 20px; }

  .routing-loom { --loom-gutter: 14px; margin-top: 40px; }
  .loom-labels,
  .loom-flow,
  .loom-response { width: calc(100% - 28px); }
  .loom-labels { grid-template-columns: 1fr 1fr; }
  .loom-labels span:nth-child(2) { text-align: right; }
  .loom-labels span:last-child { display: none; }
  .loom-flow { display: block; min-height: 0; padding: 22px 14px; }
  .request-source { max-width: 280px; }
  .incoming-track { width: calc(50% - 32px); height: 70px; margin: 16px 0 16px 32px; border-bottom: 1px solid var(--tech-rule); border-left: 1px solid var(--tech-rule); background: transparent; }
  .incoming-track::before,
  .incoming-track::after { display: none; }
  .incoming-track i { top: 0; bottom: auto; animation: route-packet-mobile var(--route-cycle) linear infinite; }
  .router-core { width: 72px; height: 72px; margin: -52px auto 34px; }
  .branch-track { display: none; }
  .model-targets { margin-top: 0; }
  .model-target:not(.active) { display: none; }
  .loom-response { grid-template-columns: 1fr auto; padding: 0 14px; }
  .loom-response code { display: none; }
}

@media (min-width: 561px) and (max-width: 760px) {
  .loom-labels { grid-template-columns: 1fr 1fr 1fr; }
  .loom-labels span:nth-child(2) { text-align: center; }
  .loom-labels span:last-child { display: block; text-align: right; }

  .loom-flow {
    display: grid;
    grid-template-columns: minmax(140px, 1fr) 34px 72px 34px minmax(170px, 1fr);
    align-items: center;
    padding: 8px 14px;
  }

  .request-source { max-width: none; }
  .incoming-track {
    width: auto;
    height: 1px;
    margin: 0 8px;
    border: 0;
    background: var(--tech-rule);
  }
  .incoming-track i { top: -2px; animation: route-packet var(--route-cycle) linear infinite; }
  .router-core { width: 72px; height: 72px; margin: 0; }
  .branch-track {
    display: block;
    height: 1px;
    margin-left: 8px;
    border: 0;
  }
  .branch-track i { display: none; }
  .branch-track i.active { display: block; }
  .model-targets { min-width: 0; margin: 0; }
  .model-target.active { display: grid; }
}

@media (prefers-reduced-motion: reduce) {
  .portal *, .portal *::before, .portal *::after {
    animation: none !important;
    transition-duration: 0.01ms !important;
  }
  .incoming-track i,
  .loom-response::before { display: none; }
  .branch-track i.active { transform: scaleX(1); }
}
</style>
