import { useTranslation } from 'react-i18next'

import { Segmented } from '@/components/Segmented'
import { uptimeRanges, type Uptime, type UptimeRange } from '@/lib/api'
import { absoluteTime } from '@/lib/format'
import { bucketColor, formatPercent, mergeBuckets } from '@/lib/uptime'
import { cn } from '@/lib/utils'

interface Props {
  uptime: Uptime
  range: UptimeRange
  /** Schmale Variante für Tabellen: Prozent neben dem Balken, ohne Überschrift */
  compact?: boolean
}

/** Uptime-Balken aus gleich langen Abschnitten (aus dnsdeck übernommen). */
export function UptimeBar({ uptime, range, compact }: Props) {
  const { t } = useTranslation()
  const start = new Date(uptime.start).getTime()
  // Schmale Balken: höchstens 30 Abschnitte, sonst wirken sie streifig
  const factor = compact ? Math.ceil(uptime.buckets.length / 30) : 1
  const buckets = mergeBuckets(uptime.buckets, factor)
  const step = uptime.bucket_seconds * 1000 * factor
  const title = t('uptime.title', { range: t(`uptime.range.${range}`) })

  const bar = (
    <div
      className={cn('flex gap-px', compact ? 'h-4 min-w-24 flex-1' : 'h-6')}
      role="img"
      aria-label={`${title}: ${formatPercent(uptime.percent)}`}
    >
      {buckets.map((b, i) => {
        const from = new Date(start + i * step).toISOString()
        const to = new Date(start + (i + 1) * step).toISOString()
        return (
          <div
            key={i}
            className={cn('min-w-0 flex-1', compact ? 'rounded-[1px]' : 'rounded-[2px]', bucketColor[b])}
            title={`${absoluteTime(from)} – ${absoluteTime(to)}: ${t(`uptime.bucket.${b}`)}`}
          />
        )
      })}
    </div>
  )

  if (compact) {
    return (
      <div className="flex items-center gap-2">
        {bar}
        <span className="w-14 shrink-0 text-right text-xs font-medium tabular-nums">{formatPercent(uptime.percent)}</span>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between text-xs">
        <span className="text-muted-foreground">{title}</span>
        <span className="font-medium tabular-nums">{formatPercent(uptime.percent)}</span>
      </div>
      {bar}
      {uptime.observed < 0.99 && <span className="text-muted-foreground text-xs">{t('uptime.gaps')}</span>}
    </div>
  )
}

/** Auswahl des Zeitraums (24 h / 7 Tage / 30 Tage). */
export function RangeToggle({ value, onChange }: { value: UptimeRange; onChange: (r: UptimeRange) => void }) {
  const { t } = useTranslation()
  return (
    <Segmented
      label={t('uptime.period')}
      value={value}
      onChange={onChange}
      options={uptimeRanges.map((r) => ({ value: r, label: t(`uptime.range.${r}`) }))}
    />
  )
}
