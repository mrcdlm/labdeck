import { currentLang } from '@/i18n'

import type { MetricRange } from './api'

/** Achsenbeschriftung der Diagramme (gedämpft, klein). */
export const axisTick = { fill: 'var(--muted-foreground)', fontSize: 12 }

/** Zeitachse je Zeitraum: Uhrzeit, Wochentag oder Datum. */
export function tickFormatter(range: MetricRange) {
  const lang = currentLang() === 'de' ? 'de-DE' : 'en-US'
  const fmt = new Intl.DateTimeFormat(
    lang,
    range === '1h' || range === '24h'
      ? { hour: '2-digit', minute: '2-digit' }
      : range === '7d'
        ? { weekday: 'short', day: 'numeric' }
        : { day: 'numeric', month: 'short' },
  )
  return (v: number) => fmt.format(new Date(v))
}
