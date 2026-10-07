import i18n from '@/i18n'

import type { Metric } from './api'
import { formatBytes, formatNumber } from './format'

/** Grundname einer Kennzahl: "user/<id>/photos" → "photos". */
export function metricBase(key: string): string {
  return key.slice(key.lastIndexOf('/') + 1)
}

/**
 * Anzeigename: bekannte Kennzahlen eingebauter Integrationen übersetzt
 * (metrics.<typ>.<grundname>), sonst der Name der Quelle (z. B. selbst
 * vergebene Namen bei HTTP-JSON und Prometheus).
 */
export function metricLabel(type: string, m: Pick<Metric, 'key' | 'name'>): string {
  const key = `metrics.${type}.${metricBase(m.key)}`
  return i18n.exists(key) ? i18n.t(key as never) : m.name
}

/** Dauer in Sekunden kurz, z. B. "45 s", "12 min", "3,5 h", "2 d". */
function formatSeconds(s: number): string {
  const abs = Math.abs(s)
  if (abs < 60) return `${formatNumber(Math.round(s * 10) / 10)} s`
  if (abs < 3600) return `${formatNumber(Math.round(s / 6) / 10)} min`
  if (abs < 86400) return `${formatNumber(Math.round(s / 360) / 10)} h`
  return `${formatNumber(Math.round(s / 8640) / 10)} d`
}

/** Wert mit Einheit lesbar, z. B. "48.213", "412 GB", "51 %". */
export function formatMetric(value: number, unit?: string): string {
  switch (unit) {
    case 'mains':
      return i18n.t(value ? 'units.mainsOn' : 'units.mainsOff')
    case 'okfail':
      return i18n.t(value ? 'units.ok' : 'units.fail')
    case 'bytes':
      return formatBytes(value)
    case 's':
      return formatSeconds(value)
    case '%':
      return `${formatNumber(Math.round(value * 10) / 10)} %`
    case undefined:
    case '':
      return formatNumber(Math.abs(value) >= 100 ? Math.round(value) : value)
  }
  return `${formatNumber(value)} ${unit}`
}

/** Zustandswerte (Netz/Batterie, OK/Fehler) stehen ohne Bezeichnung für sich. */
export function isStateUnit(unit?: string): boolean {
  return unit === 'mains' || unit === 'okfail'
}

/** Kurzform für Achsenbeschriftungen. */
export function formatMetricShort(value: number, unit?: string): string {
  if (unit === 'bytes' || unit === 's' || unit === '%' || isStateUnit(unit)) return formatMetric(value, unit)
  return new Intl.NumberFormat(i18n.language, { notation: 'compact', maximumFractionDigits: 1 }).format(value)
}
