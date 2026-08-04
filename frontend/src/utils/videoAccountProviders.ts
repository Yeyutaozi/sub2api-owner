export const SEEDANCE_VIDEO_PROVIDER_BASE_URLS = {
  fflink: 'https://api.fflink.top',
  huiqu: 'https://api.bjhuiqu.net'
} as const

export type SeedanceVideoProvider = keyof typeof SEEDANCE_VIDEO_PROVIDER_BASE_URLS

export const getSeedanceVideoProviderBaseUrl = (
  provider: SeedanceVideoProvider
): string => SEEDANCE_VIDEO_PROVIDER_BASE_URLS[provider]
