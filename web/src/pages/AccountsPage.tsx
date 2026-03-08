import { useMemo, useState } from 'react'
import {
  Badge,
  Box,
  Button,
  Callout,
  Card,
  Flex,
  Heading,
  Spinner,
  Table,
  Text,
} from '@radix-ui/themes'
import { ExclamationTriangleIcon } from '@radix-ui/react-icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { Link } from '@tanstack/react-router'
import {
  deleteAccount,
  listAccounts,
  loginCodex,
  probeAccount,
  type AccountRecord,
  type AccountStatus,
} from '../api'
import { ProxyControlCard } from '../components/ProxyControlCard'
import { nextPollMs } from '../lib/polling'
import { UsageGauge } from '../components/UsageGauge'
import { statusColor, statusLabel } from '../status'

const WINDOW_5H_SECONDS = 5 * 60 * 60
const WINDOW_WEEKLY_SECONDS = 7 * 24 * 60 * 60
const LOW_QUOTA_THRESHOLD = 20
const EMPTY_ACCOUNTS: AccountRecord[] = []

const columnHelper = createColumnHelper<AccountRecord>()

const statusFilterOptions: Array<{ value: 'all' | AccountStatus; label: string }> = [
  { value: 'all', label: 'All Status' },
  { value: 'ok', label: 'OK' },
  { value: 'auth_expired', label: 'Auth Expired' },
  { value: 'disabled', label: 'Disabled' },
  { value: 'usage_limited', label: 'Usage Limited' },
  { value: 'plan_mismatch', label: 'Plan Mismatch' },
  { value: 'network_error', label: 'Network Error' },
  { value: 'unknown', label: 'Unknown' },
]

