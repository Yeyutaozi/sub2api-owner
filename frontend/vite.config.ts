import { defineConfig, loadEnv, Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import checker from 'vite-plugin-checker'
import { resolve } from 'path'

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (character) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  })[character] || character)
}

function isSafeImageUrl(value: string): boolean {
  const trimmed = value.trim()
  if ((trimmed.startsWith('/') && !trimmed.startsWith('//')) || /^data:image\//i.test(trimmed)) {
    return true
  }
  try {
    const parsed = new URL(trimmed)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

function injectBranding(html: string, config: { site_name?: string; site_logo?: string }): string {
  let brandedHtml = html
  const siteName = config.site_name?.trim()
  if (siteName) {
    brandedHtml = brandedHtml.replace(
      /<title>[^<]*<\/title>/i,
      `<title>${escapeHtml(siteName)} - AI API Gateway</title>`,
    )
  }

  const siteLogo = config.site_logo?.trim()
  if (siteLogo && isSafeImageUrl(siteLogo)) {
    brandedHtml = brandedHtml.replace(
      /<link\s+rel=["']icon["'][^>]*>/i,
      `<link rel="icon" href="${escapeHtml(siteLogo)}" />`,
    )
  }
  return brandedHtml
}

/**
 * Vite 鎻掍欢锛氬紑鍙戞ā寮忎笅娉ㄥ叆鍏紑閰嶇疆鍒?index.html
 * 涓庣敓浜фā寮忕殑鍚庣娉ㄥ叆琛屼负淇濇寔涓€鑷达紝娑堥櫎闂儊
 */
function injectPublicSettings(backendUrl: string): Plugin {
  return {
    name: 'inject-public-settings',
    apply: 'serve',
    transformIndexHtml: {
      order: 'pre',
      async handler(html) {
        try {
          const response = await fetch(`${backendUrl}/api/v1/settings/public`, {
            signal: AbortSignal.timeout(2000)
          })
          if (response.ok) {
            const data = await response.json()
            if (data.code === 0 && data.data) {
              const script = `<script>window.__APP_CONFIG__=${JSON.stringify(data.data)};</script>`
              return injectBranding(html, data.data).replace('</head>', `${script}\n</head>`)
            }
          }
        } catch (e) {
          console.warn('[vite] 鏃犳硶鑾峰彇鍏紑閰嶇疆锛屽皢鍥為€€鍒?API 璋冪敤:', (e as Error).message)
        }
        return html
      }
    }
  }
}

export default defineConfig(({ mode }) => {
  // 鍔犺浇鐜鍙橀噺
  const env = loadEnv(mode, process.cwd(), '')
  const backendUrl = env.VITE_DEV_PROXY_TARGET || 'http://localhost:8080'
  const devPort = Number(env.VITE_DEV_PORT || 3000)

  return {
    plugins: [
      vue(),
      /* CHECKER_DISABLED_FOR_E2E */ // checker({ vueTsc: true }),
      injectPublicSettings(backendUrl)
    ],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
      // 浣跨敤 vue-i18n 杩愯鏃剁増鏈紝閬垮厤 CSP unsafe-eval 闂
      'vue-i18n': 'vue-i18n/dist/vue-i18n.runtime.esm-bundler.js'
    }
  },
  define: {
    // 鍚敤 vue-i18n JIT 缂栬瘧锛屽湪 CSP 鐜涓嬪鐞嗘秷鎭彃鍊?    // JIT 缂栬瘧鍣ㄧ敓鎴?AST 瀵硅薄鑰岄潪 JS 浠ｇ爜锛屾棤闇€ unsafe-eval
    __INTLIFY_JIT_COMPILATION__: true
  },
  build: {
    outDir: '../backend/internal/web/dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        /**
         * 鎵嬪姩鍒嗗寘閰嶇疆
         * 鍒嗙绗笁鏂瑰簱骞舵寜鍔熻兘鍚堝苟搴旂敤浠ｇ爜锛岄伩鍏嶅惊鐜緷璧?         */
        manualChunks(id: string) {
          if (id.includes('node_modules')) {
            if (id.includes('/three/')) {
              return 'vendor-three'
            }

            // Vue 鏍稿績搴?            if (
              id.includes('/vue/') ||
              id.includes('/vue-router/') ||
              id.includes('/pinia/') ||
              id.includes('/@vue/')
            ) {
              return 'vendor-vue'
            }

            // UI 宸ュ叿搴擄紙杈冨ぇ锛屽崟鐙垎绂伙級
            if (id.includes('/@vueuse/') || id.includes('/xlsx/')) {
              return 'vendor-ui'
            }

            // 鍥捐〃搴?            if (id.includes('/chart.js/') || id.includes('/vue-chartjs/')) {
              return 'vendor-chart'
            }

            // 鍥介檯鍖?            if (id.includes('/vue-i18n/') || id.includes('/@intlify/')) {
              return 'vendor-i18n'
            }

            // Stripe 浠呭湪鏀粯娴佺▼涓寜闇€鍔犺浇锛岄伩鍏嶈繘鍏ラ椤靛叕鍏变緷璧栥€?            if (id.includes('/@stripe/stripe-js/')) {
              return 'vendor-stripe'
            }

            // 鍏朵粬灏忓瀷绗笁鏂瑰簱鍚堝苟
            return 'vendor-misc'
          }

          // 搴旂敤浠ｇ爜锛氭寜鍏ュ彛鐐硅嚜鍔ㄥ垎鍖咃紝涓嶆墜鍔ㄥ共棰?          // 杩欐牱鍙互閬垮厤寰幆渚濊禆锛屽悓鏃朵繚鎸佸悎鐞嗙殑 chunk 鏁伴噺
        }
      }
    }
  },
    server: {
      host: '0.0.0.0',
      port: devPort,
      proxy: {
        '/api': {
          target: backendUrl,
          changeOrigin: true
        },
        '/v1': {
          target: backendUrl,
          changeOrigin: true
        },
        '/setup': {
          target: backendUrl,
          changeOrigin: true
        }
      }
    }
  }
})

