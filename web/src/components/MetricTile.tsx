import { ArrowDownRight, ArrowUpRight, Gauge as GaugeIcon } from 'lucide-react'
import { useId, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
  type TooltipContentProps,
} from 'recharts'
import type { NameType, ValueType } from 'recharts/types/component/DefaultTooltipContent'

import { AnimatedNumber } from '@/components/AnimatedNumber'
import { IntegrationIcon } from '@/components/IntegrationIcon'
import { StatusTile, TileValue } from '@/components/StatusTile'
import type { DashboardTile, Integration, MetricHistory, Status, TileRange } from '@/lib/api'
import { axisTick, tickFormatter } from '@/lib/chart'
import { absoluteTime } from '@/lib/format'
import { formatMetric, formatMetricShort, isStateUnit, metricTitle } from '@/lib/metrics'
import { reducedMotion } from '@/lib/motion'
import { systemLink, worstStatus } from '@/lib/problems'
import { useMetricHistory } from '@/lib/queries'
import { cn } from '@/lib/utils'

/*
 * Eigene Kennzahl-Kachel: eine bis drei Kennzahlen einer Integration als Zahl,
 * Sparkline, Flächendiagramm oder Tacho. Zeitraum 1 h = live: die Verläufe
 * laden bei jeder Abfrage neu (SSE → Query-Invalidierung), ohne neu zu
 * animieren; Zahlen gleiten zum neuen Wert.
 *
 * Reihenfarben: --chart-1…3 (validiert, siehe index.css). Ab zwei Reihen gibt
 * es immer eine Legende mit den aktuellen Werten – die Farbe trägt nie allein.
 */

const series = ['var(--chart-1)', 'var(--chart-2)', 'var(--chart-3)']
const statusColor: Record<Status, string> = {
  ok: 'var(--success)',
  warning: 'var(--warning)',
  error: 'var(--destructive)',
  unknown: 'var(--muted-foreground)',
  paused: 'var(--muted-foreground)',
}

type Row = { t: number } & Record<string, number | undefined>

/** Verläufe mehrerer Kennzahlen zu Zeilen je Zeitpunkt zusammenführen. */
function mergeRows(histories: (MetricHistory | undefined)[]): Row[] {
  const rows = new Map<number, Row>()
  histories.forEach((h, n) => {
    for (const p of h?.points ?? []) {
      const t = new Date(p.t).getTime()
      const row = rows.get(t) ?? { t }
      row[`s${n}`] = p.v
      rows.set(t, row)
    }
  })
  return [...rows.values()].sort((a, b) => a.t - b.t)
}

function ChartTooltip({
  active,
  payload,
  label: x,
  names,
  unit,
}: TooltipContentProps<ValueType, NameType> & { names: string[]; unit?: string }) {
  if (!active || !payload?.length) return null
  return (
    <div className="bg-popover text-popover-foreground rounded-md border px-3 py-2 text-xs shadow-md">
      <div className="text-muted-foreground mb-1">{absoluteTime(new Date(Number(x)).toISOString())}</div>
      {payload.map((p) => {
        const n = Number(String(p.dataKey).slice(1))
        return (
          <div key={String(p.dataKey)} className="flex items-center gap-2">
            <span className="h-0.5 w-3 rounded-full" style={{ background: series[n] }} aria-hidden />
            <span className="max-w-48 truncate">{names[n]}</span>
            <span className="ml-auto pl-3 font-medium tabular-nums">{formatMetric(Number(p.value), unit)}</span>
          </div>
        )
      })}
    </div>
  )
}

