export const SEEDANCE_VIDEO_PROVIDER_BASE_URLS = {
  fflink: 'https://api.fflink.top',
  huiqu: 'https://api.bjhuiqu.net',
  ximei: 'https://liantongyidong.ximeiedu.org',
  weijin: 'https://www.weijinapi.top',
  globalaiopc: 'https://zcbservice.aizfw.cn/kyyReactApiServer',
  lensforge: 'https://lensforge.tsxzz.com',
  openvideo: 'https://www.openvideo.top/api/v1'
} as const

export type SeedanceVideoProvider = keyof typeof SEEDANCE_VIDEO_PROVIDER_BASE_URLS

export const getSeedanceVideoProviderBaseUrl = (
  provider: SeedanceVideoProvider
): string => SEEDANCE_VIDEO_PROVIDER_BASE_URLS[provider]


export const MINIMAX_VIDEO_PROVIDER_BASE_URLS = {
  huiqu: 'https://api.bjhuiqu.net'
} as const

export type MiniMaxVideoProvider = keyof typeof MINIMAX_VIDEO_PROVIDER_BASE_URLS

export const getMiniMaxVideoProviderBaseUrl = (
  provider: MiniMaxVideoProvider
): string => MINIMAX_VIDEO_PROVIDER_BASE_URLS[provider]
