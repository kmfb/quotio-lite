export type AccountStatus =
  | 'ok'
  | 'auth_expired'
  | 'disabled'
  | 'usage_limited'
  | 'plan_mismatch'
  | 'network_error'
  | 'unknown'

export interface AccountRecord {
  file: string
  email: string
  type: string
  accountId: string
  disabled: boolean
  expired: boolean
  lastRefresh: string
  mtime: string
  status: AccountStatus
  lastProbeAt: string
  lastProbeMessage: string
  usage: AccountUsage
}

export interface AccountDetail extends AccountRecord {
  accessTokenPresent: boolean
  refreshTokenPresent: boolean
  idTokenPresent: boolean
}

export interface ProbeResult {
  httpStatus: number
  latencyMs: number
  classification: AccountStatus
  rawSnippet: string
}

export interface UsageWindow {
  usedPercent: number | null
  resetAt: string
}

export interface AccountUsage {
  window5h: UsageWindow
  weekly: UsageWindow
  planType: string
  status: string
  message: string
}

export interface LoginCapabilities {
  version: string
  supportsCodexLogin: boolean
  supportsCodexDeviceLogin: boolean
  supportsNoBrowser: boolean
  supportsIncognito: boolean
}

export interface MetaResponse {
  version: string
  mode?: 'dev' | 'service'
  host: string
  port: number
  authDir: string
  cliProxyPath: string
  probeModel: string
  frontendDistDir?: string
  frontendServing?: boolean
  usageSource: string
  authDirAccessible: boolean
  cliProxyAccessible: boolean
  proxyManagedConfigPath: string
  proxyManagedStatePath: string
  proxyDefaultPort: number
  proxyHost: string
  loginCapabilities: LoginCapabilities
  loginCapabilitiesError?: string
}

export interface PortConflict {
  occupied: boolean
  pid?: number
  command?: string
}

export interface ProxyStatus {
  running: boolean
  pid: number
  host: string
  port: number
  endpoint: string
  startedAt: string
  binaryPath: string
  binaryAccessible: boolean
  apiKeyMasked: string
  lastError: string
  portConflict?: PortConflict
}

export interface ProxyCredentials {
  endpoint: string
  apiKeyMasked: string
  apiKeyPlain: string
  sampleEnv: string
}

export interface RotateApiKeyResponse {
  status: ProxyStatus
  apiKeyPlain: string
}

const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/$/, '')

function apiURL(path: string): string {
  return API_BASE_URL ? `${API_BASE_URL}${path}` : path
}

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(apiURL(path), {
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
    ...init,
  })

  if (!response.ok) {
    const raw = await response.text()
    let message = raw || `Request failed: ${response.status}`
    try {
      const parsed = JSON.parse(raw) as { error?: string }
      if (parsed.error) {
        message = parsed.error
      }
    } catch {
      // keep raw message
    }
    throw new Error(message)
  }

  if (response.status === 204) {
    return undefined as T
  }

  return (await response.json()) as T
}

export async function getMeta(): Promise<MetaResponse> {
  return requestJSON<MetaResponse>('/api/meta')
}

export async function listAccounts(): Promise<AccountRecord[]> {
  const data = await requestJSON<{ items: AccountRecord[] }>('/api/accounts')
  return data.items
}

export async function getAccount(file: string): Promise<AccountDetail> {
  return requestJSON<AccountDetail>(
    `/api/accounts/${encodeURIComponent(file)}`,
  )
}

export async function loginCodex(payload?: {
  mode?: 'oauth' | 'device'
}): Promise<{ file: string; account: AccountDetail }> {
  return requestJSON('/api/accounts/login', {
    method: 'POST',
    body: JSON.stringify(payload ?? {}),
  })
}

export async function deleteAccount(file: string): Promise<void> {
  await requestJSON<void>(`/api/accounts/${encodeURIComponent(file)}`, {
    method: 'DELETE',
  })
}

export async function probeAccount(file: string): Promise<ProbeResult> {
  return requestJSON<ProbeResult>(
    `/api/accounts/${encodeURIComponent(file)}/probe`,
    {
      method: 'POST',
      body: '{}',
    },
  )
}

export async function getProxyStatus(): Promise<ProxyStatus> {
  return requestJSON<ProxyStatus>('/api/proxy/status')
}

export async function startProxy(): Promise<ProxyStatus> {
  return requestJSON<ProxyStatus>('/api/proxy/start', {
    method: 'POST',
    body: '{}',
  })
}

export async function stopProxy(): Promise<ProxyStatus> {
  return requestJSON<ProxyStatus>('/api/proxy/stop', {
    method: 'POST',
    body: '{}',
  })
}

export async function restartProxy(): Promise<ProxyStatus> {
  return requestJSON<ProxyStatus>('/api/proxy/restart', {
    method: 'POST',
    body: '{}',
  })
}

export async function getProxyCredentials(): Promise<ProxyCredentials> {
  return requestJSON<ProxyCredentials>('/api/proxy/credentials')
}

export async function rotateProxyApiKey(): Promise<RotateApiKeyResponse> {
  return requestJSON<RotateApiKeyResponse>('/api/proxy/api-key/rotate', {
    method: 'POST',
    body: '{}',
  })
}
