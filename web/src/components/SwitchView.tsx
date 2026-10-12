import { ArrowDown, ArrowUp, Network, X } from 'lucide-react'
import { useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { MetricChart, MetricRangeToggle } from '@/components/MetricChart'
import { Segmented } from '@/components/Segmented'
import { StatusBadge } from '@/components/StatusBadge'
import { StatusDot } from '@/components/StatusTile'
import { Card, CardContent } from '@/components/ui/card'
import type { Integration, MetricRange, Resource } from '@/lib/api'
import { formatBits, formatNumber, formatUptime } from '@/lib/format'
import { cn } from '@/lib/utils'

/*
 * Schematische Front eines Switches – herstellerneutral: Bauform (RJ45/SFP),
 * Einheit und Position kommen aus den Daten der Integration (ENTITY-MIB,
 * Portnamen oder Frontlayout). Ein 8-Port-Switch, ein 48+4-Switch und ein
 * Stack werden mit demselben Code gezeichnet.
 */

const PW = 22 // Breite eines Ports
const PH = { rj45: 17, sfp: 14 } // Höhe je Bauform
const GX = 3 // Abstand zwischen Spalten
const GY = 5 // Abstand zwischen den Reihen
const GB = 14 // Abstand zwischen Blöcken
const PAD = 10 // Rand des Gehäuses
const LABEL = 10 // Platz für Portnummern

type Media = 'rj45' | 'sfp'
interface Block {
  media: Media
  ports: Resource[]
}
interface Unit {
  unit: number
  blocks: Block[]
}

const num = (s?: string) => Number(s ?? 0) || 0

/** Ports je Einheit nach Position, zusammenhängende gleiche Bauform als Block. */
function frontModel(ports: Resource[]) {
  const phys = ports.filter((p) => p.info?.media !== 'lag')
  const lags = ports.filter((p) => p.info?.media === 'lag')
  const units: Unit[] = []
  const sorted = [...phys].sort(
    (a, b) => num(a.info?.unit) - num(b.info?.unit) || num(a.info?.pos) - num(b.info?.pos),
  )
  for (const p of sorted) {
    const unit = num(p.info?.unit)
    const media: Media = p.info?.media === 'sfp' ? 'sfp' : 'rj45'
    let u = units.at(-1)
    if (!u || u.unit !== unit) {
      u = { unit, blocks: [] }
      units.push(u)
    }
    const b = u.blocks.at(-1)
    if (b && b.media === media) b.ports.push(p)
    else u.blocks.push({ media, ports: [p] })
  }
  return { units, lags }
}

/** Datenrate eines Ports in bit/s (Ressource: Bytes/s). */
const bits = (v?: number) => (v ?? 0) * 8

function portTone(p: Resource) {
  if (p.state === 'disabled') return 'disabled'
  if (p.status === 'error') return 'error'
  if (p.state !== 'up') return 'down'
  return p.status === 'warning' ? 'warning' : 'up'
}

const toneColor = {
  up: 'var(--success)',
  warning: 'var(--warning)',
  error: 'var(--destructive)',
  down: 'var(--muted-foreground)',
  disabled: 'var(--muted-foreground)',
} as const

function usePortText() {
  const { t } = useTranslation()
  return (p: Resource) => {
    const parts = [p.name + (p.info?.alias ? ` · ${p.info.alias}` : '')]
    const speed = num(p.info?.speed)
    parts.push(
      [t(`switch.state.${p.state === 'up' ? 'up' : p.state === 'disabled' ? 'disabled' : 'down'}`), speed && p.state === 'up' ? speedLabel(speed) : '']
        .filter(Boolean)
        .join(' · '),
    )
    if (p.net_in !== undefined) parts.push(`↓ ${formatBits(bits(p.net_in))}  ↑ ${formatBits(bits(p.net_out))}`)
    if (p.info?.monitored) parts.push(t('switch.monitored'))
    return parts.join('\n')
  }
}

/** Geschwindigkeit in Mbit/s kurz: 1000 → „1G“, 100 → „100M“. */
function speedLabel(mbit: number) {
  return mbit >= 1000 ? `${formatNumber(mbit / 1000)}G` : `${mbit}M`
}

function Port({
  p,
  x,
  y,
  media,
  bottom,
  compact,
  selected,
  onSelect,
  title,
}: {
  p: Resource
  x: number
  y: number
  media: Media
  bottom: boolean
  compact: boolean
  selected: boolean
  onSelect?: (id: string) => void
  title: string
}) {
  const tone = portTone(p)
  const h = PH[media]
  const color = toneColor[tone]
  const util = Math.min(num(p.info?.util) / 100, 1)
  const active = tone === 'up' || tone === 'warning'
  const fill = active ? color : tone === 'error' ? 'var(--destructive)' : 'var(--muted)'
  const fillOpacity = active ? 0.22 + 0.6 * util : tone === 'error' ? 0.12 : 1
  const traffic = bits(p.net_in) + bits(p.net_out) > 1e6
  // RJ45: Rastnase als Kerbe – oben in der unteren Reihe, unten in der oberen (wie am Gerät)
  const notch = media === 'rj45'
  const ny = bottom ? y : y + h - 4
  const select = () => onSelect?.(p.id)
  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      select()
    }
  }
  return (
    <g
      role={onSelect ? 'button' : undefined}
      tabIndex={onSelect ? 0 : undefined}
      aria-label={title.replaceAll('\n', ', ')}
      aria-pressed={onSelect ? selected : undefined}
      onClick={onSelect ? select : undefined}
      onKeyDown={onSelect ? onKey : undefined}
      className={cn('outline-none', onSelect && 'cursor-pointer [&:focus-visible>rect:first-of-type]:stroke-[var(--ring)]')}
      data-port={p.name}
      data-tone={tone}
      opacity={tone === 'disabled' ? 0.55 : 1}
    >
      <title>{title}</title>
      <rect
        x={x}
        y={y}
        width={PW}
        height={h}
        rx={2}
        fill={fill}
        fillOpacity={fillOpacity}
        stroke={selected ? 'var(--foreground)' : tone === 'error' ? 'var(--destructive)' : 'var(--border)'}
        strokeWidth={selected ? 1.6 : 1}
        className="transition-[fill-opacity] duration-700"
      />
      {notch && <rect x={x + PW / 2 - 4} y={ny} width={8} height={4} fill="var(--card)" stroke="var(--border)" strokeWidth={0.6} />}
      {media === 'sfp' && (
        <rect x={x + 4} y={y + 3} width={PW - 8} height={h - 6} rx={1} fill="none" stroke="var(--border)" strokeWidth={0.8} />
      )}
      {/* LED */}
      {tone !== 'down' && tone !== 'disabled' && (
        <rect
          x={x + 2}
          y={bottom ? y + h - 4 : y + 1.5}
          width={5}
          height={2.5}
          rx={0.8}
          fill={color}
          className={cn(active && traffic && 'motion-safe:animate-blink')}
        />
      )}
      {tone === 'disabled' && (
        <line x1={x + 3} y1={y + 3} x2={x + PW - 3} y2={y + h - 3} stroke="var(--muted-foreground)" strokeWidth={1.2} />
      )}
      {!compact && (
        <text
          x={x + PW / 2}
          y={bottom ? y + h + 8 : y - 3}
          textAnchor="middle"
          fontSize={7.5}
          fill="var(--muted-foreground)"
          className="tabular-nums select-none"
        >
          {p.info?.pos}
        </text>
      )}
    </g>
  )
}

