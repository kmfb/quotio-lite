import { useEffect, useState } from 'react'
import { Flex, Text } from '@radix-ui/themes'

interface UsageGaugeProps {
  usedPercent: number | null
  resetAt?: string
  windowSeconds?: number
  compact?: boolean
  showUsedText?: boolean
}

export function UsageGauge({
  usedPercent,
  resetAt,
  windowSeconds,
  compact = false,
  showUsedText = false,
}: UsageGaugeProps) {
  const [nowMs, setNowMs] = useState(() => Date.now())

  useEffect(() => {
    if (!resetAt || !windowSeconds) return
    const id = window.setInterval(() => setNowMs(Date.now()), 30_000)
    return () => window.clearInterval(id)
  }, [resetAt, windowSeconds])

  const hasUsage = usedPercent !== null && !Number.isNaN(usedPercent)
  const used = hasUsage ? clamp(usedPercent) : 0
  const remaining = hasUsage ? clamp(100 - used) : 0
  const state = usageState(remaining)
  const reset = resolveReset(resetAt, windowSeconds, nowMs)

  return (
    <Flex direction="column" gap={compact ? '1' : '2'} className="usage-gauge">
      {hasUsage ? (
        <>
          <Flex align="center" justify="between">
            <Text size={compact ? '1' : '2'} weight="bold">
              剩余 {Math.round(remaining)}%
            </Text>
            {showUsedText ? (
              <Text size="1" color="gray">
                已用 {Math.round(used)}%
              </Text>
            ) : null}
          </Flex>
          <div className="usage-gauge-track">
            <div
              className={`usage-gauge-fill usage-gauge-fill-${state}`}
              style={{ width: `${remaining}%` }}
            />
          </div>
        </>
      ) : (
        <Text size={compact ? '1' : '2'} color="gray">
          -
        </Text>
      )}

      {reset ? (
        <>
          <Flex align="center" justify="between">
            <Text size="1" color="gray">
              {compact
                ? `重置剩余 ${formatCountdown(reset.remainingSec, true)}`
                : `重置周期剩余 ${formatCountdown(reset.remainingSec, false)}`}
            </Text>
            {!compact ? (
              <Text size="1" color="gray">
                {resetAt ? new Date(resetAt).toLocaleString() : ''}
              </Text>
            ) : null}
          </Flex>
          <div className="usage-reset-track">
            <div
              className={`usage-reset-fill usage-reset-fill-${usageState(
                reset.remainingPercent,
              )}`}
              style={{ width: `${reset.remainingPercent}%` }}
            />
          </div>
        </>
      ) : null}
    </Flex>
  )
}

function clamp(value: number) {
  return Math.max(0, Math.min(100, value))
}

function usageState(remaining: number) {
  if (remaining <= 20) return 'low'
  if (remaining <= 50) return 'mid'
  return 'high'
}

function resolveReset(
  resetAt: string | undefined,
  windowSeconds: number | undefined,
  nowMs: number,
) {
  if (!resetAt || !windowSeconds || windowSeconds <= 0) {
    return null
  }
  const resetMs = Date.parse(resetAt)
  if (Number.isNaN(resetMs)) {
    return null
  }
  const remainingSec = Math.max(0, Math.floor((resetMs - nowMs) / 1000))
  const remainingPercent = clamp((remainingSec / windowSeconds) * 100)
  return { remainingSec, remainingPercent }
}

function formatCountdown(seconds: number, compact: boolean) {
  if (seconds <= 0) return compact ? '0m' : '0分钟'

  const day = Math.floor(seconds / 86400)
  const hour = Math.floor((seconds % 86400) / 3600)
  const minute = Math.floor((seconds % 3600) / 60)

  if (compact) {
    if (day > 0) return `${day}d ${hour}h`
    if (hour > 0) return `${hour}h ${minute}m`
    return `${minute}m`
  }

  if (day > 0) return `${day}天 ${hour}小时 ${minute}分钟`
  if (hour > 0) return `${hour}小时 ${minute}分钟`
  return `${minute}分钟`
}
