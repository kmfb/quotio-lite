import { useMemo, useState } from 'react'
import {
  Badge,
  Box,
  Button,
  Callout,
  Card,
  Code,
  Flex,
  Heading,
  Spinner,
  Text,
} from '@radix-ui/themes'
import { ExclamationTriangleIcon } from '@radix-ui/react-icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getProxyCredentials,
  getProxyStatus,
  restartProxy,
  rotateProxyApiKey,
  startProxy,
  stopProxy,
} from '../api'

type CopyTarget = 'endpoint' | 'api-key' | 'sample-env' | ''

export function ProxyControlCard() {
  const queryClient = useQueryClient()
  const [showPlainKey, setShowPlainKey] = useState(false)
  const [copied, setCopied] = useState<CopyTarget>('')

  const statusQuery = useQuery({
    queryKey: ['proxy-status'],
    queryFn: getProxyStatus,
    refetchInterval: 5_000,
    refetchIntervalInBackground: true,
  })

  const credentialsQuery = useQuery({
    queryKey: ['proxy-credentials'],
    queryFn: getProxyCredentials,
    refetchInterval: 15_000,
    refetchIntervalInBackground: true,
  })

  async function refreshRuntimeSnapshot() {
    await queryClient.invalidateQueries({ queryKey: ['proxy-status'] })
    await queryClient.invalidateQueries({ queryKey: ['proxy-credentials'] })
    await queryClient.invalidateQueries({ queryKey: ['accounts'] })
  }

  const startMutation = useMutation({
    mutationFn: startProxy,
    onSuccess: refreshRuntimeSnapshot,
  })

  const stopMutation = useMutation({
    mutationFn: stopProxy,
    onSuccess: refreshRuntimeSnapshot,
  })

  const restartMutation = useMutation({
    mutationFn: restartProxy,
    onSuccess: refreshRuntimeSnapshot,
  })

  const rotateMutation = useMutation({
    mutationFn: rotateProxyApiKey,
    onSuccess: async (payload) => {
      await refreshRuntimeSnapshot()
      await queryClient.setQueryData(['proxy-credentials'], (current) => {
        const value =
          current &&
          typeof current === 'object' &&
          'endpoint' in current &&
          'apiKeyMasked' in current &&
          'sampleEnv' in current
            ? (current as {
                endpoint: string
                apiKeyMasked: string
                sampleEnv: string
              })
            : null
        if (!value) {
          return null
        }
        return {
          endpoint: value.endpoint,
          apiKeyMasked: value.apiKeyMasked,
          apiKeyPlain: payload.apiKeyPlain,
          sampleEnv: value.sampleEnv,
        }
      })
      setShowPlainKey(true)
    },
  })

  const status = statusQuery.data
  const credentials = credentialsQuery.data
  const endpoint = credentials?.endpoint || status?.endpoint || 'http://127.0.0.1:8317/v1'
  const displayedKey = showPlainKey
    ? (credentials?.apiKeyPlain ?? '(unavailable)')
    : (credentials?.apiKeyMasked || status?.apiKeyMasked || '-')
  const sampleEnv =
    credentials?.sampleEnv || `OPENAI_BASE_URL=${endpoint}\nOPENAI_API_KEY=<your-key>`

  const actionPending =
    startMutation.isPending ||
    stopMutation.isPending ||
    restartMutation.isPending ||
    rotateMutation.isPending

  const statusBadge = useMemo(() => {
    if (!status) return { text: 'Loading', color: 'gray' as const }
    if (status.running) return { text: 'Running', color: 'green' as const }
    if (status.portConflict?.occupied) return { text: 'Port Occupied', color: 'orange' as const }
    if (status.lastError) return { text: 'Error', color: 'red' as const }
    return { text: 'Stopped', color: 'gray' as const }
  }, [status])

  async function copyText(value: string, target: CopyTarget) {
    if (!value || value === '-') return
    try {
      await navigator.clipboard.writeText(value)
      setCopied(target)
      window.setTimeout(() => setCopied(''), 1000)
    } catch {
      // ignore clipboard failures
    }
  }

  return (
    <Card className="proxy-control-card">
      <Flex direction="column" gap="3">
        <Flex align="center" justify="between" gap="3" wrap="wrap">
          <Box>
            <Flex align="center" gap="2">
              <Heading size="4">Proxy Runtime</Heading>
              <Badge color={statusBadge.color}>{statusBadge.text}</Badge>
            </Flex>
            <Text size="2" color="gray">
              Manual CLIProxyAPI runtime control with stable endpoint and key
            </Text>
          </Box>
          <Flex gap="2" wrap="wrap">
            <Button
              onClick={() => startMutation.mutate()}
              disabled={actionPending || Boolean(status?.running)}
            >
              Start
            </Button>
            <Button
              variant="soft"
              onClick={() => stopMutation.mutate()}
              disabled={actionPending || !status?.running}
            >
              Stop
            </Button>
            <Button
              variant="soft"
              onClick={() => restartMutation.mutate()}
              disabled={actionPending}
            >
              Restart
            </Button>
            <Button
              color="orange"
              variant="soft"
              onClick={() => rotateMutation.mutate()}
              disabled={actionPending}
            >
              Rotate Key
            </Button>
          </Flex>
        </Flex>

        {statusQuery.isFetching && !statusQuery.isPending ? (
          <Flex align="center" gap="1">
            <Spinner size="1" />
            <Text size="1" color="gray">
              Runtime polling every 5s
            </Text>
          </Flex>
        ) : null}

        {status?.portConflict?.occupied ? (
          <Callout.Root color="red" role="alert">
            <Callout.Icon>
              <ExclamationTriangleIcon />
            </Callout.Icon>
            <Callout.Text>
              Port 8317 is occupied
              {status.portConflict.command || status.portConflict.pid
                ? ` (${status.portConflict.command || 'unknown'} pid=${status.portConflict.pid || 0})`
                : ''}
              . Stop the conflicting process and start again.
            </Callout.Text>
          </Callout.Root>
        ) : null}

        {!status?.binaryAccessible ? (
          <Callout.Root color="orange" role="status">
            <Callout.Icon>
              <ExclamationTriangleIcon />
            </Callout.Icon>
            <Callout.Text>
              CLIProxyAPI is missing at <Code>{status?.binaryPath || '-'}</Code>.
              Run <Code>make bootstrap</Code> first.
            </Callout.Text>
          </Callout.Root>
        ) : null}

        {status?.lastError ? (
          <Callout.Root color="red" role="alert">
            <Callout.Icon>
              <ExclamationTriangleIcon />
            </Callout.Icon>
            <Callout.Text>{status.lastError}</Callout.Text>
          </Callout.Root>
        ) : null}

        {statusQuery.isError ? (
          <Callout.Root color="red" role="alert">
            <Callout.Icon>
              <ExclamationTriangleIcon />
            </Callout.Icon>
            <Callout.Text>{statusQuery.error.message}</Callout.Text>
          </Callout.Root>
        ) : null}

        {credentialsQuery.isError ? (
          <Callout.Root color="red" role="alert">
            <Callout.Icon>
              <ExclamationTriangleIcon />
            </Callout.Icon>
            <Callout.Text>{credentialsQuery.error.message}</Callout.Text>
          </Callout.Root>
        ) : null}

        {(startMutation.isError ||
          stopMutation.isError ||
          restartMutation.isError ||
          rotateMutation.isError) ? (
          <Callout.Root color="red" role="alert">
            <Callout.Icon>
              <ExclamationTriangleIcon />
            </Callout.Icon>
            <Callout.Text>
              {startMutation.error?.message ||
                stopMutation.error?.message ||
                restartMutation.error?.message ||
                rotateMutation.error?.message}
            </Callout.Text>
          </Callout.Root>
        ) : null}

        <div className="proxy-control-grid">
          <div className="proxy-control-item">
            <Text size="1" color="gray">
              API Endpoint
            </Text>
            <Flex align="center" justify="between" gap="2">
              <Code>{endpoint}</Code>
              <Button
                size="1"
                variant="soft"
                onClick={() => copyText(endpoint, 'endpoint')}
              >
                {copied === 'endpoint' ? 'Copied' : 'Copy'}
              </Button>
            </Flex>
          </div>

          <div className="proxy-control-item">
            <Text size="1" color="gray">
              API Key
            </Text>
            <Flex align="center" gap="2" wrap="wrap">
              <Code>{displayedKey}</Code>
              <Button
                size="1"
                variant="soft"
                onClick={() => setShowPlainKey((v) => !v)}
                disabled={!credentials?.apiKeyPlain}
              >
                {showPlainKey ? 'Hide' : 'Reveal'}
              </Button>
              <Button
                size="1"
                variant="soft"
                onClick={() =>
                  copyText(credentials?.apiKeyPlain || '', 'api-key')
                }
                disabled={!credentials?.apiKeyPlain}
              >
                {copied === 'api-key' ? 'Copied' : 'Copy'}
              </Button>
            </Flex>
          </div>
        </div>

        <div className="proxy-control-item">
          <Flex align="center" justify="between" gap="2">
            <Text size="1" color="gray">
              OpenAI-compatible sample env
            </Text>
            <Button
              size="1"
              variant="soft"
              onClick={() => copyText(sampleEnv, 'sample-env')}
            >
              {copied === 'sample-env' ? 'Copied' : 'Copy'}
            </Button>
          </Flex>
          <pre className="proxy-sample-env">{sampleEnv}</pre>
        </div>

        <Text size="1" color="gray">
          PID: {status?.pid || '-'} | Started: {status?.startedAt || '-'}
        </Text>
      </Flex>
    </Card>
  )
}
