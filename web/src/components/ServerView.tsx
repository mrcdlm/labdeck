import {
  Cpu,
  Fan,
  HardDrive,
  MemoryStick,
  Network,
  PlugZap,
  Power,
  ServerCog,
  ShieldCheck,
  Thermometer,
  Zap,
  type LucideIcon,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Meter } from '@/components/Meter'
import { MetricChart, MetricRangeToggle } from '@/components/MetricChart'
import { StatusBadge } from '@/components/StatusBadge'
import { StatusDot } from '@/components/StatusTile'
import { Card, CardContent } from '@/components/ui/card'
import type { Integration, Metric, MetricRange, Resource } from '@/lib/api'
import { formatNumber } from '@/lib/format'
import { formatMetric } from '@/lib/metrics'
import { cn } from '@/lib/utils'

/*
 * Server-Hardware über den BMC (Redfish, z. B. iLO 5): Zustand je Komponente,
 * Temperaturen, Lüfter, Netzteile, Laufwerke. Alles kommt als Kennzahlen der
 * Ressource „server“; die Ansicht gruppiert nach dem Präfix des Schlüssels.
 */

const componentIcons: Record<string, LucideIcon> = {
  power_supplies: PlugZap,
  psu_redundancy: PlugZap,
  fans: Fan,
  fan_redundancy: Fan,
  temperatures: Thermometer,
  processors: Cpu,
  memory: MemoryStick,
  storage: HardDrive,
  smart_storage_battery: Zap,
  network: Network,
  bios_hardware: ShieldCheck,
}

const byPrefix = (metrics: Metric[], prefix: string) => metrics.filter((m) => m.key.startsWith(prefix))

/**
 * Zustand je Komponente als Raster. Kompakt (Dashboard-Kachel): eine Reihe
 * Punkte und „11 von 13 OK“ – die Probleme selbst stehen schon in der Kachel.
 */
export function ServerHealth({ metrics, compact = false }: { metrics: Metric[]; compact?: boolean }) {
  const { t } = useTranslation()
  const list = byPrefix(metrics, 'component/')
  if (list.length === 0) return null
  if (compact) {
    const ok = list.filter((m) => m.status === 'ok').length
    return (
      <div className="flex items-center gap-3" data-component-summary>
        <span className="flex flex-wrap gap-1" aria-hidden>
          {list.map((m) => (
            <span key={m.key} title={t(`metrics.redfish.${m.name}`, { defaultValue: m.name })}>
              <StatusDot status={m.status} />
            </span>
          ))}
        </span>
        <span className="text-muted-foreground text-xs">{t('server.componentsOk', { count: ok, total: list.length })}</span>
      </div>
    )
  }
  return (
    <ul
      className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4"
      aria-label={t('server.health')}
    >
      {list.map((m) => {
        const Icon = componentIcons[m.name] ?? ServerCog
        const label = t(`metrics.redfish.${m.name}`, { defaultValue: m.name })
        return (
          <li
            key={m.key}
            className={cn(
              'flex min-w-0 items-center gap-2 text-sm',
              'bg-card rounded-lg border px-3 py-2',
              m.status === 'error' && 'border-destructive/50',
              m.status === 'warning' && 'border-warning/60',
            )}
            data-component={m.name}
            data-status={m.status}
          >
            <StatusDot status={m.status} pulse />
            <Icon className="text-muted-foreground size-4 shrink-0" aria-hidden />
            <span className="truncate">{label}</span>
          </li>
        )
      })}
    </ul>
  )
}

/** Zeile mit Meter, anklickbar für den Verlauf. */
function SensorRow({
  m,
  pct,
  detail,
  selected,
  onSelect,
}: {
  m: Metric
  pct: number | undefined
  detail: string
  selected: boolean
  onSelect: () => void
}) {
  return (
    <li>
      <button
        type="button"
        onClick={onSelect}
        aria-pressed={selected}
        className={cn('hover:bg-accent/60 -mx-2 w-[calc(100%+1rem)] rounded-md px-2 py-1 text-left', selected && 'bg-accent')}
        data-sensor={m.group}
      >
        <Meter value={pct} size="sm" label={m.group} detail={detail} status={m.status} />
      </button>
    </li>
  )
}

/** Liste mit Zustand je Eintrag (Netzteile, Laufwerke). */
function StateList({ items, render }: { items: Metric[]; render: (m: Metric) => string | undefined }) {
  const { t } = useTranslation()
  return (
    <ul className="flex flex-col gap-1.5">
      {items.map((m) => (
        <li key={m.key} className="flex min-w-0 items-center gap-2 text-sm" data-item={m.group}>
          <StatusDot status={m.status} pulse />
          <span className="min-w-0 flex-1 truncate" title={m.group}>
            {m.group}
          </span>
          <span className="text-muted-foreground shrink-0 text-xs tabular-nums">
            {render(m) ?? (m.status === 'ok' ? t('units.ok') : t(`status.${m.status}`))}
          </span>
        </li>
      ))}
    </ul>
  )
}

function Block({ icon: Icon, title, children }: { icon: LucideIcon; title: string; children: ReactNode }) {
  return (
    <section className="flex min-w-0 flex-col gap-2">
      <h3 className="text-muted-foreground flex items-center gap-1.5 text-xs font-medium tracking-wide uppercase">
        <Icon className="size-3.5" aria-hidden />
        {title}
      </h3>
      {children}
    </section>
  )
}

