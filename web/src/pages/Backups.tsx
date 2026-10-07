import { CloudOff, DatabaseBackup, Info, Loader2, Pencil, Plug, ShieldCheck, ShieldQuestion, ShieldX } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { IntegrationIcon } from '@/components/IntegrationIcon'
import { PageHeader } from '@/components/PageHeader'
import { StatusBadge } from '@/components/StatusBadge'
import { RangeToggle, UptimeBar } from '@/components/UptimeBar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
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
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { BackupItem, BackupsOverview, BackupState, UptimeRange } from '@/lib/api'
import { absoluteTime, formatBytes, formatHours, relativeTime, useNow } from '@/lib/format'
import { useBackups, useIntegrationTypes, useSetBackupMaxAge } from '@/lib/queries'
import { cn } from '@/lib/utils'

const stateOrder: BackupState[] = ['failed', 'overdue', 'due_soon', 'unknown', 'ok', 'paused']

function State({ item }: { item: BackupItem }) {
  const { t } = useTranslation()
  return <StatusBadge status={item.status} label={t(`backups.state.${item.state}`)} />
}

function NameCell({ item, icon }: { item: BackupItem; icon?: string }) {
  return (
    <div className="min-w-0">
      <div className="font-medium [overflow-wrap:anywhere]">{item.name}</div>
      <div className="text-muted-foreground mt-0.5 flex items-center gap-1 text-xs">
        <IntegrationIcon name={icon} className="size-3 shrink-0" />
        <span className="[overflow-wrap:anywhere]">
          {item.integration_name}
          {item.group && ` · ${item.group}`}
        </span>
        {item.stale && <CloudOff className="text-warning size-3 shrink-0" aria-hidden />}
      </div>
      <Verify item={item} />
    </div>
  )
}

const verifyIcon = { ok: ShieldCheck, failed: ShieldX, none: ShieldQuestion }

/** Verify-Status der Quelle (z. B. Proxmox Backup Server). */
function Verify({ item }: { item: BackupItem }) {
  const { t } = useTranslation()
  if (!item.verify) return null
  const Icon = verifyIcon[item.verify]
  return (
    <div
      className={cn(
        'mt-0.5 flex items-center gap-1 text-xs',
        item.verify === 'failed' ? 'text-destructive' : 'text-muted-foreground',
      )}
    >
      <Icon className="size-3 shrink-0" aria-hidden />
      {t(`backups.verify.${item.verify}`)}
    </div>
  )
}

/** Letzter Lauf und wann das Backup fällig wird bzw. seit wann es überfällig ist. */
function LastRun({ item, now }: { item: BackupItem; now: number }) {
  const { t } = useTranslation()
  if (!item.last_run) return <span className="text-muted-foreground text-sm">{t('backups.never')}</span>
  const due = item.due_at ? new Date(item.due_at).getTime() : undefined
  return (
    <div className="text-sm">
      <span title={absoluteTime(item.last_run)} className="whitespace-nowrap">
        {relativeTime(item.last_run, now)}
      </span>
      {due !== undefined && item.state !== 'paused' && (
        <div
          className={cn('text-xs whitespace-nowrap', due < now ? 'text-destructive' : 'text-muted-foreground')}
          title={absoluteTime(item.due_at!)}
        >
          {t('backups.due', { time: relativeTime(item.due_at!, now) })}
        </div>
      )}
    </div>
  )
}

function Size({ item }: { item: BackupItem }) {
  if (item.size_bytes === undefined) return <span className="text-muted-foreground">—</span>
  return <span className="whitespace-nowrap tabular-nums">{formatBytes(item.size_bytes)}</span>
}

function History({ item, range }: { item: BackupItem; range: UptimeRange }) {
  const uptime = item.uptime?.[range]
  if (!uptime) return <span className="text-muted-foreground text-xs">—</span>
  return <UptimeBar uptime={uptime} range={range} compact />
}

