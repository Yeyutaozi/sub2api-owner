/**
 * Axios HTTP Client Configuration
 * Base client with interceptors for authentication, token refresh, and error handling
 */

import axios, { AxiosInstance, AxiosError, InternalAxiosRequestConfig, AxiosResponse } from 'axios'
import type { ApiResponse } from '@/types'
import { getLocale } from '@/i18n'
import {
  ADMIN_UI_REQUEST_HEADER,
  USER_UI_REQUEST_HEADER,
  shouldMarkAdminUIRequest,
  shouldMarkUserUIRequest,
} from './adminUIRequest'
import { getAPIBaseURL } from './url'
export { buildApiUrl, buildGatewayUrl } from './url'

// ==================== Axios Instance Configuration ====================

export const apiClient: AxiosInstance = axios.create({
  baseURL: getAPIBaseURL(),
  withCredentials: true,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// ==================== Token Refresh State ====================

// Track if a token refresh is in progress to prevent multiple simultaneous refresh requests
let isRefreshing = false
// Queue of requests waiting for token refresh
let refreshSubscribers: Array<(token: string) => void> = []

/**
 * Subscribe to token refresh completion
 */
function subscribeTokenRefresh(callback: (token: string) => void): void {
  refreshSubscribers.push(callback)
}


/**
 * Notify all subscribers that token has been refreshed
 */
function onTokenRefreshed(token: string): void {
  refreshSubscribers.forEach((callback) => callback(token))
  refreshSubscribers = []
}

/** HTTP statuses that usually mean "try later", not "credentials invalid". */
const TRANSIENT_HTTP_STATUSES = new Set([0, 408, 425, 429, 500, 502, 503, 504])

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function getHttpStatus(err: unknown): number | undefined {
  if (!err || typeof err !== 'object') return undefined
  const anyErr = err as { status?: unknown; response?: { status?: unknown } }
  if (typeof anyErr.status === 'number') return anyErr.status
  if (typeof anyErr.response?.status === 'number') return anyErr.response.status
  return undefined
}

/**
 * Network blips (VPN/proxy switch, DNS, timeout) should NOT force logout.
 */
function isNetworkLikeError(err: unknown): boolean {
  if (!err || typeof err !== 'object') return false
  const anyErr = err as AxiosError
  if (
    anyErr.code === 'ERR_NETWORK' ||
    anyErr.code === 'ECONNABORTED' ||
    anyErr.code === 'ETIMEDOUT' ||
    anyErr.code === 'ECONNRESET' ||
    anyErr.code === 'ERR_INTERNET_DISCONNECTED'
  ) {
    return true
  }
  // Axios network failures often have no response body at all.
  if (anyErr.isAxiosError && !anyErr.response) return true
  return false
}

/**
 * True when refresh failure is temporary and local session should be kept.
 * False when server rejected the refresh token (or refresh response is invalid).
 */
function isTransientRefreshFailure(err: unknown): boolean {
  if (isNetworkLikeError(err)) return true
  const status = getHttpStatus(err)
  if (status === undefined) return false
  if (status === 401 || status === 403) return false
  return TRANSIENT_HTTP_STATUSES.has(status) || status >= 500
}

function clearLocalAuthStorage(): void {
  localStorage.removeItem('auth_token')
  localStorage.removeItem('refresh_token')
  localStorage.removeItem('auth_user')
  localStorage.removeItem('token_expires_at')
}

function redirectToLoginIfNeeded(): void {
  if (typeof window === 'undefined') return
  if (!window.location.pathname.includes('/login')) {
    window.location.href = '/login'
  }
}

async function requestTokenRefresh(refreshToken: string): Promise<{
  access_token: string
  refresh_token: string
  expires_in: number
}> {
  // Retry once on transient network/proxy failures (common when switching VPN).
  let lastError: unknown
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      const refreshResponse = await axios.post(
        `${getAPIBaseURL()}/auth/refresh`,
        { refresh_token: refreshToken },
        // Explicit timeout: a hung refresh would leave isRefreshing=true forever.
        { headers: { 'Content-Type': 'application/json' }, timeout: 15000 }
      )

      const refreshData = refreshResponse.data as ApiResponse<{
        access_token: string
        refresh_token: string
        expires_in: number
      }>

      if (refreshData.code === 0 && refreshData.data?.access_token && refreshData.data?.refresh_token) {
        return refreshData.data
      }

      // Business-level rejection (invalid/revoked refresh token) — not transient.
      const bizError = {
        status: refreshResponse.status || 401,
        code: refreshData.code ?? 'TOKEN_REFRESH_REJECTED',
        message: refreshData.message || 'Token refresh rejected',
      }
      throw bizError
    } catch (err) {
      lastError = err
      if (!isTransientRefreshFailure(err) || attempt === 1) {
        throw err
      }
      await sleep(400 * (attempt + 1))
    }
  }
  throw lastError
}


