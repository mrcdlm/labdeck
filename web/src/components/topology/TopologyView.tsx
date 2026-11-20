import { ExternalLink, Globe, Waypoints } from 'lucide-react'
import { useCallback, useLayoutEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { IntegrationIcon } from '@/components/IntegrationIcon'
import { StatusBadge } from '@/components/StatusBadge'
import { StatusDot } from '@/components/StatusTile'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { Status, TopoDevice, TopoLink, TopoPort, Topology, TopoVLAN } from '@/lib/api'
import {
  deviceLink,
  isTrunk,
  portNumber,
  redactText,
  speedLabel,
  strokeFor,
  typeIcons,
  useRedact,
  vlanColor,
} from '@/lib/topology'
import { cn } from '@/lib/utils'

// --- Ports -------------------------------------------------------------------

const portBox: Record<Status, string> = {
  ok: 'border-foreground/40 bg-foreground/10',
  warning: 'border-warning bg-warning/15',
  error: 'border-destructive bg-destructive/15',
  paused: 'border-border bg-muted/40',
  unknown: 'border-border bg-muted/60',
}

/** Port mit VLAN-Balken darunter: hoch = ohne Tag (PVID), kurz = getaggt. */
function PortBox({ device, port, vlans, linked }: { device: string; port: TopoPort; vlans: TopoVLAN[]; linked: boolean }) {
  const { t } = useTranslation()
  const tagged = port.tagged ?? []
  const title = [
    port.name,
    port.alias,
    speedLabel(port.speed),
    port.pvid ? `VLAN ${port.pvid}` : '',
    tagged.length ? t('topology.tagged', { list: tagged.join(', ') }) : '',
    t(`topology.portState.${port.state === 'up' ? 'up' : port.state === 'disabled' ? 'disabled' : 'down'}`),
  ]
    .filter(Boolean)
    .join(' · ')
  return (
    <div className="flex w-5 flex-col items-center gap-0.5" title={title} data-port={port.name}>
      <span className="text-muted-foreground font-mono text-[9px] leading-none tabular-nums">{portNumber(port.name)}</span>
      <span
        className={cn(
          'relative h-3.5 w-5 rounded-[3px] border',
          portBox[port.status],
          port.media === 'sfp' && 'h-3 rounded-sm',
          linked && 'ring-ring/60 ring-1',
        )}
        data-anchor-port={`${device}|${port.name}`}
      />
      <span className="flex h-2.5 items-end gap-px" aria-hidden>
        {port.pvid ? <span className="h-2.5 w-1 rounded-[1px]" style={{ background: vlanColor(port.pvid, vlans) }} /> : null}
        {tagged.slice(0, 4).map((v) => (
          <span key={v} className="h-1 w-1 rounded-[1px]" style={{ background: vlanColor(v, vlans) }} />
        ))}
      </span>
    </div>
  )
}

/** Port-Leiste eines Switches/Routers: wie die Front in Reihen, LAGs weggelassen. */
function PortStrip({ device, vlans, linked }: { device: TopoDevice; vlans: TopoVLAN[]; linked: Set<string> }) {
  const ports = device.ports
    .filter((p) => p.media !== 'lag')
    .sort((a, b) => (a.unit ?? 0) - (b.unit ?? 0) || (a.pos ?? 0) - (b.pos ?? 0) || a.name.localeCompare(b.name, undefined, { numeric: true }))
  if (ports.length === 0) return null
  return (
    <div className="bg-muted/30 flex flex-wrap gap-x-1 gap-y-1.5 rounded-md border px-2 py-1.5" data-port-strip>
      {ports.map((p) => (
        <PortBox key={p.name} device={device.id} port={p} vlans={vlans} linked={linked.has(`${device.id}|${p.name}`)} />
      ))}
    </div>
  )
}

// --- Geräte ------------------------------------------------------------------

const MAX_GUESTS = 12

type FactKey = 'model' | 'os' | 'firmware' | 'cpu' | 'memory' | 'ip'

function Fact({ k, v }: { k: FactKey; v: string }) {
  const { t } = useTranslation()
  const redact = useRedact()
  return (
    <div className="flex min-w-0 gap-2 text-xs">
      <span className="text-muted-foreground w-16 shrink-0 uppercase tracking-wide">{t(`topology.fact.${k}`)}</span>
      <span className={cn('min-w-0 truncate', (k === 'ip' || k === 'firmware') && 'font-mono')} title={v}>
        {redact ? redactText(v) : v}
      </span>
    </div>
  )
}

/** Gerätekarte: Typ, Name, Status, Angaben, Ports, Gäste und Dienste. */
export function DeviceCard({ device: d, vlans, linked }: { device: TopoDevice; vlans: TopoVLAN[]; linked: Set<string> }) {
  const { t } = useTranslation()
  const Icon = typeIcons[d.type] ?? typeIcons.other
  const to = deviceLink(d)
  const groups = new Map<string, TopoDevice['guests']>()
  for (const g of d.guests) {
    const k = g.group || t(`topology.guestKind.${g.kind}`, { defaultValue: g.kind })
    groups.set(k, [...(groups.get(k) ?? []), g])
  }
  const running = d.guests.filter((g) => ['running', 'healthy', 'unhealthy', 'online'].includes(g.state)).length
  let shown = 0
  return (
    <article
      className={cn(
        'bg-card relative flex min-w-0 flex-col gap-3 rounded-xl border p-4 shadow-sm',
        d.status === 'error' && 'border-destructive/60',
        d.status === 'warning' && 'border-warning/60',
      )}
      data-anchor-device={d.id}
      data-device={d.name}
    >
      <header className="flex items-center gap-3">
        <span className="bg-muted grid size-9 shrink-0 place-items-center rounded-lg">
          <Icon className="size-4" aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            {to ? (
              <Link to={to} className="truncate font-semibold hover:underline">
                {d.name}
              </Link>
            ) : (
              <span className="truncate font-semibold">{d.name}</span>
            )}
            {d.status !== 'unknown' && <StatusDot status={d.status} pulse />}
          </div>
          <p className="text-muted-foreground truncate text-xs">
            {t(`topology.type.${d.type}`)}
            {d.sources.length > 0 && ` · ${d.sources.map((s) => s.integration_name).join(', ')}`}
          </p>
        </div>
      </header>
      {d.facts.length > 0 && (
        <div className="flex flex-col gap-0.5">
          {d.facts.map((f) => (
            <Fact key={f.key} k={f.key as FactKey} v={f.value} />
          ))}
        </div>
      )}
      {d.note && <p className="text-muted-foreground text-xs">{d.note}</p>}
      <PortStrip device={d} vlans={vlans} linked={linked} />
      {d.guests.length > 0 && (
        <div className="flex flex-col gap-2 border-t pt-2">
          <p className="text-muted-foreground text-xs">
            {t('topology.guests', { running, total: d.guests.length })}
          </p>
          {[...groups].map(([g, list]) => {
            const take = Math.max(0, Math.min(list.length, MAX_GUESTS - shown))
            shown += take
            if (take === 0) return null
            return (
              <div key={g} className="flex flex-col gap-1">
                <span className="text-muted-foreground text-[10px] font-medium tracking-wide uppercase">{g}</span>
                <div className="flex flex-wrap gap-1">
                  {list.slice(0, take).map((x) => (
                    <span key={x.id} className="bg-muted/60 flex items-center gap-1.5 rounded-md px-1.5 py-0.5 text-xs" data-guest={x.name}>
                      <StatusDot status={x.status} />
                      {x.name}
                    </span>
                  ))}
                </div>
              </div>
            )
          })}
          {d.guests.length > shown && to && (
            <Link to={to} className="text-primary text-xs hover:underline">
              {t('topology.moreGuests', { count: d.guests.length - shown })}
            </Link>
          )}
        </div>
      )}
      {d.services.length > 0 && (
        <div className="flex flex-wrap gap-1 border-t pt-2">
          {d.services.map((s) => (
            <ServiceChip key={s.integration_id} s={s} />
          ))}
        </div>
      )}
    </article>
  )
}

export function ServiceChip({ s }: { s: { integration_id: number; name: string; type: string; status: Status } }) {
  return (
    <Link
      to={`/systeme/${s.integration_id}`}
      className="hover:bg-accent flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs"
      data-service-chip={s.name}
    >
      <IntegrationIcon type={s.type} className="size-3.5" />
      {s.name}
      <StatusDot status={s.status} />
    </Link>
  )
}

// --- Kabel -------------------------------------------------------------------

type Drawn = { l: TopoLink; d: string; lx: number; ly: number; w: number }

/**
 * Kabel als SVG unter den Karten: vom Port (oder Gerät) oben zur Oberkante
 * des Geräts darunter, rechtwinklig; Breite = Geschwindigkeit, Farbe = VLAN
 * ohne Tag, Trunks hell mit dunklem Kern; gestrichelt, wenn der Link fehlt.
 */
function LinkLayer({ links, vlans, container }: { links: TopoLink[]; vlans: TopoVLAN[]; container: HTMLElement | null }) {
  const [drawn, setDrawn] = useState<Drawn[]>([])
  const [size, setSize] = useState({ w: 0, h: 0 })

  const measure = useCallback(() => {
    if (!container) return
    const base = container.getBoundingClientRect()
    const find = (dev: string, port?: string) => {
      const p = port && container.querySelector<HTMLElement>(`[data-anchor-port="${CSS.escape(`${dev}|${port}`)}"]`)
      const el = p || container.querySelector<HTMLElement>(`[data-anchor-device="${CSS.escape(dev)}"]`)
      if (!el) return undefined
      const r = el.getBoundingClientRect()
      // Unterkante der Karte: waagerechte Stücke laufen darunter, nicht hinter ihr
      const card = (el.closest('[data-anchor-device]') ?? el).getBoundingClientRect()
      return {
        x: r.left - base.left + r.width / 2,
        top: r.top - base.top,
        bottom: r.bottom - base.top,
        card: card.bottom - base.top,
        left: card.left - base.left,
        right: card.right - base.left,
        el: el.closest('[data-anchor-device]'),
      }
    }
    // Kabel zwischen denselben Ebenen auf eigene Höhen verteilen (keine deckungsgleichen Linien)
    const raw = links.flatMap((l) => {
      const a = find(l.a.device, l.a.port)
      const b = find(l.b.device, l.b.port)
      if (!a || !b) return []
      const [u, d] = a.top <= b.top ? [a, b] : [b, a]
      return [{ l, u, d }]
    })
    // Karten als Hindernisse: ein senkrechtes Kabel darf nicht hinter einer fremden Karte verlaufen
    const cards = [...container.querySelectorAll<HTMLElement>('[data-anchor-device]')].map((c) => {
      const r = c.getBoundingClientRect()
      return { c, left: r.left - base.left, right: r.right - base.left, top: r.top - base.top, bottom: r.bottom - base.top }
    })
    const blocked = (x: number, from: number, to: number, except: Element | null) =>
      cards.some((k) => k.c !== except && k.left - 2 < x && x < k.right + 2 && k.top < to && k.bottom > from)
    const lanes = new Map<number, number>()
    const out: Drawn[] = []
    for (const r of raw.sort((x, y) => x.u.x - y.u.x)) {
      const key = Math.round(r.d.top / 8)
      const n = lanes.get(key) ?? 0
      lanes.set(key, n + 1)
      const y1 = r.u.bottom
      const y2 = r.d.top
      const w = strokeFor(r.l.speed)
      if (y2 - r.u.card < 12) {
        // nebeneinander (gleiche Ebene): unter beiden Karten seitlich verbinden
        const [left, right] = r.u.x <= r.d.x ? [r.u, r.d] : [r.d, r.u]
        const ym = Math.max(left.card, right.card) + 10 + (n % 4) * 6
        out.push({ l: r.l, w, d: `M${left.x},${left.bottom} V${ym} H${right.x} V${right.bottom}`, lx: right.x, ly: ym })
        continue
      }
      const gap = y2 - r.u.card
      const ym = r.u.card + Math.max(6, Math.min(gap - 10, 12 + (n % 6) * 6))
      if (blocked(r.d.x, ym, y2, r.d.el)) {
        // Ziel liegt unter einer anderen Karte: durch die Lücke zwischen den Spalten, kurz vor dem Ziel einbiegen
        const gx = r.d.left - 8 > 4 ? r.d.left - 8 : r.d.right + 8
        const yt = y2 - 8
        out.push({ l: r.l, w, d: `M${r.u.x},${y1} V${ym} H${gx} V${yt} H${r.d.x} V${y2}`, lx: r.d.x, ly: y2 - 4 })
        continue
      }
      out.push({ l: r.l, w, d: `M${r.u.x},${y1} V${ym} H${r.d.x} V${y2}`, lx: r.d.x, ly: y2 - 4 })
    }
    setDrawn(out)
    setSize({ w: container.scrollWidth, h: container.scrollHeight })
  }, [container, links])

  // Lage der Karten aus dem DOM messen: beim Start und bei jeder Größenänderung
  useLayoutEffect(() => {
    if (!container) return
    let frame = requestAnimationFrame(measure)
    const ro = new ResizeObserver(() => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(measure)
    })
    ro.observe(container)
    for (const el of container.querySelectorAll('[data-anchor-device]')) ro.observe(el)
    return () => {
      cancelAnimationFrame(frame)
      ro.disconnect()
    }
  }, [container, measure])

  return (
    <svg className="pointer-events-none absolute inset-0 hidden sm:block" width={size.w} height={size.h} aria-hidden data-links={drawn.length}>
      {drawn.map(({ l, d, w, lx, ly }) => {
        const down = l.status === 'error' || l.status === 'paused'
        const dash = down ? '6 5' : undefined
        const color = l.status === 'error' ? 'var(--destructive)' : vlanColor(l.vlan, vlans)
        return (
          <g key={l.id} data-link={l.id}>
            {isTrunk(l) && !down ? (
              <>
                <path d={d} fill="none" stroke="var(--foreground)" strokeOpacity={0.85} strokeWidth={w + 2.5} strokeLinejoin="round" />
                <path d={d} fill="none" stroke="var(--card)" strokeWidth={Math.max(1, w - 1)} strokeLinejoin="round" />
              </>
            ) : (
              <path d={d} fill="none" stroke={color} strokeWidth={w} strokeDasharray={dash} strokeLinejoin="round" opacity={down ? 0.7 : 1} />
            )}
            {l.speed ? (
              <text x={lx + 5} y={ly} className="fill-muted-foreground font-mono text-[9px]">
                {speedLabel(l.speed)}
              </text>
            ) : null}
          </g>
        )
      })}
    </svg>
  )
}

