import type { DashboardTile, Integration, TileOptions } from './api'

/*
 * Kacheln des Dashboards – dieselben Regeln wie im Backend
 * (internal/api/dashboard.go): Raster aus 12 Spalten, Standardgrößen,
 * verfügbare Kacheln je nach eingerichteten Integrationen.
 */

export const GRID_COLS = 12
export const ROW_HEIGHT = 40
export const GRID_MARGIN = 16
export const MAX_TILE_HEIGHT = 24

/** Integrationstypen, die Hosts liefern (Kachel „Hosts“). */
const hostTypes = ['pve', 'dockhand']

/** Eingebaute und Integrations-Kacheln, die es gerade geben kann. */
export function availableTiles(integrations: Integration[]): string[] {
  const out = ['backups', 'checks']
  for (const i of integrations) if (i.type !== 'healthchecks') out.push(`integration:${i.id}`)
  if (integrations.some((i) => hostTypes.includes(i.type))) out.push('hosts')
  return [...out, 'backup-timeline', 'events']
}

/** Breite und Höhe neuer Kacheln (Rastereinheiten). */
export function defaultSize(id: string, options?: TileOptions): { w: number; h: number } {
  if (id === 'events') return { w: 12, h: 7 }
  if (id === 'backup-timeline') return { w: 12, h: 5 }
  if (id === 'hosts') return { w: 8, h: 5 }
  if (id.startsWith('metric:')) {
    // Kennzahl-Kacheln: so groß, wie die Darstellung Platz braucht
    if (options?.view === 'area') return { w: 6, h: 5 }
    if (options?.view === 'number') return { w: 4, h: 3 }
    if (options?.view === 'gauge') return { w: 4, h: 5 }
    return { w: 4, h: 4 }
  }
  return { w: 4, h: 5 }
}

/**
 * Höchste freie Stelle für eine Kachel der Breite w (bei gleicher Höhe die
 * linke) – wie im Backend (place in internal/api/dashboard.go).
 */
function place(tiles: DashboardTile[], w: number): { x: number; y: number } {
  const sky = Array<number>(GRID_COLS).fill(0)
  for (const t of tiles.filter((x) => x.visible)) {
    for (let x = t.x; x < Math.min(t.x + t.w, GRID_COLS); x++) sky[x] = Math.max(sky[x], t.y + t.h)
  }
  let best = { x: 0, y: Infinity }
  for (let x = 0; x + w <= GRID_COLS; x++) {
    const y = Math.max(...sky.slice(x, x + w))
    if (y < best.y) best = { x, y }
  }
  return best
}

/** Unterste belegte Zeile. */
export function bottom(tiles: DashboardTile[]): number {
  return tiles.filter((t) => t.visible).reduce((b, t) => Math.max(b, t.y + t.h), 0)
}

/** Neue Kachel unten anfügen (das Raster schiebt sie danach nach oben). */
export function appendTile(tiles: DashboardTile[], tile: Omit<DashboardTile, 'x' | 'y' | 'w' | 'h'>): DashboardTile[] {
  const rest = tiles.filter((t) => t.id !== tile.id)
  const size = defaultSize(tile.id, tile.options)
  // Ereignisse ganz unten: neue Kachel darüber, Ereignisse rücken nach
  const events = rest.find((t) => t.id === 'events' && t.visible)
  const others = rest.filter((t) => t !== events)
  const atBottom = events && events.y >= bottom(others)
  const pos = place(atBottom ? others : rest, size.w)
  const out = rest.map((t) => (atBottom && t === events ? { ...t, y: Math.max(t.y, pos.y + size.h) } : t))
  return [...out, { ...tile, ...pos, ...size }]
}

/** Neue, eindeutige ID für eine Kennzahl-Kachel. */
export function newMetricTileId(): string {
  const b = new Uint8Array(4)
  crypto.getRandomValues(b)
  return `metric:${Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')}`
}

/**
 * Reihenfolge für die gestapelte Ansicht (Handy): von oben nach unten, links
 * nach rechts; auf Wunsch Kacheln mit Problemen zuerst.
 */
export function stackOrder(tiles: DashboardTile[], rank?: (id: string) => number): DashboardTile[] {
  return [...tiles].sort((a, b) => (rank ? rank(a.id) - rank(b.id) : 0) || a.y - b.y || a.x - b.x)
}

/** Ausgangswerte einer neuen Kennzahl-Kachel. */
export const emptyOptions = (): TileOptions => ({ integration_id: 0, keys: [], view: 'sparkline', range: '24h' })
