import { ChevronRight } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import type { Status } from '@/lib/api'
import { useFlash } from '@/lib/motion'
import { cn } from '@/lib/utils'

// Ampel der Kachel: Leiste links und Tönung des Icon-Kastens
const bar: Record<Status, string> = {
  ok: 'before:bg-success',
  warning: 'before:bg-warning',
  error: 'before:bg-destructive',
  unknown: 'before:bg-muted-foreground/30',
  paused: 'before:bg-muted-foreground/30',
}
const box: Record<Status, string> = {
  ok: 'bg-success/12',
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

/**
 * Einheitliche Dashboard-Kachel, die selbst die Ampel ist: Leiste links in
 * Statusfarbe, Kopf mit Icon und Name (Link zur Detailseite), darunter eine
 * große Zahl, eine Nebenzeile, die Probleme dieses Bereichs und optional ein
 * Mini-Verlauf. stretch: die ganze Kachel ist klickbar (aus bei Kacheln mit
 * eigenen Bedienelementen bzw. Tooltips wie der Backup-Zeitleiste).
 */
export function StatusTile({
  to,
  icon,
  title,
  status,
  tile,
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
        'group bg-card relative flex flex-col gap-1 overflow-hidden rounded-xl border p-5 pl-6',
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
            'min-w-0 flex-1 truncate font-medium outline-none',
            'focus-visible:ring-ring/50 focus-visible:rounded-sm focus-visible:ring-[3px]',
            stretch && 'after:absolute after:inset-0',
          )}
        >
          {title}
        </Link>
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

export interface ProblemLine {
  key: string
  status: Status
  /** für Tests und Hervorhebung: Name der Quelle */
  id: string
  name: ReactNode
  /** grau hinter dem Namen, z. B. Host, Benutzer, Fehlermeldung */
  detail?: ReactNode
  /** umbrechen statt kürzen (Fehlermeldungen) */
  wrap?: boolean
  /** rechts: Zustand oder Wert */
  state?: ReactNode
  title?: string
}

/** Probleme einer Kachel: die ersten zwei, der Rest aufklappbar. */
export function ProblemLines({ lines, max = 2, label }: { lines: ProblemLine[]; max?: number; label?: string }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  if (lines.length === 0) return null
  const shown = open ? lines : lines.slice(0, max)
  return (
    <div className="mt-2 flex flex-col gap-1">
      <ul className="flex flex-col gap-1" aria-label={label}>
        {shown.map((l) => (
          <li key={l.key} className="flex items-start gap-2 text-sm" data-problem={l.id} title={l.title}>
            <span className="flex h-5 items-center">
              <StatusDot status={l.status} />
            </span>
            <span className={cn('min-w-0 flex-1', l.wrap ? 'break-words' : 'truncate')}>
              <span className="font-medium">{l.name}</span>
              {l.detail && <span className="text-muted-foreground"> · {l.detail}</span>}
            </span>
            {l.state && <span className="text-muted-foreground shrink-0 text-xs leading-5 tabular-nums">{l.state}</span>}
          </li>
        ))}
      </ul>
      {lines.length > max && (
        <button
          type="button"
          className="text-muted-foreground hover:text-foreground relative z-10 self-start pl-4 text-xs"
          onClick={() => setOpen((o) => !o)}
          aria-expanded={open}
        >
          {open ? t('dashboard.showLess') : t('dashboard.moreProblems', { count: lines.length - max })}
        </button>
      )}
    </div>
  )
}
