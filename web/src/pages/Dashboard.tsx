import {
  Activity,
  Check,
  ChevronRight,
  Cpu,
  DatabaseBackup,
  Gauge,
  GanttChart,
  HardDrive,
  History as HistoryIcon,
  House,
  Image,
  LayoutDashboard,
  Network,
  Pencil,
  Plug,
  Plus,
  Server,
  ShieldCheck,
  Trash2,
  type LucideIcon,
} from 'lucide-react'
import { useState } from 'react'
import type { Layout } from 'react-grid-layout'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, useNavigate, useParams } from 'react-router'

import { AnimatedNumber } from '@/components/AnimatedNumber'
import { DashboardGrid } from '@/components/DashboardGrid'
import { HistoryList } from '@/components/HistoryList'
import { IntegrationIcon } from '@/components/IntegrationIcon'
import { Meter } from '@/components/Meter'
import { MetricTile } from '@/components/MetricTile'
import { PageHeader } from '@/components/PageHeader'
import { Segmented } from '@/components/Segmented'
import { ServerHealth } from '@/components/ServerView'
import { Sparkline } from '@/components/Sparkline'
import { StatusBadge } from '@/components/StatusBadge'
import { ProblemList } from '@/components/ProblemList'
import { StatusDot, StatusTile, TileValue } from '@/components/StatusTile'
import { SwitchFront } from '@/components/SwitchView'
import { TileDialog } from '@/components/TileDialog'
import { UptimeBar } from '@/components/UptimeBar'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import type {
  Dashboard as DashboardT,
  DashboardTile,
  Integration,
  Metric,
  Overview,
  Problem,
  TileOptions,
  UptimeRange,
} from '@/lib/api'
import { formatBytes } from '@/lib/format'
import { formatMetric, isStateUnit, metricLabel, metricTitle } from '@/lib/metrics'
import { problemOwner, statusRank, systemLink, worstStatus } from '@/lib/problems'
import {
  useBackups,
  useCreateDashboard,
  useDashboards,
  useDeleteDashboard,
  useHistory,
  useIntegrations,
  useIntegrationTypes,
  useOverview,
  useSaveDashboard,
} from '@/lib/queries'
import { appendTile, availableTiles, newMetricTileId } from '@/lib/tiles'
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

function BackupsTile({ overview, problems }: { overview: Overview; problems: Problem[] }) {
  const { t } = useTranslation()
  const s = overview.backups
  const n = (k: keyof typeof s.states) => s.states[k] ?? 0
  const sub = [
    n('ok') > 0 && t('dashboard.backupsOk', { count: n('ok') }),
    n('due_soon') > 0 && t('dashboard.backupsDueSoon', { count: n('due_soon') }),
    n('unknown') + n('paused') > 0 && t('dashboard.other', { count: n('unknown') + n('paused') }),
  ].filter(Boolean)
  return (
    <StatusTile
      to="/backups"
      icon={<DatabaseBackup className="size-4" />}
      title={t('nav.backups')}
      tile="backups"
      status={worstStatus(problems.map((p) => p.status)) ?? (s.total > 0 ? 'ok' : 'unknown')}
      value={
        s.total === 0 ? (
          <span className="text-muted-foreground font-medium">{t('dashboard.noBackups')}</span>
        ) : (
          <TileValue label={t('dashboard.monitoredLabel', { count: s.total })}>
            <AnimatedNumber value={s.total} format={(v) => String(Math.round(v))} />
          </TileValue>
        )
      }
      sub={sub.join(' · ')}
      problems={problems.length}
    />
  )
}

