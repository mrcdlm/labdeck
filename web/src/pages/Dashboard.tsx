import { ChevronRight, Plug } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { PageHeader } from '@/components/PageHeader'
import { StatusBadge } from '@/components/StatusBadge'

/** Gesamtstatus: solange keine Integration eingerichtet ist, ein Hinweis mit Weg dorthin. */
function OverallStatus() {
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

export function Dashboard() {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('nav.dashboard')} subtitle={t('dashboard.subtitle')} />
      <OverallStatus />
    </div>
  )
}
