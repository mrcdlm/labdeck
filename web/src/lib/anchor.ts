import { useEffect } from 'react'
import { useLocation } from 'react-router'

/**
 * Sprungziel innerhalb einer Seite, z. B. anchorId('res', 'qemu/102') →
 * "res-qemu_102". Nur unkritische Zeichen, damit Hash und Ziel übereinstimmen.
 * Als data-anchor gesetzt (nicht id), weil Tabellen für Desktop und Handy
 * doppelt gerendert werden – gesprungen wird zum sichtbaren Element.
 */
export function anchorId(...parts: (string | number)[]): string {
  return parts.map((p) => String(p).replace(/[^A-Za-z0-9_-]/g, '_')).join('-')
}

/** Hash der aktuellen Adresse ohne „#“ (für vorausgewählte Elemente). */
export function useHash(): string {
  return decodeURIComponent(useLocation().hash.slice(1))
}

/**
 * Scrollt nach dem Laden zum Element aus dem Hash und hebt es kurz hervor
 * (Sprung aus der Problemliste, von einer Kachel oder aus der Suche).
 */
export function useHashTarget(ready: boolean) {
  const hash = useHash()
  useEffect(() => {
    if (!ready || !hash) return
    const all = document.querySelectorAll<HTMLElement>(`[data-anchor="${CSS.escape(hash)}"]`)
    const el = [...all].find((x) => x.offsetParent !== null || x.getClientRects().length > 0)
    if (!el) return
    el.scrollIntoView({ block: 'center' })
    el.classList.add('animate-flash')
    const id = setTimeout(() => el.classList.remove('animate-flash'), 1600)
    return () => clearTimeout(id)
  }, [ready, hash])
}
