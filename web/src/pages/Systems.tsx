import { ChevronRight, Plug, Search, Server } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { IntegrationIcon } from '@/components/IntegrationIcon'
import { PageHeader } from '@/components/PageHeader'
import { ProblemCount, StatusDot } from '@/components/StatusTile'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import type { Integration, IntegrationType, Metric, Problem, Status } from '@/lib/api'
import { anchorId } from '@/lib/anchor'
import { formatMetric, isStateUnit, metricLabel } from '@/lib/metrics'
import { rise } from '@/lib/motion'
import { statusRank, systemLink, worstStatus } from '@/lib/problems'
import { useIntegrations, useIntegrationTypes, useOverview } from '@/lib/queries'
import { cn } from '@/lib/utils'

const categories = ['compute', 'network', 'storage', 'power', 'apps', 'monitoring'] as const
type Category = (typeof categories)[number]

function metricText(type: string, m: Metric) {
  return isStateUnit(m.unit) ? formatMetric(m.value, m.unit) : `${formatMetric(m.value, m.unit)} ${metricLabel(type, m)}`
}

/** Kompakte Karte: Name, Status bzw. Anzahl der Probleme, Typ und Kernzahlen. */
function SystemCard({
  integration: i,
  type,
  problems,
  status,
}: {
  integration: Integration
  type?: IntegrationType
  problems: Problem[]
  status: Status
}) {
  const { t } = useTranslation()
  const main = (i.snapshot?.metrics ?? []).filter((m) => !m.group && !m.resource)
  const version = i.snapshot?.info?.version
  return (
    <Link
      to={systemLink(i)}
      className={cn(
        'group bg-card hover:bg-accent/40 relative flex h-full flex-col gap-2 overflow-hidden rounded-xl border p-4',
        'transition-[background-color,transform,box-shadow] duration-200 hover:-translate-y-0.5 hover:shadow-md',
        'before:absolute before:inset-y-0 before:left-0 before:w-1',
        status === 'error' && 'before:bg-destructive',
        status === 'warning' && 'before:bg-warning',
        !i.enabled && 'opacity-60',
      )}
      data-system={i.name}
      data-tone={status}
    >
      <div className="flex items-center gap-3">
        <span className="bg-muted grid size-9 shrink-0 place-items-center rounded-lg">
          <IntegrationIcon name={type?.icon} type={i.type} className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate font-medium">{i.name}</span>
            {!i.enabled ? (
              <Badge variant="outline" className="shrink-0 font-normal">
                {t('systems.disabled')}
              </Badge>
            ) : problems.length > 0 ? (
              <ProblemCount count={problems.length} status={status} />
            ) : (
              <StatusDot status={status} />
            )}
          </div>
          <p className="text-muted-foreground truncate text-xs">
            {type?.title ?? i.type}
            {version && ` · ${version}`}
          </p>
        </div>
        <ChevronRight className="text-muted-foreground size-4 shrink-0 transition-transform group-hover:translate-x-0.5" />
      </div>
      {i.enabled && i.status === 'error' && i.last_error ? (
        <p className="text-destructive line-clamp-2 text-sm">{i.last_error}</p>
      ) : (
        main.length > 0 && (
          <p className="text-muted-foreground text-sm tabular-nums">
            {main
              .slice(0, 3)
              .map((m) => metricText(i.type, m))
              .join(' · ')}
          </p>
        )
      )}
    </Link>
  )
}

interface Hit {
  key: string
  to: string
  name: string
  kind?: string
  status: Status
  source: Integration
}

/** Suche über Integrationen und ihre Ressourcen (Hosts, VMs, Container, Ports, Server). */
function search(list: Integration[], q: string): Hit[] {
  const needle = q.trim().toLowerCase()
  if (!needle) return []
  const match = (...xs: (string | undefined)[]) => xs.some((x) => x?.toLowerCase().includes(needle))
  const hits: Hit[] = []
  for (const i of list) {
    if (match(i.name, i.type)) {
      hits.push({ key: `i/${i.id}`, to: systemLink(i), name: i.name, status: i.status, source: i })
    }
    for (const r of i.snapshot?.resources ?? []) {
      if (!match(r.name, r.info?.vmid, r.info?.image, r.info?.alias, r.info?.model, ...(r.tags ?? []))) continue
      hits.push({
        key: `r/${i.id}/${r.id}`,
        to: `/systeme/${i.id}#${anchorId('res', r.id)}`,
        name: r.info?.alias ? `${r.name} · ${r.info.alias}` : r.name,
        kind: r.kind,
        status: r.status,
        source: i,
      })
    }
  }
  return hits.slice(0, 50)
}

