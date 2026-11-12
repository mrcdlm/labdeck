import type { Problem, Status } from '@/lib/api'
import { anchorId } from '@/lib/anchor'

/**
 * Kachel, zu der ein Problem gehört: Backups → Backups-Kachel, Healthchecks
 * (auch „nicht erreichbar“) → Checks-Kachel, sonst die Kachel der Integration.
 */
export function problemOwner(p: Problem): string {
  if (p.kind === 'backup') return 'backups'
  if (p.kind === 'check' || p.integration_type === 'healthchecks') return 'checks'
  return `integration:${p.integration_id}`
}

/** Seite (mit Sprungziel), auf der ein Problem im Detail steht. */
export function problemLink(p: Problem): string {
  switch (p.kind) {
    case 'backup':
      return `/backups#${anchorId('backup', p.integration_id, p.item_id ?? '')}`
    case 'check':
      return `/checks#${anchorId('check', p.integration_id, p.item_id ?? '')}`
  }
  if (p.integration_type === 'healthchecks') return '/checks'
  const base = `/systeme/${p.integration_id}`
  if (p.resource_id) return `${base}#${anchorId('res', p.resource_id)}`
  // gruppierte Kennzahl (Platte, Storage, Benutzer …): Zeile in den Details
  if (p.kind === 'metric' && p.group) return `${base}#${anchorId('grp', p.group)}`
  return base
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

/** Detailseite einer Integration; Healthchecks.io hat dafür die Checks-Seite. */
export function systemLink(i: { id: number; type: string }): string {
  return i.type === 'healthchecks' ? '/checks' : `/systeme/${i.id}`
}
