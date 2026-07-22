<template>
  <div class="auth-workbench" :class="{ 'is-dark': isDark }">
    <TechBackground3D v-if="props.immersive && authMotionAllowed" class="auth-orbit" variant="auth" />

    <header class="workbench-header">
      <router-link to="/home" class="auth-wordmark" :aria-label="siteName">
        <span class="auth-logo"><img :src="siteLogo || '/logo.svg'" alt="" /></span>
        <strong>{{ siteName }}</strong>
      </router-link>

      <div class="gateway-state"><i></i>{{ t('auth.gateway.online') }}<code>/v1</code></div>

      <div class="workbench-actions">
        <LocaleSwitcher />
        <button
          type="button"
          class="theme-action"
          :aria-label="isDark ? t('auth.gateway.switchToLight') : t('auth.gateway.switchToDark')"
          :title="isDark ? t('auth.gateway.switchToLight') : t('auth.gateway.switchToDark')"
          @click="toggleTheme"
        >
          <Icon :name="isDark ? 'sun' : 'moon'" size="sm" />
        </button>
      </div>
    </header>

    <div class="protocol-strip" aria-hidden="true">
      <span>IDENTITY / ACCESS</span>
      <code>POST /auth/session</code>
      <span>TLS 1.3 / SECURE</span>
    </div>

    <main class="auth-grid">
      <aside class="route-board" :aria-label="t('auth.gateway.eyebrow')">
        <div class="route-intro">
          <span>{{ t('auth.gateway.eyebrow') }}</span>
          <h1 class="whitespace-pre-line">{{ t('auth.gateway.headline') }}</h1>
          <p>{{ siteSubtitle }}</p>
        </div>

        <div class="route-table">
          <div class="route-table-head">
            <span>MODEL ROUTE</span><span>STATE</span>
          </div>
          <div><span><i class="provider-mark claude"><PlatformIcon platform="anthropic" size="sm" /></i>Claude</span><b>{{ t('auth.gateway.ready') }}</b></div>
          <div><span><i class="provider-mark openai"><PlatformIcon platform="openai" size="sm" /></i>OpenAI</span><b>{{ t('auth.gateway.ready') }}</b></div>
          <div><span><i class="provider-mark gemini"><PlatformIcon platform="gemini" size="sm" /></i>Gemini</span><b>{{ t('auth.gateway.ready') }}</b></div>
          <div><span><i class="provider-mark grok"><PlatformIcon platform="grok" size="sm" /></i>Grok</span><b>{{ t('auth.gateway.ready') }}</b></div>
        </div>
      </aside>

      <section class="auth-center">
        <div class="session-ports" aria-hidden="true"><i></i><i></i></div>

        <div class="form-signal">
          <span>SECURE SESSION</span>
          <i></i>
          <code>AUTH / 01</code>
        </div>

        <div class="auth-slot">
          <slot />
        </div>

        <div class="auth-switch">
          <slot name="footer" />
        </div>
      </section>

      <aside class="trace-board" aria-label="Request trace">
        <div class="trace-heading">
          <span>{{ t('auth.gateway.apiRequest') }}</span>
          <b><i></i>HANDSHAKE</b>
        </div>

        <div class="trace-request">
          <span>POST</span>
          <code>/auth/session</code>
        </div>

        <div class="trace-sequence" aria-hidden="true">
          <span>TLS</span><i></i><span>IDENTITY</span><i></i><span>SESSION</span>
        </div>

        <div class="trace-path">
          <div><i>01</i><span>{{ t('auth.gateway.protectedConnection') }}</span><b>OK</b></div>
          <div><i>02</i><span>{{ t('auth.gateway.secureRouting') }}</span><b>OK</b></div>
          <div><i>03</i><span>{{ t('auth.gateway.routed') }}</span><b>READY</b></div>
        </div>
      </aside>
    </main>

    <footer class="workbench-footer">
      <span>&copy; {{ currentYear }} {{ siteName }}</span>
      <div class="footer-track"><i></i></div>
      <span>{{ t('auth.gateway.protectedConnection') }}</span>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{ immersive?: boolean }>(), {
  immersive: false
})