function MaxAge({ item, onEdit }: { item: BackupItem; onEdit: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center justify-end gap-1">
      <span className="text-sm whitespace-nowrap" title={item.max_age_custom ? undefined : t('backups.maxAgeDefault')}>
        {formatHours(item.max_age_hours)}
        {!item.max_age_custom && <span className="text-muted-foreground"> *</span>}
      </span>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('backups.editMaxAgeLabel', { name: item.name })}
        title={t('backups.editMaxAge')}
        onClick={onEdit}
      >
        <Pencil />
      </Button>
    </div>
  )
}

function MaxAgeForm({ item, data, onClose }: { item: BackupItem; data: BackupsOverview; onClose: () => void }) {
  const { t } = useTranslation()
  const save = useSetBackupMaxAge()
  const [hours, setHours] = useState(String(item.max_age_hours))
  const min = data.limits.max_age_hours_min
  const max = data.limits.max_age_hours_max
  const n = Number(hours)
  const valid = hours.trim() !== '' && Number.isInteger(n) && n >= min && n <= max

  function submit(e: FormEvent, value: number) {
    e.preventDefault()
    save.mutate({ integrationId: item.integration_id, itemId: item.id, hours: value }, { onSuccess: onClose })
  }

  return (
    <form onSubmit={(e) => (valid ? submit(e, n) : e.preventDefault())} className="flex flex-col gap-4">
      <DialogHeader>
        <DialogTitle>{t('backups.maxAgeTitle')}</DialogTitle>
        <DialogDescription>{t('backups.maxAgeHint', { name: item.name, percent: data.warn_percent })}</DialogDescription>
      </DialogHeader>
      <div className="grid gap-2">
        <Label htmlFor="max-age">{t('backups.maxAgeHours')}</Label>
        <Input
          id="max-age"
          type="number"
          inputMode="numeric"
          className="w-32 tabular-nums"
          min={min}
          max={max}
          required
          value={hours}
          onChange={(e) => (setHours(e.target.value), save.reset())}
        />
        <p className="text-muted-foreground text-xs">
          {valid
            ? t('backups.maxAgePreview', {
                age: formatHours(n),
                warn: formatHours((n * data.warn_percent) / 100),
              })
            : t('backups.maxAgeRange', { min, max })}
        </p>
      </div>
      {save.isError && (
        <p className="text-destructive text-sm" role="alert">
          {save.error.message}
        </p>
      )}
      <DialogFooter className="gap-2 sm:justify-between">
        <Button
          type="button"
          variant="outline"
          disabled={save.isPending || !item.max_age_custom}
          onClick={(e) => submit(e, 0)}
        >
          {t('backups.useDefault', { age: formatHours(data.default_max_age_hours) })}
        </Button>
        <Button type="submit" disabled={!valid || save.isPending}>
          {save.isPending && <Loader2 className="animate-spin" />}
          {t('common.save')}
        </Button>
      </DialogFooter>
    </form>
  )
}

function MaxAgeDialog({ item, data, onClose }: { item?: BackupItem; data: BackupsOverview; onClose: () => void }) {
  return (
    <Dialog open={item !== undefined} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        {item && <MaxAgeForm key={`${item.integration_id}/${item.id}`} item={item} data={data} onClose={onClose} />}
      </DialogContent>
    </Dialog>
  )
}

function Summary({ data }: { data: BackupsOverview }) {
  const { t } = useTranslation()
  const variant: Record<BackupState, 'success' | 'warning' | 'destructive' | 'secondary'> = {
    ok: 'success',
    due_soon: 'warning',
    overdue: 'destructive',
    failed: 'destructive',
    unknown: 'secondary',
    paused: 'secondary',
  }
  return (
    <div className="flex flex-wrap items-center gap-2 text-sm">
      <span className="font-medium">{t('backups.total', { count: data.summary.total })}</span>
      {stateOrder.map((s) => {
        const n = data.summary.states[s] ?? 0
        return n > 0 ? (
          <Badge key={s} variant={variant[s]}>
            {n} × {t(`backups.state.${s}`)}
          </Badge>
        ) : null
      })}
    </div>
  )
}

function Empty() {
  const { t } = useTranslation()
  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
        <span className="bg-muted grid size-12 place-items-center rounded-lg">
          <DatabaseBackup className="text-muted-foreground size-6" />
        </span>
        <p className="font-medium">{t('backups.emptyTitle')}</p>
        <p className="text-muted-foreground max-w-md text-sm">{t('backups.empty')}</p>
        <Link to="/einstellungen" className="text-primary flex items-center gap-1 text-sm font-medium hover:underline">
          <Plug className="size-4" /> {t('backups.toSettings')}
        </Link>
      </CardContent>
    </Card>
  )
}