// ==================== Request Interceptor ====================

// Get user's timezone
const getUserTimezone = (): string => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone
  } catch {
    return 'UTC'
  }
}

apiClient.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    // Attach token from localStorage
    const token = localStorage.getItem('auth_token')
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`
    }

    // Attach locale for backend translations
    if (config.headers) {
      config.headers['Accept-Language'] = getLocale()
    }

    // Attach timezone for all GET requests (backend may use it for default date ranges)
    if (config.method === 'get') {
      if (!config.params) {
        config.params = {}
      }
      config.params.timezone = getUserTimezone()
    }

    if (config.headers) {
      const requestURL = String(config.url || '')
      if (shouldMarkAdminUIRequest(requestURL)) {
        config.headers[ADMIN_UI_REQUEST_HEADER] = '1'
      }
      if (shouldMarkUserUIRequest(requestURL)) {
        config.headers[USER_UI_REQUEST_HEADER] = '1'
      }
    }

    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// ==================== Response Interceptor ====================

apiClient.interceptors.response.use(
  (response: AxiosResponse) => {
    // Unwrap standard API response format { code, message, data }
    const apiResponse = response.data as ApiResponse<unknown>
    if (apiResponse && typeof apiResponse === 'object' && 'code' in apiResponse) {
      if (apiResponse.code === 0) {
        // Success - return the data portion
        response.data = apiResponse.data
      } else {
        // API error
        const resp = apiResponse as unknown as Record<string, unknown>
        return Promise.reject({
          status: response.status,
          code: apiResponse.code,
          message: apiResponse.message || 'Unknown error',
          reason: resp.reason,
          metadata: resp.metadata,
        })
      }
    }
    return response
  },
  async (error: AxiosError<ApiResponse<unknown>>) => {
    // Request cancellation: keep the original axios cancellation error so callers can ignore it.
    // Otherwise we'd misclassify it as a generic "network error".
    if (error.code === 'ERR_CANCELED' || axios.isCancel(error)) {
      return Promise.reject(error)
    }

    const originalRequest = error.config as InternalAxiosRequestConfig & { _retry?: boolean }

    // Handle common errors
    if (error.response) {
      const { status } = error.response
      let data: unknown = error.response.data
      const url = String(error.config?.url || '')

      // responseType: 'blob' keeps JSON error envelopes as Blob — parse so callers get message/reason.
      if (typeof Blob !== 'undefined' && data instanceof Blob) {
        try {
          const textBody = await data.text()
          const trimmed = textBody.trim()
          if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
            data = JSON.parse(trimmed)
          } else if (trimmed) {
            data = { message: trimmed.slice(0, 500) }
          } else {
            data = {}
          }
        } catch {
          data = {}
        }
      }

      // Validate `data` shape to avoid HTML error pages breaking our error handling.
      const apiData = (typeof data === 'object' && data !== null ? data : {}) as Record<string, any>

      // Ops monitoring disabled: treat as feature-flagged 404, and proactively redirect away
      // from ops pages to avoid broken UI states.
      if (status === 404 && apiData.message === 'Ops monitoring is disabled') {
        try {
          localStorage.setItem('ops_monitoring_enabled_cached', 'false')
        } catch {
          // ignore localStorage failures
        }
        try {
          window.dispatchEvent(new CustomEvent('ops-monitoring-disabled'))
        } catch {
          // ignore event failures
        }

        if (window.location.pathname.startsWith('/admin/ops')) {
          window.location.href = '/admin/settings'
        }

        return Promise.reject({
          status,
          code: 'OPS_DISABLED',
          message: apiData.message || error.message,
          url
        })
      }

      if (status === 423 && apiData.code === 'ADMIN_COMPLIANCE_ACK_REQUIRED') {
        try {
          window.dispatchEvent(new CustomEvent('admin-compliance-required', {
            detail: apiData.metadata || {}
          }))
        } catch {
          // ignore event failures
        }

        return Promise.reject({
          status,
          code: apiData.code,
          message: apiData.message || error.message,
          metadata: apiData.metadata,
        })
      }

      // 401: Try to refresh the token if we have a refresh token
      // This handles TOKEN_EXPIRED, INVALID_TOKEN, TOKEN_REVOKED, etc.
      if (status === 401 && !originalRequest._retry) {
        const refreshToken = localStorage.getItem('refresh_token')
        const isAuthEndpoint =
          url.includes('/auth/login') || url.includes('/auth/register') || url.includes('/auth/refresh')

        // If we have a refresh token and this is not an auth endpoint, try to refresh
        if (refreshToken && !isAuthEndpoint) {
          if (isRefreshing) {
            // Wait for the ongoing refresh to complete
            return new Promise((resolve, reject) => {
              subscribeTokenRefresh((newToken: string) => {
                if (newToken) {
                  // Mark as retried to prevent infinite loop if retry also returns 401
                  originalRequest._retry = true
                  if (originalRequest.headers) {
                    originalRequest.headers.Authorization = `Bearer ${newToken}`
                  }
                  resolve(apiClient(originalRequest))
                } else {
                  // Refresh failed, reject with original error
                  reject({
                    status,
                    code: apiData.code,
                    message: apiData.message || apiData.detail || error.message
                  })
                }
              })
            })
          }

          originalRequest._retry = true
          isRefreshing = true

          try {
            const refreshed = await requestTokenRefresh(refreshToken)
            const { access_token, refresh_token: newRefreshToken, expires_in } = refreshed

            // Update tokens in localStorage (convert expires_in to timestamp)
            localStorage.setItem('auth_token', access_token)
            localStorage.setItem('refresh_token', newRefreshToken)
            localStorage.setItem('token_expires_at', String(Date.now() + expires_in * 1000))

            // Notify subscribers with new token
            onTokenRefreshed(access_token)

            // Retry the original request with new token
            if (originalRequest.headers) {
              originalRequest.headers.Authorization = `Bearer ${access_token}`
            }

            isRefreshing = false
            return apiClient(originalRequest)
          } catch (refreshError) {
            // Always release waiters so they don't hang.
            onTokenRefreshed('')
            isRefreshing = false

            // VPN/proxy/network blips: keep local session so user does not re-login.
            if (isTransientRefreshFailure(refreshError)) {
              return Promise.reject({
                status: 0,
                code: 'AUTH_REFRESH_TRANSIENT',
                message: 'Network unstable while renewing session. Please retry shortly.',
              })
            }

            // Definitive auth rejection (invalid/revoked refresh token, 401/403).
            clearLocalAuthStorage()
            sessionStorage.setItem('auth_expired', '1')
            redirectToLoginIfNeeded()

            return Promise.reject({
              status: 401,
              code: 'TOKEN_REFRESH_FAILED',
              message: 'Session expired. Please log in again.'
            })
          }
        }

        // No refresh token or is auth endpoint - clear auth and redirect
        const hasToken = !!localStorage.getItem('auth_token')
        const headers = error.config?.headers as Record<string, unknown> | undefined
        const authHeader = headers?.Authorization ?? headers?.authorization
        const sentAuth =
          typeof authHeader === 'string'
            ? authHeader.trim() !== ''
            : Array.isArray(authHeader)
              ? authHeader.length > 0
              : !!authHeader

        clearLocalAuthStorage()
        if ((hasToken || sentAuth) && !isAuthEndpoint) {
          sessionStorage.setItem('auth_expired', '1')
        }
        // Only redirect if not already on login page
        redirectToLoginIfNeeded()
      }

      // Return structured error
      return Promise.reject({
        status,
        code: apiData.code,
        reason: apiData.reason,
        error: apiData.error,
        message: apiData.message || apiData.detail || error.message,
        metadata: apiData.metadata,
      })
    }

    // Network error
    return Promise.reject({
      status: 0,
      message: 'Network error. Please check your connection.'
    })
  }
)

export default apiClient