/** Healthchecks.io als Ampel: grün, wenn alle Checks laufen, rot mit den ausgefallenen. */
function ChecksTile({ overview, problems }: { overview: Overview; problems: Problem[] }) {
  const { t } = useTranslation()
  const c = overview.checks
  const down = c.statuses.error ?? 0
  const active = c.total - (c.statuses.paused ?? 0)
  return (
    <StatusTile
      to="/checks"
      icon={<IntegrationIcon type="healthchecks" className="size-4" />}
      title={t('nav.checks')}
      tile="checks"
      status={
        worstStatus([...problems.map((p) => p.status), down > 0 ? 'error' : undefined]) ?? (c.total > 0 ? 'ok' : 'unknown')
      }
      value={
        c.total === 0 ? (
          <span className="text-muted-foreground font-medium">{t('dashboard.noChecks')}</span>
        ) : (
          <TileValue label={t('dashboard.checksLabel', { count: active })}>
            <AnimatedNumber value={active} format={(v) => String(Math.round(v))} />
          </TileValue>
        )
      }
      sub={
        c.total > 0 && (
          <span className={cn('font-medium', down > 0 ? 'text-destructive' : 'text-success')}>
            {down > 0 ? t('dashboard.checksFailed', { count: down }) : t('dashboard.checksAllOk', { count: active })}
          </span>
        )
      }
      problems={problems.length}
    />
  )
}

/** Kennzahl für den Mini-Verlauf: zuerst eine Prozent-Kennzahl (24 h), sonst die erste Zahl (30 Tage). */
function sparkMetric(main: Metric[]): { key: string; range: '24h' | '30d' } | undefined {
  const pct = main.find((m) => m.unit === '%')
  if (pct) return { key: pct.key, range: '24h' }
  const first = main.find((m) => !isStateUnit(m.unit))
  return first && { key: first.key, range: '30d' }
}

function metricText(type: string, m: Metric) {
  return isStateUnit(m.unit)
    ? formatMetric(m.value, m.unit)
    : `${formatMetric(m.value, m.unit)} ${metricLabel(type, m)}`
}

/** Kachel je Dienst: erste Kennzahl groß, zwei weitere als Nebenzeile, Probleme, Mini-Verlauf. */
function ServiceTile({
  integration: i,
  icon,
  problems,
}: {
  integration: Integration
  icon?: string
  problems: Problem[]
}) {
  const { t } = useTranslation()
  const main = (i.snapshot?.metrics ?? []).filter((m) => !m.group && !m.resource)
  const [first, ...rest] = main
  const spark = sparkMetric(main)
  const ports = (i.snapshot?.resources ?? []).filter((r) => r.kind === 'port')
  const server = i.snapshot?.resources?.find((r) => r.kind === 'server')
  return (
    <StatusTile
      to={systemLink(i)}
      icon={<IntegrationIcon name={icon} type={i.type} className="size-4" />}
      title={i.name}
      tile={i.name}
      status={
        worstStatus([...problems.map((p) => p.status), i.status === 'error' ? 'error' : undefined]) ??
        (first ? 'ok' : 'unknown')
      }
      value={
        !first ? (
          <span className="text-muted-foreground font-medium">{t('services.noMetrics')}</span>
        ) : isStateUnit(first.unit) ? (
          <TileValue>{formatMetric(first.value, first.unit)}</TileValue>
        ) : (
          <TileValue label={metricLabel(i.type, first)}>
            <AnimatedNumber value={first.value} format={(v) => formatMetric(v, first.unit)} />
          </TileValue>
        )
      }
      sub={rest
        .slice(0, 2)
        .map((m) => metricText(i.type, m))
        .join(' · ')}
      problems={problems.length}
    >
      {ports.length > 0 ? (
        <div className="mt-3">
          <SwitchFront ports={ports} compact />
        </div>
      ) : (
        <>
          {server && (
            <div className="mt-3">
              <ServerHealth metrics={(i.snapshot?.metrics ?? []).filter((m) => m.resource === server.id)} compact />
            </div>
          )}
          {spark && <Sparkline integrationId={i.id} metricKey={spark.key} range={spark.range} />}
        </>
      )}
    </StatusTile>
  )
}

const pct = (used?: number, total?: number) => (used !== undefined && total ? (used / total) * 100 : undefined)

