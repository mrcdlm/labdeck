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

import { Segmented } from '@/components/Segmented'
import { Skeleton } from '@/components/ui/skeleton'
import { metricRanges, type MetricRange } from '@/lib/api'
import { currentLang } from '@/i18n'
import { absoluteTime } from '@/lib/format'
import { axisTick, tickFormatter } from '@/lib/chart'
import { formatMetric, formatMetricShort } from '@/lib/metrics'
import { reducedMotion } from '@/lib/motion'
import { useMetricHistory } from '@/lib/queries'


/** Auswahl des Zeitraums der Kennzahl-Diagramme. */
export function MetricRangeToggle({ value, onChange }: { value: MetricRange; onChange: (r: MetricRange) => void }) {
  const { t } = useTranslation()
  return (
    <Segmented
      label={t('uptime.period')}
      value={value}
      onChange={onChange}
      options={metricRanges.map((r) => ({ value: r, label: t(`services.range.${r}`) }))}
    />
  )
}

function ChartTooltip({
  active,
  payload,
  label: x,
  name,
  unit,
  daily,
}: TooltipContentProps<ValueType, NameType> & { name: string; unit?: string; daily: boolean }) {
  if (!active || !payload?.length) return null
  return (
    <div className="bg-popover text-popover-foreground rounded-md border px-3 py-2 text-xs shadow-md">
      <div className="text-muted-foreground mb-1">
        {daily ? new Date(Number(x)).toLocaleDateString(currentLang()) : absoluteTime(new Date(Number(x)).toISOString())}
      </div>
      <div className="flex items-center gap-2">
        <span className="bg-chart-1 h-0.5 w-3 rounded-full" aria-hidden />
        <span>{name}</span>
        <span className="ml-auto pl-3 font-medium tabular-nums">{formatMetric(Number(payload[0].value), unit)}</span>
      </div>
    </div>
  )
}

/** Verlauf einer Kennzahl als Flächendiagramm. */
export function MetricChart({
  integrationId,
  metricKey,
  label,
  range,
}: {
  integrationId: number
  metricKey: string
  label: string
  range: MetricRange
}) {
  const { t } = useTranslation()
  const history = useMetricHistory(integrationId, metricKey, range)

  if (history.isPending) return <Skeleton className="h-48" />
  if (history.isError) {
    return <p className="text-destructive py-6 text-sm">{t('services.chartError', { error: history.error.message })}</p>
  }
  const h = history.data
  const points = h.points.map((p) => ({ t: new Date(p.t).getTime(), v: p.v }))
  if (points.length < 2) {
    return <p className="text-muted-foreground py-10 text-center text-sm">{t('services.chartTooFew')}</p>
  }
  const daily = range === '30d' || range === '1y'
  // Zählwerte nur ganzzahlig; fast konstante Werte mit etwas Abstand (sonst 5× dieselbe Zahl)
  const isCount = !h.unit
  const values = points.map((p) => p.v)
  const lo = Math.min(...values)
  const hi = Math.max(...values)
  const flat = hi - lo < (isCount ? 4 : Math.abs(hi) * 0.02)
  const pad = flat ? (isCount ? 2 : Math.max(Math.abs(hi) * 0.05, 1)) : 0
  const domain: [number, number] = [Math.max(0, lo - pad), hi + pad]

  return (
    <div className="h-48" role="img" aria-label={t('services.chartLabel', { metric: label })}>
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={points} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id={`fill-${integrationId}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="var(--chart-1)" stopOpacity={0.25} />
              <stop offset="100%" stopColor="var(--chart-1)" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid vertical={false} stroke="var(--border)" />
          <XAxis
            dataKey="t"
            type="number"
            scale="time"
            domain={['dataMin', 'dataMax']}
            tickFormatter={tickFormatter(range)}
            tick={axisTick}
            tickLine={false}
            axisLine={{ stroke: 'var(--border)' }}
            minTickGap={40}
          />
          <YAxis
            tick={axisTick}
            tickLine={false}
            axisLine={false}
            width={64}
            domain={domain}
            allowDecimals={!isCount}
            tickFormatter={(v: number) => formatMetricShort(v, h.unit)}
          />
          <Tooltip
            content={(p) => <ChartTooltip {...p} name={label} unit={h.unit} daily={daily} />}
            cursor={{ stroke: 'var(--muted-foreground)', strokeDasharray: '3 3' }} />
          <Area
            type="monotone"
            dataKey="v"
            stroke="var(--chart-1)"
            strokeWidth={2}
            fill={`url(#fill-${integrationId})`}
            dot={points.length <= 31 ? { r: 2.5, strokeWidth: 0, fill: 'var(--chart-1)' } : false}
            activeDot={{ r: 5, strokeWidth: 2, stroke: 'var(--card)' }}
            isAnimationActive={!reducedMotion()}
            animationDuration={800}
            animationEasing="ease-out"
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}