const TechBackground3D = defineAsyncComponent({
  loader: () => import('@/components/visual/TechBackground3D.vue'),
  suspensible: false
})

const appStore = useAppStore()
const { t } = useI18n()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const currentYear = computed(() => new Date().getFullYear())
const isDark = ref(document.documentElement.classList.contains('dark'))
const authMotionAllowed = ref(!window.matchMedia('(prefers-reduced-motion: reduce)').matches)
let authMotionPreference: MediaQueryList | undefined

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

function handleAuthMotionChange() {
  authMotionAllowed.value = !authMotionPreference?.matches
}

onMounted(() => {
  initTheme()
  appStore.fetchPublicSettings()
  authMotionPreference = window.matchMedia('(prefers-reduced-motion: reduce)')
  authMotionAllowed.value = !authMotionPreference.matches
  authMotionPreference.addEventListener('change', handleAuthMotionChange)
})

onBeforeUnmount(() => {
  authMotionPreference?.removeEventListener('change', handleAuthMotionChange)
})
</script>

<style scoped>
.auth-workbench {
  --canvas: #e8efed;
  --surface: #f8fbfa;
  --ink: #0a1213;
  --muted: #536562;
  --rule: #b7c5c2;
  --cobalt: #176bff;
  --coral: #ef6b45;
  --mint: #68dfbd;
  --shell: #071012;
  --shell-raised: #0e1a1d;
  --shell-ink: #eef7f5;
  --shell-muted: #8ba09d;
  --shell-rule: #29403f;
  --auth-signal: var(--cobalt);
  --auth-panel: #0d191c;
  --auth-field: #142326;
  --auth-ink: #eef7f5;
  --auth-muted: #a5b7b3;
  --auth-rule: #314846;
  --auth-accent: #6f91ff;
  --auth-cycle: 8.4s;
  display: flex;
  position: relative;
  min-height: 100svh;
  flex-direction: column;
  overflow: hidden;
  background: var(--shell);
  color: var(--shell-ink);
  font-family: Aptos, Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
}

.auth-orbit {
  position: fixed;
  z-index: 0;
  width: 100vw;
  height: 100svh;
}

.auth-workbench.is-dark {
  --canvas: #070b0d;
  --surface: #101719;
  --ink: #eef5f3;
  --muted: #99aaa6;
  --rule: #2a3938;
  --cobalt: #6f91ff;
  --coral: #ff8768;
  --mint: #42d5b5;
  --shell: #04090b;
  --shell-raised: #0a1316;
  --shell-rule: #243837;
  --auth-panel: #091315;
  --auth-field: #101d20;
  --auth-ink: #eef7f5;
  --auth-muted: #9eb0ac;
  --auth-rule: #293d3b;
  --auth-accent: #7898ff;
}

.workbench-header,
.protocol-strip,
.auth-grid,
.workbench-footer {
  width: min(1280px, calc(100% - 48px));
  margin: 0 auto;
}

.workbench-header {
  display: grid;
  min-height: 68px;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  gap: 24px;
}

.auth-workbench::before {
  position: fixed;
  z-index: 0;
  top: 0;
  bottom: 0;
  left: 50%;
  width: min(1280px, calc(100% - 48px));
  border-right: 1px solid var(--shell-rule);
  border-left: 1px solid var(--shell-rule);
  content: "";
  opacity: 0.55;
  pointer-events: none;
  transform: translateX(-50%);
}

.workbench-header,
.protocol-strip,
.auth-grid,
.workbench-footer { position: relative; z-index: 1; }

.auth-wordmark {
  display: inline-flex;
  align-items: center;
  gap: 11px;
  color: var(--shell-ink);
  font-size: 14px;
  text-decoration: none;
}

.auth-logo {
  display: grid;
  width: 32px;
  height: 32px;
  place-items: center;
  overflow: hidden;
  border: 1px solid var(--shell-rule);
  border-radius: 4px;
  background: var(--shell-raised);
}