/** Front eines Switches als SVG; Klick auf einen Port wählt ihn aus. */
export function SwitchFront({
  ports,
  compact = false,
  selected,
  onSelect,
}: {
  ports: Resource[]
  compact?: boolean
  selected?: string
  onSelect?: (id: string) => void
}) {
  const { t } = useTranslation()
  const text = usePortText()
  const { units, lags } = frontModel(ports)
  const label = compact ? 0 : LABEL
  const rowH = (b: Block) => PH[b.media]
  const unitH = (u: Unit) => Math.max(...u.blocks.map(rowH)) * 2 + GY + 2 * label
  const blockW = (b: Block) => Math.ceil(b.ports.length / 2) * (PW + GX) - GX
  const unitW = (u: Unit) => u.blocks.reduce((w, b, n) => w + blockW(b) + (n ? GB : 0), 0)
  const stack = units.length > 1
  const tag = stack ? 18 : 0 // Einheitennummer links
  const width = Math.max(...units.map(unitW), 60) + 2 * PAD + tag
  // Einheiten untereinander: Oberkante = Summe der Höhen davor
  const layout = units.map((u, n) => ({
    u,
    top: units.slice(0, n).reduce((y, prev) => y + unitH(prev) + 2 * PAD + 6, 0),
  }))
  const height = units.reduce((y, u) => y + unitH(u) + 2 * PAD + 6, 0) - 6

  if (units.length === 0) return <p className="text-muted-foreground text-sm">{t('switch.noPorts')}</p>

  return (
    <div className="flex flex-col gap-2">
      <div className="-mx-1 overflow-x-auto px-1 pb-1">
        <svg
          viewBox={`0 0 ${width} ${height}`}
          className="block w-full"
          style={{ minWidth: compact ? undefined : width, maxWidth: width * (compact ? 1.3 : 1.9) }}
          role="group"
          aria-label={t('switch.front')}
        >
          {layout.map(({ u, top }) => {
            const h = unitH(u)
            // linke Kante je Block: Breiten der Blöcke davor
            const left = (bi: number) => PAD + tag + u.blocks.slice(0, bi).reduce((w, b) => w + blockW(b) + GB, 0)
            return (
              <g key={u.unit} data-unit={u.unit}>
                <rect x={0.5} y={top + 0.5} width={width - 1} height={h + 2 * PAD - 1} rx={6} fill="var(--card)" stroke="var(--border)" />
                {stack && (
                  <text x={PAD} y={top + PAD + h / 2 + 3} fontSize={9} fill="var(--muted-foreground)" className="select-none">
                    {u.unit}
                  </text>
                )}
                {u.blocks.map((b, bi) => {
                  const bx = left(bi)
                  const ph = PH[b.media]
                  const rowTop = top + PAD + label + (Math.max(...u.blocks.map(rowH)) - ph) // gleiche Unterkante je Reihe
                  return (
                    <g key={bi} data-media={b.media}>
                      {b.ports.map((p, k) => {
                        const bottom = k % 2 === 1
                        return (
                          <Port
                            key={p.id}
                            p={p}
                            x={bx + Math.floor(k / 2) * (PW + GX)}
                            y={bottom ? rowTop + ph + GY : rowTop}
                            media={b.media}
                            bottom={bottom}
                            compact={compact}
                            selected={selected === p.id}
                            onSelect={onSelect}
                            title={text(p)}
                          />
                        )
                      })}
                    </g>
                  )
                })}
              </g>
            )
          })}
        </svg>
      </div>
      {lags.length > 0 && !compact && (
        <ul className="flex flex-wrap gap-1.5" aria-label={t('switch.lags')}>
          {lags.map((p) => (
            <li key={p.id}>
              <button
                type="button"
                onClick={() => onSelect?.(p.id)}
                title={text(p)}
                className={cn(
                  'hover:bg-accent flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs',
                  selected === p.id && 'border-foreground',
                )}
                data-port={p.name}
              >
                <StatusDot status={p.state === 'up' ? p.status : p.status === 'error' ? 'error' : 'unknown'} />
                <span className="font-medium">{p.name}</span>
                {p.info?.alias && <span className="text-muted-foreground">{p.info.alias}</span>}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/** Verlauf eines Ports: Traffic rein/raus und Fehler. */
function PortDetail({ integration, port, onClose }: { integration: Integration; port: Resource; onClose: () => void }) {
  const { t } = useTranslation()
  const keys = (integration.snapshot?.metrics ?? []).filter((m) => m.resource === port.id && m.unit !== 'okfail')
  const [metric, setMetric] = useState(keys[0]?.name ?? 'in')
  const [range, setRange] = useState<MetricRange>('24h')
  const current = keys.find((k) => k.name === metric) ?? keys[0]
  const speed = num(port.info?.speed)
  return (
    <div className="bg-muted/40 flex flex-col gap-3 rounded-lg border p-4" data-port-detail={port.name}>
      <div className="flex flex-wrap items-start gap-x-4 gap-y-2">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium">{port.name}</span>
            <StatusBadge
              status={port.state === 'up' ? port.status : port.status === 'error' ? 'error' : 'unknown'}
              label={t(`switch.state.${port.state === 'up' ? 'up' : port.state === 'disabled' ? 'disabled' : 'down'}`)}
            />
            {port.info?.monitored && <span className="text-muted-foreground text-xs">{t('switch.monitored')}</span>}
          </div>
          <p className="text-muted-foreground mt-0.5 text-sm">
            {[
              port.info?.alias,
              t(`switch.media.${port.info?.media === 'sfp' ? 'sfp' : port.info?.media === 'lag' ? 'lag' : 'rj45'}`),
              speed && port.state === 'up' ? speedLabel(speed) : '',
              port.info?.util ? t('switch.util', { value: formatNumber(num(port.info.util)) }) : '',
            ]
              .filter(Boolean)
              .join(' · ')}
          </p>
        </div>
        {port.net_in !== undefined && (
          <div className="flex gap-4 text-sm tabular-nums">
            <span>
              <ArrowDown className="inline size-3.5" aria-hidden /> {formatBits(bits(port.net_in))}
            </span>
            <span>
              <ArrowUp className="inline size-3.5" aria-hidden /> {formatBits(bits(port.net_out))}
            </span>
          </div>
        )}
        <button
          type="button"
          onClick={onClose}
          className="text-muted-foreground hover:text-foreground -m-1 p-1"
          aria-label={t('common.close')}
        >
          <X className="size-4" />
        </button>
      </div>
      {current ? (
        <>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Segmented
              label={t('resources.metric')}
              value={current.name}
              onChange={setMetric}
              options={keys.map((k) => ({ value: k.name, label: t(`switch.metric.${k.name}`, { defaultValue: k.name }) }))}
            />
            <MetricRangeToggle value={range} onChange={setRange} />
          </div>
          <MetricChart
            integrationId={integration.id}
            metricKey={current.key}
            label={`${port.name} · ${t(`switch.metric.${current.name}`, { defaultValue: current.name })}`}
            range={range}
          />
        </>
      ) : (
        <p className="text-muted-foreground text-sm">{t('switch.noTraffic')}</p>
      )}
    </div>
  )
}

/** Karte eines Switches: Kopf, Front, Ports mit Problemen, Details zum gewählten Port. */
export function SwitchCard({ integration, sw }: { integration: Integration; sw: Resource }) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState<string>()
  const ports = (integration.snapshot?.resources ?? []).filter((r) => r.kind === 'port' && r.parent === sw.name)
  const port = ports.find((p) => p.id === selected)
  const problems = ports.filter((p) => p.status === 'error' || p.status === 'warning')
  const total = num(sw.info?.ports_total)
  const up = num(sw.info?.ports_up)
  const sum = (k: 'net_in' | 'net_out') =>
    ports.filter((p) => p.info?.media !== 'lag').reduce((s, p) => s + bits(p[k]), 0)
  const haveRates = ports.some((p) => p.net_in !== undefined)
  return (
    <Card className="gap-4 py-5" data-switch={sw.name}>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center gap-3">
          <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg">
            <Network className="size-5" />
          </span>
          <div className="min-w-0 flex-1 basis-56">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-lg font-semibold">{sw.name}</h2>
              <StatusBadge status={sw.status} />
            </div>
            <p className="text-muted-foreground text-sm">
              {[
                sw.info?.model ?? integration.name,
                sw.info?.firmware && `${t('switch.firmware')} ${sw.info.firmware}`,
                sw.uptime ? t('resources.uptime', { time: formatUptime(sw.uptime) }) : '',
              ]
                .filter(Boolean)
                .join(' · ')}
            </p>
          </div>
          <dl className="flex gap-5 text-sm">
            <div>
              <dt className="text-muted-foreground text-xs">{t('switch.portsUp')}</dt>
              <dd className="font-medium tabular-nums">
                {up} / {total}
              </dd>
            </div>
            {haveRates && (
              <div>
                <dt className="text-muted-foreground text-xs">{t('switch.traffic')}</dt>
                <dd className="font-medium whitespace-nowrap tabular-nums">
                  <ArrowDown className="inline size-3.5" aria-hidden /> {formatBits(sum('net_in'))}{' '}
                  <ArrowUp className="inline size-3.5" aria-hidden /> {formatBits(sum('net_out'))}
                </dd>
              </div>
            )}
          </dl>
        </div>
        <SwitchFront ports={ports} selected={selected} onSelect={(id) => setSelected((s) => (s === id ? undefined : id))} />
        <Legend />
        {problems.length > 0 && (
          <ul className="flex flex-wrap gap-1.5" aria-label={t('switch.problems')}>
            {problems.map((p) => (
              <li key={p.id}>
                <button
                  type="button"
                  onClick={() => setSelected(p.id)}
                  className="hover:bg-accent flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs"
                >
                  <StatusDot status={p.status} />
                  <span className="font-medium">{p.name}</span>
                  {p.info?.alias && <span className="text-muted-foreground">{p.info.alias}</span>}
                  <span className="text-muted-foreground">
                    {p.state !== 'up'
                      ? t('switch.state.down')
                      : p.info?.errors && num(p.info.errors) > 0
                        ? t('switch.errors', { count: num(p.info.errors) })
                        : t('switch.util', { value: formatNumber(num(p.info?.util)) })}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
        {port && <PortDetail key={port.id} integration={integration} port={port} onClose={() => setSelected(undefined)} />}
      </CardContent>
    </Card>
  )
}

function Legend() {
  const { t } = useTranslation()
  const item = (color: string, label: string, opacity = 1) => (
    <span className="flex items-center gap-1.5">
      <span className="inline-block h-2 w-3 rounded-sm" style={{ background: color, opacity }} aria-hidden />
      {label}
    </span>
  )
  return (
    <div className="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs">
      {item('var(--success)', t('switch.legend.up'))}
      {item('var(--warning)', t('switch.legend.warning'))}
      {item('var(--destructive)', t('switch.legend.monitoredDown'))}
      {item('var(--muted-foreground)', t('switch.legend.down'), 0.35)}
      <span>{t('switch.legend.fill')}</span>
    </div>
  )
}
