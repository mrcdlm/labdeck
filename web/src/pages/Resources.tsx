import { ArrowDown, ArrowUp, ChevronDown, Container, Cpu, Plug, Search, Server } from 'lucide-react'
import { Fragment, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { IntegrationIcon } from '@/components/IntegrationIcon'
import { Meter } from '@/components/Meter'
import { MetricChart, MetricRangeToggle } from '@/components/MetricChart'
import { LiveItem, LiveTableRow } from '@/components/LiveRow'
import { PageHeader } from '@/components/PageHeader'
import { Segmented } from '@/components/Segmented'
import { StatusBadge } from '@/components/StatusBadge'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { Integration, MetricRange, Resource } from '@/lib/api'
import { formatBytes, formatNumber, formatUptime } from '@/lib/format'
import { useIntegrations, useIntegrationTypes } from '@/lib/queries'
import { cn } from '@/lib/utils'

type StateFilter = 'all' | 'running' | 'stopped'
type SortKey = 'name' | 'cpu' | 'mem'

const pct = (used?: number, total?: number) => (used !== undefined && total ? (used / total) * 100 : undefined)
const running = (r: Resource) => ['running', 'online', 'healthy', 'unhealthy'].includes(r.state)

function KindBadge({ r }: { r: Resource }) {
  const { t } = useTranslation()
  return (
    <Badge variant="outline" className="px-1.5 py-0 text-[11px] font-normal">
      {t(`resources.kind.${r.kind}`)}
      {r.info?.vmid && ` ${r.info.vmid}`}
    </Badge>
  )
}

/** Zustand der Quelle als Badge (laufend grün, gestoppt grau, unhealthy rot). */
function StateBadge({ r }: { r: Resource }) {
  const { t } = useTranslation()
  const known = ['running', 'stopped', 'online', 'offline', 'healthy', 'unhealthy', 'exited', 'paused', 'starting'] as const
  const s = known.find((k) => k === r.state)
  return <StatusBadge status={r.status} label={s ? t(`resources.state.${s}`) : r.state} />
}

function Net({ r }: { r: Resource }) {
  if (r.net_in === undefined || r.net_out === undefined) return <span className="text-muted-foreground">—</span>
  const f = (v: number) => (r.net_rate ? `${formatBytes(v)}/s` : formatBytes(v))
  return (
    <span className="flex flex-col text-xs whitespace-nowrap tabular-nums">
      <span>
        <ArrowDown className="inline size-3" aria-hidden /> {f(r.net_in)}
      </span>
      <span className="text-muted-foreground">
        <ArrowUp className="inline size-3" aria-hidden /> {f(r.net_out)}
      </span>
    </span>
  )
}

function CpuCell({ r }: { r: Resource }) {
  const { t } = useTranslation()
  if (r.cpu === undefined) return <span className="text-muted-foreground">—</span>
  return (
    <Meter
      value={r.cpu}
      size="sm"
      detail={`${formatNumber(Math.round(r.cpu * 10) / 10)} %`}
      label={r.cpus ? t('resources.cores', { count: r.cpus }) : undefined}
      className="w-28"
    />
  )
}

function MemCell({ used, total }: { used?: number; total?: number }) {
  if (used === undefined || !total) {
    return total ? <span className="text-muted-foreground text-xs">{formatBytes(total)}</span> : <span className="text-muted-foreground">—</span>
  }
  return <Meter value={pct(used, total)} size="sm" detail={`${formatBytes(used)} / ${formatBytes(total)}`} className="w-36" />
}

/** Verlauf einer Ressource: CPU, RAM, Platte – Diagramme aus den Kennzahlen. */
function ResourceHistory({ integration, r }: { integration: Integration; r: Resource }) {
  const { t } = useTranslation()
  const keys = (integration.snapshot?.metrics ?? []).filter(
    (m) => m.resource === r.id && (m.unit === '%' || m.unit === 'bytes'),
  )
  const [selected, setSelected] = useState(keys[0]?.key)
  const [range, setRange] = useState<MetricRange>('24h')
  const current = keys.find((k) => k.key === selected) ?? keys[0]
  if (!current) return <p className="text-muted-foreground text-sm">{t('resources.noHistory')}</p>
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Segmented
          label={t('resources.metric')}
          value={current.key}
          onChange={setSelected}
          options={keys.map((k) => ({ value: k.key, label: t(`resources.metrics.${k.name}`, { defaultValue: k.name }) }))}
        />
        <MetricRangeToggle value={range} onChange={setRange} />
      </div>
      <MetricChart
        integrationId={integration.id}
        metricKey={current.key}
        label={`${r.name} · ${t(`resources.metrics.${current.name}`, { defaultValue: current.name })}`}
        range={range}
      />
    </div>
  )
}

