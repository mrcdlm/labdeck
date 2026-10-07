import { ArrowLeft, Check, ChevronDown, ExternalLink, KeyRound, Loader2, Plus, PlugZap, Trash2, X } from 'lucide-react'
import { useState, type FormEvent, type ReactNode } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { IntegrationIcon } from '@/components/IntegrationIcon'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import type { Integration, IntegrationInput, IntegrationType, SchemaField } from '@/lib/api'
import { formatDuration } from '@/lib/format'
import {
  draftToConfig,
  initialDraft,
  intervalPresets,
  newItem,
  visible,
  type Draft,
  type DraftItem,
  type DraftValue,
} from '@/lib/integrations'
import { useIntegrationTypes, useSaveIntegration, useTestIntegration } from '@/lib/queries'
import { cn } from '@/lib/utils'

interface Props {
  open: boolean
  /** zu bearbeitende Integration; undefined = neu anlegen */
  integration?: Integration
  onOpenChange: (open: boolean) => void
}

export function IntegrationDialog({ open, integration, onOpenChange }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        {open && <Body key={integration?.id ?? 'new'} integration={integration} onDone={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  )
}

function Body({ integration, onDone }: { integration?: Integration; onDone: () => void }) {
  const { t } = useTranslation()
  const types = useIntegrationTypes()
  const [chosen, setChosen] = useState<string | undefined>(integration?.type)

  if (types.isPending) {
    return <Skeleton className="h-64" />
  }
  if (types.isError) {
    return (
      <p className="text-destructive text-sm" role="alert">
        {t('integrations.typesError', { error: types.error.message })}
      </p>
    )
  }
  const list = types.data.types
  // Nur ein Typ verfügbar: Auswahl überspringen
  const typeName = chosen ?? (list.length === 1 ? list[0].type : undefined)
  const type = list.find((ty) => ty.type === typeName)

  if (!type) {
    if (integration) {
      return <p className="text-destructive text-sm">{t('integrations.typeUnknown', { type: integration.type })}</p>
    }
    return <TypePicker types={list} onPick={setChosen} />
  }
  return (
    <IntegrationForm
      type={type}
      integration={integration}
      limits={types.data.limits}
      secretKeySet={types.data.secret_key_set}
      onBack={!integration && list.length > 1 ? () => setChosen(undefined) : undefined}
      onDone={onDone}
    />
  )
}

function TypePicker({ types, onPick }: { types: IntegrationType[]; onPick: (type: string) => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-5">
      <DialogHeader>
        <DialogTitle>{t('integrations.addTitle')}</DialogTitle>
        <DialogDescription>{t('integrations.chooseType')}</DialogDescription>
      </DialogHeader>
      <div className="grid gap-2">
        {types.map((ty) => {
          return (
            <button
              key={ty.type}
              type="button"
              onClick={() => onPick(ty.type)}
              className="hover:bg-accent/40 flex items-center gap-3 rounded-lg border p-3 text-left transition-colors"
            >
              <span className="bg-muted grid size-9 shrink-0 place-items-center rounded-md">
                <IntegrationIcon name={ty.icon} type={ty.type} className="size-4" />
              </span>
              <span className="min-w-0">
                <span className="block font-medium">{ty.title}</span>
                <span className="text-muted-foreground block text-sm">{ty.description}</span>
              </span>
            </button>
          )
        })}
      </div>
    </div>
  )
}

function FieldRow({ id, label, required, help, children }: {
  id: string
  label: string
  required?: boolean
  help?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>
        {label}
        {required && <span className="text-muted-foreground">*</span>}
      </Label>
      {children}
      {help && <p className="text-muted-foreground text-xs">{help}</p>}
    </div>
  )
}

function SchemaFieldInput({
  field,
  value,
  onChange,
  secret,
  cleared = false,
  onClear = () => {},
  idPrefix = 'field',
}: {
  field: SchemaField
  value: string | boolean
  onChange: (v: string | boolean) => void
  /** gespeicherter Zustand eines Secret-Felds */
  secret?: { set: boolean; ref?: string }
  cleared?: boolean
  onClear?: (clear: boolean) => void
  /** eindeutige IDs je Listeneintrag */
  idPrefix?: string
}) {
  const { t } = useTranslation()
  const id = `${idPrefix}-${field.key}`

  if (field.type === 'bool') {
    return (
      <div className="flex items-start justify-between gap-4">
        <div className="grid gap-1">
          <Label htmlFor={id}>{field.label}</Label>
          {field.help && <p className="text-muted-foreground text-xs">{field.help}</p>}
        </div>
        <Switch id={id} checked={value === true} onCheckedChange={onChange} />
      </div>
    )
  }

  const str = typeof value === 'string' ? value : ''
  let input: ReactNode
  switch (field.type) {
    case 'text':
      input = (
        <Textarea id={id} value={str} placeholder={field.placeholder} onChange={(e) => onChange(e.target.value)} />
      )
      break
    case 'select':
      input = (
        <Select value={str} onValueChange={onChange}>
          <SelectTrigger id={id} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {field.options?.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label || o.value}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )
      break
    case 'secret': {
      const stored = secret?.set && !cleared
      input = (
        <div className="flex gap-2">
          <Input
            id={id}
            type="password"
            autoComplete="new-password"
            spellCheck={false}
            value={str}
            placeholder={
              stored ? (secret?.ref ? t('integrations.secretRef', { ref: secret.ref }) : t('integrations.secretSet')) : ''
            }
            onChange={(e) => {
              onChange(e.target.value)
              if (e.target.value) onClear(false)
            }}
          />
          {secret?.set && !field.required && (
            <Button type="button" variant="outline" onClick={() => onClear(!cleared)} aria-pressed={cleared}>
              {cleared ? t('integrations.secretKeep') : t('integrations.secretClear')}
            </Button>
          )}
        </div>
      )
      break
    }
    default:
      input = (
        <Input
          id={id}
          type={field.type === 'number' ? 'number' : 'text'}
          inputMode={field.type === 'url' ? 'url' : field.type === 'number' ? 'decimal' : undefined}
          min={field.min}
          max={field.max}
          step="any"
          autoComplete="off"
          spellCheck={false}
          value={str}
          placeholder={field.placeholder}
          onChange={(e) => onChange(e.target.value)}
        />
      )
  }

  const help =
    field.type === 'secret' ? (
      <>
        {field.help}
        {field.help && ' '}
        <Trans
          i18nKey="integrations.secretHint"
          values={{ example: '${SECRET_NAME}' }}
          components={{ code: <code className="font-mono" /> }}
        />
      </>
    ) : (
      field.help
    )

  return (
    <FieldRow id={id} label={field.label} required={field.required} help={help}>
      {input}
    </FieldRow>
  )
}

/** Liste gleich aufgebauter Einträge (z. B. Abfragen), Formular aus field.item. */
function ListField({
  field,
  items,
  onChange,
}: {
  field: SchemaField
  items: DraftItem[]
  onChange: (items: DraftItem[]) => void
}) {
  const { t } = useTranslation()
  const full = field.max !== undefined && items.length >= field.max
  const update = (i: number, key: string, v: DraftValue) =>
    onChange(items.map((it, n) => (n === i ? { ...it, [key]: v } : it)))

  return (
    <fieldset className="grid gap-2" data-list={field.key}>
      <legend className="mb-2 text-sm font-medium">
        {field.label}
        {field.required && <span className="text-muted-foreground">*</span>}
      </legend>
      {field.help && <p className="text-muted-foreground -mt-1 text-xs">{field.help}</p>}
      {items.map((it, i) => (
        <div key={i} className="bg-muted/30 flex flex-col gap-3 rounded-md border p-3" data-item={i + 1}>
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground text-xs font-medium">
              {t('integrations.listItem', { n: i + 1 })}
            </span>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-7"
              aria-label={t('integrations.listRemove', { label: field.label, n: i + 1 })}
              onClick={() => onChange(items.filter((_, n) => n !== i))}
            >
              <Trash2 />
            </Button>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            {(field.item ?? []).filter((sub) => visible(sub, it)).map((sub) => (
              <div key={sub.key} className={cn(!sub.half && 'sm:col-span-2')}>
                <SchemaFieldInput
                  field={sub}
                  idPrefix={`field-${field.key}-${i}`}
                  value={it[sub.key] ?? ''}
                  onChange={(v) => update(i, sub.key, v)}
                />
              </div>
            ))}
          </div>
        </div>
      ))}
      {items.length === 0 && <p className="text-muted-foreground text-sm">{t('integrations.listEmpty')}</p>}
      <Button
        type="button"
        variant="outline"
        className="w-fit"
        disabled={full}
        onClick={() => onChange([...items, newItem(field)])}
      >
        <Plus />
        {t('integrations.listAdd')}
      </Button>
    </fieldset>
  )
}

function IntegrationForm({
  type,
  integration,
  limits,
  secretKeySet,
  onBack,
  onDone,
}: {
  type: IntegrationType
  integration?: Integration
  limits: Record<string, number>
  secretKeySet: boolean
  onBack?: () => void
  onDone: () => void
}) {
  const { t } = useTranslation()
  const save = useSaveIntegration()
  const test = useTestIntegration()
  const [name, setName] = useState(integration?.name ?? type.title)
  const [enabled, setEnabled] = useState(integration?.enabled ?? true)
  const [interval, setIntervalSec] = useState(integration?.interval_seconds ?? limits.interval_default ?? 60)
  const [timeout, setTimeoutSec] = useState(String(integration?.timeout_seconds ?? limits.timeout_default ?? 10))
  const [tlsVerify, setTLSVerify] = useState(integration?.tls_verify ?? true)
  const [draft, setDraft] = useState<Draft>(() => initialDraft(type, integration))
  const [cleared, setCleared] = useState<string[]>([])
  const [advanced, setAdvanced] = useState(false)
  const intervals = intervalPresets.includes(interval) ? intervalPresets : [...intervalPresets, interval].sort((a, b) => a - b)

  // Jede Änderung macht ein altes Testergebnis ungültig.
  function touch() {
    if (test.data || test.isError) test.reset()
  }

  function input(): IntegrationInput {
    const { config, secrets } = draftToConfig(type.fields, draft)
    return {
      type: type.type,
      name: name.trim(),
      enabled,
      interval_seconds: interval,
      timeout_seconds: Number(timeout) || 0,
      tls_verify: tlsVerify,
      config,
      secrets,
      clear_secrets: cleared,
    }
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    save.mutate({ id: integration?.id, input: input() }, { onSuccess: onDone })
  }

  const hasSecrets = type.fields.some((f) => f.type === 'secret')

  return (
    <form onSubmit={submit} className="flex flex-col gap-5" noValidate>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          {onBack && (
            <Button type="button" variant="ghost" size="icon" className="-ml-2 size-8" onClick={onBack} aria-label={t('integrations.back')}>
              <ArrowLeft />
            </Button>
          )}
          <IntegrationIcon name={type.icon} type={type.type} className="size-5" />
          {integration ? t('integrations.editTitle', { type: type.title }) : t('integrations.addTypeTitle', { type: type.title })}
        </DialogTitle>
        <DialogDescription>
          {type.description}{' '}
          {type.docs && (
            <a href={type.docs} target="_blank" rel="noreferrer" className="inline-flex items-center gap-0.5 underline">
              {t('integrations.docs')}
              <ExternalLink className="size-3" />
            </a>
          )}
        </DialogDescription>
      </DialogHeader>

      {!secretKeySet && hasSecrets && (
        <p className="border-warning/40 bg-warning/10 flex items-start gap-2 rounded-md border p-3 text-xs">
          <KeyRound className="text-warning mt-0.5 size-4 shrink-0" />
          <span>
            <Trans
              i18nKey="integrations.noSecretKey"
              values={{ example: '${SECRET_NAME}' }}
              components={{ code: <code className="font-mono" /> }}
            />
          </span>
        </p>
      )}

      <FieldRow id="integration-name" label={t('integrations.name')} required help={t('integrations.nameHint')}>
        <Input
          id="integration-name"
          value={name}
          maxLength={100}
          onChange={(e) => {
            setName(e.target.value)
            touch()
          }}
        />
      </FieldRow>

      {type.fields.filter((f) => visible(f, draft)).map((f) =>
        f.type === 'list' ? (
          <ListField
            key={f.key}
            field={f}
            items={Array.isArray(draft[f.key]) ? (draft[f.key] as DraftItem[]) : []}
            onChange={(items) => {
              setDraft((d) => ({ ...d, [f.key]: items }))
              touch()
            }}
          />
        ) : (
        <SchemaFieldInput
          key={f.key}
          field={f}
          value={(draft[f.key] as DraftValue | undefined) ?? ''}
          secret={integration?.secrets[f.key]}
          cleared={cleared.includes(f.key)}
          onClear={(clear) => {
            setCleared((c) => (clear ? [...c, f.key] : c.filter((k) => k !== f.key)))
            touch()
          }}
          onChange={(v) => {
            setDraft((d) => ({ ...d, [f.key]: v }))
            touch()
          }}
        />
        ),
      )}

      <div className="rounded-md border">
        <button
          type="button"
          className="flex w-full items-center justify-between px-3 py-2 text-sm font-medium"
          onClick={() => setAdvanced((a) => !a)}
          aria-expanded={advanced}
        >
          {t('integrations.advanced')}
          <ChevronDown className={cn('size-4 transition-transform', advanced && 'rotate-180')} />
        </button>
        {advanced && (
          <div className="flex flex-col gap-4 border-t p-3">
            <FieldRow id="integration-interval" label={t('integrations.interval')} help={t('integrations.intervalHint')}>
              <Select
                value={String(interval)}
                onValueChange={(v) => {
                  setIntervalSec(Number(v))
                  touch()
                }}
              >
                <SelectTrigger id="integration-interval" className="w-full sm:w-48">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {intervals.map((s) => (
                    <SelectItem key={s} value={String(s)}>
                      {formatDuration(s)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FieldRow>
            <FieldRow
              id="integration-timeout"
              label={t('integrations.timeout')}
              help={t('integrations.timeoutHint', { min: limits.timeout_min, max: limits.timeout_max })}
            >
              <Input
                id="integration-timeout"
                type="number"
                inputMode="numeric"
                className="w-28 tabular-nums"
                min={limits.timeout_min}
                max={limits.timeout_max}
                value={timeout}
                onChange={(e) => {
                  setTimeoutSec(e.target.value)
                  touch()
                }}
              />
            </FieldRow>
            <div className="flex items-start justify-between gap-4">
              <div className="grid gap-1">
                <Label htmlFor="integration-tls">{t('integrations.tlsVerify')}</Label>
                <p className="text-muted-foreground text-xs">{t('integrations.tlsVerifyHint')}</p>
              </div>
              <Switch
                id="integration-tls"
                checked={tlsVerify}
                onCheckedChange={(v) => {
                  setTLSVerify(v)
                  touch()
                }}
              />
            </div>
            <div className="flex items-start justify-between gap-4">
              <div className="grid gap-1">
                <Label htmlFor="integration-enabled">{t('integrations.enabled')}</Label>
                <p className="text-muted-foreground text-xs">{t('integrations.enabledHint')}</p>
              </div>
              <Switch id="integration-enabled" checked={enabled} onCheckedChange={setEnabled} />
            </div>
          </div>
        )}
      </div>

      {(test.data || test.isError) && (
        <p
          role="status"
          className={cn(
            'flex items-start gap-1.5 text-sm break-words',
            test.data?.ok ? 'text-success' : 'text-destructive',
          )}
        >
          {test.data?.ok ? <Check className="mt-0.5 size-4 shrink-0" /> : <X className="mt-0.5 size-4 shrink-0" />}
          <span>
            {test.isError
              ? test.error.message
              : test.data?.ok
                ? t('integrations.testOk', {
                    summary: summary(t, test.data),
                    ms: test.data.duration_ms,
                  })
                : test.data?.error}
          </span>
        </p>
      )}

      {save.isError && (
        <p className="text-destructive text-sm break-words" role="alert">
          {save.error.message}
        </p>
      )}

      <DialogFooter className="gap-2 sm:justify-between">
        <Button
          type="button"
          variant="outline"
          disabled={test.isPending}
          onClick={() => test.mutate({ id: integration?.id, input: input() })}
        >
          {test.isPending ? <Loader2 className="animate-spin" /> : <PlugZap />}
          {t('integrations.test')}
        </Button>
        <div className="flex gap-2">
          <Button type="button" variant="outline" onClick={onDone}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" disabled={save.isPending || !name.trim()}>
            {save.isPending && <Loader2 className="animate-spin" />}
            {t('common.save')}
          </Button>
        </div>
      </DialogFooter>
    </form>
  )
}

function summary(t: ReturnType<typeof useTranslation>['t'], r: { checks: number; backups: number; metrics: number }) {
  const parts: string[] = []
  if (r.checks) parts.push(t('integrations.count.checks', { count: r.checks }))
  if (r.backups) parts.push(t('integrations.count.backups', { count: r.backups }))
  if (!r.checks && !r.backups && r.metrics) parts.push(t('integrations.count.metrics', { count: r.metrics }))
  return parts.length ? parts.join(' · ') : t('integrations.count.none')
}