/** Alle Hosts/Nodes (Proxmox VE, Dockhand) mit kleinen CPU- und RAM-Balken. */
function HostsTile({
  integrations,
  icons,
}: {
  integrations: Integration[]
  icons: Record<string, string | undefined>
}) {
  const { t } = useTranslation()
  const hosts = integrations.flatMap((i) =>
    (i.snapshot?.resources ?? []).filter((r) => r.kind === 'node').map((r) => ({ i, r })),
  )
  if (hosts.length === 0) return null
  hosts.sort((a, b) => statusRank(a.r.status) - statusRank(b.r.status) || a.r.name.localeCompare(b.r.name))
  const online = hosts.filter((h) => h.r.status !== 'error').length
  return (
    <StatusTile
      to="/systeme"
      icon={<Server className="size-4" />}
      title={t('dashboard.tile.hosts')}
      tile="hosts"
      status={worstStatus(hosts.map((h) => h.r.status)) ?? 'unknown'}
      sub={t('dashboard.hostsOnline', { count: online, total: hosts.length })}
    >
      <ul className="mt-2 flex flex-col gap-2.5">
        {hosts.map(({ i, r }) => (
          <li
            key={`${i.id}/${r.id}`}
            className="grid grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)_minmax(0,1fr)] items-center gap-x-4"
            data-host={r.name}
          >
            <span className="flex min-w-0 items-center gap-2">
              <IntegrationIcon name={icons[i.type]} type={i.type} className="size-4 shrink-0" />
              <span className="truncate text-sm font-medium">{r.name}</span>
              <StatusDot status={r.status} pulse />
            </span>
            <Meter
              size="sm"
              value={r.cpu}
              label={t('resources.cpu')}
              detail={r.cpu !== undefined ? formatMetric(r.cpu, '%') : '—'}
            />
            <Meter
              size="sm"
              value={pct(r.mem_used, r.mem_total)}
              label={t('resources.mem')}
              detail={
                r.mem_used !== undefined && r.mem_total
                  ? `${formatBytes(r.mem_used)} / ${formatBytes(r.mem_total)}`
                  : '—'
              }
            />
          </li>
        ))}
      </ul>
    </StatusTile>
  )
}

const timelineRanges: UptimeRange[] = ['24h', '7d']

/** Zeitleiste: je Backup ein Verlaufsbalken, Probleme zuerst. */
function BackupTimelineTile() {
  const { t } = useTranslation()
  const backups = useBackups()
  const [range, setRange] = useState<UptimeRange>('24h')
  const list = [...(backups.data?.backups ?? [])].sort(
    (a, b) => statusRank(a.status) - statusRank(b.status) || a.name.localeCompare(b.name),
  )
  const max = 8
  return (
    <StatusTile
      to="/backups"
      icon={<GanttChart className="size-4" />}
      title={t('dashboard.tile.backupTimeline')}
      tile="backup-timeline"
      status={worstStatus(list.map((b) => b.status)) ?? 'unknown'}
      stretch={false}
      action={
        <Segmented
          label={t('uptime.period')}
          value={range}
          onChange={setRange}
          options={timelineRanges.map((r) => ({
            value: r,
            label: t(`uptime.range.${r}`),
          }))}
        />
      }
    >
      {backups.isPending ? (
        <Skeleton className="mt-2 h-24" />
      ) : list.length === 0 ? (
        <p className="text-muted-foreground mt-2 text-sm">{t('dashboard.noBackups')}</p>
      ) : (
        <ul className="mt-3 flex flex-col gap-2">
          {list.slice(0, max).map((b) => (
            <li
              key={`${b.integration_id}/${b.id}`}
              className="grid grid-cols-[minmax(0,12rem)_minmax(0,1fr)] items-center gap-x-4"
              data-backup={b.name}
            >
              <span className="flex min-w-0 items-center gap-2 text-sm">
                <StatusDot status={b.status} />
                <span className="truncate font-medium">{b.name}</span>
                {b.group && <span className="text-muted-foreground truncate text-xs">{b.group}</span>}
              </span>
              {b.uptime ? (
                <UptimeBar uptime={b.uptime[range]} range={range} compact />
              ) : (
                <span className="text-muted-foreground text-sm">—</span>
              )}
            </li>
          ))}
          {list.length > max && (
            <li>
              <Link to="/backups" className="text-muted-foreground hover:text-foreground text-xs">
                {t('dashboard.moreBackups', { count: list.length - max })}
              </Link>
            </li>
          )}
        </ul>
      )}
    </StatusTile>
  )
}