/** Host bzw. Node: große Anzeige für CPU, RAM, Platte. */
function HostCard({ integration, host, icon, guests }: {
  integration: Integration
  host: Resource
  icon?: string
  guests: Resource[]
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const up = guests.filter(running).length
  return (
    <Card className="gap-4 py-5" data-host={host.name}>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center gap-3">
          <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg">
            <IntegrationIcon name={icon} type={integration.type} className="size-5" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-lg font-semibold">{host.name}</h2>
              <StateBadge r={host} />
            </div>
            <p className="text-muted-foreground text-sm">
              {integration.name}
              {host.uptime ? ` · ${t('resources.uptime', { time: formatUptime(host.uptime) })}` : ''}
              {guests.length > 0 && ` · ${t('resources.running', { count: up, total: guests.length })}`}
            </p>
          </div>
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-sm"
            onClick={() => setOpen((o) => !o)}
            aria-expanded={open}
          >
            {t('resources.history')}
            <ChevronDown className={cn('size-4 transition-transform', open && 'rotate-180')} />
          </button>
        </div>
        <div className={cn('grid gap-4', host.disk_total ? 'sm:grid-cols-3' : 'sm:grid-cols-2')}>
          <Meter
            size="lg"
            value={host.cpu}
            label={t('resources.cpu')}
            detail={host.cpu !== undefined ? `${formatNumber(Math.round(host.cpu * 10) / 10)} %` : '—'}
          />
          <Meter
            size="lg"
            value={pct(host.mem_used, host.mem_total)}
            label={t('resources.mem')}
            detail={host.mem_used !== undefined && host.mem_total ? `${formatBytes(host.mem_used)} / ${formatBytes(host.mem_total)}` : '—'}
          />
          {host.disk_total ? (
            <Meter
              size="lg"
              value={pct(host.disk_used, host.disk_total)}
              label={t('resources.disk')}
              detail={host.disk_used !== undefined ? `${formatBytes(host.disk_used)} / ${formatBytes(host.disk_total)}` : '—'}
            />
          ) : null}
        </div>
        {open && <ResourceHistory integration={integration} r={host} />}
      </CardContent>
    </Card>
  )
}

function GuestTable({ integration, list }: { integration: Integration; list: Resource[] }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState<string>()
  if (list.length === 0) return null
  return (
    <Card className="gap-0 py-0">
      <div className="hidden md:block">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="pl-6">{t('resources.col.name')}</TableHead>
              <TableHead>{t('resources.col.state')}</TableHead>
              <TableHead>{t('resources.cpu')}</TableHead>
              <TableHead>{t('resources.mem')}</TableHead>
              <TableHead className="hidden lg:table-cell">{t('resources.disk')}</TableHead>
              <TableHead className="hidden xl:table-cell">{t('resources.col.net')}</TableHead>
              <TableHead className="pr-6">{t('resources.col.uptime')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {list.map((r, n) => (
              <Fragment key={r.id}>
                <LiveTableRow
                  watch={r.state}
                  index={n}
                  className={cn('cursor-pointer', !running(r) && 'opacity-60')}
                  onClick={() => setOpen((o) => (o === r.id ? undefined : r.id))}
                  data-resource={r.name}
                  aria-expanded={open === r.id}
                >
                  <TableCell className="max-w-[18rem] min-w-40 pl-6 whitespace-normal">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className="font-medium [overflow-wrap:anywhere]">{r.name}</span>
                      <KindBadge r={r} />
                    </div>
                    {(r.tags?.length || r.info?.image) && (
                      <div className="text-muted-foreground mt-0.5 truncate text-xs">
                        {r.info?.image ?? r.tags?.join(' · ')}
                      </div>
                    )}
                  </TableCell>
                  <TableCell>
                    <StateBadge r={r} />
                  </TableCell>
                  <TableCell>
                    <CpuCell r={r} />
                  </TableCell>
                  <TableCell>
                    <MemCell used={r.mem_used} total={r.mem_total} />
                  </TableCell>
                  <TableCell className="hidden lg:table-cell">
                    <MemCell used={r.disk_used} total={r.disk_total} />
                  </TableCell>
                  <TableCell className="hidden xl:table-cell">
                    <Net r={r} />
                  </TableCell>
                  <TableCell className="pr-6 text-sm whitespace-nowrap">
                    {r.uptime ? formatUptime(r.uptime) : (r.info?.uptime ?? '—')}
                  </TableCell>
                </LiveTableRow>
                {open === r.id && (
                  <TableRow className="hover:bg-transparent">
                    <TableCell colSpan={7} className="px-6 pb-4 whitespace-normal">
                      <ResourceHistory integration={integration} r={r} />
                    </TableCell>
                  </TableRow>
                )}
              </Fragment>
            ))}
          </TableBody>
        </Table>
      </div>
      <ul className="divide-y md:hidden">
        {list.map((r, n) => (
          <LiveItem watch={r.state} index={n} key={r.id} className={cn('flex flex-col gap-2 p-4', !running(r) && 'opacity-60')} data-resource={r.name}>
            <div className="flex items-start justify-between gap-2">
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="font-medium">{r.name}</span>
                <KindBadge r={r} />
              </div>
              <StateBadge r={r} />
            </div>
            {r.cpu !== undefined && (
              <div className="grid grid-cols-2 gap-3">
                <CpuCell r={r} />
                <MemCell used={r.mem_used} total={r.mem_total} />
              </div>
            )}
          </LiveItem>
        ))}
      </ul>
    </Card>
  )
}

function Empty() {
  const { t } = useTranslation()
  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
        <span className="bg-muted grid size-12 place-items-center rounded-lg">
          <Cpu className="text-muted-foreground size-6" />
        </span>
        <p className="font-medium">{t('resources.emptyTitle')}</p>
        <p className="text-muted-foreground max-w-md text-sm">{t('resources.empty')}</p>
        <Link to="/einstellungen" className="text-primary flex items-center gap-1 text-sm font-medium hover:underline">
          <Plug className="size-4" /> {t('checks.toSettings')}
        </Link>
      </CardContent>
    </Card>
  )
}

export function Resources() {
  const { t } = useTranslation()
  const integrations = useIntegrations()
  const types = useIntegrationTypes()
  const [query, setQuery] = useState('')
  const [state, setState] = useState<StateFilter>('all')
  const [sort, setSort] = useState<SortKey>('name')

  const sources = useMemo(
    () => (integrations.data ?? []).filter((i) => i.enabled && (i.snapshot?.resources?.length ?? 0) > 0),
    [integrations.data],
  )

  function children(list: Resource[], host: Resource) {
    const q = query.trim().toLowerCase()
    return list
      .filter((r) => r.kind !== 'node' && r.parent === host.name)
      .filter((r) => state === 'all' || (state === 'running') === running(r))
      .filter((r) => !q || [r.name, r.info?.vmid, r.info?.image, ...(r.tags ?? [])].some((x) => x?.toLowerCase().includes(q)))
      .sort((a, b) =>
        sort === 'cpu'
          ? (b.cpu ?? -1) - (a.cpu ?? -1)
          : sort === 'mem'
            ? (b.mem_used ?? -1) - (a.mem_used ?? -1)
            : a.name.localeCompare(b.name, undefined, { numeric: true }),
      )
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('nav.resources')} subtitle={t('resources.subtitle')} />
      {integrations.isPending ? (
        <Skeleton className="h-64 rounded-xl" />
      ) : sources.length === 0 ? (
        <Empty />
      ) : (
        <>
          <div className="flex flex-wrap items-center gap-3">
            <div className="relative w-full sm:w-64">
              <Search className="text-muted-foreground absolute top-2.5 left-2.5 size-4" aria-hidden />
              <Input
                className="pl-8"
                placeholder={t('resources.search')}
                aria-label={t('resources.search')}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </div>
            <Segmented
              label={t('resources.filter')}
              value={state}
              onChange={setState}
              options={(['all', 'running', 'stopped'] as const).map((v) => ({ value: v, label: t(`resources.filters.${v}`) }))}
            />
            <Segmented
              label={t('resources.sort')}
              value={sort}
              onChange={setSort}
              options={(['name', 'cpu', 'mem'] as const).map((v) => ({ value: v, label: t(`resources.sorts.${v}`) }))}
            />
          </div>
          {sources.map((i) => {
            const list = i.snapshot!.resources!
            const icon = types.data?.types.find((x) => x.type === i.type)?.icon
            return list
              .filter((r) => r.kind === 'node')
              .map((host) => {
                const kids = children(list, host)
                return (
                  <section key={`${i.id}/${host.id}`} className="flex flex-col gap-3">
                    <HostCard integration={i} host={host} icon={icon} guests={list.filter((r) => r.parent === host.name)} />
                    <GuestTable integration={i} list={kids} />
                  </section>
                )
              })
          })}
          <p className="text-muted-foreground flex items-center gap-2 text-sm">
            <Server className="size-4 shrink-0" aria-hidden />
            <Container className="size-4 shrink-0" aria-hidden />
            {t('resources.hint')}
          </p>
        </>
      )}
    </div>
  )
}
