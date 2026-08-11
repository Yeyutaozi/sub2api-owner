<template>
  <span
    class="mp-vendor"
    :class="['mp-vendor--' + size, 'mp-vendor--' + vendor.mark]"
    :style="cssVars"
    :title="vendor.label"
    :aria-label="vendor.label"
    role="img"
  >
    <svg
      v-if="vendor.mark !== 'letter'"
      class="mp-vendor__svg"
      viewBox="0 0 24 24"
      width="100%"
      height="100%"
      aria-hidden="true"
    >
      <!-- OpenAI blossom (official path) -->
      <g v-if="vendor.mark === 'openai'" fill="currentColor">
        <path
          d="M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9807 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.02 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1253a4.4708 4.4708 0 0 1-.5346-3.0137l.1412.0849 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-5.1408-1.6464zM2.3408 7.2781a4.485 4.485 0 0 1 2.3655-1.9728V11.6a.7664.7664 0 0 0 .3879.6765l5.8144 3.3543-2.0201 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.2781zm16.5963 3.8888-5.8334-3.3871 2.0198-1.1685a.0757.0757 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1 .676 8.1042v-5.6772a.79.79 0 0 0-.407-.667zm2.0107-3.0231-.1419-.085-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L9.409 9.2297V6.8974a.0662.0662 0 0 1 .0281-.0615l4.8303-2.7866a4.4992 4.4992 0 0 1 6.6802 4.66zM8.3065 12.863l-2.02-1.1638a.0804.0804 0 0 1-.038-0.0567V6.0742a4.4992 4.4992 0 0 1 7.3757-3.4537l-.142.0805L8.704 5.459a.7948.7948 0 0 0-.3927.6813zm1.0976-2.3654 2.602-1.4998 2.6069 1.4998v2.9994l-2.5974 1.4997-2.6067-1.4997Z"
        />
      </g>

      <!-- Anthropic A -->
      <g v-else-if="vendor.mark === 'anthropic'" fill="currentColor">
        <path d="M13.827 3.52h3.603L24 20.48h-3.84l-1.406-3.68H9.069l-1.406 3.68H3.84L13.827 3.52Zm.22 5.2-2.62 6.84h5.24l-2.62-6.84Z" />
      </g>

      <!-- Google G -->
      <g v-else-if="vendor.mark === 'google'">
        <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1Z" />
        <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23Z" />
        <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62Z" />
        <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53Z" />
      </g>

      <!-- xAI X -->
      <g v-else-if="vendor.mark === 'xai'" fill="currentColor">
        <path d="M3.2 3.2h4.7l3.95 5.9L16.1 3.2H21l-6.55 8.35L21.4 20.8h-4.8l-4.25-6.35-4.55 6.35H3.05l6.9-8.7L3.2 3.2Z" />
      </g>

      <!-- MiniMax: dual-peak M emblem -->
      <g v-else-if="vendor.mark === 'minimax'" fill="currentColor">
        <path d="M3.5 19.2V4.8h3.5l4.2 8.4 4.2-8.4h3.5v14.4h-3.1v-8.7l-3.5 6.6h-2.2l-3.5-6.6v8.7H3.5Z" />
        <path d="M5.1 20.6h13.8v1.4H5.1z" opacity="0.35" />
      </g>

      <!-- Seedance / Jimeng sprout -->
      <g v-else-if="vendor.mark === 'seedance'" fill="currentColor">
        <path d="M12 3c4.8 1.4 7.8 4.8 8.2 9.2.2 2.1-.3 4-1.2 5.6-2.2-1.3-4.3-3.3-5.7-5.9-1 3-1.1 5.8-.2 8.3H10.9c.9-2.5.8-5.3-.2-8.3-1.4 2.6-3.5 4.6-5.7 5.9-.9-1.6-1.4-3.5-1.2-5.6C4.2 7.8 7.2 4.4 12 3Z" />
        <path d="M12 9.6c1.5 2.3 2.4 4.5 2.6 6.8h-5.2c.2-2.3 1.1-4.5 2.6-6.8Z" opacity="0.45" />
        <rect x="11.2" y="16.4" width="1.6" height="4.2" rx="0.7" opacity="0.7" />
      </g>

      <!-- BytePlus: bold B+ tile -->
      <g v-else-if="vendor.mark === 'byteplus'" fill="currentColor">
        <path d="M4 4h7.1c3.6 0 5.9 2 5.9 5.1 0 2-1 3.6-2.7 4.4 2.2.8 3.6 2.6 3.6 5.1C17.9 21.7 15.2 24 11.3 24H4V4Zm3.5 3v4.4h3.4c1.6 0 2.5-.8 2.5-2.2S12.5 7 10.9 7H7.5Zm0 7.4v5.1h3.8c1.8 0 2.9-.9 2.9-2.5s-1.1-2.6-2.9-2.6H7.5Z" transform="scale(0.78) translate(1.2,0)" />
        <circle cx="19.2" cy="5.2" r="2.4" opacity="0.95" />
        <path d="M18.4 5.2h1.6M19.2 4.4v1.6" stroke="#fff" stroke-width="1.2" stroke-linecap="round" fill="none" />
      </g>

      <!-- DeepSeek face -->
      <g v-else-if="vendor.mark === 'deepseek'" fill="currentColor">
        <path d="M4.2 13c.5-4.4 4-8 8.6-8 3.3 0 6.1 1.7 7.6 4.3.7-.2 1.4-.3 2.1-.3.2 1.8-.4 3.5-1.5 4.7-1.1 3.5-4.4 6-8.2 6-3.9 0-7.2-2.6-8.4-6.2-.4.1-.9.2-1.3.2-.6 0-1.2-.1-1.7-.3.7-1.3 1.8-2.3 2.8-2.4Z" />
        <circle cx="10" cy="13.2" r="1.55" fill="#fff" />
        <circle cx="15.1" cy="13.2" r="1.55" fill="#fff" />
        <circle cx="18.5" cy="8.1" r="1.15" />
      </g>

      <!-- Qwen ring -->
      <g v-else-if="vendor.mark === 'qwen'" fill="currentColor">
        <path d="M12 2.2c5.4 0 9.8 4.4 9.8 9.8 0 2.3-.8 4.4-2.1 6.1l1.9 3.4h-3.7l-1.2-2.1a9.7 9.7 0 0 1-4.7 1.2c-5.4 0-9.8-4.4-9.8-9.8S6.6 2.2 12 2.2Zm0 3.3a6.5 6.5 0 1 0 .1 13 6.5 6.5 0 0 0-.1-13Zm-2.2 3.2h4.4l.95 5.8h-2.05l-.22-1.55h-1.76l-.22 1.55H9.05l.95-5.8Zm1.15 2.2-.2 1.55h1.5l-.2-1.55h-1.1Z" />
      </g>

      <!-- Meta infinity -->
      <g v-else-if="vendor.mark === 'meta'" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M3.8 15.2c1.5-4.6 3.5-7.1 5.7-7.1 2.4 0 3.6 3 4.9 6.2 1.2 3 2.3 5.2 3.7 5.2 1.9 0 3.5-2.6 4.7-6.7" />
        <path d="M5.2 10.1c1-2.2 2.4-3.4 3.9-3.4 2.4 0 3.7 3.1 5 6.5 1.3 3.2 2.5 5.5 4.1 5.5 1.4 0 2.7-1.2 3.8-3.3" />
      </g>

      <!-- Mistral orange bars -->
      <g v-else-if="vendor.mark === 'mistral'" fill="currentColor">
        <path d="M2.6 19.6 8 4.4h3.5L6.1 19.6H2.6Zm5.9 0 5.3-15.2h3.5L12 19.6H8.5Zm5.9 0 5.3-15.2H23l-5.3 15.2h-3.3Z" />
      </g>

      <!-- Flux F mark -->
      <g v-else-if="vendor.mark === 'flux'" fill="currentColor">
        <path d="M4 4h7.4v3.3H7.3v3.5H11v3.3H7.3V20H4V4Zm9.3 0H20v3.3h-3.9v2.7H20v3.3h-3.9V20h-2.8V4Z" />
      </g>

      <!-- Midjourney star -->
      <g v-else-if="vendor.mark === 'midjourney'" fill="currentColor">
        <path d="M12 2.4 14.5 9h6.9l-5.6 4.1 2.2 6.6L12 15.8 6.1 19.7 8.3 13.1 2.7 9h6.9L12 2.4Z" />
      </g>

      <!-- Sora play tile -->
      <g v-else-if="vendor.mark === 'sora'" fill="currentColor">
        <rect x="3.4" y="5.4" width="17.2" height="13.2" rx="3.2" />
        <path d="M10.2 9.1v5.8l5.2-2.9-5.2-2.9Z" fill="#fff" />
      </g>

      <!-- LTX signal bars -->
      <g v-else-if="vendor.mark === 'ltx'" fill="currentColor">
        <rect x="3.4" y="13.2" width="4.1" height="7" rx="1.1" />
        <rect x="9.95" y="8" width="4.1" height="12.2" rx="1.1" />
        <rect x="16.5" y="3.6" width="4.1" height="16.6" rx="1.1" />
      </g>

      <!-- Kling K -->
      <g v-else-if="vendor.mark === 'kling'" fill="currentColor">
        <path d="M5 3.5h3.8v6.9L15.4 3.5H20l-7.2 8.2L20.6 20.5h-5l-6.8-8.3v8.3H5V3.5Z" />
      </g>

      <!-- Runway R -->
      <g v-else-if="vendor.mark === 'runway'" fill="currentColor">
        <path d="M5.2 3.5h7.6c3.6 0 6 2.1 6 5.2 0 2.5-1.4 4.4-3.8 5.1l4.2 6.7h-4.3l-3.7-6.2H8.8v6.2H5.2V3.5Zm3.6 3v5.2h3.6c1.8 0 2.8-.9 2.8-2.6s-1-2.6-2.8-2.6H8.8Z" />
      </g>

      <!-- Zhipu GLM hex -->
      <g v-else-if="vendor.mark === 'glm'" fill="currentColor">
        <path d="M12 2.3 20.8 7.2v9.6L12 21.7 3.2 16.8V7.2L12 2.3Zm0 3.2L6.2 8.6v6.8L12 18.5l5.8-3.1V8.6L12 5.5Zm-2.3 4.3h4.6v1.9h-1.6V15h-2v-3.5H9.7V9.8Z" />
      </g>

      <!-- Antigravity -->
      <g v-else-if="vendor.mark === 'antigravity'" fill="currentColor">
        <path d="M12 2.6 20.4 19H3.6L12 2.6Zm0 5.4-4.3 8.4h8.6L12 8Z" />
        <circle cx="12" cy="19.5" r="1.45" />
      </g>

      <!-- HappyHorse -->
      <g v-else-if="vendor.mark === 'happyhorse'" fill="currentColor">
        <path d="M5 16.9c1.1-3.5 3.6-6 7-6.6.4-1.9 1.5-3.4 3-4.3.7-.4 1.7.1 1.7.9v1.3c1.6.4 3 1.4 3.8 2.9.3.5 0 1.2-.6 1.2h-1.1c.7 1.2 1.1 2.7 1.1 4.2 0 .7-.6 1.3-1.3 1.3H6.3c-.8 0-1.5-.7-1.3-1.5v-.4Zm4.3-1.6h2.2v2.2H9.3v-2.2Zm4.3 0h2.2v2.2h-2.2v-2.2Z" />
      </g>
    </svg>
    <span v-else class="mp-vendor__letter">{{ vendor.monogram }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { resolvePlazaVendor } from './plazaVendors'

const props = withDefaults(
  defineProps<{
    platform: string
    modelName?: string
    size?: 'sm' | 'md' | 'lg'
  }>(),
  {
    modelName: '',
    size: 'md'
  }
)

const vendor = computed(() => resolvePlazaVendor(props.platform, props.modelName))

const cssVars = computed(() => ({
  '--vendor-color': vendor.value.color,
  '--vendor-soft': vendor.value.soft,
  color: vendor.value.color
}))
</script>

<style scoped>
.mp-vendor {
  --vendor-color: #10a37f;
  --vendor-soft: #d1fae5;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border-radius: 13px;
  background:
    radial-gradient(120% 120% at 20% 15%, color-mix(in srgb, var(--vendor-soft) 92%, #fff), transparent 62%),
    linear-gradient(160deg, color-mix(in srgb, var(--vendor-soft) 88%, #fff), #fff);
  border: 1px solid color-mix(in srgb, var(--vendor-color) 22%, #d7dee8);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.7),
    0 8px 16px -12px color-mix(in srgb, var(--vendor-color) 45%, transparent);
  color: var(--vendor-color);
}

.mp-vendor--sm {
  width: 22px;
  height: 22px;
  border-radius: 8px;
}

.mp-vendor--md {
  width: 34px;
  height: 34px;
}

.mp-vendor--lg {
  width: 42px;
  height: 42px;
  border-radius: 14px;
}

.mp-vendor__svg {
  display: block;
  width: 64%;
  height: 64%;
}

.mp-vendor--openai {
  color: #10a37f;
  background:
    radial-gradient(120% 120% at 30% 20%, #ecfdf5, transparent 60%),
    linear-gradient(160deg, #d1fae5, #fff);
  border-color: rgba(16, 163, 127, 0.28);
}

.mp-vendor--openai .mp-vendor__svg,
.mp-vendor--google .mp-vendor__svg,
.mp-vendor--anthropic .mp-vendor__svg {
  width: 70%;
  height: 70%;
}

.mp-vendor--google {
  background: #fff;
  border-color: #e5e7eb;
  color: #4285f4;
}

.mp-vendor--anthropic {
  color: #d4a27f;
  background: linear-gradient(160deg, #faf6f1, #fff);
  border-color: rgba(212, 162, 127, 0.35);
}

.mp-vendor--xai {
  color: #111827;
  background: linear-gradient(160deg, #f3f4f6, #fff);
}

.mp-vendor--deepseek {
  color: #4d6bfe;
  background: linear-gradient(160deg, #eef2ff, #fff);
}

.mp-vendor--qwen {
  color: #615ced;
  background: linear-gradient(160deg, #eef2ff, #fff);
}

.mp-vendor--minimax {
  color: #e11d48;
  background: linear-gradient(160deg, #fff1f2, #fff);
}

.mp-vendor--seedance,
.mp-vendor--byteplus {
  color: #0d9488;
  background: linear-gradient(160deg, #ccfbf1, #fff);
}

.mp-vendor--glm {
  color: #3859ff;
  background: linear-gradient(160deg, #dbeafe, #fff);
}

.mp-vendor--flux,
.mp-vendor--runway {
  color: #111827;
  background: linear-gradient(160deg, #f8fafc, #fff);
}

.mp-vendor--letter {
  font-family: "JetBrains Mono", ui-monospace, monospace;
  font-weight: 800;
  letter-spacing: -0.04em;
}

.mp-vendor__letter {
  font-size: 11px;
  font-weight: 800;
  letter-spacing: -0.04em;
  color: var(--vendor-color);
}

.mp-vendor--sm .mp-vendor__letter {
  font-size: 9px;
}

.mp-vendor--lg .mp-vendor__letter {
  font-size: 13px;
}
</style>
