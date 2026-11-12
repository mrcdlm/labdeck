import { ArrowLeft, CloudOff, HeartPulse, Pencil, RefreshCw, Search } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router'

import { IntegrationIcon } from '@/components/IntegrationIcon'
import { MetricChart, MetricRangeToggle } from '@/components/MetricChart'
import { ProblemRows } from '@/components/ProblemList'
import { Segmented } from '@/components/Segmented'
import { ServerCard } from '@/components/ServerView'
import { StatusBadge } from '@/components/StatusBadge'
import { SwitchCard } from '@/components/SwitchView'
import { GroupTables, Stat } from '@/components/systems/MetricParts'
import { GuestTable, HostCard } from '@/components/systems/ResourceParts'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import type { Integration, IntegrationType, MetricRange, Resource } from '@/lib/api'
import { useHashTarget } from '@/lib/anchor'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { isStateUnit, metricLabel } from '@/lib/metrics'
import { running } from '@/lib/resources'
import { useIntegrations, useIntegrationTypes, useOverview, usePollIntegration } from '@/lib/queries'
import { cn } from '@/lib/utils'

type StateFilter = 'all' | 'running' | 'stopped'
type SortKey = 'name' | 'cpu' | 'mem'

function Section({ title, children, action }: { title: string; children: ReactNode; action?: ReactNode }) {
  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-medium">{title}</h2>
        {action}
      </div>
      {children}
    </section>
  )
}

/** Kopf: Symbol, Name, Status, Typ · Version · letzte Abfrage, Aktionen. */
function Header({ integration: i, type }: { integration: Integration; type?: IntegrationType }) {
  const { t } = useTranslation()
  const now = useNow()
  const poll = usePollIntegration()
  const version = i.snapshot?.info?.version
  return (
    <div className="flex flex-col gap-3">
      <Link to="/systeme" className="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1 text-sm">
        <ArrowLeft className="size-4" /> {t('nav.systems')}
      </Link>
      <div className="flex flex-wrap items-center gap-3">
        <span className="bg-muted grid size-11 shrink-0 place-items-center rounded-lg">
          <IntegrationIcon name={type?.icon} type={i.type} className="size-5" />
        </span>
        <div className="min-w-0 flex-1 basis-60">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">{i.name}</h1>
            <StatusBadge status={i.status} label={t(`integrations.status.${i.status}`)} />
          </div>
          <p className="text-muted-foreground text-sm">
            {type?.title ?? i.type}
            {version && ` · ${version}`}
            {i.last_poll_at && (
              <span title={absoluteTime(i.last_poll_at)}>
                {' · '}
                {t('integrations.lastPoll', { time: relativeTime(i.last_poll_at, now) })}
              </span>
            )}
          </p>
        </div>
        <div className="flex gap-2">
          {i.enabled && (
            <Button variant="outline" size="sm" onClick={() => poll.mutate(i.id)} disabled={poll.isPending}>
              <RefreshCw className={cn('size-4', poll.isPending && 'animate-spin')} />
              {t('systems.pollNow')}
            </Button>
          )}
          <Button variant="outline" size="sm" asChild>
            <Link to="/einstellungen">
              <Pencil className="size-4" />
              {t('systems.edit')}
            </Link>
          </Button>
        </div>
      </div>
      {i.status === 'error' && i.last_error && (
        <p className="text-destructive flex items-start gap-2 text-sm">
          <CloudOff className="mt-0.5 size-4 shrink-0" aria-hidden />
          <span>
            {i.last_error}
            {i.last_success_at && (i.snapshot?.metrics?.length ?? 0) > 0 && (
              <span className="text-muted-foreground">
                {' – '}
                {t('services.staleSince', { time: relativeTime(i.last_success_at, now) })}
              </span>
            )}
          </span>
        </p>
      )}
    </div>
  )
}

/** Hauptkennzahlen als Kacheln, darunter der Verlauf der gewählten. */
function Metrics({ integration: i }: { integration: Integration }) {
  const { t } = useTranslation()
  const main = (i.snapshot?.metrics ?? []).filter((m) => !m.resource && !m.group)
  const [selected, setSelected] = useState<string>()
  const [range, setRange] = useState<MetricRange>('30d')
  // Zustandswerte (Netz/Batterie, OK/Fehler) haben kein Diagramm
  const current = main.find((m) => m.key === selected) ?? main.find((m) => !isStateUnit(m.unit))
  const chartable = current && !isStateUnit(current.unit)
  if (main.length === 0) return null
  return (
    <Section title={t('systems.metrics')}>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        {main.map((m) => (
          <Stat
            key={m.key}
            m={m}
            label={metricLabel(i.type, m)}
            selected={m.key === current?.key}
            onSelect={() => setSelected(m.key)}
          />
        ))}
      </div>
      {chartable && current && (
        <Card className="gap-2 py-4">
          <CardContent className="flex flex-col gap-2">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="text-sm font-medium">
                {t('services.chartTitle', { metric: metricLabel(i.type, current) })}
              </span>
              <MetricRangeToggle value={range} onChange={setRange} />
            </div>
            <MetricChart integrationId={i.id} metricKey={current.key} label={metricLabel(i.type, current)} range={range} />
          </CardContent>
        </Card>
      )}
    </Section>
  )
}

