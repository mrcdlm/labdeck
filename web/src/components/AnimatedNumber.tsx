import { useCountUp } from '@/lib/motion'

/**
 * Zahl, die beim ersten Zeigen hochzählt und bei Live-Updates weich zum neuen
 * Wert gleitet. format bekommt den Zwischenwert; ganzzahlige Werte bleiben
 * ganzzahlig.
 */
export function AnimatedNumber({ value, format }: { value: number; format: (v: number) => string }) {
  const v = useCountUp(value)
  const shown = Number.isInteger(value) ? Math.round(v) : v
  return <span className="tabular-nums">{format(shown)}</span>
}
