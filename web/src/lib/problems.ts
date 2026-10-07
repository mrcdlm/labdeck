import type { Problem, Status } from '@/lib/api'

/**
 * Kachel, zu der ein Problem gehört: Backups → Backups-Kachel, Healthchecks
 * (auch „nicht erreichbar“) → Checks-Kachel, sonst die Kachel der Integration.
 */
export function problemOwner(p: Problem): string {
  if (p.kind === 'backup') return 'backups'
  if (p.kind === 'checks' || p.integration_type === 'healthchecks') return 'checks'
  return `integration:${p.integration_id}`
}

const rank: Record<Status, number> = {
  error: 0,
  warning: 1,
  ok: 2,
  unknown: 3,
  paused: 4,
}

/** Sortierschlüssel: Fehler vor Warnungen vor allem anderen. */
export function statusRank(s: Status | undefined): number {
  return s ? rank[s] : 5
}

/** Schlechtester Status (Fehler > Warnung > OK > unbekannt); leer = undefined. */
export function worstStatus(list: (Status | undefined)[]): Status | undefined {
  let out: Status | undefined
  for (const s of list) {
    if (s && (out === undefined || rank[s] < rank[out])) out = s
  }
  return out
}
