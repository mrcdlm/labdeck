import type { Status } from './api'
import { formatNumber } from './format'

// Farben wie StatusBadge: grün = ok, gelb = Warnung, rot = Fehler, grau = pausiert/keine Daten
export const bucketColor: Record<Status, string> = {
  ok: 'bg-success',
  warning: 'bg-warning',
  error: 'bg-destructive',
  paused: 'bg-muted-foreground/40',
  unknown: 'bg-muted',
}

export function formatPercent(p: number | null): string {
  if (p === null) return '—'
  // 99,95 % soll nicht als 100 % erscheinen
  const v = p >= 99.995 ? 100 : Math.floor(p * 100) / 100
  return `${formatNumber(v)} %`
}

// In einem Abschnitt gewinnt der schlechteste Status (wie im Server).
const severity: Record<Status, number> = { unknown: 0, ok: 1, paused: 2, warning: 3, error: 4 }

/** Fasst je factor Abschnitte zu einem zusammen. */
export function mergeBuckets(buckets: Status[], factor: number): Status[] {
  if (factor <= 1) return buckets
  const out: Status[] = []
  for (let i = 0; i < buckets.length; i += factor) {
    out.push(buckets.slice(i, i + factor).reduce((a, b) => (severity[b] > severity[a] ? b : a)))
  }
  return out
}
