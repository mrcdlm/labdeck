import { useEffect, useState } from 'react'

import { cn } from '@/lib/utils'

/** Farbe nach Füllstand: blau, ab 80 % gelb, ab 90 % rot. */
function meterColor(pct: number): string {
  if (pct >= 90) return 'bg-destructive'
  if (pct >= 80) return 'bg-warning'
  return 'bg-chart-1'
}

/**
 * Füllstandsanzeige (0–100 %). Füllt sich beim ersten Zeigen; Änderungen
 * (Live-Updates) gleiten weich auf den neuen Wert.
 */
export function Meter({
  value,
  label,
  detail,
  size = 'md',
  className,
}: {
  value: number | undefined
  label?: string
  detail?: string
  size?: 'sm' | 'md' | 'lg'
  className?: string
}) {
  const pct = value === undefined ? undefined : Math.max(0, Math.min(100, value))
  // erst nach dem ersten Zeichnen auf den Wert setzen → Fülleffekt
  const [shown, setShown] = useState(0)
  useEffect(() => {
    const id = requestAnimationFrame(() => setShown(pct ?? 0))
    return () => cancelAnimationFrame(id)
  }, [pct])

  return (
    <div className={cn('flex min-w-0 flex-col gap-1', className)}>
      {(label || detail) && (
        <div className="flex items-baseline justify-between gap-2 text-xs">
          {label && <span className="text-muted-foreground">{label}</span>}
          {detail && <span className="truncate font-medium tabular-nums">{detail}</span>}
        </div>
      )}
      <div
        className={cn('bg-muted overflow-hidden rounded-full', size === 'sm' ? 'h-1.5' : size === 'lg' ? 'h-3' : 'h-2')}
        role="meter"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct === undefined ? undefined : Math.round(pct)}
        aria-label={label}
      >
        {pct !== undefined && (
          <div
            className={cn('h-full rounded-full transition-[width] duration-700 ease-out', meterColor(pct))}
            style={{ width: `${shown}%` }}
          />
        )}
      </div>
    </div>
  )
}
