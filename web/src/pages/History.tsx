import { History as HistoryIcon, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { HistoryList } from '@/components/HistoryList'
import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import type { HistoryFilter, HistoryKind } from '@/lib/api'
import { useHistory, useIntegrations } from '@/lib/queries'

// Radix-Select erlaubt keinen leeren Wert.
const ALL = 'all'
const kinds: HistoryKind[] = ['integration', 'check', 'backup', 'metric']

export function History() {
  const { t } = useTranslation()
  const integrations = useIntegrations()
  const [filter, setFilter] = useState<HistoryFilter>({})
  const history = useHistory(filter)
  const entries = history.data?.pages.flatMap((p) => p.entries) ?? []

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('nav.history')} subtitle={t('history.subtitle')} />

      <div className="flex flex-wrap items-end gap-4">
        <div className="grid gap-1.5">
          <Label htmlFor="history-integration">{t('history.integration')}</Label>
          <Select
            value={filter.integration ? String(filter.integration) : ALL}
            onValueChange={(v) => setFilter((f) => ({ ...f, integration: v === ALL ? undefined : Number(v) }))}
          >
            <SelectTrigger id="history-integration" className="w-56">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t('history.all')}</SelectItem>
              {(integrations.data ?? []).map((i) => (
                <SelectItem key={i.id} value={String(i.id)}>
                  {i.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="history-kind">{t('history.kind')}</Label>
          <Select
            value={filter.kind ?? ALL}
            onValueChange={(v) => setFilter((f) => ({ ...f, kind: v === ALL ? undefined : (v as HistoryKind) }))}
          >
            <SelectTrigger id="history-kind" className="w-44">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t('history.all')}</SelectItem>
              {kinds.map((k) => (
                <SelectItem key={k} value={k}>
                  {t(`history.kinds.${k}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex h-9 items-center gap-2">
          <Switch
            id="history-problems"
            checked={!!filter.problems}
            onCheckedChange={(on) => setFilter((f) => ({ ...f, problems: on || undefined }))}
          />
          <Label htmlFor="history-problems">{t('history.problemsOnly')}</Label>
        </div>
      </div>

      <Card className="py-0">
        {history.isPending ? (
          <div className="flex flex-col gap-2 p-6">
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
          </div>
        ) : history.isError ? (
          <p className="text-destructive p-6 text-sm">{t('history.loadError', { error: history.error.message })}</p>
        ) : entries.length === 0 ? (
          <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
            <span className="bg-muted grid size-12 place-items-center rounded-lg">
              <HistoryIcon className="text-muted-foreground size-6" />
            </span>
            <p className="text-muted-foreground max-w-md text-sm">{t('history.empty')}</p>
          </CardContent>
        ) : (
          <HistoryList entries={entries} />
        )}
      </Card>

      {history.hasNextPage && (
        <Button
          variant="outline"
          className="w-fit self-center"
          onClick={() => history.fetchNextPage()}
          disabled={history.isFetchingNextPage}
        >
          {history.isFetchingNextPage && <Loader2 className="animate-spin" />}
          {t('history.more')}
        </Button>
      )}
    </div>
  )
}