/** Letzte Statuswechsel aller Integrationen: Überschrift außen, Liste flach. */
function EventsTile() {
  const { t } = useTranslation()
  const history = useHistory({}, 8)
  const entries = history.data?.pages[0]?.entries ?? []
  return (
    <section className="flex h-full min-h-0 flex-col gap-2" data-tile="events">
      <div className="flex items-center justify-between gap-2 px-1">
        <h2 className="flex items-center gap-2 font-medium">
          <HistoryIcon className="size-4" />
          {t('dashboard.events')}
        </h2>
        <Link to="/verlauf" className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-sm">
          {t('dashboard.allEvents')}
          <ChevronRight className="size-4" />
        </Link>
      </div>
      <div className="bg-card min-h-0 flex-1 overflow-y-auto rounded-xl border px-5">
        {history.isPending ? (
          <Skeleton className="my-4 h-24" />
        ) : entries.length === 0 ? (
          <p className="text-muted-foreground py-4 text-sm">{t('history.empty')}</p>
        ) : (
          <HistoryList entries={entries} compact />
        )}
      </div>
    </section>
  )
}


const dashboardIcons: Record<string, LucideIcon> = {
  'layout-dashboard': LayoutDashboard,
  network: Network,
  server: Server,
  'hard-drive': HardDrive,
  'shield-check': ShieldCheck,
  house: House,
  activity: Activity,
  gauge: Gauge,
  cpu: Cpu,
  image: Image,
}

function DashboardIcon({ name, className }: { name?: string; className?: string }) {
  const Icon = (name && dashboardIcons[name]) || LayoutDashboard
  return <Icon className={className} aria-hidden />
}

/** Bezeichnung einer Kachel (Editor, Dialog, Bedienelemente). */
function useTileLabel(tiles: DashboardTile[]) {
  const { t } = useTranslation()
  const integrations = useIntegrations().data ?? []
  return (id: string) => {
    if (id === 'backups') return t('nav.backups')
    if (id === 'checks') return t('nav.checks')
    if (id === 'events') return t('dashboard.events')
    if (id === 'hosts') return t('dashboard.tile.hosts')
    if (id === 'backup-timeline') return t('dashboard.tile.backupTimeline')
    const tile = tiles.find((x) => x.id === id)
    if (tile?.options) {
      const i = integrations.find((x) => x.id === tile.options!.integration_id)
      const m = i?.snapshot?.metrics?.find((x) => x.key === tile.options!.keys[0])
      return tile.options.label || (i && m ? metricTitle(i.type, m) : t('tile.metric'))
    }
    return integrations.find((x) => `integration:${x.id}` === id)?.name ?? id
  }
}

/**
 * Problemliste und Kacheln eines Dashboards. Die Kacheln zeigen nur die
 * Anzahl ihrer Probleme; was genau kaputt ist, steht oben in der Liste.
 */
function Board({
  overview,
  dashboard,
  editing,
  onLayout,
  onRemove,
  onSettings,
}: {
  overview: Overview
  dashboard: DashboardT
  editing: boolean
  onLayout: (l: Layout) => void
  onRemove: (id: string) => void
  onSettings: (id: string) => void
}) {
  const integrations = useIntegrations()
  const types = useIntegrationTypes()
  const list = integrations.data ?? []
  const icons = Object.fromEntries((types.data?.types ?? []).map((x) => [x.type, x.icon]))
  const label = useTileLabel(dashboard.tiles)

  const byOwner = new Map<string, Problem[]>()
  for (const p of overview.problems) {
    const o = problemOwner(p)
    byOwner.set(o, [...(byOwner.get(o) ?? []), p])
  }
  const mine = (id: string) => byOwner.get(id) ?? []

  function render(tile: DashboardTile) {
    const id = tile.id
    switch (id) {
      case 'backups':
        return <BackupsTile overview={overview} problems={mine(id)} />
      case 'checks':
        return <ChecksTile overview={overview} problems={mine(id)} />
      case 'hosts':
        return <HostsTile integrations={list.filter((i) => i.enabled)} icons={icons} />
      case 'backup-timeline':
        return <BackupTimelineTile />
      case 'events':
        return <EventsTile />
    }
    if (tile.options) {
      const i = list.find((x) => x.id === tile.options!.integration_id)
      return <MetricTile tile={tile} integration={i} icon={i ? icons[i.type] : undefined} />
    }
    const i = list.find((x) => `integration:${x.id}` === id)
    if (!i || !i.enabled) return null
    return <ServiceTile integration={i} icon={icons[i.type]} problems={mine(id)} />
  }

  const rank = (id: string) => Math.min(statusRank(worstStatus(mine(id).map((p) => p.status)) ?? 'ok'), 2)

  return (
    <>
      <ProblemList overview={overview} />
      <DashboardGrid
        tiles={dashboard.tiles}
        render={render}
        label={label}
        editing={editing}
        rank={dashboard.problems_first ? rank : undefined}
        onLayout={onLayout}
        onRemove={onRemove}
        onSettings={onSettings}
      />
    </>
  )
}