// --- Plan --------------------------------------------------------------------

/** Ebenen: Internet → Router/Firewall → Switches → Geräte; Kabel dazwischen. */
export function Plan({ topo }: { topo: Topology }) {
  const { t } = useTranslation()
  // Callback-Ref: die Kabel-Ebene misst, sobald der Plan im DOM ist
  const [el, setEl] = useState<HTMLDivElement | null>(null)
  const linked = new Set(topo.links.flatMap((l) => [`${l.a.device}|${l.a.port}`, `${l.b.device}|${l.b.port}`]))
  const tiers = [0, 1, 2, 3].map((n) => topo.devices.filter((d) => d.tier === n)).filter((x) => x.length > 0)
  return (
    <div ref={setEl} className="relative flex flex-col gap-12" data-plan>
      <LinkLayer links={topo.links} vlans={topo.vlans} container={el} />
      {tiers.map((list, n) => (
        <section key={n} className="relative z-10" aria-label={t(`topology.tier.${list[0].tier as 0 | 1 | 2 | 3}`)}>
          <div
            className={cn(
              'grid items-start gap-4',
              list[0].tier === 3
                ? 'grid-cols-[repeat(auto-fill,minmax(min(100%,17rem),1fr))]'
                : 'grid-cols-[repeat(auto-fit,minmax(min(100%,20rem),34rem))] justify-center',
            )}
          >
            {list.map((d) => (
              <DeviceCard key={d.id} device={d} vlans={topo.vlans} linked={linked} />
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}

// --- Seitenleiste ------------------------------------------------------------

export function Panel({ title, icon, children, className }: { title: string; icon?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={cn('bg-card flex flex-col gap-3 rounded-xl border p-4', className)}>
      <h2 className="flex items-center gap-2 text-sm font-semibold">
        {icon}
        {title}
      </h2>
      {children}
    </section>
  )
}

/** Legende der Netze: Farbe, VLAN-Nummer, Name, Subnetz – dazu die Linienarten. */
export function NetworksPanel({ vlans }: { vlans: TopoVLAN[] }) {
  const { t } = useTranslation()
  const redact = useRedact()
  return (
    <Panel title={t('topology.networks')}>
      {vlans.length === 0 ? (
        <p className="text-muted-foreground text-xs">{t('topology.noVlans')}</p>
      ) : (
        <ul className="flex flex-col gap-1.5" data-vlans>
          {vlans.map((v) => (
            <li key={v.id} className="flex items-start gap-2 text-xs" data-vlan={v.id}>
              <span className="mt-0.5 h-3.5 w-1 shrink-0 rounded-sm" style={{ background: vlanColor(v.id, vlans) }} />
              <span className="w-14 shrink-0 font-mono tabular-nums">VLAN {v.id}</span>
              <span className="min-w-0 flex-1">
                <span className="font-medium">{v.name || '—'}</span>
                {v.subnet && <span className="text-muted-foreground ml-2 font-mono">{redact ? redactText(v.subnet) : v.subnet}</span>}
                {v.note && <span className="text-muted-foreground block">{v.note}</span>}
              </span>
            </li>
          ))}
        </ul>
      )}
      <div className="text-muted-foreground flex flex-col gap-1.5 border-t pt-3 text-xs">
        <Legend line={<span className="border-foreground/80 bg-card block h-1.5 w-8 rounded-full border-2" />} text={t('topology.legend.trunk')} />
        <Legend line={<span className="block h-0.5 w-8 rounded-full" style={{ background: 'var(--vlan-1)' }} />} text={t('topology.legend.access')} />
        <Legend line={<span className="border-muted-foreground block w-8 border-t-2 border-dashed" />} text={t('topology.legend.down')} />
        <Legend
          line={
            <span className="flex items-end gap-1">
              {[1, 2.5, 10].map((g) => (
                <span key={g} className="bg-foreground/70 block w-2.5 rounded-full" style={{ height: strokeFor(g * 1000) }} />
              ))}
            </span>
          }
          text={t('topology.legend.speed')}
        />
        <Legend
          line={
            <span className="flex items-end gap-px">
              <span className="bg-foreground/70 block h-2.5 w-1 rounded-[1px]" />
              <span className="bg-foreground/70 block h-1 w-1 rounded-[1px]" />
              <span className="bg-foreground/70 block h-1 w-1 rounded-[1px]" />
            </span>
          }
          text={t('topology.legend.portVlans')}
        />
      </div>
    </Panel>
  )
}

function Legend({ line, text }: { line: ReactNode; text: string }) {
  return (
    <div className="flex items-center gap-3">
      <span className="flex w-8 shrink-0 justify-center">{line}</span>
      {text}
    </div>
  )
}

export function ExposedPanel({ topo }: { topo: Topology }) {
  const { t } = useTranslation()
  const redact = useRedact()
  if (topo.exposed.length === 0 && topo.vpn.length === 0) return null
  return (
    <Panel title={t('topology.exposed')} icon={<Globe className="size-4" />}>
      {topo.exposed.length > 0 && (
        <ul className="flex flex-col gap-1.5">
          {topo.exposed.map((x) => (
            <li key={`${x.source}/${x.name}`} className="flex items-center gap-2 text-xs" data-exposed={x.name}>
              <StatusDot status={x.status} />
              <span className="min-w-0 flex-1 truncate font-mono" title={x.targets}>
                {redact ? redactText(x.name) : x.name}
              </span>
              <span className="text-muted-foreground shrink-0">{x.source}</span>
            </li>
          ))}
        </ul>
      )}
      {topo.vpn.map((v) => (
        <Link
          key={v.integration_id}
          to={`/systeme/${v.integration_id}`}
          className="hover:bg-accent/50 flex items-center gap-2 rounded-md border px-2 py-1.5 text-xs"
        >
          <Waypoints className="size-3.5" />
          <span className="flex-1">{v.name}</span>
          <span className="tabular-nums">{t('topology.vpnPeers', { online: v.online, total: v.total })}</span>
          {v.problems > 0 && <StatusDot status="error" />}
          <ExternalLink className="text-muted-foreground size-3" />
        </Link>
      ))}
    </Panel>
  )
}

/** Alle Kabel als Tabelle (auch für Screenreader und das Handy). */
export function LinkTable({ topo }: { topo: Topology }) {
  const { t } = useTranslation()
  const name = (id: string) => topo.devices.find((d) => d.id === id)?.name ?? id
  if (topo.links.length === 0) return null
  return (
    <section className="bg-card overflow-hidden rounded-xl border" aria-label={t('topology.cables')}>
      <h2 className="px-4 pt-3 text-sm font-semibold">{t('topology.cables')}</h2>
      <div className="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="pl-4">{t('topology.col.from')}</TableHead>
              <TableHead>{t('topology.col.to')}</TableHead>
              <TableHead className="text-right">{t('topology.col.speed')}</TableHead>
              <TableHead>{t('topology.col.vlans')}</TableHead>
              <TableHead>{t('topology.col.state')}</TableHead>
              <TableHead className="pr-4">{t('topology.col.source')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {topo.links.map((l) => (
              <TableRow key={l.id} data-cable={l.id}>
                <TableCell className="pl-4">
                  {name(l.a.device)}
                  {l.a.port && <span className="text-muted-foreground font-mono"> · {l.a.port}</span>}
                </TableCell>
                <TableCell>
                  {name(l.b.device)}
                  {l.b.port && <span className="text-muted-foreground font-mono"> · {l.b.port}</span>}
                </TableCell>
                <TableCell className="text-right font-mono tabular-nums">{speedLabel(l.speed) || '—'}</TableCell>
                <TableCell className="font-mono text-xs">
                  {l.vlan ? <span className="inline-flex items-center gap-1"><span className="size-2 rounded-full" style={{ background: vlanColor(l.vlan, topo.vlans) }} />{l.vlan}</span> : '—'}
                  {isTrunk(l) && <span className="text-muted-foreground"> + {l.tagged!.join(', ')}</span>}
                </TableCell>
                <TableCell>
                  <StatusBadge status={l.status} label={t(`topology.linkState.${l.status}`)} />
                </TableCell>
                <TableCell className="text-muted-foreground pr-4 text-xs">{t(`topology.source.${l.source}`)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
  )
}
