import { ChevronRight, CloudOff, Cpu, Plug, Server } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router'

import { AnimatedNumber } from '@/components/AnimatedNumber'
import { IntegrationIcon } from '@/components/IntegrationIcon'
import { MetricChart, MetricRangeToggle } from '@/components/MetricChart'
import { PageHeader } from '@/components/PageHeader'
import { StatusBadge } from '@/components/StatusBadge'
import { ServerHealth } from '@/components/ServerView'
import { SwitchFront } from '@/components/SwitchView'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { Integration, IntegrationType, Metric, MetricRange } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { formatMetric, isStateUnit, metricBase, metricLabel } from '@/lib/metrics'
import { rise } from '@/lib/motion'
import { useIntegrations, useIntegrationTypes } from '@/lib/queries'
import { cn } from '@/lib/utils'

const statusDot: Record<string, string> = { warning: 'bg-warning', error: 'bg-destructive' }

// Überschrift der Gruppen-Spalte je Integrationstyp
const groupTitle: Record<string, 'user' | 'datastore'> = { immich: 'user', pbs: 'datastore' }

/** Große Kennzahl; anklickbar, um sie im Diagramm zu zeigen. */
function Stat({ m, label, selected, onSelect }: { m: Metric; label: string; selected: boolean; onSelect: () => void }) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className={cn(
        'hover:bg-accent/40 flex flex-col items-start gap-1 rounded-lg border p-3 text-left transition-colors',
        selected && 'border-ring bg-accent/40',
      )}
    >
      <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
        {statusDot[m.status] && <span className={cn('size-2 rounded-full', statusDot[m.status])} aria-hidden />}
        <span className="inline-block first-letter:uppercase">{label}</span>
      </span>
      <span className="text-xl font-semibold tabular-nums">
        {isStateUnit(m.unit) ? formatMetric(m.value, m.unit) : <AnimatedNumber value={m.value} format={(v) => formatMetric(v, m.unit)} />}
      </span>
    </button>
  )
}

/** Kennzahlen je Gruppe (z. B. je Benutzer) als Tabelle. */
// Bekannte Arten gruppierter Kennzahlen (Anfang des Schlüssels) bekommen je
// eine eigene Tabelle; alles andere landet gemeinsam in einer.
const sections = ['storage', 'disk', 'zfs', 'fs', 'ups', 'user'] as const
type Section = (typeof sections)[number] | 'other'

function sectionOf(key: string): Section {
  const prefix = key.slice(0, key.indexOf('/'))
  return sections.find((s) => s === prefix) ?? 'other'
}

/** Gruppierte Kennzahlen, je Art eine Tabelle. */
function GroupTables({ type, metrics }: { type: string; metrics: Metric[] }) {
  const bySection = new Map<Section, Metric[]>()
  for (const m of metrics) {
    const s = sectionOf(m.key)
    bySection.set(s, [...(bySection.get(s) ?? []), m])
  }
  return [...bySection].map(([s, list]) => <Groups key={s} type={type} section={s} metrics={list} />)
}

