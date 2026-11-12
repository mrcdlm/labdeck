import { AlertTriangle, ChevronDown, ChevronRight, PauseCircle } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { StatusDot } from '@/components/StatusTile'
import type { Overview, Problem } from '@/lib/api'
import { absoluteTime, formatUptime, relativeTime, useNow } from '@/lib/format'
import { formatMetric, metricLabel } from '@/lib/metrics'
import { problemLink } from '@/lib/problems'
import { cn } from '@/lib/utils'

type Tone = 'idle' | 'ok' | 'warning' | 'error'

/**
 * Ampel des Gesamtstatus: bei Fehlern pulsiert ein Ring um das Symbol, bei
 * „Alles in Ordnung“ zeichnet sich ein Häkchen.
 */
function StatusMark({ tone }: { tone: Tone }) {
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

const backupStates = ['failed', 'overdue', 'due_soon'] as const

/** Was, wo und Zustand eines Problems als Text. */
function useProblemText() {
  const { t } = useTranslation()
  return (p: Problem, withSource: boolean) => {
    const source = withSource ? p.integration_name : undefined
    switch (p.kind) {
      case 'integration':
        return {
          name: p.integration_name,
          context: p.detail,
          state: t('dashboard.problem.integration'),
        }
      case 'metric':
        return {
          name: metricLabel(p.integration_type, { key: p.item_id ?? '', name: p.name }),
          context: [source, p.resource_name || p.group].filter(Boolean).join(' › '),
          // OK/Fehler-Werte kennen nur zwei Texte – Warnungen heißen so
          state: p.unit === 'okfail' ? t(`status.${p.status}`) : formatMetric(p.value ?? 0, p.unit),
        }
      case 'check':
        return {
          name: p.name,
          context: [source, p.group].filter(Boolean).join(' · '),
          state: t('dashboard.down'),
        }
      default: {
        const b = backupStates.find((s) => s === p.state)
        return {
          name: p.name,
          context: [source, p.group].filter(Boolean).join(' · '),
          state: b ? t(`backups.state.${b}`) : t(`status.${p.status}`),
        }
      }
    }
  }
}

/**
 * Probleme als verlinkte Zeilen: Status, was, wo, Zustand bzw. Wert und seit
 * wann. Jede Zeile führt zur Stelle, an der das Problem im Detail steht.
 */
export function ProblemRows({
  problems,
  max = 6,
  withSource = true,
}: {
  problems: Problem[]
  /** sichtbare Zeilen, der Rest aufklappbar */
  max?: number
  /** Integration im Kontext nennen (aus auf deren Detailseite) */
  withSource?: boolean
}) {
  const { t } = useTranslation()
  const now = useNow()
  const text = useProblemText()
  const [open, setOpen] = useState(false)
  if (problems.length === 0) return null
  const shown = open ? problems : problems.slice(0, max)
  return (
    <div className="bg-card overflow-hidden rounded-xl border" data-problem-list>
      <ul className="divide-y">
        {shown.map((p) => {
          const { name, context, state } = text(p, withSource)
          const since = p.since ?? p.at
          return (
            <li key={p.id}>
              <Link
                to={problemLink(p)}
                className={cn(
                  'group hover:bg-accent/40 flex items-center gap-3 px-4 py-2.5 transition-colors outline-none',
                  'focus-visible:bg-accent/40',
                )}
                data-problem={p.kind === 'integration' ? p.integration_name : p.name}
                data-status={p.status}
              >
                <StatusDot status={p.status} pulse />
                <span className="min-w-0 flex-1">
                  <span
                    className={cn(
                      'block truncate text-sm font-medium',
                      // Kennzahlen heißen klein („belegung“), Namen bleiben wie sie sind
                      p.kind === 'metric' && 'first-letter:uppercase',
                    )}
                  >
                    {name}
                  </span>
                  {context && (
                    <span className="text-muted-foreground block truncate text-xs" title={context}>
                      {context}
                    </span>
                  )}
                </span>
                <span className="flex shrink-0 flex-col items-end text-right">
                  <span
                    className={cn(
                      'text-sm font-medium tabular-nums',
                      p.status === 'error' ? 'text-destructive' : 'text-warning',
                    )}
                  >
                    {state}
                  </span>
                  {since && (
                    <span className="text-muted-foreground text-xs tabular-nums" title={absoluteTime(since)}>
                      {p.since
                        ? t('problems.since', {
                            duration: formatUptime(Math.max(0, (now - new Date(p.since).getTime()) / 1000)),
                          })
                        : t(`dashboard.problem.at.${p.kind}`, { time: relativeTime(since, now) })}
                    </span>
                  )}
                </span>
                <ChevronRight className="text-muted-foreground size-4 shrink-0 transition-transform group-hover:translate-x-0.5" />
              </Link>
            </li>
          )
        })}
      </ul>
      {problems.length > max && (
        <button
          type="button"
          className="text-muted-foreground hover:text-foreground hover:bg-accent/40 flex w-full items-center justify-center gap-1 border-t py-2 text-sm"
          onClick={() => setOpen((o) => !o)}
          aria-expanded={open}
        >
          {open ? t('dashboard.showLess') : t('problems.showAll', { count: problems.length })}
          <ChevronDown className={cn('size-4 transition-transform', open && 'rotate-180')} />
        </button>
      )}
    </div>
  )
}

/**
 * Gesamtstatus ganz oben auf der Übersicht: Ampel, „Alles in Ordnung“ bzw.
 * „12 Probleme“ mit Zählern und darunter jedes Problem als eigene Zeile.
 */
export function ProblemList({ overview }: { overview: Overview }) {
  const { t } = useTranslation()
  const problems = overview.problems
  const errors = problems.filter((p) => p.status === 'error').length
  const warnings = problems.filter((p) => p.status === 'warning').length
  const ok = problems.length === 0
  const idle = overview.integrations.active === 0 // alle deaktiviert
  const tone: Tone = idle ? 'idle' : ok ? 'ok' : errors > 0 ? 'error' : 'warning'

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
                : t('dashboard.problems', { count: problems.length })}
          </p>
          <p className="text-muted-foreground text-sm">
            {t('dashboard.activeIntegrations', { count: overview.integrations.active })}
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
      <ProblemRows problems={problems} />
    </section>
  )
}
