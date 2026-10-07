import { useId } from 'react'
import { Area, AreaChart, ResponsiveContainer, YAxis } from 'recharts'

import type { MetricRange } from '@/lib/api'
import { reducedMotion } from '@/lib/motion'
import { useMetricHistory } from '@/lib/queries'

/**
 * Mini-Verlauf einer Kennzahl (ohne Achsen und Tooltip) für Kacheln. Erst ab
 * zwei Punkten sichtbar – vorher nimmt er keinen Platz ein.
 */
export function Sparkline({
  integrationId,
  metricKey,
  range,
}: {
  integrationId: number
  metricKey: string
  range: MetricRange
}) {
  const id = useId()
  const history = useMetricHistory(integrationId, metricKey, range)
  const points = (history.data?.points ?? []).map((p) => ({
    t: new Date(p.t).getTime(),
    v: p.v,
  }))
  if (points.length < 2) return null
  const values = points.map((p) => p.v)
  const lo = Math.min(...values)
  const hi = Math.max(...values)
  // (fast) gerade Linie sagt nichts
  if (hi - lo <= Math.abs(hi) * 0.01) return null
  const pad = (hi - lo) * 0.1
  const fill = `spark-${id.replace(/:/g, '')}`

  return (
    <div className="pointer-events-none -mx-1 mt-1 h-8" aria-hidden data-sparkline={metricKey}>
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={points} margin={{ top: 2, right: 0, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id={fill} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="var(--chart-1)" stopOpacity={0.25} />
              <stop offset="100%" stopColor="var(--chart-1)" stopOpacity={0} />
            </linearGradient>
          </defs>
          <YAxis hide domain={[lo - pad, hi + pad]} />
          <Area
            type="monotone"
            dataKey="v"
            stroke="var(--chart-1)"
            strokeWidth={1.5}
            fill={`url(#${fill})`}
            dot={false}
            isAnimationActive={!reducedMotion()}
            animationDuration={800}
            animationEasing="ease-out"
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}