/** Hosts mit ihren Gästen (Proxmox VE, Dockhand), filter- und sortierbar. */
function Hosts({ integration: i, icon }: { integration: Integration; icon?: string }) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [state, setState] = useState<StateFilter>('all')
  const [sort, setSort] = useState<SortKey>('name')
  const list = i.snapshot?.resources ?? []
  const hosts = list.filter((r) => r.kind === 'node')
  if (hosts.length === 0) return null
  const guests = list.filter((r) => r.kind !== 'node' && hosts.some((h) => h.name === r.parent))

  function children(host: Resource) {
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
    <Section title={t('systems.hosts')}>
      {guests.length > 0 && (
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
      )}
      {hosts.map((host) => (
        <div key={host.id} className="flex flex-col gap-3">
          <HostCard integration={i} host={host} icon={icon} guests={list.filter((r) => r.parent === host.name)} />
          <GuestTable integration={i} list={children(host)} />
        </div>
      ))}
      {guests.length > 0 && <p className="text-muted-foreground text-sm">{t('resources.hint')}</p>}
    </Section>
  )
}

/** Alles zu einer Integration: Probleme, Kennzahlen, Hosts, Hardware, Netzwerk, Tabellen. */
export function SystemDetail() {
  const { t } = useTranslation()
  const { id } = useParams()
  const integrations = useIntegrations()
  const types = useIntegrationTypes()
  const overview = useOverview()
  const i = integrations.data?.find((x) => String(x.id) === id)
  useHashTarget(integrations.isSuccess && !!i)

  if (integrations.isPending) {
    return (
      <div className="flex flex-col gap-6">
        <Skeleton className="h-16 rounded-xl" />
        <Skeleton className="h-64 rounded-xl" />
      </div>
    )
  }
  if (!i) {
    return (
      <div className="flex flex-col items-start gap-3">
        <p className="text-muted-foreground">{t('systems.notFound')}</p>
        <Link to="/systeme" className="text-primary flex items-center gap-1 text-sm font-medium hover:underline">
          <ArrowLeft className="size-4" /> {t('nav.systems')}
        </Link>
      </div>
    )
  }

  const type = types.data?.types.find((x) => x.type === i.type)
  const resources = i.snapshot?.resources ?? []
  const servers = resources.filter((r) => r.kind === 'server')
  const switches = resources.filter((r) => r.kind === 'switch')
  const grouped = (i.snapshot?.metrics ?? []).filter((m) => !m.resource && m.group)
  const problems = (overview.data?.problems ?? []).filter((p) => p.integration_id === i.id)
  const empty = (i.snapshot?.metrics?.length ?? 0) === 0 && resources.length === 0

  return (
    <div className={cn('flex flex-col gap-8', !i.enabled && 'opacity-80')} data-service={i.name}>
      <Header integration={i} type={type} />
      {problems.length > 0 && (
        <Section title={t('dashboard.problems', { count: problems.length })}>
          <ProblemRows problems={problems} withSource={false} max={8} />
        </Section>
      )}
      {i.type === 'healthchecks' && (
        <Link to="/checks" className="text-primary flex w-fit items-center gap-1 text-sm font-medium hover:underline">
          <HeartPulse className="size-4" /> {t('systems.toChecks')}
        </Link>
      )}
      {empty ? (
        <p className="text-muted-foreground text-sm">{t('services.noMetrics')}</p>
      ) : (
        <>
          <Metrics integration={i} />
          <Hosts integration={i} icon={type?.icon} />
          {servers.length > 0 && (
            <Section title={t('resources.hardware')}>
              {servers.map((s) => (
                <ServerCard key={s.id} integration={i} server={s} />
              ))}
            </Section>
          )}
          {switches.length > 0 && (
            <Section title={t('resources.network')}>
              {switches.map((sw) => (
                <SwitchCard key={sw.id} integration={i} sw={sw} />
              ))}
            </Section>
          )}
          {grouped.length > 0 && (
            <Section title={t('systems.details')}>
              <GroupTables type={i.type} metrics={grouped} />
            </Section>
          )}
        </>
      )}
    </div>
  )
}