/** Karte eines Servers: Kopf, Komponenten, Sensoren, Netzteile, Laufwerke. */
export function ServerCard({ integration, server }: { integration: Integration; server: Resource }) {
  const { t } = useTranslation()
  const all = (integration.snapshot?.metrics ?? []).filter((m) => m.resource === server.id)
  const main = integration.snapshot?.metrics ?? []
  const [selected, setSelected] = useState<string>()
  const [range, setRange] = useState<MetricRange>('24h')
  const temps = byPrefix(all, 'temp/')
  const fans = byPrefix(all, 'fan/')
  const psus = byPrefix(all, 'psu/').filter((m) => m.name === 'supply')
  const output = (group?: string) => all.find((m) => m.name === 'output' && m.group === group)
  const storage = all.filter((m) => /^(controller|drive|volume)\//.test(m.key) && m.name !== 'life')
  const life = (m: Metric) => all.find((x) => x.name === 'life' && x.key.startsWith(m.key.replace(/\/drive$/, '/')))
  const watts = main.find((m) => m.key === 'power')
  const current = all.find((m) => m.key === selected)
  const on = server.state === 'on'
  const info = server.info ?? {}

  return (
    <Card className="gap-4 py-5" data-server={server.name}>
      <CardContent className="flex flex-col gap-5">
        <div className="flex flex-wrap items-center gap-3">
          <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg">
            <ServerCog className="size-5" />
          </span>
          <div className="min-w-0 flex-1 basis-56">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-lg font-semibold">{server.name}</h2>
              <StatusBadge status={server.status} />
              <span
                className={cn(
                  'flex items-center gap-1 text-xs',
                  on ? 'text-muted-foreground' : 'text-warning font-medium',
                )}
              >
                <Power className="size-3.5" aria-hidden />
                {t(on ? 'server.on' : 'server.off')}
              </span>
            </div>
            <p className="text-muted-foreground text-sm">
              {[info.model, info.serial && `${t('server.serial')} ${info.serial}`, info.bios && `BIOS ${info.bios}`, info.bmc_firmware]
                .filter(Boolean)
                .join(' · ')}
            </p>
          </div>
          <dl className="flex flex-wrap gap-5 text-sm">
            {watts && (
              <div>
                <dt className="text-muted-foreground text-xs">{t('metrics.redfish.power')}</dt>
                <dd className="font-medium tabular-nums">{formatMetric(watts.value, watts.unit)}</dd>
              </div>
            )}
            {info.cpus && (
              <div className="max-w-56">
                <dt className="text-muted-foreground text-xs">{t('server.cpus')}</dt>
                <dd className="truncate font-medium" title={info.cpu_model}>
                  {info.cpus} × {(info.cpu_model ?? '').replace(/\(R\)|\(TM\)|CPU|@.*$/g, '').trim()}
                </dd>
              </div>
            )}
            {info.memory_gib && (
              <div>
                <dt className="text-muted-foreground text-xs">{t('server.memory')}</dt>
                <dd className="font-medium tabular-nums">{formatNumber(Number(info.memory_gib))} GiB</dd>
              </div>
            )}
          </dl>
        </div>

        <ServerHealth metrics={all} />

        <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-4">
          {temps.length > 0 && (
            <Block icon={Thermometer} title={t('server.temperatures')}>
              <ul className="flex flex-col gap-1">
                {temps.map((m) => {
                  const crit = Number(info[`temp_crit:${m.group}`]) || undefined
                  return (
                    <SensorRow
                      key={m.key}
                      m={m}
                      pct={crit ? (m.value / crit) * 100 : undefined}
                      detail={crit ? `${formatMetric(m.value, m.unit)} / ${formatMetric(crit, m.unit)}` : formatMetric(m.value, m.unit)}
                      selected={selected === m.key}
                      onSelect={() => setSelected((s) => (s === m.key ? undefined : m.key))}
                    />
                  )
                })}
              </ul>
            </Block>
          )}
          {fans.length > 0 && (
            <Block icon={Fan} title={t('server.fans')}>
              <ul className="flex flex-col gap-1">
                {fans.map((m) => (
                  <SensorRow
                    key={m.key}
                    m={m}
                    pct={m.unit === '%' ? m.value : undefined}
                    detail={formatMetric(m.value, m.unit)}
                    selected={selected === m.key}
                    onSelect={() => setSelected((s) => (s === m.key ? undefined : m.key))}
                  />
                ))}
              </ul>
            </Block>
          )}
          {psus.length > 0 && (
            <Block icon={PlugZap} title={t('server.psus')}>
              <StateList items={psus} render={(m) => {
                const o = output(m.group)
                return m.status === 'ok' && o ? formatMetric(o.value, o.unit) : undefined
              }} />
            </Block>
          )}
          {storage.length > 0 && (
            <Block icon={HardDrive} title={t('server.storage')}>
              <StateList items={storage} render={(m) => {
                const l = life(m)
                return m.status === 'ok' && l ? t('server.lifeLeft', { value: formatNumber(Math.round(l.value)) }) : undefined
              }} />
            </Block>
          )}
        </div>

        {current && (
          <div className="flex flex-col gap-2" data-sensor-history={current.group}>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="text-sm font-medium">{t('services.chartTitle', { metric: current.group })}</span>
              <MetricRangeToggle value={range} onChange={setRange} />
            </div>
            <MetricChart integrationId={integration.id} metricKey={current.key} label={current.group ?? current.name} range={range} />
          </div>
        )}
      </CardContent>
    </Card>
  )
}
