import {
  Box,
  Cable,
  Cloud,
  Container,
  Cpu,
  HardDrive,
  Laptop,
  type LucideIcon,
  Network,
  Router,
  Server,
  ShieldHalf,
  Wifi,
} from 'lucide-react'
import { createContext, useContext } from 'react'

import type { TopoDeviceType, TopoLink, TopoVLAN } from './api'

/** Symbol je Gerätetyp. */
export const typeIcons: Record<TopoDeviceType, LucideIcon> = {
  internet: Cloud,
  router: Router,
  firewall: ShieldHalf,
  switch: Network,
  ap: Wifi,
  server: Server,
  hypervisor: Server,
  docker: Container,
  nas: HardDrive,
  pc: Laptop,
  iot: Cpu,
  other: Box,
}

export const CableIcon = Cable

/** Anzahl der validierten VLAN-Farben (--vlan-1 … --vlan-6, siehe index.css). */
export const VLAN_COLORS = 6

/** Helle Stufen der VLAN-Farben – gespeichert wird der Hex-Wert, gezeigt das Token (hell/dunkel). */
export const VLAN_HEX = ['#2a78d6', '#eb6834', '#1baf7a', '#eda100', '#e87ba4', '#6250d6']

/**
 * Farbe eines VLANs: gepflegt (#rrggbb) oder aus der validierten Palette in
 * fester Reihenfolge der VLAN-Liste (VLAN 1 „default“ bleibt neutral). Ab der
 * siebten Farbe neutral – die Legende beschriftet ohnehin jedes VLAN.
 */
export function vlanColor(id: number | undefined, vlans: TopoVLAN[]): string {
  if (!id) return 'var(--muted-foreground)'
  const v = vlans.find((x) => x.id === id)
  if (v?.color) {
    const i = VLAN_HEX.indexOf(v.color.toLowerCase())
    return i >= 0 ? `var(--vlan-${i + 1})` : v.color
  }
  // automatisch: freie Plätze der Palette in fester Reihenfolge (gewählte Farben sind belegt)
  const taken = new Set(vlans.map((x) => VLAN_HEX.indexOf(x.color?.toLowerCase() ?? '')).filter((i) => i >= 0))
  const free = Array.from({ length: VLAN_COLORS }, (_, i) => i).filter((i) => !taken.has(i))
  const ordered = vlans.filter((x) => x.id !== 1 && !x.color)
  const n = ordered.findIndex((x) => x.id === id)
  if (id === 1 || n < 0 || n >= free.length) return 'var(--muted-foreground)'
  return `var(--vlan-${free[n] + 1})`
}

/** Linienstärke nach Geschwindigkeit (Mbit/s). */
export function strokeFor(speed?: number): number {
  if (!speed) return 1.5
  if (speed >= 10000) return 5
  if (speed >= 2500) return 3.5
  if (speed >= 1000) return 2.25
  return 1.25
}

/** „10G“, „2.5G“, „1G“, „100M“. */
export function speedLabel(speed?: number): string {
  if (!speed) return ''
  if (speed >= 1000) return `${Number((speed / 1000).toFixed(1))}G`
  return `${speed}M`
}

export const isTrunk = (l: TopoLink) => (l.tagged?.length ?? 0) > 0

/** Schwärzen für Bild-Export und Bildschirmfotos: IPs, Subnetze, Domains. */
export const RedactContext = createContext(false)
export const useRedact = () => useContext(RedactContext)

const ipRe = /\b\d{1,3}(\.\d{1,3}){3}(\/\d{1,2})?\b/g
const domainRe = /\b([a-z0-9-]+)\.((?:[a-z0-9-]+\.)*[a-z]{2,})\b/gi

/** Ersetzt IPs/Subnetze durch „•••“ und Domains durch „name.•••“. */
export function redactText(s: string): string {
  return s.replace(ipRe, '•••').replace(domainRe, (_, host: string) => `${host}.•••`)
}

/** Link zur Detailseite der ersten Quelle eines Geräts. */
export function deviceLink(d: { sources: { integration_id: number; resource_id?: string }[] }): string | undefined {
  const s = d.sources?.[0]
  if (!s) return undefined
  const anchor = s.resource_id && s.resource_id !== 'switch' && s.resource_id !== 'server' ? `#res-${s.resource_id.replace(/[^A-Za-z0-9_-]/g, '_')}` : ''
  return `/systeme/${s.integration_id}${anchor}`
}

/** Kurzform eines Port-Namens für die Front („gi1/0/12“ → „12“). */
export function portNumber(name: string): string {
  const m = name.match(/(\d+)\s*$/)
  return m ? m[1] : name
}

/** Neue, eindeutige ID für gepflegte Geräte/Kabel. */
export function newTopoId(prefix: string, taken: string[]): string {
  const base = prefix
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 30) || 'x'
  let id = base
  for (let n = 2; taken.includes(id); n++) id = `${base}-${n}`
  return id
}

/** Wie normName im Backend: „PVE-01.lan“ = „pve1“. */
export function normName(s: string): string {
  let t = s.trim().toLowerCase()
  const dot = t.indexOf('.')
  if (dot > 0) t = t.slice(0, dot)
  let out = ''
  let zeros = true
  for (const c of t) {
    if (c >= '0' && c <= '9') {
      if (c === '0' && zeros) continue
      zeros = false
      out += c
    } else if (c >= 'a' && c <= 'z') {
      zeros = true
      out += c
    } else zeros = true
  }
  return out === '' && t !== '' ? '0' : out
}