function SearchResults({ hits, icons }: { hits: Hit[]; icons: Record<string, string | undefined> }) {
  const { t } = useTranslation()
  if (hits.length === 0) return <p className="text-muted-foreground text-sm">{t('systems.noHits')}</p>
  return (
    <ul className="bg-card divide-y overflow-hidden rounded-xl border" data-search-results>
      {hits.map((h) => (
        <li key={h.key}>
          <Link to={h.to} className="group hover:bg-accent/40 flex items-center gap-3 px-4 py-2.5" data-hit={h.name}>
            <StatusDot status={h.status} />
            <span className="min-w-0 flex-1">
              <span className="flex items-center gap-2">
                <span className="truncate text-sm font-medium">{h.name}</span>
                {h.kind && (
                  <Badge variant="outline" className="px-1.5 py-0 text-[11px] font-normal">
                    {t(`resources.kind.${h.kind}`, { defaultValue: h.kind })}
                  </Badge>
                )}
              </span>
              {h.kind && (
                <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
                  <IntegrationIcon name={icons[h.source.type]} type={h.source.type} className="size-3" />
                  {h.source.name}
                </span>
              )}
            </span>
            <ChevronRight className="text-muted-foreground size-4 shrink-0 transition-transform group-hover:translate-x-0.5" />
          </Link>
        </li>
      ))}
    </ul>
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

/**
 * Alle Integrationen nach Bereich gruppiert, Probleme zuerst. Jede Karte führt
 * zur Detailseite mit allem, was die Integration liefert.
 */
export function Systems() {
  const { t } = useTranslation()
  const integrations = useIntegrations()
  const types = useIntegrationTypes()
  const overview = useOverview()
  const [query, setQuery] = useState('')
  const list = integrations.data ?? []
  const typeOf = (i: Integration) => types.data?.types.find((x) => x.type === i.type)
  const icons = Object.fromEntries((types.data?.types ?? []).map((x) => [x.type, x.icon]))

  const problemsOf = (i: Integration) => (overview.data?.problems ?? []).filter((p) => p.integration_id === i.id)
  const statusOf = (i: Integration): Status => {
    if (!i.enabled) return 'paused'
    return worstStatus([...problemsOf(i).map((p) => p.status), i.status === 'error' ? 'error' : undefined]) ?? i.status
  }
  const byCategory = new Map<Category, Integration[]>()
  for (const i of list) {
    const c = categories.find((x) => x === typeOf(i)?.category) ?? 'monitoring'
    byCategory.set(c, [...(byCategory.get(c) ?? []), i])
  }
  // Probleme zuerst, deaktivierte zuletzt, sonst nach Name
  const order = (a: Integration, b: Integration) =>
    Number(b.enabled) - Number(a.enabled) ||
    Math.min(statusRank(statusOf(a)), 2) - Math.min(statusRank(statusOf(b)), 2) ||
    a.name.localeCompare(b.name, undefined, { numeric: true })
  const hits = search(list, query)

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('nav.systems')} subtitle={t('systems.subtitle')} />
      {integrations.isPending ? (
        <Skeleton className="h-64 rounded-xl" />
      ) : integrations.isError ? (
        <p className="text-destructive text-sm">{t('integrations.loadError', { error: integrations.error.message })}</p>
      ) : list.length === 0 ? (
        <Empty />
      ) : (
        <>
          <div className="relative w-full sm:w-80">
            <Search className="text-muted-foreground absolute top-2.5 left-2.5 size-4" aria-hidden />
            <Input
              className="pl-8"
              type="search"
              placeholder={t('systems.search')}
              aria-label={t('systems.search')}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>
          {query.trim() ? (
            <SearchResults hits={hits} icons={icons} />
          ) : (
            categories
              .filter((c) => byCategory.has(c))
              .map((c, n) => {
                const motion = rise(n)
                return (
                  <section
                    key={c}
                    className={cn('flex flex-col gap-3', motion.className)}
                    style={motion.style}
                    aria-labelledby={`cat-${c}`}
                    data-category={c}
                  >
                    <h2 id={`cat-${c}`} className="text-muted-foreground text-sm font-medium">
                      {t(`systems.category.${c}`)}
                    </h2>
                    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                      {[...byCategory.get(c)!].sort(order).map((i) => (
                        <SystemCard
                          key={i.id}
                          integration={i}
                          type={typeOf(i)}
                          problems={i.enabled ? problemsOf(i) : []}
                          status={statusOf(i)}
                        />
                      ))}
                    </div>
                  </section>
                )
              })
          )}
        </>
      )}
    </div>
  )
}
