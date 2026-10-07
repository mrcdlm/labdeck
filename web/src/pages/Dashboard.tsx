import {
  AlertTriangle,
  ChevronRight,
  DatabaseBackup,
  GanttChart,
  History as HistoryIcon,
  PauseCircle,
  Plug,
  Server,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { AnimatedNumber } from '@/components/AnimatedNumber'
import { HistoryList } from '@/components/HistoryList'
import { IntegrationIcon } from '@/components/IntegrationIcon'
import { Meter } from '@/components/Meter'
import { PageHeader } from '@/components/PageHeader'
import { Segmented } from '@/components/Segmented'
import { Sparkline } from '@/components/Sparkline'
import { StatusBadge } from '@/components/StatusBadge'
import { ProblemLines, StatusDot, StatusTile, TileValue, type ProblemLine } from '@/components/StatusTile'
import { UptimeBar } from '@/components/UptimeBar'
import { Skeleton } from '@/components/ui/skeleton'
import type { DashboardTile, Integration, Metric, Overview, Problem, UptimeRange } from '@/lib/api'
import { absoluteTime, formatBytes, relativeTime, useNow } from '@/lib/format'
import { formatMetric, isStateUnit, metricLabel } from '@/lib/metrics'
import { rise } from '@/lib/motion'
import { problemOwner, statusRank, worstStatus } from '@/lib/problems'
import {
  useBackups,
  useDashboardLayout,
  useHistory,
  useIntegrations,
  useIntegrationTypes,
  useOverview,
} from '@/lib/queries'
import { cn } from '@/lib/utils'

/** Hinweis, solange keine Integration eingerichtet ist. */
function NoIntegrations() {
  const { t } = useTranslation()
  return (
    <Link
      to="/einstellungen"
      className="group bg-card hover:bg-accent/40 flex items-center gap-4 rounded-xl border p-5 transition-colors"
    >
      <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg">
        <Plug className="size-5" />
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{t('dashboard.noIntegrations')}</span>
          <StatusBadge status="unknown" />
        </div>
        <p className="text-muted-foreground mt-1 text-sm">{t('dashboard.noIntegrationsHint')}</p>
      </div>
      <ChevronRight className="text-muted-foreground size-4 transition-transform group-hover:translate-x-0.5" />
    </Link>
  )
}

const backupStates = ['failed', 'overdue', 'due_soon'] as const

/**
 * Problem als kurze Zeile für die Kachel: was, wo (grau) und Zustand bzw.
 * Wert rechts; Quelle und Zeitpunkt im Tooltip. Checks-Probleme werden zu
 * einer Zeile je ausgefallenem Check.
 */
function useProblemLines(now: number) {
  const { t } = useTranslation()
  return (problems: Problem[], withSource = false): ProblemLine[] =>
    problems.flatMap((p): ProblemLine[] => {
      const key = `${p.kind}/${p.integration_id}/${p.item_id ?? ''}`
      const at = p.at ? t(`dashboard.problem.at.${p.kind}`, { time: relativeTime(p.at, now) }) : ''
      const title = [p.integration_name, at, p.at && absoluteTime(p.at)].filter(Boolean).join(' · ')
      const source = withSource ? `${p.integration_name}: ` : ''
      if (p.kind === 'checks') {
        return (p.items ?? []).map((name) => ({
          key: `${key}/${name}`,
          status: p.status,
          id: name,
          name: source + name,
          state: t('dashboard.down'),
          title,
        }))
      }
      if (p.kind === 'integration') {
        return [
          {
            key,
            status: p.status,
            id: p.name,
            name: source + t('dashboard.problem.integration'),
            detail: p.detail,
            wrap: true,
            title,
          },
        ]
      }
      if (p.kind === 'metric') {
        const name = metricLabel(p.integration_type, {
          key: p.item_id ?? '',
          name: p.name,
        })
        return [
          {
            key,
            status: p.status,
            id: p.name,
            name: <span className="inline-block first-letter:uppercase">{source + name}</span>,
            detail: p.group,
            state: formatMetric(p.value ?? 0, p.unit),
            title,
          },
        ]
      }
      const b = backupStates.find((s) => s === p.state)
      return [
        {
          key,
          status: p.status,
          id: p.name,
          name: source + p.name,
          detail: p.group,
          state: b ? t(`backups.state.${b}`) : t(`status.${p.status}`),
          title,
        },
      ]
    })
}

/**
 * Ampel des Gesamtstatus: bei Fehlern pulsiert ein Ring um das Symbol, bei
 * „Alles in Ordnung“ zeichnet sich ein Häkchen.
 */
function StatusMark({ tone }: { tone: 'idle' | 'ok' | 'warning' | 'error' }) {
  const box = {
    idle: 'bg-muted text-muted-foreground',
    ok: 'bg-success/15 text-success',
    warning: 'bg-warning/15 text-warning',
    error: 'bg-destructive/15 text-destructive',
  }[tone]
  return (
    <span className={cn('relative grid size-10 shrink-0 place-items-center rounded-lg', box)} data-tone={tone}>
      {(tone === 'error' || tone === 'warning') && (
        <span
          className={cn(
            'animate-ring absolute inset-0 rounded-lg',
            tone === 'error' ? 'bg-destructive/30' : 'bg-warning/30',
          )}
          aria-hidden
        />
      )}
      {tone === 'idle' && <PauseCircle className="size-5" />}
      {tone === 'ok' && (
        <svg viewBox="0 0 24 24" className="size-6" fill="none" stroke="currentColor" strokeWidth={2.5} aria-hidden>
          <path d="M5 12.5l4.5 4.5L19 7.5" className="animate-draw" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      )}
      {(tone === 'error' || tone === 'warning') && <AlertTriangle className="relative size-5" />}
    </span>
  )
}

/**
 * Gesamtstatus als schlanke Zeile: Ampel, „11 Probleme in 4 Bereichen“,
 * Zähler für Fehler und Warnungen. Die Probleme selbst stehen in den Kacheln;
 * nur solche ohne sichtbare Kachel (orphans) folgen hier als flache Liste.
 */
function OverallStatus({ overview, orphans }: { overview: Overview; orphans: Problem[] }) {
  const { t } = useTranslation()
  const now = useNow()
  const toLines = useProblemLines(now)
  const problems = overview.problems
  const errors = problems.filter((p) => p.status === 'error').length
  const warnings = problems.filter((p) => p.status === 'warning').length
  const areas = new Set(problems.map(problemOwner)).size
  const ok = problems.length === 0
  const idle = overview.integrations.active === 0 // alle deaktiviert
  const tone = idle ? 'idle' : ok ? 'ok' : errors > 0 ? 'error' : 'warning'

  return (
    <section className="flex flex-col gap-3" aria-label={t('dashboard.overall')}>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-3">
        <StatusMark tone={tone} />
        <div className="min-w-48 flex-1">
          <p className="text-lg font-semibold">
            {idle
              ? t('dashboard.noneActive')
              : ok
                ? t('dashboard.allGood')
                : `${t('dashboard.problems', { count: problems.length })}${areas > 1 ? ` ${t('dashboard.inAreas', { count: areas })}` : ''}`}
          </p>
          <p className="text-muted-foreground text-sm">
            {t('dashboard.activeIntegrations', {
              count: overview.integrations.active,
            })}
          </p>
        </div>
        {!ok && (
          // mobil in eigener Zeile unter dem Text
          <div className="flex w-full flex-wrap gap-2 pl-14 sm:w-auto sm:pl-0">
            {errors > 0 && (
              <span className="bg-destructive/10 text-destructive inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-sm font-medium tabular-nums">
                <StatusDot status="error" />
                {t('dashboard.errorsCount', { count: errors })}
              </span>
            )}
            {warnings > 0 && (
              <span className="bg-warning/15 text-warning inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-sm font-medium tabular-nums">
                <StatusDot status="warning" />
                {t('dashboard.warningsCount', { count: warnings })}
              </span>
            )}
          </div>
        )}
      </div>
      {orphans.length > 0 && (
        <div className="pl-14">
          <ProblemLines lines={toLines(orphans, true)} max={5} label={t('dashboard.problemList')} />
        </div>
      )}
    </section>
  )
}

function BackupsTile({ overview, problems }: { overview: Overview; problems: Problem[] }) {
  const { t } = useTranslation()
  const toLines = useProblemLines(useNow())
  const s = overview.backups
  const n = (k: keyof typeof s.states) => s.states[k] ?? 0
  const sub = [
    n('ok') > 0 && t('dashboard.backupsOk', { count: n('ok') }),
    n('due_soon') > 0 && t('dashboard.backupsDueSoon', { count: n('due_soon') }),
    n('overdue') + n('failed') > 0 && t('dashboard.problems', { count: n('overdue') + n('failed') }),
    n('unknown') + n('paused') > 0 && t('dashboard.other', { count: n('unknown') + n('paused') }),
  ].filter(Boolean)
  return (
    <StatusTile
      to="/backups"
      icon={<DatabaseBackup className="size-4" />}
      title={t('nav.backups')}
      tile="backups"
      status={worstStatus(problems.map((p) => p.status)) ?? (s.total > 0 ? 'ok' : 'unknown')}
      value={
        s.total === 0 ? (
          <span className="text-muted-foreground font-medium">{t('dashboard.noBackups')}</span>
        ) : (
          <TileValue label={t('dashboard.monitoredLabel', { count: s.total })}>
            <AnimatedNumber value={s.total} format={(v) => String(Math.round(v))} />
          </TileValue>
        )
      }
      sub={sub.join(' · ')}
    >
      <ProblemLines lines={toLines(problems)} />
    </StatusTile>
  )
}

/** Healthchecks.io als Ampel: grün, wenn alle Checks laufen, rot mit den ausgefallenen. */
function ChecksTile({ overview, problems }: { overview: Overview; problems: Problem[] }) {
  const { t } = useTranslation()
  const toLines = useProblemLines(useNow())
  const c = overview.checks
  const down = c.statuses.error ?? 0
  const active = c.total - (c.statuses.paused ?? 0)
  return (
    <StatusTile
      to="/checks"
      icon={<IntegrationIcon type="healthchecks" className="size-4" />}
      title={t('nav.checks')}
      tile="checks"
      status={
        worstStatus([...problems.map((p) => p.status), down > 0 ? 'error' : undefined]) ?? (c.total > 0 ? 'ok' : 'unknown')
      }
      value={
        c.total === 0 ? (
          <span className="text-muted-foreground font-medium">{t('dashboard.noChecks')}</span>
        ) : (
          <TileValue label={t('dashboard.checksLabel', { count: active })}>
            <AnimatedNumber value={active} format={(v) => String(Math.round(v))} />
          </TileValue>
        )
      }
      sub={
        c.total > 0 && (
          <span className={cn('font-medium', down > 0 ? 'text-destructive' : 'text-success')}>
            {down > 0 ? t('dashboard.checksFailed', { count: down }) : t('dashboard.checksAllOk', { count: active })}
          </span>
        )
      }
    >
      <ProblemLines lines={toLines(problems)} />
    </StatusTile>
  )
}

/** Kennzahl für den Mini-Verlauf: zuerst eine Prozent-Kennzahl (24 h), sonst die erste Zahl (30 Tage). */
function sparkMetric(main: Metric[]): { key: string; range: '24h' | '30d' } | undefined {
  const pct = main.find((m) => m.unit === '%')
  if (pct) return { key: pct.key, range: '24h' }
  const first = main.find((m) => !isStateUnit(m.unit))
  return first && { key: first.key, range: '30d' }
}

function metricText(type: string, m: Metric) {
  return isStateUnit(m.unit)
    ? formatMetric(m.value, m.unit)
    : `${formatMetric(m.value, m.unit)} ${metricLabel(type, m)}`
}

/** Kachel je Dienst: erste Kennzahl groß, zwei weitere als Nebenzeile, Probleme, Mini-Verlauf. */
function ServiceTile({
  integration: i,
  icon,
  problems,
}: {
  integration: Integration
  icon?: string
  problems: Problem[]
}) {
  const { t } = useTranslation()
  const toLines = useProblemLines(useNow())
  const main = (i.snapshot?.metrics ?? []).filter((m) => !m.group && !m.resource)
  const [first, ...rest] = main
  const spark = sparkMetric(main)
  return (
    <StatusTile
      to={`/dienste#integration-${i.id}`}
      icon={<IntegrationIcon name={icon} type={i.type} className="size-4" />}
      title={i.name}
      tile={i.name}
      status={
        worstStatus([...problems.map((p) => p.status), i.status === 'error' ? 'error' : undefined]) ??
        (first ? 'ok' : 'unknown')
      }
      value={
        !first ? (
          <span className="text-muted-foreground font-medium">{t('services.noMetrics')}</span>
        ) : isStateUnit(first.unit) ? (
          <TileValue>{formatMetric(first.value, first.unit)}</TileValue>
        ) : (
          <TileValue label={metricLabel(i.type, first)}>
            <AnimatedNumber value={first.value} format={(v) => formatMetric(v, first.unit)} />
          </TileValue>
        )
      }
      sub={rest
        .slice(0, 2)
        .map((m) => metricText(i.type, m))
        .join(' · ')}
    >
      <ProblemLines lines={toLines(problems)} />
      {spark && <Sparkline integrationId={i.id} metricKey={spark.key} range={spark.range} />}
    </StatusTile>
  )
}

const pct = (used?: number, total?: number) => (used !== undefined && total ? (used / total) * 100 : undefined)

/** Alle Hosts/Nodes (Proxmox VE, Dockhand) mit kleinen CPU- und RAM-Balken. */
function HostsTile({
  integrations,
  icons,
}: {
  integrations: Integration[]
  icons: Record<string, string | undefined>
}) {
  const { t } = useTranslation()
  const hosts = integrations.flatMap((i) =>
    (i.snapshot?.resources ?? []).filter((r) => r.kind === 'node').map((r) => ({ i, r })),
  )
  if (hosts.length === 0) return null
  hosts.sort((a, b) => statusRank(a.r.status) - statusRank(b.r.status) || a.r.name.localeCompare(b.r.name))
  const online = hosts.filter((h) => h.r.status !== 'error').length
  return (
    <StatusTile
      to="/ressourcen"
      icon={<Server className="size-4" />}
      title={t('dashboard.tile.hosts')}
      tile="hosts"
      status={worstStatus(hosts.map((h) => h.r.status)) ?? 'unknown'}
      sub={t('dashboard.hostsOnline', { count: online, total: hosts.length })}
    >
      <ul className="mt-2 flex flex-col gap-2.5">
        {hosts.map(({ i, r }) => (
          <li
            key={`${i.id}/${r.id}`}
            className="grid grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)_minmax(0,1fr)] items-center gap-x-4"
            data-host={r.name}
          >
            <span className="flex min-w-0 items-center gap-2">
              <IntegrationIcon name={icons[i.type]} type={i.type} className="size-4 shrink-0" />
              <span className="truncate text-sm font-medium">{r.name}</span>
              <StatusDot status={r.status} pulse />
            </span>
            <Meter
              size="sm"
              value={r.cpu}
              label={t('resources.cpu')}
              detail={r.cpu !== undefined ? formatMetric(r.cpu, '%') : '—'}
            />
            <Meter
              size="sm"
              value={pct(r.mem_used, r.mem_total)}
              label={t('resources.mem')}
              detail={
                r.mem_used !== undefined && r.mem_total
                  ? `${formatBytes(r.mem_used)} / ${formatBytes(r.mem_total)}`
                  : '—'
              }
            />
          </li>
        ))}
      </ul>
    </StatusTile>
  )
}

const timelineRanges: UptimeRange[] = ['24h', '7d']

/** Zeitleiste: je Backup ein Verlaufsbalken, Probleme zuerst. */
function BackupTimelineTile() {
  const { t } = useTranslation()
  const backups = useBackups()
  const [range, setRange] = useState<UptimeRange>('24h')
  const list = [...(backups.data?.backups ?? [])].sort(
    (a, b) => statusRank(a.status) - statusRank(b.status) || a.name.localeCompare(b.name),
  )
  const max = 8
  return (
    <StatusTile
      to="/backups"
      icon={<GanttChart className="size-4" />}
      title={t('dashboard.tile.backupTimeline')}
      tile="backup-timeline"
      status={worstStatus(list.map((b) => b.status)) ?? 'unknown'}
      stretch={false}
      action={
        <Segmented
          label={t('uptime.period')}
          value={range}
          onChange={setRange}
          options={timelineRanges.map((r) => ({
            value: r,
            label: t(`uptime.range.${r}`),
          }))}
        />
      }
    >
      {backups.isPending ? (
        <Skeleton className="mt-2 h-24" />
      ) : list.length === 0 ? (
        <p className="text-muted-foreground mt-2 text-sm">{t('dashboard.noBackups')}</p>
      ) : (
        <ul className="mt-3 flex flex-col gap-2">
          {list.slice(0, max).map((b) => (
            <li
              key={`${b.integration_id}/${b.id}`}
              className="grid grid-cols-[minmax(0,12rem)_minmax(0,1fr)] items-center gap-x-4"
              data-backup={b.name}
            >
              <span className="flex min-w-0 items-center gap-2 text-sm">
                <StatusDot status={b.status} />
                <span className="truncate font-medium">{b.name}</span>
                {b.group && <span className="text-muted-foreground truncate text-xs">{b.group}</span>}
              </span>
              {b.uptime ? (
                <UptimeBar uptime={b.uptime[range]} range={range} compact />
              ) : (
                <span className="text-muted-foreground text-sm">—</span>
              )}
            </li>
          ))}
          {list.length > max && (
            <li>
              <Link to="/backups" className="text-muted-foreground hover:text-foreground text-xs">
                {t('dashboard.moreBackups', { count: list.length - max })}
              </Link>
            </li>
          )}
        </ul>
      )}
    </StatusTile>
  )
}

/** Letzte Statuswechsel aller Integrationen: Überschrift außen, Liste flach. */
function EventsTile() {
  const { t } = useTranslation()
  const history = useHistory({}, 8)
  const entries = history.data?.pages[0]?.entries ?? []
  return (
    <section className="flex flex-col gap-2" data-tile="events">
      <div className="flex items-center justify-between gap-2 px-1">
        <h2 className="flex items-center gap-2 font-medium">
          <HistoryIcon className="size-4" />
          {t('dashboard.events')}
        </h2>
        <Link to="/verlauf" className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-sm">
          {t('dashboard.allEvents')}
          <ChevronRight className="size-4" />
        </Link>
      </div>
      <div className="bg-card rounded-xl border px-5">
        {history.isPending ? (
          <Skeleton className="my-4 h-24" />
        ) : entries.length === 0 ? (
          <p className="text-muted-foreground py-4 text-sm">{t('history.empty')}</p>
        ) : (
          <HistoryList entries={entries} compact />
        )}
      </div>
    </section>
  )
}

const span: Record<number, string> = {
  1: '',
  2: 'sm:col-span-2',
  3: 'sm:col-span-2 lg:col-span-3',
}
const defaultTiles: DashboardTile[] = [
  { id: 'backups', visible: true, size: 1 },
  { id: 'checks', visible: true, size: 1 },
  { id: 'events', visible: true, size: 3 },
]

/**
 * Gesamtstatus und Kacheln in der Anordnung aus Einstellungen → Dashboard;
 * Kacheln mit Problemen auf Wunsch zuerst (Fehler vor Warnungen).
 */
function Board({ overview }: { overview: Overview }) {
  const layout = useDashboardLayout()
  const integrations = useIntegrations()
  const types = useIntegrationTypes()
  const tiles = (layout.data?.tiles ?? defaultTiles).filter((x) => x.visible)
  const problemsFirst = layout.data?.problems_first ?? true
  const list = integrations.data ?? []
  const icons = Object.fromEntries((types.data?.types ?? []).map((x) => [x.type, x.icon]))

  const byOwner = new Map<string, Problem[]>()
  for (const p of overview.problems) {
    const o = problemOwner(p)
    byOwner.set(o, [...(byOwner.get(o) ?? []), p])
  }
  const mine = (id: string) => byOwner.get(id) ?? []

  function render(id: string) {
    switch (id) {
      case 'backups':
        return <BackupsTile overview={overview} problems={mine(id)} />
      case 'checks':
        return <ChecksTile overview={overview} problems={mine(id)} />
      case 'hosts':
        return <HostsTile integrations={list.filter((i) => i.enabled)} icons={icons} />
      case 'backup-timeline':
        return <BackupTimelineTile />
      case 'events':
        return <EventsTile />
    }
    const i = list.find((x) => `integration:${x.id}` === id)
    if (!i || !i.enabled) return null
    return <ServiceTile integration={i} icon={icons[i.type]} problems={mine(id)} />
  }

  const shown = tiles.map((x) => ({ ...x, content: render(x.id) })).filter((x) => x.content)
  // Probleme ohne sichtbare Kachel stehen direkt unter dem Gesamtstatus
  const visible = new Set(shown.map((x) => x.id))
  const orphans = overview.problems.filter((p) => !visible.has(problemOwner(p)))
  if (problemsFirst) {
    const rank = (id: string) => statusRank(worstStatus(mine(id).map((p) => p.status)) ?? 'ok')
    shown.sort((a, b) => Math.min(rank(a.id), 2) - Math.min(rank(b.id), 2))
  }

  return (
    <>
      <OverallStatus overview={overview} orphans={orphans} />
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {shown.map((x, n) => {
          const motion = rise(n)
          return (
            <div key={x.id} className={cn('min-w-0 [&>*]:h-full', span[x.size], motion.className)} style={motion.style}>
              {x.content}
            </div>
          )
        })}
      </div>
    </>
  )
}

export function Dashboard() {
  const { t } = useTranslation()
  const overview = useOverview()
  const o = overview.data

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('nav.dashboard')} subtitle={t('dashboard.subtitle')} />
      {overview.isPending ? (
        <>
          <Skeleton className="h-12 rounded-xl" />
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Skeleton className="h-32 rounded-xl" />
            <Skeleton className="h-32 rounded-xl" />
            <Skeleton className="h-32 rounded-xl" />
          </div>
        </>
      ) : overview.isError ? (
        <p className="text-destructive text-sm">{t('dashboard.loadError', { error: overview.error.message })}</p>
      ) : o!.integrations.total === 0 ? (
        <NoIntegrations />
      ) : (
        <Board overview={o!} />
      )}
    </div>
  )
}
