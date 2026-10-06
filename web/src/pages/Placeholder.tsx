import type { LucideIcon } from 'lucide-react'
import { DatabaseBackup, HeartPulse, History, Server } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/PageHeader'
import { Card, CardContent } from '@/components/ui/card'

type PageKey = 'backups' | 'checks' | 'services' | 'history'

const icons: Record<PageKey, LucideIcon> = {
  backups: DatabaseBackup,
  checks: HeartPulse,
  services: Server,
  history: History,
}

/** Seiten, die in einer späteren Version gefüllt werden. */
export function Placeholder({ page }: { page: PageKey }) {
  const { t } = useTranslation()
  const Icon = icons[page]
  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t(`nav.${page}`)} subtitle={t(`${page}.subtitle`)} />
      <Card>
        <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
          <span className="bg-muted grid size-12 place-items-center rounded-lg">
            <Icon className="text-muted-foreground size-6" />
          </span>
          <p className="font-medium">{t('placeholder.title')}</p>
          <p className="text-muted-foreground max-w-md text-sm">{t(`${page}.empty`)}</p>
        </CardContent>
      </Card>
    </div>
  )
}
