import { CloudOff, Plug, Server } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router'

import { IntegrationIcon } from '@/components/IntegrationIcon'
import { MetricChart, MetricRangeToggle } from '@/components/MetricChart'
import { PageHeader } from '@/components/PageHeader'
import { StatusBadge } from '@/components/StatusBadge'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { Integration, IntegrationType, Metric, MetricRange } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { formatMetric, metricBase, metricLabel } from '@/lib/metrics'
import { useIntegrations, useIntegrationTypes } from '@/lib/queries'
import { cn } from '@/lib/utils'

const statusDot: Record<string, string> = { warning: 'bg-warning', error: 'bg-destructive' }

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
      <span className="text-xl font-semibold tabular-nums">{formatMetric(m.value, m.unit)}</span>
    </button>
  )
}

/** Kennzahlen je Gruppe (z. B. je Benutzer) als Tabelle. */
function Groups({ type, metrics }: { type: string; metrics: Metric[] }) {
  const { t } = useTranslation()
  const groups = [...new Set(metrics.map((m) => m.group!))]
  const columns = [...new Set(metrics.map((m) => metricBase(m.key)))]
  const cell = (g: string, c: string) => metrics.find((m) => m.group === g && metricBase(m.key) === c)
  return (
    <div className="overflow-x-auto rounded-md border">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead>{t(`services.groupTitle.${type === 'immich' ? 'user' : 'group'}`)}</TableHead>
            {columns.map((c) => (
              <TableHead key={c} className="text-right">
                <span className="inline-block first-letter:uppercase">{metricLabel(type, { key: c, name: c })}</span>
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
  const metrics = i.snapshot?.metrics ?? []
  const main = metrics.filter((m) => !m.group)
  const grouped = metrics.filter((m) => m.group)
  const [selected, setSelected] = useState<string>()
  const [range, setRange] = useState<MetricRange>('30d')
  const current = main.find((m) => m.key === selected) ?? main[0]
  const version = i.snapshot?.info?.version

  return (
    <Card id={`integration-${i.id}`} className={cn('scroll-mt-6', !i.enabled && 'opacity-60')} data-service={i.name}>
      <CardContent className="flex flex-col gap-5">
        <div className="flex flex-wrap items-center gap-3">
          <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg">
            <IntegrationIcon name={type?.icon} className="size-5" />
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
            {current && (
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
            {grouped.length > 0 && <Groups type={i.type} metrics={grouped} />}
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
          .map((i) => (
            <ServiceCard key={i.id} integration={i} type={types.data?.types.find((x) => x.type === i.type)} />
          ))
      )}
    </div>
  )
}