.auth-logo img { width: 100%; height: 100%; object-fit: contain; }

.gateway-state {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--shell-muted);
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 12px;
  text-transform: uppercase;
}

.gateway-state i,
.trace-heading b i {
  width: 6px;
  height: 6px;
  border-radius: 1px;
  background: var(--mint);
  box-shadow: none;
}

.gateway-state code { margin-left: 4px; color: var(--shell-ink); }

.workbench-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
}

.theme-action {
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  border: 1px solid transparent;
  border-radius: 4px;
  background: transparent;
  color: var(--shell-muted);
}

.workbench-actions :deep(.relative > button:first-child) { color: var(--shell-muted); }

.theme-action:hover,
.theme-action:focus-visible {
  border-color: var(--shell-rule);
  background: var(--shell-raised);
  color: var(--shell-ink);
  outline: none;
}

.protocol-strip {
  display: grid;
  min-height: 38px;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  border-top: 1px solid var(--shell-rule);
  border-bottom: 1px solid var(--shell-rule);
  color: var(--shell-muted);
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 11px;
  font-weight: 700;
}

.protocol-strip code { color: var(--mint); text-align: center; }
.protocol-strip span:last-child { text-align: right; }

.auth-grid {
  display: grid;
  position: relative;
  min-height: calc(100svh - 154px);
  flex: 1;
  grid-template-columns: minmax(250px, 0.95fr) minmax(410px, 470px) minmax(240px, 0.9fr);
  align-items: center;
  gap: 62px;
  padding: 54px 0 48px;
}

.auth-grid::before {
  position: absolute;
  z-index: 0;
  top: 50%;
  right: 0;
  left: 0;
  height: 1px;
  background: var(--shell-rule);
  content: "";
  pointer-events: none;
}

.auth-grid::after {
  position: absolute;
  z-index: 1;
  top: calc(50% - 2px);
  left: 0;
  width: 12px;
  height: 4px;
  border: 0;
  border-radius: 0;
  background: var(--mint);
  box-shadow: none;
  content: "";
  pointer-events: none;
  animation: auth-handshake var(--auth-cycle) linear infinite;
}

.route-board,
.trace-board {
  position: relative;
  z-index: 2;
  min-width: 0;
}

.route-board { animation: auth-module-enter 420ms 80ms ease-out both; }
.trace-board { animation: auth-module-enter 420ms 180ms ease-out both; }

.route-intro > span,
.form-signal,
.trace-heading,
.route-table-head {
  color: var(--mint);
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 11px;
  font-weight: 800;
  text-transform: uppercase;
}

.route-intro h1 {
  margin: 19px 0 0;
  font-family: Bahnschrift, "Arial Narrow", "Aptos Narrow", "PingFang SC", sans-serif;
  font-size: 51px;
  font-weight: 750;
  line-height: 0.97;
  color: var(--shell-ink);
}

.route-intro p {
  max-width: 330px;
  margin: 18px 0 0;
  color: var(--shell-muted);
  font-size: 14px;
  line-height: 1.65;
}

.route-table {
  margin-top: 48px;
  border-top: 1px solid var(--shell-ink);
}

.route-table-head,
.route-table > div:not(.route-table-head) {
  display: grid;
  min-height: 43px;
  grid-template-columns: 1fr auto;
  align-items: center;
  border-bottom: 1px solid var(--shell-rule);
}

.route-table-head span:last-child { text-align: right; }

.route-table > div:not(.route-table-head) > span {
  display: flex;
  align-items: center;
  gap: 9px;
  font-size: 12px;
  font-weight: 700;
}

.route-table > div:not(.route-table-head) > b {
  color: var(--shell-muted);
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 11px;
  font-weight: 500;
}

.provider-mark {
  display: grid;
  width: 23px;
  height: 23px;
  place-items: center;
  border-radius: 3px;
  color: white;
  font-size: 11px;
  font-style: normal;
}

