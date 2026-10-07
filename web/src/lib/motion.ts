import { useEffect, useRef, useState, type CSSProperties } from 'react'

/** Bewegung reduzieren (Betriebssystem)? */
export function reducedMotion(): boolean {
  return typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
}

/** Gestaffeltes Einblenden: i-tes Element etwas später. */
export function rise(i: number): { className: string; style: CSSProperties } {
  return { className: 'animate-rise', style: { animationDelay: `${Math.min(i, 12) * 45}ms` } }
}

/**
 * true für kurze Zeit, wenn sich value ändert (nicht beim ersten Zeigen) –
 * für das Aufleuchten bei Live-Updates.
 */
export function useFlash(value: unknown): boolean {
  const first = useRef(true)
  const [flash, setFlash] = useState(false)
  const key = JSON.stringify(value)
  useEffect(() => {
    if (first.current) {
      first.current = false
      return
    }
    setFlash(true)
    const id = setTimeout(() => setFlash(false), 1600)
    return () => clearTimeout(id)
  }, [key])
  return flash
}

/** Zählt weich vom bisherigen zum neuen Wert (ease-out, ~700 ms). */
export function useCountUp(value: number, duration = 700): number {
  const still = reducedMotion() || !Number.isFinite(value)
  const [shown, setShown] = useState(0)
  const from = useRef(0)
  useEffect(() => {
    if (still) {
      from.current = value
      return
    }
    const start = performance.now()
    const a = from.current
    let raf = 0
    const step = (t: number) => {
      const p = Math.min(1, (t - start) / duration)
      const v = a + (value - a) * (1 - Math.pow(1 - p, 3))
      setShown(v)
      from.current = v
      if (p < 1) raf = requestAnimationFrame(step)
    }
    raf = requestAnimationFrame(step)
    return () => cancelAnimationFrame(raf)
  }, [value, duration, still])
  return still ? value : shown
}
