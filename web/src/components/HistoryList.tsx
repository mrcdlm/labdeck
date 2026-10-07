import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { IntegrationIcon } from '@/components/IntegrationIcon'
import { LiveItem } from '@/components/LiveRow'
import { StatusBadge } from '@/components/StatusBadge'
import type { BackupState, HistoryEntry, Status } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { metricLabel } from '@/lib/metrics'
import { useIntegrationTypes } from '@/lib/queries'
import { cn } from '@/lib/utils'

const statuses: Status[] = ['ok', 'warning', 'error', 'unknown', 'paused']
const backupStates: BackupState[] = ['ok', 'due_soon', 'overdue', 'failed', 'unknown', 'paused']

// Farbe der Backup-Zustände wie auf der Backups-Seite
const backupStatus: Record<BackupState, Status> = {
  ok: 'ok',
  due_soon: 'warning',
  overdue: 'error',
  failed: 'error',
  unknown: 'unknown',
  paused: 'paused',
}

/** Status bzw. Backup-Zustand als Badge. */
function StateBadge({ kind, value }: { kind: HistoryEntry['kind']; value: string }) {
  const { t } = useTranslation()
  if (value === 'new') return <StatusBadge status="unknown" label={t('history.new')} />
  if (kind === 'backup') {
    const s = backupStates.find((x) => x === value) ?? 'unknown'
    return <StatusBadge status={backupStatus[s]} label={t(`backups.state.${s}`)} />
  }
  const s = statuses.find((x) => x === value) ?? 'unknown'
  return <StatusBadge status={s} label={kind === 'integration' ? t(`integrations.status.${s}`) : undefined} />
}

/** Bezeichnung des Eintrags (Kennzahlen übersetzt, mit Gruppe). */
function entryLabel(e: HistoryEntry): string {
  if (e.kind === 'integration') return e.integration_name
  const name = e.kind === 'metric' ? metricLabel(e.integration_type, { key: e.item_id ?? '', name: e.item_name }) : e.item_name
  return e.item_group ? `${name} · ${e.item_group}` : name
}

/** Liste von Statuswechseln: Zeit, Eintrag, von → nach, Meldung. */
export function HistoryList({ entries, compact }: { entries: HistoryEntry[]; compact?: boolean }) {
  const { t } = useTranslation()
  const now = useNow()
  const types = useIntegrationTypes()
  const icon = (type: string) => types.data?.types.find((x) => x.type === type)?.icon

  return (
    <ul className="divide-y">
      {entries.map((e, n) => (
        <LiveItem
          watch={e.id}
          index={n}
          key={e.id}
          className={cn('flex flex-col gap-1.5 py-3 sm:flex-row sm:items-center sm:gap-4', !compact && 'px-4 md:px-6')}
          data-entry={entryLabel(e)}
        >
          <span
            className="text-muted-foreground w-28 shrink-0 text-xs whitespace-nowrap tabular-nums"
            title={absoluteTime(e.at)}
          >
            {relativeTime(e.at, now)}
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <span className={cn('font-medium [overflow-wrap:anywhere]', e.kind === 'metric' && 'first-letter:uppercase')}>
                {entryLabel(e)}
              </span>
              <span className="text-muted-foreground text-xs">{t(`dashboard.kind.${e.kind}`)}</span>
            </div>
            <div className="text-muted-foreground flex items-center gap-1 text-xs">
              <IntegrationIcon name={icon(e.integration_type)} type={e.integration_type} className="size-3 shrink-0" />
              <span className="[overflow-wrap:anywhere]">
                {e.integration_name}
                {e.message && ` · ${e.message}`}
              </span>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            <StateBadge kind={e.kind} value={e.from} />
            <ArrowRight className="text-muted-foreground size-3.5" aria-label={t('history.to')} />
            <StateBadge kind={e.kind} value={e.to} />
          </div>
        </LiveItem>
      ))}
    </ul>
  )
}