/** Flächendiagramm (mit Achsen) bzw. Sparkline (ohne) für 1–3 Reihen. */
function Chart({
  d0,
  d1,
  d2,
  count,
  names,
  unit,
  range,
  axes,
  live,
}: {
  d0?: MetricHistory
  d1?: MetricHistory
  d2?: MetricHistory
  count: number
  names: string[]
  unit?: string
  range: TileRange
  axes: boolean
  live: boolean
}) {
  const id = useId().replace(/:/g, '')
  // gleiches Array, solange die Daten gleich sind (sonst startet Recharts die Animation neu)
  const rows = useMemo(() => mergeRows([d0, d1, d2].slice(0, count)), [d0, d1, d2, count])
  const animate = !live && !reducedMotion()
  return (
    <ResponsiveContainer width="100%" height="100%">
      <AreaChart data={rows} margin={axes ? { top: 6, right: 6, bottom: 0, left: 0 } : { top: 2, right: 0, bottom: 0, left: 0 }}>
        <defs>
          {series.slice(0, count).map((c, n) => (
            <linearGradient key={n} id={`${id}-${n}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={c} stopOpacity={count > 1 ? 0.12 : 0.25} />
              <stop offset="100%" stopColor={c} stopOpacity={0} />
            </linearGradient>
          ))}
        </defs>
        {axes && <CartesianGrid vertical={false} stroke="var(--border)" />}
        <XAxis
          dataKey="t"
          type="number"
          scale="time"
          domain={['dataMin', 'dataMax']}
          hide={!axes}
          tickFormatter={tickFormatter(range)}
          tick={axisTick}
          tickLine={false}
          axisLine={{ stroke: 'var(--border)' }}
          minTickGap={40}
        />
        <YAxis
          hide={!axes}
          tick={axisTick}
          tickLine={false}
          axisLine={false}
          width={72}
          domain={['auto', 'auto']}
          tickFormatter={(v: number) => formatMetricShort(v, unit)}
        />
        <Tooltip
          content={(p) => <ChartTooltip {...p} names={names} unit={unit} />}
          cursor={{ stroke: 'var(--muted-foreground)', strokeDasharray: '3 3' }}
        />
        {series.slice(0, count).map((c, n) => (
          <Area
            key={n}
            type="monotone"
            dataKey={`s${n}`}
            stroke={c}
            strokeWidth={2}
            fill={`url(#${id}-${n})`}
            dot={false}
            activeDot={{ r: 4, strokeWidth: 2, stroke: 'var(--card)' }}
            connectNulls
            isAnimationActive={animate}
            animationDuration={700}
          />
        ))}
      </AreaChart>
    </ResponsiveContainer>
  )
}

/** Halbkreis-Tacho in Statusfarbe; Wert und Skala als Text daneben. */
function Gauge({ value, min, max, status, label }: { value: number; min: number; max: number; status: Status; label: string }) {
  const pct = Math.max(0, Math.min(1, (value - min) / (max - min || 1)))
  const r = 40
  const len = Math.PI * r
  return (
    <svg viewBox="0 0 100 56" className="h-full max-h-36 w-full" role="meter" aria-label={label}
      aria-valuemin={min} aria-valuemax={max} aria-valuenow={value}>
      <path d="M10 50 A40 40 0 0 1 90 50" fill="none" stroke="var(--muted)" strokeWidth={9} strokeLinecap="round" />
      <path
        d="M10 50 A40 40 0 0 1 90 50"
        fill="none"
        stroke={statusColor[status]}
        strokeWidth={9}
        strokeLinecap="round"
        strokeDasharray={`${len * pct} ${len}`}
        className="transition-[stroke-dasharray] duration-700 ease-out"
      />
    </svg>
  )
}

/** Live-Punkt: pulsiert, solange der Zeitraum „1 h“ ist. */
function LiveDot() {
  const { t } = useTranslation()
  return (
    <span className="text-muted-foreground flex items-center gap-1.5 text-xs" title={t('tile.liveHint')}>
      <span className="relative flex size-2">
        <span className="bg-success/60 absolute inset-0 rounded-full motion-safe:animate-ping" />
        <span className="bg-success relative size-2 rounded-full" />
      </span>
      {t('live.on')}
    </span>
  )
}

export function MetricTile({ tile, integration: i, icon }: { tile: DashboardTile; integration?: Integration; icon?: string }) {
  const { t } = useTranslation()
  const o = tile.options!
  const keys = o.keys.slice(0, 3)
  // feste Anzahl Hooks (höchstens drei Kennzahlen)
  const h0 = useMetricHistory(o.integration_id, keys[0], o.range)
  const h1 = useMetricHistory(o.integration_id, keys[1], o.range)
  const h2 = useMetricHistory(o.integration_id, keys[2], o.range)
  const histories = [h0, h1, h2].slice(0, keys.length)

  const type = i?.type ?? ''
  const metrics = keys.map((k) => i?.snapshot?.metrics?.find((m) => m.key === k))
  const names = keys.map((k, n) => (metrics[n] ? metricTitle(type, metrics[n]) : k))
  const first = metrics[0]
  const unit = first?.unit ?? histories[0].data?.unit
  const status: Status = !i ? 'unknown' : i.status === 'error' ? 'error' : (worstStatus(metrics.map((m) => m?.status)) ?? 'unknown')
  const title = o.label || (keys.length === 1 ? names[0] : (i?.name ?? ''))
  const live = o.range === '1h'
  // nur für „genug Punkte?“ – das Diagramm bildet seine Zeilen selbst (stabil per useMemo)
  const rows = mergeRows(histories.map((h) => h.data))

  // Veränderung gegenüber dem Beginn des Zeitraums (nur eine Kennzahl)
  const pts = histories[0].data?.points ?? []
  const delta = first && pts.length > 1 && !isStateUnit(unit) ? first.value - pts[0].v : undefined

  const value = !first ? (
    <span className="text-muted-foreground font-medium">{t('tile.noData')}</span>
  ) : isStateUnit(first.unit) ? (
    <TileValue>{formatMetric(first.value, first.unit)}</TileValue>
  ) : (
    <TileValue>
      <AnimatedNumber value={first.value} format={(v) => formatMetric(v, first.unit)} />
    </TileValue>
  )

  const deltaLine =
    delta !== undefined && Math.abs(delta) > 1e-9 ? (
      <span className="flex items-center gap-1">
        {delta > 0 ? <ArrowUpRight className="size-3.5" aria-hidden /> : <ArrowDownRight className="size-3.5" aria-hidden />}
        {`${delta > 0 ? '+' : '−'}${formatMetric(Math.abs(delta), unit)}`}
        <span>· {t(`tile.since.${o.range}`)}</span>
      </span>
    ) : (
      <span>{t(`tile.range.${o.range}`)}</span>
    )

  const legend =
    keys.length > 1 ? (
      <ul className="mt-1 flex flex-wrap gap-x-4 gap-y-0.5 text-xs" aria-label={t('tile.legend')}>
        {keys.map((k, n) => (
          <li key={k} className="flex min-w-0 items-center gap-1.5">
            <span className="h-0.5 w-3 shrink-0 rounded-full" style={{ background: series[n] }} aria-hidden />
            <span className="text-muted-foreground truncate">{names[n]}</span>
            {metrics[n] && <span className="font-medium tabular-nums">{formatMetric(metrics[n].value, metrics[n].unit)}</span>}
          </li>
        ))}
      </ul>
    ) : null

  const chartable = rows.length >= 2 && !isStateUnit(unit)
  let body = null
  switch (o.view) {
    case 'sparkline':
    case 'area':
      body = chartable ? (
        <div className={cn('mt-2 min-h-0 flex-1', o.view === 'sparkline' && 'pointer-events-none -mx-1')} data-chart={o.view}>
          <Chart d0={h0.data} d1={h1.data} d2={h2.data} count={keys.length} names={names} unit={unit} range={o.range} axes={o.view === 'area'} live={live} />
        </div>
      ) : (
        <p className="text-muted-foreground mt-2 text-xs">{t('services.chartTooFew')}</p>
      )
      break
    case 'gauge': {
      const lo = o.min ?? 0
      const hi = o.max ?? (unit === '%' ? 100 : Math.max(...pts.map((p) => p.v), first?.value ?? 0, 1) * 1.2)
      body = first ? (
        <div className="mt-1 flex min-h-0 flex-1 flex-col items-center justify-end" data-chart="gauge">
          <Gauge value={first.value} min={lo} max={hi} status={first.status} label={names[0]} />
          <div className="text-muted-foreground flex w-full justify-between px-1 text-xs tabular-nums">
            <span>{formatMetricShort(lo, unit)}</span>
            <span>{formatMetricShort(hi, unit)}</span>
          </div>
        </div>
      ) : null
      break
    }
  }

  return (
    <StatusTile
      to={i ? systemLink(i) : '/systeme'}
      icon={i ? <IntegrationIcon name={icon} type={i.type} className="size-4" /> : <GaugeIcon className="size-4" />}
      title={title}
      tile={tile.id}
      status={status}
      stretch={o.view === 'number' || o.view === 'gauge'}
      action={live ? <LiveDot /> : undefined}
      value={o.view === 'area' && keys.length > 1 ? undefined : value}
      sub={keys.length === 1 ? deltaLine : undefined}
    >
      {legend}
      {body}
    </StatusTile>
  )
}
