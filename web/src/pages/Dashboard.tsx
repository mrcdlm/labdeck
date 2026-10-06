import { AlertTriangle, ChevronRight, Plug, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { PageHeader } from '@/components/PageHeader'
import { StatusBadge } from '@/components/StatusBadge'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { Integration } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { useIntegrations } from '@/lib/queries'
import { cn } from '@/lib/utils'

/** Hinweis, solange keine Integration eingerichtet ist. */
function NoIntegrations() {
  const { t } = useTranslation()
  return (
    <Link
      to="/einstellungen"
      className="group bg-card hover:bg-accent/40 flex items-center gap-4 rounded-xl border p-5 transition-colors"
    >
      <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg">
        <Plug className="size-5" />
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{t('dashboard.noIntegrations')}</span>
          <StatusBadge status="unknown" />
        </div>
        <p className="text-muted-foreground mt-1 text-sm">{t('dashboard.noIntegrationsHint')}</p>
      </div>
      <ChevronRight className="text-muted-foreground size-4 transition-transform group-hover:translate-x-0.5" />
    </Link>
  )
}

/** Gesamtstatus: alles in Ordnung oder Liste der Probleme (ganz oben). */
function OverallStatus({ integrations }: { integrations: Integration[] }) {
  const { t } = useTranslation()
  const now = useNow()
  const active = integrations.filter((i) => i.enabled)
  const problems = active.filter((i) => i.status === 'error')
  const ok = problems.length === 0
  const Icon = ok ? ShieldCheck : AlertTriangle

  return (
    <Card className={cn(!ok && 'border-destructive/40')}>
      <CardContent className="flex flex-col gap-4">
        <div className="flex items-center gap-4">
          <span
            className={cn(
              'grid size-10 shrink-0 place-items-center rounded-lg',
              ok ? 'bg-success/15 text-success' : 'bg-destructive/15 text-destructive',
            )}
          >
            <Icon className="size-5" />
          </span>
          <div className="min-w-0">
            <p className="text-lg font-semibold">
              {ok ? t('dashboard.allGood') : t('dashboard.problems', { count: problems.length })}
            </p>
            <p className="text-muted-foreground text-sm">
              {t('dashboard.activeIntegrations', { count: active.length })}
            </p>
          </div>
        </div>
        {problems.length > 0 && (
          <ul className="divide-y rounded-md border">
            {problems.map((i) => (
              <li key={i.id}>
                <Link
                  to="/einstellungen"
                  className="hover:bg-accent/40 flex items-center gap-3 px-3 py-2.5 transition-colors"
                >
                  <StatusBadge status="error" />
                  <span className="min-w-0 flex-1">
                    <span className="block font-medium">{i.name}</span>
                    <span className="text-muted-foreground block text-xs break-words">
                      {i.last_error}
                      {i.last_success_at && (
                        <span title={absoluteTime(i.last_success_at)}>
                          {' · '}
                          {t('dashboard.lastSuccess', { time: relativeTime(i.last_success_at, now) })}
                        </span>
                      )}
                    </span>
                  </span>
                  <ChevronRight className="text-muted-foreground size-4" />
                </Link>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

export function Dashboard() {
  const { t } = useTranslation()
  const integrations = useIntegrations()
  const list = integrations.data ?? []

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('nav.dashboard')} subtitle={t('dashboard.subtitle')} />
      {integrations.isPending ? (
        <Skeleton className="h-24 rounded-xl" />
      ) : integrations.isError ? (
        <p className="text-destructive text-sm">{t('integrations.loadError', { error: integrations.error.message })}</p>
      ) : list.length === 0 ? (
        <NoIntegrations />
      ) : (
        <OverallStatus integrations={list} />
      )}
    </div>
  )
}
