import { AlertTriangle, Check, KeyRound, Loader2, Pencil, Plus, Send, Trash2, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { Section } from '@/components/Section'
import { WebhookDialog } from '@/components/WebhookDialog'
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
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import type { Webhook } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { useDeleteWebhook, useSaveWebhook, useTestWebhook, useWebhooks } from '@/lib/queries'
import { webhookInput } from '@/lib/webhooks'

// Aus dnsdeck übernommen; zusätzlich Hinweis, wenn ${TOKEN} ohne Token verwendet wird.

function WebhookCard({ webhook, onEdit, onDelete }: { webhook: Webhook; onEdit: () => void; onDelete: () => void }) {
  const { t } = useTranslation()
  const test = useTestWebhook()
  const save = useSaveWebhook()
  const now = useNow()
  const { id, missing_env, last_error, last_sent_at } = webhook

  // Testergebnis nur kurz zeigen – danach gilt wieder der gespeicherte Status.
  const { reset: resetTest, data: testData } = test
  useEffect(() => {
    if (!testData) return
    const timer = setTimeout(resetTest, 8000)
    return () => clearTimeout(timer)
  }, [testData, resetTest])

  return (
    <li className="flex flex-col gap-2 px-3 py-3" data-webhook={webhook.name}>
      <div className="flex flex-wrap items-center gap-2">
        <Switch
          checked={webhook.enabled}
          aria-label={t('webhooks.activeLabel', { name: webhook.name })}
          disabled={save.isPending}
          onCheckedChange={(on) => save.mutate({ id, input: { ...webhookInput(webhook), enabled: on } })}
        />
        <span className="font-medium">{webhook.name}</span>
        {!webhook.enabled && <Badge variant="outline">{t('webhooks.inactive')}</Badge>}
        <div className="ml-auto flex gap-1">
          <Button type="button" variant="ghost" size="sm" onClick={() => test.mutate(id)} disabled={test.isPending}>
            {test.isPending ? <Loader2 className="animate-spin" /> : <Send />}
            {t('webhooks.test')}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('webhooks.editLabel', { name: webhook.name })}
            onClick={onEdit}
          >
            <Pencil />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('webhooks.deleteLabel', { name: webhook.name })}
            onClick={onDelete}
          >
            <Trash2 />
          </Button>
        </div>
      </div>
      <p className="text-muted-foreground font-mono text-xs break-all">
        {webhook.method} {webhook.url}
      </p>
      <div className="flex flex-wrap gap-1">
        {webhook.events.length === 0 ? (
          <span className="text-warning text-xs">{t('webhooks.noEvents')}</span>
        ) : (
          webhook.events.map((e) => (
            <Badge key={e} variant="secondary">
              {t(`events.${e}.label`)}
            </Badge>
          ))
        )}
      </div>
      {missing_env.length > 0 && (
        <p className="text-warning flex items-start gap-1.5 text-xs">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
          <span>
            <Trans
              i18nKey="webhooks.missingEnv"
              values={{ names: missing_env.join(', ') }}
              components={{ code: <code className="font-mono" /> }}
            />
          </span>
        </p>
      )}
      {webhook.token_missing && (
        <p className="text-warning flex items-start gap-1.5 text-xs">
          <KeyRound className="mt-0.5 size-3.5 shrink-0" />
          <span>{t('webhooks.tokenMissing')}</span>
        </p>
      )}
      {save.isError && (
        <p className="text-destructive text-xs break-words" role="alert">
          {t('webhooks.saveFailed', { error: save.error.message })}
        </p>
      )}
      <p className="text-xs">
        {test.data ? (
          test.data.ok ? (
            <span className="text-success flex items-center gap-1">
              <Check className="size-3.5" /> {t('webhooks.testDelivered')}
            </span>
          ) : (
            <span className="text-destructive flex items-center gap-1 break-words">
              <X className="size-3.5 shrink-0" /> {test.data.error}
            </span>
          )
        ) : last_error ? (
          <span className="text-destructive break-words">{t('webhooks.lastFailed', { error: last_error })}</span>
        ) : last_sent_at ? (
          <span className="text-muted-foreground" title={absoluteTime(last_sent_at)}>
            {t('webhooks.lastSent', { time: relativeTime(last_sent_at, now) })}
          </span>
        ) : (
          <span className="text-muted-foreground">{t('webhooks.nothingSent')}</span>
        )}
      </p>
    </li>
  )
}

export function WebhooksSection() {
  const { t } = useTranslation()
  const webhooks = useWebhooks()
  const del = useDeleteWebhook()
  const [dialog, setDialog] = useState<{ open: boolean; webhook?: Webhook }>({ open: false })
  const [toDelete, setToDelete] = useState<Webhook>()
  const list = webhooks.data ?? []

  return (
    <Section icon={<Send className="size-5" />} title={t('webhooks.title')} description={t('webhooks.subtitle')}>
      {webhooks.isPending ? (
        <Skeleton className="h-16" />
      ) : webhooks.isError ? (
        <p className="text-destructive text-sm">{t('webhooks.loadError', { error: webhooks.error.message })}</p>
      ) : list.length === 0 ? (
        <p className="text-muted-foreground text-sm">{t('webhooks.empty')}</p>
      ) : (
        <ul className="divide-y rounded-md border">
          {list.map((w) => (
            <WebhookCard
              key={w.id}
              webhook={w}
              onEdit={() => setDialog({ open: true, webhook: w })}
              onDelete={() => setToDelete(w)}
            />
          ))}
        </ul>
      )}
      <div>
        <Button type="button" variant="outline" onClick={() => setDialog({ open: true })}>
          <Plus /> {t('webhooks.add')}
        </Button>
      </div>

      <WebhookDialog
        open={dialog.open}
        webhook={dialog.webhook}
        onOpenChange={(open) => setDialog((d) => ({ ...d, open }))}
      />

      <AlertDialog open={toDelete !== undefined} onOpenChange={(open) => !open && setToDelete(undefined)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('webhooks.deleteTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('webhooks.deleteHint', { name: toDelete?.name })}</AlertDialogDescription>
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