.provider-mark.claude { background: #bf6746; }
.provider-mark.openai { background: #24725b; }
.provider-mark.gemini { background: #3568a8; }
.provider-mark.grok { background: #dbe4e2; color: #071012; }

.auth-center {
  --surface: var(--auth-field);
  --ink: var(--auth-ink);
  --muted: var(--auth-muted);
  --rule: var(--auth-rule);
  --cobalt: var(--auth-accent);
  position: relative;
  z-index: 3;
  isolation: isolate;
  align-self: center;
  border: 1px solid var(--auth-rule);
  border-top: 4px solid var(--auth-accent);
  padding: 22px 30px 28px;
  background: color-mix(in srgb, var(--auth-panel) 96%, transparent);
  color: var(--auth-ink);
  box-shadow: inset 0 1px 0 color-mix(in srgb, var(--auth-ink) 5%, transparent);
  animation: auth-panel-enter 620ms 100ms ease-out both, auth-panel-state var(--auth-cycle) 720ms linear infinite;
}

.auth-center::before,
.auth-center::after {
  position: absolute;
  z-index: -1;
  top: 50%;
  width: 37px;
  height: 1px;
  background: var(--shell-rule);
  content: "";
}

.auth-center::before { left: -38px; }
.auth-center::after { right: -38px; }

.session-ports {
  position: absolute;
  z-index: 3;
  inset: 0;
  overflow: visible;
  pointer-events: none;
}

.session-ports i {
  position: absolute;
  top: calc(50% - 2px);
  width: 12px;
  height: 4px;
  background: var(--mint);
  opacity: 0;
}

.session-ports i:first-child { left: -38px; animation: auth-port-in var(--auth-cycle) linear infinite; }
.session-ports i:last-child { right: -38px; animation: auth-port-out var(--auth-cycle) linear infinite; }

.form-signal {
  display: grid;
  min-height: 28px;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 12px;
  margin-bottom: 25px;
  color: var(--cobalt);
  animation: auth-content-enter 420ms 330ms ease-out both;
}

.form-signal i {
  position: relative;
  display: block;
  overflow: hidden;
  height: 1px;
  background: var(--rule);
}

.form-signal i::before {
  position: absolute;
  inset: 0 auto 0 0;
  width: 26%;
  background: var(--mint);
  content: "";
  opacity: 0;
  transform: translateX(-110%);
  animation: auth-form-scan var(--auth-cycle) linear infinite;
}

.form-signal i::after {
  position: absolute;
  top: -2px;
  left: 0;
  width: 12px;
  height: 4px;
  border-radius: 0;
  background: var(--cobalt);
  content: "";
  box-shadow: none;
  transition: left 320ms ease;
}

.auth-center:focus-within .form-signal i::after { left: calc(100% - 12px); }
.form-signal code { color: var(--muted); font-size: 11px; }
.auth-slot { width: 100%; animation: auth-content-enter 460ms 390ms ease-out both; }

.auth-switch {
  margin-top: 24px;
  padding-top: 19px;
  border-top: 1px solid var(--rule);
  color: var(--muted);
  font-size: 14px;
  text-align: left;
  animation: auth-content-enter 460ms 450ms ease-out both;
}

.trace-board {
  border-top: 1px solid var(--shell-ink);
  border-bottom: 1px solid var(--shell-ink);
  background: var(--shell-raised);
  color: var(--shell-ink);
}

.trace-heading,
.trace-request {
  display: flex;
  min-height: 43px;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 0 14px;
  border-bottom: 1px solid var(--shell-rule);
}

.trace-heading b { display: inline-flex; align-items: center; gap: 7px; color: var(--shell-muted); font-size: 11px; }
.trace-request { justify-content: flex-start; font-family: "SFMono-Regular", Consolas, monospace; font-size: 11px; }
.trace-request span { border-radius: 3px; padding: 5px 6px; background: var(--coral); color: white; font-size: 11px; font-weight: 800; }
.trace-request code { overflow: hidden; color: var(--shell-muted); text-overflow: ellipsis; white-space: nowrap; }

.trace-sequence {
  display: grid;
  min-height: 82px;
  grid-template-columns: auto 1fr auto 1fr auto;
  align-items: center;
  gap: 8px;
  padding: 0 14px;
  border-bottom: 1px solid var(--shell-rule);
  color: var(--shell-muted);
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 11px;
  font-weight: 700;
}

.trace-sequence i {
  position: relative;
  height: 1px;
  overflow: hidden;
  background: var(--shell-rule);
}

.trace-sequence i::after {
  position: absolute;
  inset: 0;
  background: var(--mint);
  content: "";
  transform: scaleX(0);
  transform-origin: left center;
}

.trace-sequence i:first-of-type::after { animation: trace-link-one var(--auth-cycle) linear infinite; }
.trace-sequence i:last-of-type::after { animation: trace-link-two var(--auth-cycle) linear infinite; }
.trace-sequence span { animation: trace-node-state var(--auth-cycle) linear infinite; }
.trace-sequence span:nth-of-type(2) { animation-delay: 240ms; }
.trace-sequence span:nth-of-type(3) { animation-delay: 480ms; }

.trace-path { padding: 7px 14px; }
.trace-path > div {
  display: grid;
  min-height: 39px;
  grid-template-columns: 25px 1fr auto;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid var(--shell-rule);
  font-size: 11px;
  animation: trace-step-state var(--auth-cycle) linear infinite;
}
.trace-path > div:nth-child(2) { animation-delay: 260ms; }
.trace-path > div:nth-child(3) { animation-delay: 520ms; }
.trace-path > div:last-child { border-bottom: 0; }
.trace-path i { display: grid; width: 23px; height: 23px; place-items: center; border: 1px solid var(--shell-rule); color: var(--shell-muted); font-family: "SFMono-Regular", Consolas, monospace; font-size: 11px; font-style: normal; }
.trace-path span { color: var(--shell-muted); }
.trace-path b { color: var(--mint); font-family: "SFMono-Regular", Consolas, monospace; font-size: 11px; }

.workbench-footer {
  display: grid;
  min-height: 48px;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 24px;
  color: var(--shell-muted);
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 11px;
  text-transform: uppercase;
}

.footer-track { position: relative; height: 1px; background: var(--shell-rule); }
.footer-track i { position: absolute; top: -2px; right: 22%; width: 12px; height: 4px; background: var(--mint); }

.auth-slot :deep(.space-y-6 > :not([hidden]) ~ :not([hidden])) { margin-top: 26px; }
.auth-slot :deep(.text-center) { text-align: left; }
.auth-slot :deep(h2) {
  color: var(--ink);
  font-family: Bahnschrift, "Arial Narrow", "Aptos Narrow", "PingFang SC", sans-serif;
  font-size: 32px;
  font-weight: 750;
  line-height: 1.12;
}
.auth-slot :deep(h2 + p) { margin-top: 9px; color: var(--muted); font-size: 14px; line-height: 1.6; }
.auth-slot :deep(form.space-y-5 > :not([hidden]) ~ :not([hidden])) { margin-top: 17px; }
.auth-slot :deep(.input-label) { margin-bottom: 6px; color: var(--ink); font-size: 13px; font-weight: 750; }
.auth-slot :deep(.input) {
  min-height: 46px;
  border: 1px solid var(--rule);
  border-radius: 3px;
  background: var(--surface);
  color: var(--ink);
  box-shadow: none;
  font-size: 14px;
  transition: border-color 150ms ease, box-shadow 150ms ease;
}
.auth-slot :deep(.input::placeholder) { color: color-mix(in srgb, var(--muted) 82%, transparent); }
.auth-slot :deep(.input:hover:not(:disabled)) { border-color: color-mix(in srgb, var(--ink) 48%, var(--rule)); }
.auth-slot :deep(.input:focus) { border-color: var(--cobalt); outline: none; box-shadow: inset 3px 0 0 var(--cobalt), 0 0 0 3px color-mix(in srgb, var(--cobalt) 14%, transparent); }
.auth-slot :deep(.input-error) { border-color: var(--coral); animation: field-reject 240ms ease-out; }
.auth-slot :deep(.input-hint) { margin-top: 6px; color: var(--muted); font-size: 12px; }
.auth-slot :deep(.btn) {
  min-height: 46px;
  border-radius: 3px;
  box-shadow: none;
  font-size: 14px;
  font-weight: 750;
  transform: none;
  transition: transform 150ms ease, border-color 150ms ease, background-color 150ms ease;
}
.auth-slot :deep(.btn-primary) { border: 1px solid var(--cobalt); background: var(--cobalt); color: #071012; }
.auth-slot :deep(.btn-primary svg) { color: currentColor; }
.auth-slot :deep(.btn-primary:hover:not(:disabled)) { border-color: var(--mint); background: var(--mint); color: #071012; transform: translateY(-1px); }
.auth-slot :deep(.btn-secondary) { border: 1px solid var(--rule); background: var(--surface); color: var(--ink); }
.auth-slot :deep(.btn-secondary:hover:not(:disabled)) { border-color: var(--cobalt); background: var(--surface); transform: translateY(-1px); }
.auth-slot :deep(.btn:focus-visible),
.auth-slot :deep(a:focus-visible) { outline: 3px solid color-mix(in srgb, var(--cobalt) 24%, transparent); outline-offset: 3px; }
.auth-slot :deep(a) { color: var(--cobalt); }
.auth-slot :deep(.text-gray-400),
.auth-slot :deep(.text-gray-500),
.auth-switch :deep(.text-gray-500),
.auth-slot :deep(.password-toggle) { color: var(--muted); }
.auth-slot :deep(.text-gray-600) { color: var(--muted); }
.auth-slot :deep(.text-gray-700),
.auth-slot :deep(.text-gray-800),
.auth-slot :deep(.text-gray-900) { color: var(--ink); }
.auth-slot :deep(.text-green-600),
.auth-slot :deep(.text-green-700) { color: #7de2a6; }
.auth-slot :deep(.text-amber-600),
.auth-slot :deep(.text-amber-700) { color: #f2c66d; }
.auth-slot :deep(.text-red-500),
.auth-slot :deep(.text-red-600),
.auth-slot :deep(.text-red-700) { color: #ff957d; }
.auth-slot :deep(.text-primary-900) { color: var(--ink); }
.auth-slot :deep(.text-primary-700) { color: var(--muted); }
.auth-slot :deep(.auth-divider > div) { background: var(--rule); }
.auth-slot :deep(.bg-green-100) { background: color-mix(in srgb, #22c55e 16%, var(--auth-field)); }
.auth-slot :deep(.bg-green-50) { background: color-mix(in srgb, #22c55e 10%, var(--auth-field)); }
.auth-slot :deep(.bg-amber-50) { background: color-mix(in srgb, #f59e0b 10%, var(--auth-field)); }
.auth-slot :deep([class*="bg-primary-50"]) { background: color-mix(in srgb, var(--cobalt) 10%, var(--auth-field)); }
.auth-slot :deep(.registration-notice) { border-color: color-mix(in srgb, #f59e0b 38%, var(--rule)); }
.auth-slot :deep(.rounded-xl),
.auth-slot :deep(.rounded-lg) { border-radius: 3px; }

@keyframes auth-handshake {
  0%, 6% { left: 0; opacity: 0; }
  8% { left: 0; opacity: 1; }
  54% { left: calc(100% - 12px); opacity: 1; }
  58%, 100% { left: calc(100% - 12px); opacity: 0; }
}

@keyframes auth-module-enter {
  from { opacity: 0; clip-path: inset(0 0 100% 0); }
  to { opacity: 1; clip-path: inset(0); }
}

@keyframes auth-panel-enter {
  from { opacity: 0; transform: translateY(10px); clip-path: inset(0 100% 0 0); }
  to { opacity: 1; transform: translateY(0); clip-path: inset(0); }
}

@keyframes auth-content-enter {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}

@keyframes auth-panel-state {
  0%, 24%, 62%, 100% { border-top-color: var(--auth-accent); box-shadow: inset 0 1px 0 color-mix(in srgb, var(--auth-ink) 5%, transparent); }
  29%, 57% { border-top-color: var(--mint); box-shadow: inset 3px 0 0 color-mix(in srgb, var(--mint) 72%, transparent); }
}

@keyframes auth-port-in {
  0%, 17%, 38%, 100% { opacity: 0; transform: translateX(-16px); }
  21% { opacity: 1; transform: translateX(-16px); }
  32% { opacity: 1; transform: translateX(36px); }
}

@keyframes auth-port-out {
  0%, 45%, 64%, 100% { opacity: 0; transform: translateX(-36px); }
  49% { opacity: 1; transform: translateX(-36px); }
  59% { opacity: 1; transform: translateX(16px); }
}

@keyframes auth-form-scan {
  0%, 25%, 58%, 100% { opacity: 0; transform: translateX(-110%); }
  29% { opacity: 1; transform: translateX(-110%); }
  48% { opacity: 1; transform: translateX(390%); }
  52% { opacity: 0; transform: translateX(390%); }
}

@keyframes trace-link-one {
  0%, 17%, 66%, 100% { transform: scaleX(0); }
  25%, 60% { transform: scaleX(1); }
}

@keyframes trace-link-two {
  0%, 28%, 66%, 100% { transform: scaleX(0); }
  38%, 60% { transform: scaleX(1); }
}

@keyframes trace-node-state {
  0%, 12%, 66%, 100% { color: var(--shell-muted); }
  20%, 60% { color: var(--shell-ink); }
}

@keyframes trace-step-state {
  0%, 20%, 66%, 100% { background: transparent; box-shadow: inset 0 0 0 transparent; }
  28%, 60% { background: color-mix(in srgb, var(--mint) 5%, transparent); box-shadow: inset 2px 0 0 var(--mint); }
}

@keyframes field-reject {
  0%, 100% { transform: translateX(0); }
  35% { transform: translateX(-3px); }
  70% { transform: translateX(3px); }
}

@media (max-width: 1100px) {
  .auth-grid { grid-template-columns: minmax(220px, 0.8fr) minmax(400px, 460px); gap: 48px; }
  .route-intro h1 { font-size: 48px; }
  .trace-board { display: none; }
}

@media (max-width: 780px) {
  .auth-workbench::before { width: calc(100% - 28px); }
  .workbench-header,
  .protocol-strip,
  .auth-grid,
  .workbench-footer { width: min(100% - 28px, 1280px); }
  .workbench-header { min-height: 62px; grid-template-columns: auto 1fr; }
  .gateway-state { display: none; }
  .workbench-actions { justify-self: end; }
  .protocol-strip { grid-template-columns: 1fr auto; }
  .protocol-strip code { text-align: right; }
  .protocol-strip span:last-child { display: none; }
  .auth-grid { display: block; min-height: calc(100svh - 148px); padding: 38px 0 34px; }
  .auth-grid::before,
  .auth-grid::after { display: none; }
  .route-board,
  .trace-board { display: none; }
  .auth-center { width: min(100%, 470px); margin: 0 auto; padding: 20px 22px 25px; background: color-mix(in srgb, var(--auth-panel) 90%, transparent); box-shadow: inset 0 1px 0 color-mix(in srgb, var(--auth-ink) 5%, transparent); }
  .auth-center::before,
  .auth-center::after,
  .session-ports { display: none; }
  .workbench-footer { min-height: 48px; }
}

@media (max-width: 430px) {
  .auth-wordmark strong { display: none; }
  .auth-center { padding: 18px 14px 23px; }
  .auth-slot :deep(h2) { font-size: 28px; }
  .workbench-footer span:last-child { display: none; }
  .workbench-footer { grid-template-columns: auto 1fr; }
}

@media (prefers-reduced-motion: reduce) {
  .auth-workbench *, .auth-workbench *::before, .auth-workbench *::after {
    animation: none !important;
    transition-duration: 0.01ms !important;
  }
  .auth-grid::after { display: none; }
  .trace-sequence i::after { transform: scaleX(1); }
}
</style>
