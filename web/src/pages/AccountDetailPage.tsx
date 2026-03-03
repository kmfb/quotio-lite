import {
  Badge,
  Box,
  Button,
  Callout,
  Card,
  Code,
  DataList,
  Flex,
  Heading,
  Spinner,
  Text,
} from '@radix-ui/themes'
import { ExclamationTriangleIcon } from '@radix-ui/react-icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { getAccount, probeAccount } from '../api'
import { nextPollMs } from '../lib/polling'
import { UsageGauge } from '../components/UsageGauge'
import { statusColor, statusLabel } from '../status'

const WINDOW_5H_SECONDS = 5 * 60 * 60
const WINDOW_WEEKLY_SECONDS = 7 * 24 * 60 * 60

export function AccountDetailPage() {
  const { file } = useParams({ from: '/accounts/$file' })
  const queryClient = useQueryClient()

  const detailQuery = useQuery({
    queryKey: ['account', file],
    queryFn: () => getAccount(file),
    refetchInterval: () => nextPollMs(60_000, 120_000),
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
  })

  const probeMutation = useMutation({
    mutationFn: () => probeAccount(file),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['account', file] })
      await queryClient.invalidateQueries({ queryKey: ['accounts'] })
    },
  })

  if (detailQuery.isPending) {
    return (
      <Flex align="center" justify="center" p="8" gap="2">
        <Spinner />
        <Text>Loading account detail...</Text>
      </Flex>
    )
  }

  if (detailQuery.isError) {
    return (
      <Callout.Root color="red" role="alert">
        <Callout.Icon>
          <ExclamationTriangleIcon />
        </Callout.Icon>
        <Callout.Text>{detailQuery.error.message}</Callout.Text>
      </Callout.Root>
    )
  }

  const detail = detailQuery.data

  return (
    <Flex direction="column" gap="4">
      <Card className="detail-header-card">
        <Flex align="center" justify="between" gap="2" wrap="wrap">
          <Box>
            <Flex align="center" gap="2" wrap="wrap">
              <Heading size="4">Account Detail</Heading>
              <Badge color={statusColor[detail.status]}>
                {statusLabel[detail.status]}
              </Badge>
              <Badge variant="soft" color="gray">
                {detail.usage.planType || detail.type || '-'}
              </Badge>
            </Flex>
            <Text color="gray" size="2">
              {detail.email}
            </Text>
          </Box>
          <Flex gap="2" align="center">
            <Button asChild variant="soft">
              <Link to="/accounts">Back to List</Link>
            </Button>
            <Button
              onClick={() => probeMutation.mutate()}
              disabled={probeMutation.isPending}
            >
              {probeMutation.isPending ? 'Probing...' : 'Probe Now'}
            </Button>
          </Flex>
        </Flex>
      </Card>

      {probeMutation.isError ? (
        <Callout.Root color="red" role="alert">
          <Callout.Icon>
            <ExclamationTriangleIcon />
          </Callout.Icon>
          <Callout.Text>{probeMutation.error.message}</Callout.Text>
        </Callout.Root>
      ) : null}

      {detail.usage.status !== 'ok' && detail.usage.message ? (
        <Callout.Root color="orange" role="status">
          <Callout.Icon>
            <ExclamationTriangleIcon />
          </Callout.Icon>
          <Callout.Text>{detail.usage.message}</Callout.Text>
        </Callout.Root>
      ) : null}

      <Card>
        <Flex direction="column" gap="3">
          <Heading size="3">Quota Overview</Heading>
          <Flex className="quota-grid" gap="3" wrap="wrap">
            <Box className="quota-item">
              <Text size="2" weight="bold">
                5h Window
              </Text>
              <UsageGauge
                usedPercent={detail.usage.window5h.usedPercent}
                resetAt={detail.usage.window5h.resetAt}
                windowSeconds={WINDOW_5H_SECONDS}
                showUsedText
              />
            </Box>
            <Box className="quota-item">
              <Text size="2" weight="bold">
                Weekly Window
              </Text>
              <UsageGauge
                usedPercent={detail.usage.weekly.usedPercent}
                resetAt={detail.usage.weekly.resetAt}
                windowSeconds={WINDOW_WEEKLY_SECONDS}
                showUsedText
              />
            </Box>
          </Flex>
        </Flex>
      </Card>

      <Flex className="detail-grid" gap="3" wrap="wrap">
        <Card className="detail-panel">
          <Heading size="3">Account Metadata</Heading>
          <DataList.Root>
            <DataList.Item>
              <DataList.Label minWidth="120px">File</DataList.Label>
              <DataList.Value>
                <Code>{detail.file}</Code>
              </DataList.Value>
            </DataList.Item>
            <DataList.Item>
              <DataList.Label minWidth="120px">Type</DataList.Label>
              <DataList.Value>{detail.type || '-'}</DataList.Value>
            </DataList.Item>
            <DataList.Item>
              <DataList.Label minWidth="120px">Account ID</DataList.Label>
              <DataList.Value>{detail.accountId || '-'}</DataList.Value>
            </DataList.Item>
            <DataList.Item>
              <DataList.Label minWidth="120px">Disabled</DataList.Label>
              <DataList.Value>{String(detail.disabled)}</DataList.Value>
            </DataList.Item>
            <DataList.Item>
              <DataList.Label minWidth="120px">Expired</DataList.Label>
              <DataList.Value>{String(detail.expired)}</DataList.Value>
            </DataList.Item>
            <DataList.Item>
              <DataList.Label minWidth="120px">Last Refresh</DataList.Label>
              <DataList.Value>{detail.lastRefresh || '-'}</DataList.Value>
            </DataList.Item>
            <DataList.Item>
              <DataList.Label minWidth="120px">Last Probe At</DataList.Label>
              <DataList.Value>{detail.lastProbeAt || '-'}</DataList.Value>
            </DataList.Item>
          </DataList.Root>
        </Card>

        <Card className="detail-panel">
          <Heading size="3">Credential Signals</Heading>
          <DataList.Root>
            <DataList.Item>
              <DataList.Label minWidth="120px">Access Token</DataList.Label>
              <DataList.Value>{String(detail.accessTokenPresent)}</DataList.Value>
            </DataList.Item>
            <DataList.Item>
              <DataList.Label minWidth="120px">Refresh Token</DataList.Label>
              <DataList.Value>{String(detail.refreshTokenPresent)}</DataList.Value>
            </DataList.Item>
            <DataList.Item>
              <DataList.Label minWidth="120px">ID Token</DataList.Label>
              <DataList.Value>{String(detail.idTokenPresent)}</DataList.Value>
            </DataList.Item>
          </DataList.Root>
          <Box mt="3">
            <Text size="2" weight="bold">
              Last Probe Message
            </Text>
            <pre className="raw-snippet">{detail.lastProbeMessage || '-'}</pre>
          </Box>
        </Card>
      </Flex>
    </Flex>
  )
}
