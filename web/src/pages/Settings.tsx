import { Check, Info as InfoIcon, Loader2, SlidersHorizontal } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import type { Settings as SettingsData } from '@/lib/api'
import { useInfo, useSaveSettings, useSettings } from '@/lib/queries'

// Radix-Select erlaubt keinen leeren Wert.
const BROWSER_TZ = 'browser'

const browserTimeZone = Intl.DateTimeFormat().resolvedOptions().timeZone

function timeZones(current: string): string[] {
  let list: string[] = []
  try {
    list = Intl.supportedValuesOf('timeZone')
  } catch {
    // ältere Browser
  }
  if (current && !list.includes(current)) list = [...list, current].sort()
  return list
}

function Section({
  icon,
  title,
  description,
  children,
}: {
  icon: ReactNode
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg">
          {icon}
          {title}
        </CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-5">{children}</CardContent>
    </Card>
  )
}

function SettingRow({ id, label, hint, children }: { id: string; label: string; hint: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div className="grid gap-1">
        <Label htmlFor={id}>{label}</Label>
        <p className="text-muted-foreground text-xs">{hint}</p>
      </div>
      {children}
    </div>
  )
}

function DaysInput({
  id,
  value,
  min,
  max,
  onChange,
}: {
  id: string
  value: number
  min: number
  max: number
  onChange: (v: number) => void
}) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center gap-2">
      <Input
        id={id}
        type="number"
        inputMode="numeric"
        className="w-28 tabular-nums"
        min={min}
        max={max}
        required
        value={Number.isNaN(value) ? '' : value}
        onChange={(e) => onChange(e.target.valueAsNumber)}
      />
      <span className="text-muted-foreground w-12 text-sm">{t('settings.days')}</span>
    </div>
  )
}

// saved liegt beim Aufrufer: das Formular wird nach dem Speichern mit den neuen
// Werten neu aufgebaut (key) und verlöre sonst die Bestätigung.
function SettingsForm({
  initial,
  saved,
  setSaved,
}: {
  initial: SettingsData
  saved: boolean
  setSaved: (v: boolean) => void
}) {
  const { t } = useTranslation()
  const save = useSaveSettings()
  const [draft, setDraft] = useState(initial)
  const dirty = JSON.stringify(draft) !== JSON.stringify(initial)
  const l = draft.limits

  function update(patch: Partial<SettingsData>) {
    setSaved(false)
    setDraft((d) => ({ ...d, ...patch }))
  }

  return (
    <form
      className="flex flex-col gap-6"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate(draft, { onSuccess: (s) => (setDraft(s), setSaved(true)) })
      }}
    >
      <Section
        icon={<SlidersHorizontal className="size-5" />}
        title={t('settings.general')}
        description={t('settings.generalHint')}
      >
        <SettingRow id="timezone" label={t('settings.timezone')} hint={t('settings.timezoneHint')}>
          <Select
            value={draft.timezone || BROWSER_TZ}
            onValueChange={(v) => update({ timezone: v === BROWSER_TZ ? '' : v })}
          >
            <SelectTrigger id="timezone" className="w-full sm:w-64">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={BROWSER_TZ}>{t('settings.timezoneBrowser', { zone: browserTimeZone })}</SelectItem>
              {timeZones(draft.timezone).map((z) => (
                <SelectItem key={z} value={z}>
                  {z}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingRow>
        <SettingRow id="raw-retention" label={t('settings.rawRetention')} hint={t('settings.rawRetentionHint')}>
          <DaysInput
            id="raw-retention"
            value={draft.raw_retention_days}
            min={l.raw_retention_days_min}
            max={l.raw_retention_days_max}
            onChange={(v) => update({ raw_retention_days: v })}
          />
        </SettingRow>
        <SettingRow id="daily-retention" label={t('settings.dailyRetention')} hint={t('settings.dailyRetentionHint')}>
          <DaysInput
            id="daily-retention"
            value={draft.daily_retention_days}
            min={l.daily_retention_days_min}
            max={l.daily_retention_days_max}
            onChange={(v) => update({ daily_retention_days: v })}
          />
        </SettingRow>
      </Section>

      <div className="bg-background/90 sticky bottom-0 -mx-4 flex flex-wrap items-center justify-end gap-3 border-t px-4 py-3 backdrop-blur md:-mx-8 md:px-8">
        {save.isError && (
          <p className="text-destructive mr-auto text-sm" role="alert">
            {save.error.message}
          </p>
        )}
        {saved && !dirty && (
          <span className="text-success mr-auto flex items-center gap-1 text-sm">
            <Check className="size-4" /> {t('settings.saved')}
          </span>
        )}
        <Button
          type="button"
          variant="outline"
          disabled={!dirty || save.isPending}
          onClick={() => (setDraft(initial), setSaved(false))}
        >
          {t('settings.discard')}
        </Button>
        <Button type="submit" disabled={!dirty || save.isPending}>
          {save.isPending && <Loader2 className="animate-spin" />}
          {t('common.save')}
        </Button>
      </div>
    </form>
  )
}

function InfoRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex justify-between gap-4">
      <dt className="text-muted-foreground shrink-0">{label}</dt>
      <dd className="min-w-0 text-right break-all">{children}</dd>
    </div>
  )
}

function SystemInfo() {
  const { t } = useTranslation()
  const info = useInfo()
  const d = info.data
  return (
    <Section icon={<InfoIcon className="size-5" />} title={t('system.title')} description={t('system.hint')}>
      {!d ? (
        <Skeleton className="h-16" />
      ) : (
        <dl className="grid gap-2 text-sm">
          <InfoRow label={t('system.version')}>
            <span className="font-mono">{d.version}</span>
          </InfoRow>
          <InfoRow label={t('system.secretKey')}>
            {d.secret_key_set ? (
              t('system.set')
            ) : (
              <span className="text-warning" title={t('system.secretKeyMissingHint')}>
                {t('system.missing')}
              </span>
            )}
          </InfoRow>
          <InfoRow label={t('system.dataDir')}>
            <span className="font-mono">{d.data_dir}</span>
          </InfoRow>
        </dl>
      )}
    </Section>
  )
}

export function Settings() {
  const { t } = useTranslation()
  const settings = useSettings()
  const [saved, setSaved] = useState(false)

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('nav.settings')} subtitle={t('settings.subtitle')} />
      {settings.isPending ? (
        <Skeleton className="h-64" />
      ) : settings.isError ? (
        <p className="text-destructive text-sm">{t('settings.loadError', { error: settings.error.message })}</p>
      ) : (
        // key: nach dem Speichern (oder Änderung in anderem Tab) neu initialisieren
        <SettingsForm key={JSON.stringify(settings.data)} initial={settings.data} saved={saved} setSaved={setSaved} />
      )}
      <SystemInfo />
    </div>
  )
}