/** Reiter der Dashboards (ab zwei bzw. im Bearbeiten-Modus). */
function DashboardTabs({
  list,
  current,
  editing,
  onCreate,
}: {
  list: DashboardT[]
  current: string
  editing: boolean
  onCreate: () => void
}) {
  const { t } = useTranslation()
  const name = useDashboardName()
  if (list.length < 2 && !editing) return null
  return (
    <nav className="-mx-1 flex items-center gap-1 overflow-x-auto px-1 pb-1" aria-label={t('dashboard.tabs')}>
      {list.map((d) => (
        <Link
          key={d.id}
          to={d.id === 'main' ? '/' : `/d/${d.id}`}
          aria-current={d.id === current ? 'page' : undefined}
          className={cn(
            'flex shrink-0 items-center gap-2 rounded-lg px-3 py-1.5 text-sm transition-colors',
            d.id === current ? 'bg-accent text-foreground font-medium' : 'text-muted-foreground hover:bg-accent/60 hover:text-foreground',
          )}
          data-dashboard-tab={d.id}
        >
          <DashboardIcon name={d.icon} className="size-4" />
          {name(d)}
        </Link>
      ))}
      {editing && (
        <button
          type="button"
          onClick={onCreate}
          className="text-muted-foreground hover:bg-accent/60 hover:text-foreground flex shrink-0 items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm"
        >
          <Plus className="size-4" />
          {t('dashboard.newDashboard')}
        </button>
      )}
    </nav>
  )
}

function useDashboardName() {
  const { t } = useTranslation()
  return (d: DashboardT) => d.name || t('dashboard.mainName')
}

