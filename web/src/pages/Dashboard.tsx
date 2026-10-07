import { AlertTriangle, ChevronRight, DatabaseBackup, History as HistoryIcon, PauseCircle, Plug } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { AnimatedNumber } from '@/components/AnimatedNumber'
import { HistoryList } from '@/components/HistoryList'
import { IntegrationIcon } from '@/components/IntegrationIcon'
import { PageHeader } from '@/components/PageHeader'
import { StatusBadge } from '@/components/StatusBadge'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { DashboardTile, Integration, Overview, Problem } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { formatMetric, isStateUnit, metricLabel } from '@/lib/metrics'
import { rise, useFlash } from '@/lib/motion'
import { useDashboardLayout, useHistory, useIntegrations, useIntegrationTypes, useOverview } from '@/lib/queries'
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

function problemLink(p: Problem): string {
  switch (p.kind) {
    case 'integration':
      return '/einstellungen'
    case 'backup':
      return '/backups'
    case 'checks':
      return '/checks'
  }
  return `/dienste#integration-${p.integration_id}`
}

const backupStates = ['failed', 'overdue', 'due_soon'] as const

/** Eine Zeile der Problemliste: was, wo, seit wann. */
function ProblemRow({ p, now, index }: { p: Problem; now: number; index: number }) {
  const { t } = useTranslation()
  const flash = useFlash([p.status, p.state, p.count, p.value])
  const motion = rise(index)
  let label: string
  let detail: ReactNode
  let name = p.name
  if (p.kind === 'integration') {
    label = t('dashboard.problem.integration')
    detail = p.detail
  } else if (p.kind === 'metric') {
    label = t(`status.${p.status}`)
    name = metricLabel(p.integration_type, { key: p.item_id ?? '', name: p.name })
    detail = `${p.integration_name} · ${t('dashboard.problem.value', { value: formatMetric(p.value ?? 0, p.unit) })}`
  } else if (p.kind === 'checks') {
    // Healthchecks als Ampel: eine Zeile je Projekt mit den ausgefallenen Checks
    label = t('dashboard.checksDownBadge', { count: p.count ?? 0 })
    const items = p.items ?? []
    detail = items.slice(0, 3).join(', ') + (items.length > 3 ? ` +${items.length - 3}` : '')
  } else {
    const b = backupStates.find((s) => s === p.state)
    label = b ? t(`backups.state.${b}`) : t(`status.${p.status}`)
    detail = p.integration_name
  }
  const at =
    p.at &&
    t(`dashboard.problem.at.${p.kind}`, {
      time: relativeTime(p.at, now),
    })

  return (
    <li className={motion.className} style={motion.style}>
      <Link
        to={problemLink(p)}
        className={cn('hover:bg-accent/40 flex items-center gap-3 px-3 py-2.5 transition-colors', flash && 'animate-flash')}
        data-problem={p.name}
      >
        {p.kind === 'checks' ? (
          <IntegrationIcon type="healthchecks" className="size-4 shrink-0" />
        ) : null}
        <StatusBadge status={p.status} label={label} />
        <span className="min-w-0 flex-1">
          <span className="block font-medium [overflow-wrap:anywhere]">
            <span className={cn('inline-block', p.kind === 'metric' && 'first-letter:uppercase')}>{name}</span>
            {p.group && <span className="text-muted-foreground font-normal"> · {p.group}</span>}
            <span className="text-muted-foreground ml-2 text-xs font-normal">{t(`dashboard.kind.${p.kind}`)}</span>
          </span>
          <span className="text-muted-foreground block text-xs break-words">
            {detail}
            {at && (
              <span title={absoluteTime(p.at!)}>
                {detail ? ' · ' : ''}
                {at}
              </span>
            )}
          </span>
        </span>
        <ChevronRight className="text-muted-foreground size-4 shrink-0" />
      </Link>
    </li>
  )
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
          className={cn('animate-ring absolute inset-0 rounded-lg', tone === 'error' ? 'bg-destructive/30' : 'bg-warning/30')}
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

/** Gesamtstatus: alles in Ordnung oder Liste der Probleme (ganz oben). */
function OverallStatus({ overview }: { overview: Overview }) {
  const { t } = useTranslation()
  const now = useNow()
  const problems = overview.problems
  const errors = problems.filter((p) => p.status === 'error').length
  const ok = problems.length === 0
  const idle = overview.integrations.active === 0 // alle deaktiviert
  const tone = idle ? 'idle' : ok ? 'ok' : errors > 0 ? 'error' : 'warning'

  return (
    <Card className={cn(errors > 0 && 'border-destructive/40', !ok && errors === 0 && 'border-warning/40')}>
      <CardContent className="flex flex-col gap-4">
        <div className="flex items-center gap-4">
          <StatusMark tone={tone} />
          <div className="min-w-0">
            <p className="text-lg font-semibold">
              {idle ? t('dashboard.noneActive') : ok ? t('dashboard.allGood') : t('dashboard.problems', { count: problems.length })}
            </p>
            <p className="text-muted-foreground text-sm">
              {t('dashboard.activeIntegrations', { count: overview.integrations.active })}
            </p>
          </div>
        </div>
        {problems.length > 0 && (
          <ul className="divide-y rounded-md border" aria-label={t('dashboard.problemList')}>
            {problems.map((p, n) => (
              <ProblemRow key={`${p.kind}/${p.integration_id}/${p.item_id ?? ''}`} p={p} now={now} index={n} />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

/** Übersichtskachel wie in dnsdeck: Icon-Box links, Titel, Zahl + Badges, Chevron → Detailseite. */
const tileClass =
  'group bg-card hover:bg-accent/40 flex items-center gap-4 rounded-xl border p-5 transition-[background-color,transform,box-shadow] duration-200 hover:-translate-y-0.5 hover:shadow-md'

function Tile({ to, icon, title, children, tile }: { to: string; icon: ReactNode; title: string; children: ReactNode; tile?: string }) {
  return (
    <Link to={to} className={tileClass} data-tile={tile}>
      <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg transition-transform duration-200 group-hover:scale-105">
        {icon}
      </span>
      <div className="min-w-0 flex-1">
        <div className="text-muted-foreground text-sm">{title}</div>
        {children}
      </div>
      <ChevronRight className="text-muted-foreground size-4 transition-transform group-hover:translate-x-0.5" />
    </Link>
  )
}

function BackupsTile({ overview }: { overview: Overview }) {
  const { t } = useTranslation()
  const s = overview.backups
  const n = (k: keyof typeof s.states) => s.states[k] ?? 0
  const bad = n('overdue') + n('failed')
  return (
    <Tile to="/backups" icon={<DatabaseBackup className="size-5" />} title={t('nav.backups')} tile="backups">
      {s.total === 0 ? (
        <div className="text-muted-foreground font-medium">{t('dashboard.noBackups')}</div>
      ) : (
        <div className="flex flex-wrap items-center gap-2 font-medium">
          <span className="tabular-nums">
            <AnimatedNumber value={s.total} format={(v) => t('dashboard.monitored', { count: v })} />
          </span>
          {n('ok') > 0 && <Badge variant="success">{t('dashboard.backupsOk', { count: n('ok') })}</Badge>}
          {n('due_soon') > 0 && <Badge variant="warning">{t('dashboard.backupsDueSoon', { count: n('due_soon') })}</Badge>}
          {bad > 0 && <Badge variant="destructive">{t('dashboard.backupsBad', { count: bad })}</Badge>}
          {n('unknown') + n('paused') > 0 && (
            <Badge variant="secondary">{t('dashboard.other', { count: n('unknown') + n('paused') })}</Badge>
          )}
        </div>
      )}
    </Tile>
  )
}

/** Healthchecks.io als Ampel: grün, wenn alle Checks laufen, rot mit den ausgefallenen. */
function ChecksTile({ overview }: { overview: Overview }) {
  const { t } = useTranslation()
  const c = overview.checks
  const down = c.statuses.error ?? 0
  const active = c.total - (c.statuses.paused ?? 0)
  const names = overview.problems.filter((p) => p.kind === 'checks').flatMap((p) => p.items ?? [])
  const flash = useFlash(down)
  return (
    <Tile to="/checks" icon={<IntegrationIcon type="healthchecks" className="size-5" />} title={t('nav.checks')} tile="checks">
      {c.total === 0 ? (
        <div className="text-muted-foreground font-medium">{t('dashboard.noChecks')}</div>
      ) : (
        <div className={cn('flex flex-col gap-0.5 rounded-md', flash && 'animate-flash')}>
          <span className="flex items-center gap-2 font-medium">
            <span className="relative flex size-2.5" aria-hidden>
              {down > 0 && <span className="bg-destructive/60 animate-ring absolute inset-0 rounded-full" />}
              <span className={cn('relative size-2.5 rounded-full', down > 0 ? 'bg-destructive' : 'bg-success')} />
            </span>
            {down > 0 ? t('dashboard.checksFailed', { count: down }) : t('dashboard.checksAllOk', { count: active })}
          </span>
          {down > 0 && names.length > 0 && (
            <span className="text-muted-foreground truncate text-xs">{names.join(', ')}</span>
          )}
        </div>
      )}
    </Tile>
  )
}

/** Kachel je Dienst mit Kennzahlen, z. B. „Immich – 48.213 Fotos · 1.204 Videos · 412 GB“. */
function ServiceTile({ integration: i, icon }: { integration: Integration; icon?: string }) {
  const { t } = useTranslation()
  const main = (i.snapshot?.metrics ?? []).filter((m) => !m.group).slice(0, 3)
  const bad = (i.snapshot?.metrics ?? []).filter((m) => m.status === 'error' || m.status === 'warning')
  return (
    <Link to={`/dienste#integration-${i.id}`} className={tileClass} data-tile={i.name}>
      <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg transition-transform duration-200 group-hover:scale-105">
        <IntegrationIcon name={icon} type={i.type} className="size-5" />
      </span>
      <div className="min-w-0 flex-1">
        <div className="text-muted-foreground flex items-center gap-2 text-sm">
          {i.name}
          {i.status === 'error' && <StatusBadge status="error" label={t('dashboard.problem.integration')} />}
        </div>
        {main.length === 0 ? (
          <div className="text-muted-foreground font-medium">{t('services.noMetrics')}</div>
        ) : (
          <div className="flex flex-wrap items-baseline gap-x-1.5 font-medium tabular-nums">
            {main.map((m, n) => (
              <span key={m.key} className="whitespace-nowrap">
                {n > 0 && <span className="text-muted-foreground mr-1.5">·</span>}
                {isStateUnit(m.unit) ? (
                  formatMetric(m.value, m.unit)
                ) : (
                  <AnimatedNumber value={m.value} format={(v) => formatMetric(v, m.unit)} />
                )}
                {!isStateUnit(m.unit) && (
                  <>
                    {' '}
                    <span className="text-muted-foreground text-sm font-normal">{metricLabel(i.type, m)}</span>
                  </>
                )}
              </span>
            ))}
            {bad.length > 0 && (
              <Badge variant={bad.some((m) => m.status === 'error') ? 'destructive' : 'warning'} className="ml-1">
                {t('dashboard.thresholds', { count: bad.length })}
              </Badge>
            )}
          </div>
        )}
      </div>
      <ChevronRight className="text-muted-foreground size-4 transition-transform group-hover:translate-x-0.5" />
    </Link>
  )
}

/** Letzte Statuswechsel aller Integrationen (unten auf dem Dashboard). */
function EventsTile() {
  const { t } = useTranslation()
  const history = useHistory({}, 8)
  const entries = history.data?.pages[0]?.entries ?? []
  return (
    <Card className="gap-2" data-tile="events">
      <CardContent className="flex flex-col gap-1">
        <div className="flex items-center justify-between gap-2">
          <span className="flex items-center gap-2 font-medium">
            <HistoryIcon className="size-4" />
            {t('dashboard.events')}
          </span>
          <Link to="/verlauf" className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-sm">
            {t('dashboard.allEvents')}
            <ChevronRight className="size-4" />
          </Link>
        </div>
        {history.isPending ? (
          <Skeleton className="h-24" />
        ) : entries.length === 0 ? (
          <p className="text-muted-foreground py-4 text-sm">{t('history.empty')}</p>
        ) : (
          <HistoryList entries={entries} compact />
        )}
      </CardContent>
    </Card>
  )
}

const span: Record<number, string> = { 1: '', 2: 'sm:col-span-2', 3: 'sm:col-span-2 lg:col-span-3' }
const defaultTiles: DashboardTile[] = [
  { id: 'backups', visible: true, size: 1 },
  { id: 'checks', visible: true, size: 1 },
  { id: 'events', visible: true, size: 3 },
]

/** Kacheln in der Anordnung aus Einstellungen → Dashboard. */
function Tiles({ overview }: { overview: Overview }) {
  const layout = useDashboardLayout()
  const integrations = useIntegrations()
  const types = useIntegrationTypes()
  const tiles = layout.data?.tiles ?? defaultTiles

  function tile(id: string) {
    switch (id) {
      case 'backups':
        return <BackupsTile overview={overview} />
      case 'checks':
        return <ChecksTile overview={overview} />
      case 'events':
        return <EventsTile />
    }
    const i = integrations.data?.find((x) => `integration:${x.id}` === id)
    if (!i || !i.enabled) return null
    return <ServiceTile integration={i} icon={types.data?.types.find((x) => x.type === i.type)?.icon} />
  }

  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {tiles
        .filter((x) => x.visible)
        .map((x, n) => {
          const content = tile(x.id)
          const motion = rise(n)
          return content ? (
            <div key={x.id} className={cn('min-w-0 [&>*]:h-full', span[x.size], motion.className)} style={motion.style}>
              {content}
            </div>
          ) : null
        })}
    </div>
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
          <Skeleton className="h-24 rounded-xl" />
          <div className="grid gap-4 sm:grid-cols-2">
            <Skeleton className="h-20 rounded-xl" />
            <Skeleton className="h-20 rounded-xl" />
          </div>
        </>
      ) : overview.isError ? (
        <p className="text-destructive text-sm">{t('dashboard.loadError', { error: overview.error.message })}</p>
      ) : o!.integrations.total === 0 ? (
        <NoIntegrations />
      ) : (
        <>
          <OverallStatus overview={o!} />
          <Tiles overview={o!} />
        </>
      )}
    </div>
  )
}
