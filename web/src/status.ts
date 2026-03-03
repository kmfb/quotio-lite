import type { AccountStatus } from './api'

export const statusLabel: Record<AccountStatus, string> = {
  ok: 'OK',
  auth_expired: 'Auth Expired',
  disabled: 'Disabled',
  usage_limited: 'Usage Limited',
  plan_mismatch: 'Plan Mismatch',
  network_error: 'Network Error',
  unknown: 'Unknown',
}

export const statusColor: Record<
  AccountStatus,
  'green' | 'red' | 'orange' | 'yellow' | 'gray'
> = {
  ok: 'green',
  auth_expired: 'red',
  disabled: 'red',
  usage_limited: 'orange',
  plan_mismatch: 'yellow',
  network_error: 'orange',
  unknown: 'gray',
}
