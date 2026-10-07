import type { LucideIcon } from 'lucide-react'
import { AlertTriangle, ChevronRight, DatabaseBackup, HeartPulse, PauseCircle, Plug, ShieldCheck } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { PageHeader } from '@/components/PageHeader'
import { StatusBadge } from '@/components/StatusBadge'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { Overview, Problem } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { useOverview } from '@/lib/queries'
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

const problemLink: Record<Problem['kind'], string> = {
  integration: '/einstellungen',
  backup: '/backups',
  check: '/checks',
}

const checkStates = ['down', 'grace'] as const
const backupStates = ['failed', 'overdue', 'due_soon'] as const

/** Eine Zeile der Problemliste: was, wo, seit wann. */
function ProblemRow({ p, now }: { p: Problem; now: number }) {
  const { t } = useTranslation()
  let label: string
  let detail: ReactNode
  if (p.kind === 'integration') {
    label = t('dashboard.problem.integration')
    detail = p.detail
  } else {
    const b = backupStates.find((s) => s === p.state)
    const c = checkStates.find((s) => s === p.state)
    if (p.kind === 'backup' && b) label = t(`backups.state.${b}`)
    else if (p.kind === 'check' && c) label = t(`checks.raw.${c}`)
    else label = t(`status.${p.status}`)
    detail = p.integration_name
  }
  const at =
    p.at &&
    t(`dashboard.problem.at.${p.kind}`, {
      time: relativeTime(p.at, now),
    })

  return (
    <li>
      <Link
        to={problemLink[p.kind]}
        className="hover:bg-accent/40 flex items-center gap-3 px-3 py-2.5 transition-colors"
        data-problem={p.name}
      >
        <StatusBadge status={p.status} label={label} />
        <span className="min-w-0 flex-1">
          <span className="block font-medium [overflow-wrap:anywhere]">
            {p.name}
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

/** Gesamtstatus: alles in Ordnung oder Liste der Probleme (ganz oben). */
function OverallStatus({ overview }: { overview: Overview }) {
  const { t } = useTranslation()
  const now = useNow()
  const problems = overview.problems
  const errors = problems.filter((p) => p.status === 'error').length
  const ok = problems.length === 0
  const idle = overview.integrations.active === 0 // alle deaktiviert
  const Icon = idle ? PauseCircle : ok ? ShieldCheck : AlertTriangle

  return (
    <Card className={cn(errors > 0 && 'border-destructive/40', !ok && errors === 0 && 'border-warning/40')}>
      <CardContent className="flex flex-col gap-4">
        <div className="flex items-center gap-4">
          <span
            className={cn(
              'grid size-10 shrink-0 place-items-center rounded-lg',
              idle
                ? 'bg-muted text-muted-foreground'
                : ok
                  ? 'bg-success/15 text-success'
                : errors > 0
                  ? 'bg-destructive/15 text-destructive'
                  : 'bg-warning/15 text-warning',
            )}
          >
            <Icon className="size-5" />
          </span>
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
            {problems.map((p) => (
              <ProblemRow key={`${p.kind}/${p.integration_id}/${p.item_id ?? ''}`} p={p} now={now} />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

/** Übersichtskachel wie in dnsdeck: Icon-Box links, Titel, Zahl + Badges, Chevron → Detailseite. */
function Tile({ to, icon: Icon, title, children }: { to: string; icon: LucideIcon; title: string; children: ReactNode }) {
  return (
    <Link
      to={to}
      className="group bg-card hover:bg-accent/40 flex items-center gap-4 rounded-xl border p-5 transition-colors"
    >
      <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg">
        <Icon className="size-5" />
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
    <Tile to="/backups" icon={DatabaseBackup} title={t('nav.backups')}>
      {s.total === 0 ? (
        <div className="text-muted-foreground font-medium">{t('dashboard.noBackups')}</div>
      ) : (
        <div className="flex flex-wrap items-center gap-2 font-medium">
          <span className="tabular-nums">{t('dashboard.monitored', { count: s.total })}</span>
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

function ChecksTile({ overview }: { overview: Overview }) {
  const { t } = useTranslation()
  const c = overview.checks
  const n = (k: keyof typeof c.statuses) => c.statuses[k] ?? 0
  return (
    <Tile to="/checks" icon={HeartPulse} title={t('nav.checks')}>
      {c.total === 0 ? (
        <div className="text-muted-foreground font-medium">{t('dashboard.noChecks')}</div>
      ) : (
        <div className="flex flex-wrap items-center gap-2 font-medium">
          <span className="tabular-nums">{t('dashboard.checks', { count: c.total })}</span>
          {n('ok') > 0 && <Badge variant="success">{t('dashboard.checksUp', { count: n('ok') })}</Badge>}
          {n('warning') > 0 && <Badge variant="warning">{t('dashboard.checksGrace', { count: n('warning') })}</Badge>}
          {n('error') > 0 && <Badge variant="destructive">{t('dashboard.checksDown', { count: n('error') })}</Badge>}
          {n('paused') + n('unknown') > 0 && (
            <Badge variant="secondary">{t('dashboard.other', { count: n('paused') + n('unknown') })}</Badge>
          )}
        </div>
      )}
    </Tile>
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
          <div className="grid gap-4 sm:grid-cols-2">
            <BackupsTile overview={o!} />
            <ChecksTile overview={o!} />
          </div>
        </>
      )}
    </div>
  )
}