function Groups({ type, section, metrics }: { type: string; section: Section; metrics: Metric[] }) {
  const { t } = useTranslation()
  const groups = [...new Set(metrics.map((m) => m.group!))]
  const columns = [...new Set(metrics.map((m) => metricBase(m.key)))]
  const cell = (g: string, c: string) => metrics.find((m) => m.group === g && metricBase(m.key) === c)
  return (
    <div className="overflow-x-auto rounded-md border">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead>
              {section === 'other'
                ? t(`services.groupTitle.${groupTitle[type] ?? 'group'}`)
                : t(`services.sections.${section}`)}
            </TableHead>
            {columns.map((c) => (
              <TableHead key={c} className="text-right">
                <span className="inline-block first-letter:uppercase">{metricLabel(type, metrics.find((m) => metricBase(m.key) === c)!)}</span>
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {groups.map((g) => (
            <TableRow key={g}>
              <TableCell className="font-medium">{g}</TableCell>
              {columns.map((c) => {
                const m = cell(g, c)
                return (
                  <TableCell
                    key={c}
                    className={cn(
                      'text-right tabular-nums',
                      m?.status === 'warning' && 'text-warning',
                      m?.status === 'error' && 'text-destructive',
                    )}
                  >
                    {m ? formatMetric(m.value, m.unit) : '—'}
                  </TableCell>
                )
              })}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

function ServiceCard({ integration: i, type }: { integration: Integration; type?: IntegrationType }) {
  const { t } = useTranslation()
  const now = useNow()
  // Werte einzelner VMs/Container stehen auf der Ressourcen-Seite
  const metrics = (i.snapshot?.metrics ?? []).filter((m) => !m.resource)
  // Ports zählen nicht einzeln (sie stehen in der Front des Switches)
  const resources = (i.snapshot?.resources ?? []).filter((r) => r.kind !== 'port').length
  const main = metrics.filter((m) => !m.group)
  const grouped = metrics.filter((m) => m.group)
  const switchPorts = (i.snapshot?.resources ?? []).filter((r) => r.kind === 'port')
  const server = i.snapshot?.resources?.find((r) => r.kind === 'server')
  const [selected, setSelected] = useState<string>()
  const [range, setRange] = useState<MetricRange>('30d')
  // Zustandswerte (Netz/Batterie, OK/Fehler) haben kein Diagramm
  const current = main.find((m) => m.key === selected) ?? main.find((m) => !isStateUnit(m.unit))
  const chartable = current && !isStateUnit(current.unit)
  const version = i.snapshot?.info?.version

  return (
    <Card id={`integration-${i.id}`} className={cn('scroll-mt-6', !i.enabled && 'opacity-60')} data-service={i.name}>
      <CardContent className="flex flex-col gap-5">
        <div className="flex flex-wrap items-center gap-3">
          <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg">
            <IntegrationIcon name={type?.icon} type={i.type} className="size-5" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-lg font-semibold">{i.name}</h2>
              <StatusBadge status={i.status} label={t(`integrations.status.${i.status}`)} />
            </div>
            <p className="text-muted-foreground text-sm">
              {type?.title ?? i.type}
              {version && ` · ${version}`}
              {i.last_poll_at && (
                <span title={absoluteTime(i.last_poll_at)}>
                  {' · '}
                  {t('integrations.lastPoll', { time: relativeTime(i.last_poll_at, now) })}
                </span>
              )}
            </p>
          </div>
        </div>

        {resources > 0 && (
          <Link to="/ressourcen" className="text-primary flex w-fit items-center gap-1 text-sm font-medium hover:underline">
            <Cpu className="size-4" /> {t('services.toResources', { count: resources })}
          </Link>
        )}

        {i.status === 'error' && i.last_error && (
          <p className="text-destructive flex items-start gap-2 text-sm">
            <CloudOff className="mt-0.5 size-4 shrink-0" aria-hidden />
            <span>
              {i.last_error}
              {i.last_success_at && metrics.length > 0 && (
                <span className="text-muted-foreground">
                  {' – '}
                  {t('services.staleSince', { time: relativeTime(i.last_success_at, now) })}
                </span>
              )}
            </span>
          </p>
        )}

        {metrics.length === 0 ? (
          <p className="text-muted-foreground text-sm">{t('services.noMetrics')}</p>
        ) : (
          <>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
              {main.map((m) => (
                <Stat
                  key={m.key}
                  m={m}
                  label={metricLabel(i.type, m)}
                  selected={m.key === current?.key}
                  onSelect={() => setSelected(m.key)}
                />
              ))}
            </div>
            {switchPorts.length > 0 && (
              <Link to="/ressourcen" className="block rounded-lg" aria-label={t('services.toResources', { count: resources })}>
                <SwitchFront ports={switchPorts} compact />
              </Link>
            )}
            {server && <ServerHealth metrics={(i.snapshot?.metrics ?? []).filter((m) => m.resource === server.id)} />}
            {chartable && current && (
              <div className="flex flex-col gap-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-sm font-medium">
                    {t('services.chartTitle', { metric: metricLabel(i.type, current) })}
                  </span>
                  <MetricRangeToggle value={range} onChange={setRange} />
                </div>
                <MetricChart
                  integrationId={i.id}
                  metricKey={current.key}
                  label={metricLabel(i.type, current)}
                  range={range}
                />
              </div>
            )}
            {grouped.length > 0 && <GroupTables type={i.type} metrics={grouped} />}
          </>
        )}
      </CardContent>
    </Card>
  )
}

function Empty() {
  const { t } = useTranslation()
  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
        <span className="bg-muted grid size-12 place-items-center rounded-lg">
          <Server className="text-muted-foreground size-6" />
        </span>
        <p className="font-medium">{t('services.emptyTitle')}</p>
        <p className="text-muted-foreground max-w-md text-sm">{t('services.empty')}</p>
        <Link to="/einstellungen" className="text-primary flex items-center gap-1 text-sm font-medium hover:underline">
          <Plug className="size-4" /> {t('checks.toSettings')}
        </Link>
      </CardContent>
    </Card>
  )
}

/** Healthchecks.io als Ampel: grün oder rot mit Anzahl, Details auf der Checks-Seite. */
function HealthchecksRow({ integration: i }: { integration: Integration }) {
  const { t } = useTranslation()
  const metrics = i.snapshot?.metrics ?? []
  const count = (key: string) => metrics.find((m) => m.key === key)?.value ?? 0
  const down = count('checks_down')
  const active = count('checks_total') - count('checks_paused')
  const unreachable = i.status === 'error'
  return (
    <Link
      to="/checks"
      id={`integration-${i.id}`}
      data-service={i.name}
      className="group bg-card hover:bg-accent/40 flex scroll-mt-6 items-center gap-4 rounded-xl border px-5 py-4 transition-[background-color,transform,box-shadow] duration-200 hover:-translate-y-0.5 hover:shadow-md"
    >
      <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg">
        <IntegrationIcon type="healthchecks" className="size-5" />
      </span>
      <div className="min-w-0 flex-1">
        <div className="font-semibold">{i.name}</div>
        <div className="flex items-center gap-2 text-sm">
          <span className="relative flex size-2.5" aria-hidden>
            {(down > 0 || unreachable) && <span className="bg-destructive/60 animate-ring absolute inset-0 rounded-full" />}
            <span className={cn('relative size-2.5 rounded-full', down > 0 || unreachable ? 'bg-destructive' : 'bg-success')} />
          </span>
          {unreachable
            ? i.last_error
            : down > 0
              ? t('dashboard.checksFailed', { count: down })
              : t('dashboard.checksAllOk', { count: active })}
        </div>
      </div>
      <ChevronRight className="text-muted-foreground size-4 transition-transform group-hover:translate-x-0.5" />
    </Link>
  )
}

export function Services() {
  const { t } = useTranslation()
  const integrations = useIntegrations()
  const types = useIntegrationTypes()
  const list = integrations.data ?? []
  const { hash } = useLocation()
  const loaded = integrations.isSuccess

  // Sprung von einer Dashboard-Kachel (#integration-<id>)
  useEffect(() => {
    if (loaded && hash) document.getElementById(hash.slice(1))?.scrollIntoView({ block: 'start' })
  }, [loaded, hash])

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('nav.services')} subtitle={t('services.subtitle')} />
      {integrations.isPending ? (
        <Skeleton className="h-80 rounded-xl" />
      ) : integrations.isError ? (
        <p className="text-destructive text-sm">{t('integrations.loadError', { error: integrations.error.message })}</p>
      ) : list.length === 0 ? (
        <Empty />
      ) : (
        // aktive zuerst
        [...list]
          .sort((a, b) => Number(b.enabled) - Number(a.enabled))
          .map((i, n) => {
            const motion = rise(n)
            return (
              <div key={i.id} className={motion.className} style={motion.style}>
                {i.type === 'healthchecks' ? (
                  <HealthchecksRow integration={i} />
                ) : (
                  <ServiceCard integration={i} type={types.data?.types.find((x) => x.type === i.type)} />
                )}
              </div>
            )
          })
      )}
    </div>
  )
}