export function Backups() {
  const { t } = useTranslation()
  const backups = useBackups()
  const types = useIntegrationTypes()
  const now = useNow()
  const [range, setRange] = useState<UptimeRange>('7d')
  const [editing, setEditing] = useState<BackupItem>()
  const icon = (type: string) => types.data?.types.find((x) => x.type === type)?.icon
  const data = backups.data
  const stale = data?.backups.some((b) => b.stale)

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={t('nav.backups')}
        subtitle={t('backups.subtitle')}
        actions={<RangeToggle value={range} onChange={setRange} />}
      />

      {backups.isPending ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-6 w-64" />
          <Skeleton className="h-48 rounded-xl" />
        </div>
      ) : backups.isError ? (
        <p className="text-destructive text-sm">{t('backups.loadError', { error: backups.error.message })}</p>
      ) : data!.backups.length === 0 ? (
        <Empty />
      ) : (
        <>
          <Summary data={data!} />
          {stale && (
            <p className="text-warning flex items-center gap-2 text-sm">
              <CloudOff className="size-4 shrink-0" aria-hidden />
              {t('backups.stale')}
            </p>
          )}
          <Card className="py-0">
            <div className="hidden md:block">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="pl-6">{t('backups.col.name')}</TableHead>
                    <TableHead>{t('backups.col.status')}</TableHead>
                    <TableHead>{t('backups.col.lastRun')}</TableHead>
                    <TableHead className="hidden lg:table-cell">{t('backups.col.size')}</TableHead>
                    <TableHead>{t('backups.col.history', { range: t(`uptime.range.${range}`) })}</TableHead>
                    <TableHead className="pr-6 text-right">{t('backups.col.maxAge')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data!.backups.map((b) => (
                    <TableRow
                      key={`${b.integration_id}/${b.id}`}
                      className={cn(b.state === 'paused' && 'opacity-60')}
                      data-backup={b.name}
                    >
                      <TableCell className="max-w-[20rem] min-w-44 pl-6 whitespace-normal">
                        <NameCell item={b} icon={icon(b.integration_type)} />
                      </TableCell>
                      <TableCell>
                        <State item={b} />
                      </TableCell>
                      <TableCell>
                        <LastRun item={b} now={now} />
                      </TableCell>
                      <TableCell className="hidden text-sm lg:table-cell">
                        <Size item={b} />
                      </TableCell>
                      <TableCell className="w-52 min-w-36">
                        <History item={b} range={range} />
                      </TableCell>
                      <TableCell className="pr-4">
                        <MaxAge item={b} onEdit={() => setEditing(b)} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            <ul className="divide-y md:hidden">
              {data!.backups.map((b) => (
                <li
                  key={`${b.integration_id}/${b.id}`}
                  className={cn('flex flex-col gap-2 p-4', b.state === 'paused' && 'opacity-60')}
                  data-backup={b.name}
                >
                  <div className="flex items-start justify-between gap-2">
                    <NameCell item={b} icon={icon(b.integration_type)} />
                    <State item={b} />
                  </div>
                  <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-1">
                    <LastRun item={b} now={now} />
                    {b.size_bytes !== undefined && (
                      <span className="text-sm">
                        <Size item={b} />
                      </span>
                    )}
                    <MaxAge item={b} onEdit={() => setEditing(b)} />
                  </div>
                  <History item={b} range={range} />
                </li>
              ))}
            </ul>
          </Card>

          <div className="text-muted-foreground flex gap-3 text-sm">
            <Info className="mt-0.5 size-4 shrink-0" aria-hidden />
            <p>
              {t('backups.howItWorks', {
                age: formatHours(data!.default_max_age_hours),
                percent: data!.warn_percent,
              })}
            </p>
          </div>

          <MaxAgeDialog item={editing} data={data!} onClose={() => setEditing(undefined)} />
        </>
      )}
    </div>
  )
}
