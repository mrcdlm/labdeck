import { Pencil, Plug, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IntegrationDialog } from '@/components/IntegrationDialog'
import { IntegrationIcon } from '@/components/IntegrationIcon'
import { Section } from '@/components/Section'
import { StatusBadge } from '@/components/StatusBadge'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import type { Integration } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { inputFromIntegration } from '@/lib/integrations'
import {
  useDeleteIntegration,
  useIntegrations,
  useIntegrationTypes,
  usePollIntegration,
  useSaveIntegration,
} from '@/lib/queries'

/** Status einer Integration als Badge: verbunden / Fehler / noch nicht abgefragt / deaktiviert. */
export function IntegrationStatus({ integration }: { integration: Integration }) {
  const { t } = useTranslation()
  return <StatusBadge status={integration.status} label={t(`integrations.status.${integration.status}`)} />
}

function IntegrationItem({
  integration,
  typeTitle,
  icon,
  onEdit,
  onDelete,
}: {
  integration: Integration
  typeTitle: string
  icon?: string
  onEdit: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const save = useSaveIntegration()
  const poll = usePollIntegration()
  const now = useNow()
  const snap = integration.snapshot
  const counts: string[] = []
  if (snap?.checks?.length) counts.push(t('integrations.count.checks', { count: snap.checks.length }))
  if (snap?.backups?.length) counts.push(t('integrations.count.backups', { count: snap.backups.length }))

  return (
    <li className="flex flex-col gap-2 px-3 py-3" data-integration={integration.name}>
      <div className="flex flex-wrap items-center gap-2">
        <Switch
          checked={integration.enabled}
          aria-label={t('integrations.enabledLabel', { name: integration.name })}
          disabled={save.isPending}
          onCheckedChange={(on) =>
            save.mutate({ id: integration.id, input: inputFromIntegration(integration, { enabled: on }) })
          }
        />
        <span className="bg-muted grid size-7 shrink-0 place-items-center rounded-md">
          <IntegrationIcon name={icon} className="size-4" />
        </span>
        <span className="min-w-0">
          <span className="block leading-tight font-medium">{integration.name}</span>
          <span className="text-muted-foreground block text-xs">{typeTitle}</span>
        </span>
        <IntegrationStatus integration={integration} />
        <div className="ml-auto flex gap-1">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => poll.mutate(integration.id)}
            disabled={poll.isPending || !integration.enabled}
          >
            <RefreshCw className={poll.isPending ? 'animate-spin' : undefined} />
            {t('integrations.pollNow')}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('integrations.editLabel', { name: integration.name })}
            onClick={onEdit}
          >
            <Pencil />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('integrations.deleteLabel', { name: integration.name })}
            onClick={onDelete}
          >
            <Trash2 />
          </Button>
        </div>
      </div>
      <p className="text-xs">
        {integration.status === 'error' && integration.last_error ? (
          <span className="text-destructive break-words">{integration.last_error}</span>
        ) : integration.last_poll_at ? (
          <span className="text-muted-foreground" title={absoluteTime(integration.last_poll_at)}>
            {t('integrations.lastPoll', { time: relativeTime(integration.last_poll_at, now) })}
            {counts.length > 0 && ` · ${counts.join(' · ')}`}
          </span>
        ) : (
          <span className="text-muted-foreground">{t('integrations.notPolled')}</span>
        )}
      </p>
      {(save.isError || poll.isError) && (
        <p className="text-destructive text-xs break-words" role="alert">
          {(save.error ?? poll.error)?.message}
        </p>
      )}
    </li>
  )
}

export function IntegrationsSection() {
  const { t } = useTranslation()
  const integrations = useIntegrations()
  const types = useIntegrationTypes()
  const del = useDeleteIntegration()
  const [dialog, setDialog] = useState<{ open: boolean; integration?: Integration }>({ open: false })
  const [toDelete, setToDelete] = useState<Integration>()
  const list = integrations.data ?? []
  const typeOf = (type: string) => types.data?.types.find((ty) => ty.type === type)

  return (
    <Section
      icon={<Plug className="size-5" />}
      title={t('integrations.title')}
      description={t('integrations.subtitle')}
      action={
        <Button type="button" variant="outline" size="sm" onClick={() => setDialog({ open: true })}>
          <Plus /> {t('integrations.add')}
        </Button>
      }
    >
      {integrations.isPending ? (
        <Skeleton className="h-16" />
      ) : integrations.isError ? (
        <p className="text-destructive text-sm">{t('integrations.loadError', { error: integrations.error.message })}</p>
      ) : list.length === 0 ? (
        <p className="text-muted-foreground text-sm">{t('integrations.empty')}</p>
      ) : (
        <ul className="divide-y rounded-md border">
          {list.map((i) => (
            <IntegrationItem
              key={i.id}
              integration={i}
              typeTitle={typeOf(i.type)?.title ?? i.type}
              icon={typeOf(i.type)?.icon}
              onEdit={() => setDialog({ open: true, integration: i })}
              onDelete={() => setToDelete(i)}
            />
          ))}
        </ul>
      )}

      <IntegrationDialog
        open={dialog.open}
        integration={dialog.integration}
        onOpenChange={(open) => setDialog((d) => ({ ...d, open }))}
      />

      <AlertDialog open={toDelete !== undefined} onOpenChange={(open) => !open && setToDelete(undefined)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('integrations.deleteTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('integrations.deleteHint', { name: toDelete?.name })}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault()
                if (toDelete) del.mutate(toDelete.id, { onSuccess: () => setToDelete(undefined) })
              }}
              disabled={del.isPending}
            >
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Section>
  )
}
