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

export interface MetaResponse {
  version: string
  host: string
  port: number
  authDir: string
  cliProxyPath: string
  probeModel: string
  usageSource: string
  authDirAccessible: boolean
  cliProxyAccessible: boolean
}

const API_BASE_URL =
  import.meta.env.VITE_API_BASE_URL ?? 'http://127.0.0.1:18417'

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
    ...init,
  })

  if (!response.ok) {
    const raw = await response.text()
    throw new Error(raw || `Request failed: ${response.status}`)
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

export async function loginCodex(payload: {
  incognito?: boolean
}): Promise<{ file: string; account: AccountDetail }> {
  return requestJSON('/api/accounts/login', {
    method: 'POST',
    body: JSON.stringify(payload),
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
