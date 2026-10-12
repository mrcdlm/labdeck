import { useEffect, useState } from 'react'

import i18n, { currentLang } from '@/i18n'

const locales = { de: 'de-DE', en: 'en-US' } as const

// Zeitzone für absolute Zeitangaben (Einstellungen → Allgemein); undefined = Browser.
let timeZone: string | undefined

/** Zeitzone der Anzeige setzen; leer = Zeitzone des Browsers. */
export function setDisplayTimeZone(tz: string) {
  timeZone = tz || undefined
}

// Formatierer je Sprache und Zeitzone zwischenspeichern
const cache = new Map<string, { rtf: Intl.RelativeTimeFormat; dtf: Intl.DateTimeFormat; nf: Intl.NumberFormat }>()
function fmt() {
  const lang = currentLang()
  const key = `${lang}|${timeZone ?? ''}`
  let f = cache.get(key)
  if (!f) {
    let dtf: Intl.DateTimeFormat
    try {
      dtf = new Intl.DateTimeFormat(locales[lang], { dateStyle: 'medium', timeStyle: 'short', timeZone })
    } catch {
      // Zeitzone dem Browser unbekannt
      dtf = new Intl.DateTimeFormat(locales[lang], { dateStyle: 'medium', timeStyle: 'short' })
    }
    f = {
      rtf: new Intl.RelativeTimeFormat(lang, { numeric: 'auto' }),
      dtf,
      nf: new Intl.NumberFormat(locales[lang], { maximumFractionDigits: 2 }),
    }
    cache.set(key, f)
  }
  return f
}

const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['week', 7 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
]

export function relativeTime(iso: string, now: number): string {
  const diff = (new Date(iso).getTime() - now) / 1000
  const abs = Math.abs(diff)
  if (abs < 45) return i18n.t('time.justNow')
  for (const [i, [unit, secs]] of units.entries()) {
    if (abs < secs) continue
    // 59,7 Minuten nicht als „60 Minuten“, sondern als „1 Stunde“
    const bigger = units[i - 1]
    if (bigger && Math.round(abs / secs) * secs >= bigger[1]) return fmt().rtf.format(Math.sign(diff), bigger[0])
    return fmt().rtf.format(Math.round(diff / secs), unit)
  }
  return fmt().rtf.format(Math.round(diff / 60), 'minute')
}

export function absoluteTime(iso: string): string {
  return fmt().dtf.format(new Date(iso))
}

export function formatNumber(n: number): string {
  return fmt().nf.format(n)
}

/** Liefert die aktuelle Zeit und aktualisiert sie regelmäßig (für relative Angaben). */
export function useNow(intervalMs = 30_000): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs)
    return () => clearInterval(id)
  }, [intervalMs])
  return now
}

/** Dauer in Sekunden lesbar, z. B. "30 Sekunden", "5 Minuten", "1 Stunde". */
export function formatDuration(seconds: number): string {
  if (seconds >= 86400 && seconds % 86400 === 0) return i18n.t('duration.days', { count: seconds / 86400 })
  if (seconds >= 3600 && seconds % 3600 === 0) return i18n.t('duration.hours', { count: seconds / 3600 })
  if (seconds >= 60 && seconds % 60 === 0) return i18n.t('duration.minutes', { count: seconds / 60 })
  return i18n.t('duration.seconds', { count: seconds })
}

const byteUnits = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']

/** Größe lesbar (Basis 1000 wie bei Festplatten), z. B. "412 GB". */
export function formatBytes(n: number): string {
  let i = 0
  while (n >= 1000 && i < byteUnits.length - 1) {
    n /= 1000
    i++
  }
  return `${formatNumber(i === 0 || n >= 100 ? Math.round(n) : Math.round(n * 10) / 10)} ${byteUnits[i]}`
}

/** Stunden lesbar: ganze Tage als Tage, sonst Stunden ("26 Stunden", "7 Tage", "20,8 Stunden"). */
export function formatHours(hours: number): string {
  if (Number.isInteger(hours)) return formatDuration(hours * 3600)
  return i18n.t('duration.hoursFraction', { value: formatNumber(Math.round(hours * 10) / 10) })
}

/** Laufzeit kurz, z. B. "12 d 3 h", "3 h 4 min", "5 min". */
export function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d} d ${h} h`
  if (h > 0) return `${h} h ${m} min`
  return `${m} min`
}

const bitUnits = ['bit/s', 'kbit/s', 'Mbit/s', 'Gbit/s', 'Tbit/s']

/** Datenrate lesbar (Basis 1000), z. B. "940 Mbit/s", "1,2 Gbit/s". */
export function formatBits(n: number): string {
  let i = 0
  while (Math.abs(n) >= 1000 && i < bitUnits.length - 1) {
    n /= 1000
    i++
  }
  return `${formatNumber(i === 0 || Math.abs(n) >= 100 ? Math.round(n) : Math.round(n * 10) / 10)} ${bitUnits[i]}`
}