/** Leiste im Bearbeiten-Modus: Name, Symbol, Kachel hinzufügen, Löschen, Fertig. */
function EditBar({
  draft,
  isMain,
  canDelete,
  saving,
  error,
  onChange,
  onAdd,
  onDelete,
  onCancel,
  onDone,
}: {
  draft: DashboardT
  isMain: boolean
  canDelete: boolean
  saving: boolean
  error?: string
  onChange: (d: DashboardT) => void
  onAdd: () => void
  onDelete: () => void
  onCancel: () => void
  onDone: () => void
}) {
  const { t } = useTranslation()
  const [confirm, setConfirm] = useState(false)
  return (
    <div className="bg-card flex flex-col gap-3 rounded-xl border p-4" data-edit-bar>
      <div className="flex flex-wrap items-end gap-3">
        <div className="grid min-w-48 flex-1 gap-1.5">
          <Label htmlFor="dashboard-name">{t('dashboard.name')}</Label>
          <Input
            id="dashboard-name"
            maxLength={60}
            value={draft.name}
            placeholder={isMain ? t('dashboard.mainName') : ''}
            onChange={(e) => onChange({ ...draft, name: e.target.value })}
          />
        </div>
        <div className="grid gap-1.5">
          <span className="text-sm font-medium">{t('dashboard.icon')}</span>
          <div className="flex flex-wrap gap-1" role="radiogroup" aria-label={t('dashboard.icon')}>
            {Object.keys(dashboardIcons).map((n) => (
              <button
                key={n}
                type="button"
                role="radio"
                aria-checked={(draft.icon || 'layout-dashboard') === n}
                aria-label={n}
                onClick={() => onChange({ ...draft, icon: n })}
                className={cn(
                  'grid size-9 place-items-center rounded-md border',
                  (draft.icon || 'layout-dashboard') === n ? 'border-primary bg-accent' : 'hover:bg-accent/60',
                )}
              >
                <DashboardIcon name={n} className="size-4" />
              </button>
            ))}
          </div>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="outline" onClick={onAdd} data-action="add-tile">
          <Plus className="size-4" />
          {t('tile.addTitle')}
        </Button>
        <label className="text-muted-foreground flex items-center gap-2 text-sm">
          <Switch checked={draft.problems_first} onCheckedChange={(v) => onChange({ ...draft, problems_first: v })} />
          {t('dashboard.problemsFirst')}
        </label>
        <div className="ml-auto flex flex-wrap gap-2">
          {canDelete && (
            <Button variant="ghost" className="text-destructive" onClick={() => setConfirm(true)}>
              <Trash2 className="size-4" />
              {t('dashboard.delete')}
            </Button>
          )}
          <Button variant="outline" onClick={onCancel}>
            {t('common.cancel')}
          </Button>
          <Button onClick={onDone} disabled={saving || (!isMain && !draft.name.trim())} data-action="done">
            <Check className="size-4" />
            {t('dashboard.done')}
          </Button>
        </div>
      </div>
      <p className="text-muted-foreground text-xs">{t('dashboard.editHint')}</p>
      {error && <p className="text-destructive text-sm">{error}</p>}
      <AlertDialog open={confirm} onOpenChange={setConfirm}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('dashboard.deleteTitle', { name: draft.name || t('dashboard.mainName') })}</AlertDialogTitle>
            <AlertDialogDescription>{t('dashboard.deleteHint')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={onDelete}>{t('common.delete')}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

export function Dashboard() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const params = useParams()
  const overview = useOverview()
  const dashboards = useDashboards()
  const integrations = useIntegrations()
  const types = useIntegrationTypes()
  const save = useSaveDashboard()
  const create = useCreateDashboard()
  const remove = useDeleteDashboard()
  const name = useDashboardName()
  // Bearbeiten gilt für genau ein Dashboard; ein anderer Reiter beendet es
  const [editId, setEditId] = useState<string>()
  const [draft, setDraft] = useState<DashboardT>()
  const [dialog, setDialog] = useState<{ edit?: string }>()

  const list = dashboards.data ?? []
  const mainId = list.find((d) => d.id === 'main')?.id ?? list[0]?.id
  const current = list.find((d) => d.id === (params.id ?? mainId))
  const editing = !!current && editId === current.id && !!draft
  const shown = editing ? draft : current
  const o = overview.data
  const icons = Object.fromEntries((types.data?.types ?? []).map((x) => [x.type, x.icon]))
  const label = useTileLabel(shown?.tiles ?? [])

  if (dashboards.isSuccess && params.id && !current) return <Navigate to="/" replace />

  const isMain = current?.id === mainId
  const update = (fn: (d: DashboardT) => DashboardT) => setDraft((d) => (d ? fn(d) : d))

  function startEdit() {
    if (!current) return
    setDraft(structuredClone(current))
    setEditId(current.id)
    save.reset()
  }
  function onLayout(l: Layout) {
    update((d) => ({
      ...d,
      tiles: d.tiles.map((tile) => {
        const p = l.find((x) => x.i === tile.id)
        return p ? { ...tile, x: p.x, y: p.y, w: p.w, h: p.h } : tile
      }),
    }))
  }
  function onRemove(id: string) {
    // eingebaute Kacheln auf dem Haupt-Dashboard nur ausblenden (sonst kommen sie wieder)
    update((d) => ({
      ...d,
      tiles:
        isMain && !id.startsWith('metric:')
          ? d.tiles.map((x) => (x.id === id ? { ...x, visible: false } : x))
          : d.tiles.filter((x) => x.id !== id),
    }))
  }
  function onAdd(id: string) {
    update((d) => ({ ...d, tiles: appendTile(d.tiles, { id, visible: true }) }))
    setDialog(undefined)
  }
  function onSaveMetric(options: TileOptions) {
    const edit = dialog?.edit
    update((d) => ({
      ...d,
      tiles: edit
        ? d.tiles.map((x) => (x.id === edit ? { ...x, options } : x))
        : appendTile(d.tiles, { id: newMetricTileId(), visible: true, options }),
    }))
    setDialog(undefined)
  }
  function onDone() {
    if (!draft) return
    save.mutate(draft, { onSuccess: () => setEditId(undefined) })
  }
  function onCreate() {
    create.mutate(
      { name: t('dashboard.newName', { n: list.length + 1 }), icon: 'layout-dashboard' },
      {
        onSuccess: (r) => {
          const d = r.dashboards[r.dashboards.length - 1]
          setDraft(structuredClone(d))
          setEditId(d.id)
          navigate(`/d/${d.id}`)
        },
      },
    )
  }
  function onDelete() {
    if (!current) return
    remove.mutate(current.id, {
      onSuccess: () => {
        setEditId(undefined)
        navigate('/')
      },
    })
  }

  const visibleIds = new Set((shown?.tiles ?? []).filter((x) => x.visible).map((x) => x.id))
  const addable = availableTiles(integrations.data ?? []).filter((id) => !visibleIds.has(id))
  const editTile = dialog?.edit ? shown?.tiles.find((x) => x.id === dialog.edit) : undefined

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <PageHeader
          title={list.length > 1 && current ? name(current) : t('nav.dashboard')}
          subtitle={t('dashboard.subtitle')}
        />
        {o && o.integrations.total > 0 && current && !editing && (
          <Button variant="outline" size="sm" onClick={startEdit} data-action="edit-dashboard">
            <Pencil className="size-4" />
            {t('dashboard.edit')}
          </Button>
        )}
      </div>
      {current && <DashboardTabs list={list} current={current.id} editing={editing} onCreate={onCreate} />}
      {editing && draft && (
        <EditBar
          draft={draft}
          isMain={isMain}
          canDelete={list.length > 1}
          saving={save.isPending}
          error={save.error?.message ?? create.error?.message ?? remove.error?.message}
          onChange={setDraft}
          onAdd={() => setDialog({})}
          onDelete={onDelete}
          onCancel={() => setEditId(undefined)}
          onDone={onDone}
        />
      )}
      {overview.isPending || dashboards.isPending ? (
        <>
          <Skeleton className="h-12 rounded-xl" />
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Skeleton className="h-32 rounded-xl" />
            <Skeleton className="h-32 rounded-xl" />
            <Skeleton className="h-32 rounded-xl" />
          </div>
        </>
      ) : overview.isError ? (
        <p className="text-destructive text-sm">{t('dashboard.loadError', { error: overview.error.message })}</p>
      ) : dashboards.isError ? (
        <p className="text-destructive text-sm">{t('dashboard.loadError', { error: dashboards.error.message })}</p>
      ) : o!.integrations.total === 0 ? (
        <NoIntegrations />
      ) : (
        shown && (
          <>
            <Board
              overview={o!}
              dashboard={shown}
              editing={editing}
              onLayout={onLayout}
              onRemove={onRemove}
              onSettings={(id) => setDialog({ edit: id })}
            />
            {!editing && shown.tiles.filter((x) => x.visible).length === 0 && (
              <button
                type="button"
                onClick={startEdit}
                className="text-muted-foreground hover:bg-accent/40 flex flex-col items-center gap-2 rounded-xl border border-dashed p-10 text-sm"
              >
                <Plus className="size-5" />
                {t('dashboard.empty')}
              </button>
            )}
          </>
        )
      )}
      {dialog && (
        <TileDialog
          open
          onOpenChange={(v) => !v && setDialog(undefined)}
          available={addable}
          label={label}
          integrations={integrations.data ?? []}
          icons={icons}
          edit={editTile?.options}
          onAdd={onAdd}
          onSaveMetric={onSaveMetric}
        />
      )}
    </div>
  )
}
