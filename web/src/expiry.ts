import type { AccountExpiry } from './api'

export function expiryTone(
  expiry: AccountExpiry,
): 'green' | 'yellow' | 'orange' | 'red' | 'gray' {
  if (expiry.status === 'missing_email' || expiry.status === 'unavailable') {
    return 'gray'
  }
  if (isExpiryExpired(expiry)) {
    return 'red'
  }
  if (expiry.status === 'active') {
    if (expiry.daysRemaining === null) {
      return 'green'
    }
    if (expiry.daysRemaining <= 3) {
      return 'red'
    }
    if (expiry.daysRemaining <= 7) {
      return 'orange'
    }
    if (expiry.daysRemaining <= 30) {
      return 'yellow'
    }
    return 'green'
  }
  return 'yellow'
}

export function expiryHeadline(expiry: AccountExpiry): string {
  if (expiry.status === 'missing_email') {
    return 'No Email'
  }
  if (expiry.status === 'unavailable') {
    return 'Unavailable'
  }
  if (isExpiryExpired(expiry)) {
    return 'Expired'
  }
  if (expiry.status === 'active' && expiry.daysRemaining !== null) {
    if (expiry.daysRemaining === 0) {
      return 'Expires Today'
    }
    return `${expiry.daysRemaining}d Left`
  }
  if (expiry.status === 'active') {
    return 'Active'
  }
  return titleize(expiry.status)
}

export function expiryDetail(expiry: AccountExpiry): string {
  if (expiry.expireDate) {
    return `Expires ${expiry.expireDate}`
  }
  if (expiry.message) {
    return expiry.message
  }
  if (expiry.joinDate) {
    return `Joined ${expiry.joinDate}`
  }
  return '-'
}

export function isExpirySoon(expiry: AccountExpiry, thresholdDays = 7): boolean {
  return (
    expiry.status === 'active' &&
    expiry.daysRemaining !== null &&
    expiry.daysRemaining >= 0 &&
    expiry.daysRemaining <= thresholdDays
  )
}

export function isExpiryExpired(expiry: AccountExpiry): boolean {
  return (
    expiry.status === 'expired' ||
    expiry.status === 'inactive' ||
    expiry.status === 'canceled' ||
    expiry.status === 'cancelled' ||
    (expiry.daysRemaining !== null && expiry.daysRemaining < 0)
  )
}

function titleize(value: string): string {
  return value
    .split(/[_\s-]+/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}