export function AccountsPage() {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | AccountStatus>('all')

  const accountsQuery = useQuery({
    queryKey: ['accounts'],
    queryFn: listAccounts,
    refetchInterval: () => nextPollMs(60_000, 120_000),
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
  })

  const loginMutation = useMutation({
    mutationFn: () => loginCodex(),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['accounts'] })
    },
  })

  const probeMutation = useMutation({
    mutationFn: (file: string) => probeAccount(file),
    onSuccess: async (_, file) => {
      await queryClient.invalidateQueries({ queryKey: ['accounts'] })
      await queryClient.invalidateQueries({ queryKey: ['account', file] })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (file: string) => deleteAccount(file),
    onSuccess: async (_, file) => {
      await queryClient.invalidateQueries({ queryKey: ['accounts'] })
      queryClient.removeQueries({ queryKey: ['account', file] })
    },
  })

  const allAccounts = accountsQuery.data ?? EMPTY_ACCOUNTS

  const summary = useMemo(() => {
    const total = allAccounts.length
    const ok = allAccounts.filter((item) => item.status === 'ok').length
    const team = allAccounts.filter((item) => item.type === 'team').length
    const lowQuota = allAccounts.filter(isLowQuota).length
    return { total, ok, team, lowQuota }
  }, [allAccounts])

  const filteredAccounts = useMemo(() => {
    const keyword = search.trim().toLowerCase()
    return allAccounts.filter((item) => {
      if (statusFilter !== 'all' && item.status !== statusFilter) {
        return false
      }
      if (!keyword) {
        return true
      }
      const haystack = [
        item.email,
        item.file,
        item.type,
        item.accountId,
        item.usage.planType,
      ]
        .join(' ')
        .toLowerCase()
      return haystack.includes(keyword)
    })
  }, [allAccounts, search, statusFilter])

  const columns = [
    columnHelper.accessor('email', {
      header: 'Email',
      cell: (info) => (
        <Link
          to="/accounts/$file"
          params={{ file: info.row.original.file }}
          className="inline-link"
        >
          {info.getValue() || '(no email)'}
        </Link>
      ),
    }),
    columnHelper.accessor('type', {
      header: 'Type',
      cell: (info) => <Text>{info.getValue() || '-'}</Text>,
    }),
    columnHelper.accessor('status', {
      header: 'Status',
      cell: (info) => (
        <Badge color={statusColor[info.getValue()]}>
          {statusLabel[info.getValue()]}
        </Badge>
      ),
    }),
    columnHelper.display({
      id: 'plan',
      header: 'Plan',
      cell: (info) => (
        <Badge variant="soft" color="gray">
          {info.row.original.usage.planType || '-'}
        </Badge>
      ),
    }),
    columnHelper.display({
      id: 'usage5h',
      header: '5h Quota',
      cell: (info) => (
        <UsageGauge
          usedPercent={info.row.original.usage.window5h.usedPercent}
          resetAt={info.row.original.usage.window5h.resetAt}
          windowSeconds={WINDOW_5H_SECONDS}
          compact
        />
      ),
    }),
    columnHelper.display({
      id: 'usageWeekly',
      header: 'Weekly Quota',
      cell: (info) => (
        <UsageGauge
          usedPercent={info.row.original.usage.weekly.usedPercent}
          resetAt={info.row.original.usage.weekly.resetAt}
          windowSeconds={WINDOW_WEEKLY_SECONDS}
          compact
        />
      ),
    }),
    columnHelper.accessor('mtime', {
      header: 'Updated',
      cell: (info) => (
        <Text size="1" color="gray">
          {new Date(info.getValue()).toLocaleString()}
        </Text>
      ),
    }),
    columnHelper.display({
      id: 'actions',
      header: 'Actions',
      cell: (info) => {
        const row = info.row.original
        const probingThisRow =
          probeMutation.isPending && probeMutation.variables === row.file
        const deletingThisRow =
          deleteMutation.isPending && deleteMutation.variables === row.file

        return (
          <Flex gap="2" wrap="wrap">
            <Button
              size="1"
              variant="soft"
              onClick={() => probeMutation.mutate(row.file)}
              disabled={probingThisRow}
            >
              {probingThisRow ? 'Probing...' : 'Probe'}
            </Button>
            <Button asChild size="1" variant="soft">
              <Link to="/accounts/$file" params={{ file: row.file }}>
                Details
              </Link>
            </Button>
            <Button
              size="1"
              color="red"
              variant="soft"
              disabled={deletingThisRow}
              onClick={() => {
                const first = window.confirm(`Delete account ${row.email}?`)
                if (!first) return
                const second = window.confirm(
                  `Confirm deleting file ${row.file}?`,
                )
                if (!second) return
                deleteMutation.mutate(row.file)
              }}
            >
              {deletingThisRow ? 'Deleting...' : 'Delete'}
            </Button>
          </Flex>
        )
      },
    }),
  ]

  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({
    data: filteredAccounts,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  return (
    <Flex direction="column" gap="4">
      <ProxyControlCard />

      <Card className="accounts-toolbar-card">
        <Flex align="center" justify="between" gap="3" wrap="wrap">
          <Box>
            <Heading size="4">Accounts</Heading>
            <Text color="gray" size="2">
              Foreground refresh every 60-120s with randomized intervals
            </Text>
          </Box>
          <Flex gap="2" align="center">
            {accountsQuery.isFetching && !accountsQuery.isPending ? (
              <Flex gap="1" align="center">
                <Spinner size="1" />
                <Text size="1" color="gray">
                  Refreshing
                </Text>
              </Flex>
            ) : null}
            <Button
              onClick={() => loginMutation.mutate()}
              disabled={loginMutation.isPending}
            >
              {loginMutation.isPending ? 'Logging in...' : 'Add Codex Account'}
            </Button>
          </Flex>
        </Flex>

        <Flex className="accounts-filters" gap="3" wrap="wrap" align="end">
          <Box className="accounts-filter-grow">
            <Text as="label" size="1" color="gray" className="accounts-filter-label">
              Search
            </Text>
            <input
              className="accounts-search-input"
              placeholder="Email / file / account id / plan"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </Box>

          <Box>
            <Text as="label" size="1" color="gray" className="accounts-filter-label">
              Status
            </Text>
            <select
              className="accounts-filter-select"
              value={statusFilter}
              onChange={(event) =>
                setStatusFilter(event.target.value as 'all' | AccountStatus)
              }
            >
              {statusFilterOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </Box>

          <Button
            variant="soft"
            color="gray"
            onClick={() => {
              setSearch('')
              setStatusFilter('all')
            }}
          >
            Reset
          </Button>
        </Flex>

        <Text size="1" color="gray">
          Showing {filteredAccounts.length} of {allAccounts.length} accounts
        </Text>
      </Card>

      <Flex className="accounts-summary" gap="3" wrap="wrap">
        <Card className="summary-tile">
          <Text size="1" color="gray">
            Total Accounts
          </Text>
          <Heading size="5">{summary.total}</Heading>
        </Card>
        <Card className="summary-tile">
          <Text size="1" color="gray">
            Healthy Status
          </Text>
          <Heading size="5">{summary.ok}</Heading>
        </Card>
        <Card className="summary-tile">
          <Text size="1" color="gray">
            Team Accounts
          </Text>
          <Heading size="5">{summary.team}</Heading>
        </Card>
        <Card className="summary-tile">
          <Text size="1" color="gray">
            Low Quota Risk
          </Text>
          <Heading size="5" color={summary.lowQuota > 0 ? 'red' : 'gray'}>
            {summary.lowQuota}
          </Heading>
        </Card>
      </Flex>

      {accountsQuery.isError ? (
        <Callout.Root color="red" role="alert">
          <Callout.Icon>
            <ExclamationTriangleIcon />
          </Callout.Icon>
          <Callout.Text>{accountsQuery.error.message}</Callout.Text>
        </Callout.Root>
      ) : null}

      {loginMutation.isError ? (
        <Callout.Root color="red" role="alert">
          <Callout.Icon>
            <ExclamationTriangleIcon />
          </Callout.Icon>
          <Callout.Text>{loginMutation.error.message}</Callout.Text>
        </Callout.Root>
      ) : null}

      {probeMutation.isError ? (
        <Callout.Root color="red" role="alert">
          <Callout.Icon>
            <ExclamationTriangleIcon />
          </Callout.Icon>
          <Callout.Text>{probeMutation.error.message}</Callout.Text>
        </Callout.Root>
      ) : null}

      {deleteMutation.isError ? (
        <Callout.Root color="red" role="alert">
          <Callout.Icon>
            <ExclamationTriangleIcon />
          </Callout.Icon>
          <Callout.Text>{deleteMutation.error.message}</Callout.Text>
        </Callout.Root>
      ) : null}

      <Card>
        {accountsQuery.isPending ? (
          <Flex align="center" justify="center" p="6" gap="2">
            <Spinner />
            <Text>Loading accounts...</Text>
          </Flex>
        ) : (
          <div className="accounts-table-wrap">
            <Table.Root variant="surface" size="2" className="accounts-table">
              <Table.Header>
                {table.getHeaderGroups().map((headerGroup) => (
                  <Table.Row key={headerGroup.id}>
                    {headerGroup.headers.map((header) => (
                      <Table.ColumnHeaderCell key={header.id}>
                        {header.isPlaceholder
                          ? null
                          : flexRender(
                              header.column.columnDef.header,
                              header.getContext(),
                            )}
                      </Table.ColumnHeaderCell>
                    ))}
                  </Table.Row>
                ))}
              </Table.Header>
              <Table.Body>
                {table.getRowModel().rows.length === 0 ? (
                  <Table.Row>
                    <Table.Cell colSpan={8}>
                      <Text color="gray">No matching accounts found.</Text>
                    </Table.Cell>
                  </Table.Row>
                ) : (
                  table.getRowModel().rows.map((row) => (
                    <Table.Row
                      key={row.id}
                      className={
                        isLowQuota(row.original) ? 'accounts-row-risk' : undefined
                      }
                    >
                      {row.getVisibleCells().map((cell) => (
                        <Table.Cell key={cell.id}>
                          {flexRender(
                            cell.column.columnDef.cell,
                            cell.getContext(),
                          )}
                        </Table.Cell>
                      ))}
                    </Table.Row>
                  ))
                )}
              </Table.Body>
            </Table.Root>
          </div>
        )}
      </Card>
    </Flex>
  )
}

function remainingPercent(usedPercent: number | null): number | null {
  if (usedPercent === null || Number.isNaN(usedPercent)) {
    return null
  }
  return Math.max(0, Math.min(100, 100 - usedPercent))
}

function isLowQuota(record: AccountRecord): boolean {
  const remain5h = remainingPercent(record.usage.window5h.usedPercent)
  const remainWeekly = remainingPercent(record.usage.weekly.usedPercent)
  return (
    (remain5h !== null && remain5h <= LOW_QUOTA_THRESHOLD) ||
    (remainWeekly !== null && remainWeekly <= LOW_QUOTA_THRESHOLD)
  )
}
