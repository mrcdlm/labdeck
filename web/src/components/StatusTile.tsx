import { ChevronRight } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import type { Status } from '@/lib/api'
import { useFlash } from '@/lib/motion'
import { cn } from '@/lib/utils'

// Leiste links nur bei Problemen – ruhige Kacheln, wenn alles läuft
const bar: Partial<Record<Status, string>> = {
  warning: 'before:bg-warning',
  error: 'before:bg-destructive',
}
const box: Record<Status, string> = {
  ok: 'bg-muted',
  warning: 'bg-warning/15',
  error: 'bg-destructive/12',
  unknown: 'bg-muted',
  paused: 'bg-muted',
}
const dot: Record<Status, string> = {
  ok: 'bg-success',
  warning: 'bg-warning',
  error: 'bg-destructive',
  unknown: 'bg-muted-foreground/50',
  paused: 'bg-muted-foreground/50',
}

/** Farbiger Punkt für Status (Problemzeilen, Hosts). */
export function StatusDot({ status, pulse }: { status: Status; pulse?: boolean }) {
  return (
    <span className="relative flex size-2 shrink-0" aria-hidden>
      {pulse && status === 'error' && <span className="bg-destructive/60 animate-ring absolute inset-0 rounded-full" />}
      <span className={cn('relative size-2 rounded-full', dot[status])} />
    </span>
  )
}

/** Anzahl der Probleme eines Bereichs als kleines Badge (Details stehen in der Problemliste). */
export function ProblemCount({ count, status }: { count: number; status: Status }) {
  const { t } = useTranslation()
  if (count === 0) return null
  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center rounded-full px-2 py-0.5 text-xs font-medium tabular-nums',
        status === 'error' ? 'bg-destructive/10 text-destructive' : 'bg-warning/15 text-warning',
      )}
      data-problem-count={count}
    >
      {t('dashboard.problems', { count })}
    </span>
  )
}

/**
 * Einheitliche Dashboard-Kachel: Kopf mit Icon, Name (Link zur Detailseite),
 * Statuspunkt bzw. Anzahl der Probleme; darunter eine große Zahl, eine
 * Nebenzeile und optional ein Mini-Verlauf. Nur Kacheln mit Problemen
 * bekommen links eine farbige Leiste. stretch: die ganze Kachel ist klickbar
 * (aus bei Kacheln mit eigenen Bedienelementen wie der Backup-Zeitleiste).
 */
export function StatusTile({
  to,
  icon,
  title,
  status,
  tile,
  problems = 0,
  value,
  sub,
  action,
  children,
  stretch = true,
}: {
  to: string
  icon: ReactNode
  title: ReactNode
  status: Status
  tile: string
  /** Anzahl der Probleme dieses Bereichs */
  problems?: number
  value?: ReactNode
  sub?: ReactNode
  /** Bedienelement rechts im Kopf (z. B. Zeitraum) */
  action?: ReactNode
  children?: ReactNode
  stretch?: boolean
}) {
  const flash = useFlash(status)
  return (
    <div
      className={cn(
        'group bg-card relative flex flex-col gap-1 overflow-hidden rounded-xl border p-5',
        'transition-[background-color,transform,box-shadow] duration-200',
        'before:absolute before:inset-y-0 before:left-0 before:w-1',
        bar[status],
        stretch && 'hover:bg-accent/40 hover:-translate-y-0.5 hover:shadow-md',
        flash && 'animate-flash',
      )}
      data-tile={tile}
      data-tone={status}
    >
      <div className="flex items-center gap-3">
        <span
          className={cn(
            'grid size-8 shrink-0 place-items-center rounded-lg transition-transform duration-200 group-hover:scale-105',
            box[status],
          )}
        >
          {icon}
        </span>
        <Link
          to={to}
          className={cn(
            'min-w-0 truncate font-medium outline-none',
            'focus-visible:ring-ring/50 focus-visible:rounded-sm focus-visible:ring-[3px]',
            stretch && 'after:absolute after:inset-0',
          )}
        >
          {title}
        </Link>
        {problems > 0 ? <ProblemCount count={problems} status={status} /> : <StatusDot status={status} />}
        <span className="flex-1" />
        {action && <div className="relative z-10">{action}</div>}
        <ChevronRight className="text-muted-foreground size-4 shrink-0 transition-transform group-hover:translate-x-0.5" />
      </div>
      {value !== undefined && <div className="mt-2 flex flex-wrap items-baseline gap-x-1.5">{value}</div>}
      {sub && <div className="text-muted-foreground text-sm tabular-nums">{sub}</div>}
      {children}
    </div>
  )
}

/** Große Zahl einer Kachel mit grauem Label dahinter. */
export function TileValue({ children, label }: { children: ReactNode; label?: ReactNode }) {
  return (
    <>
      <span className="text-2xl leading-tight font-semibold tracking-tight tabular-nums">{children}</span>
      {label && <span className="text-muted-foreground text-sm"> {label}</span>}
    </>
  )
}
