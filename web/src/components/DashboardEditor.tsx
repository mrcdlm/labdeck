import { ArrowDown, ArrowUp, ChevronDown, LayoutDashboard, Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { Section } from '@/components/Section'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import type { Dashboard, DashboardTile } from '@/lib/api'
import { metricTitle } from '@/lib/metrics'
import {
  useCreateDashboard,
  useDashboards,
  useDeleteDashboard,
  useIntegrations,
  useOrderDashboards,
  useSaveDashboard,
} from '@/lib/queries'
import { appendTile } from '@/lib/tiles'
import { cn } from '@/lib/utils'

/** Anzeigename einer Kachel. */
function useTileLabel() {
  const { t } = useTranslation()
  const integrations = useIntegrations().data ?? []
  return (tile: DashboardTile) => {
    const id = tile.id
    if (id === 'backups') return t('nav.backups')
    if (id === 'checks') return t('nav.checks')
    if (id === 'events') return t('dashboard.events')
    if (id === 'hosts') return t('dashboard.tile.hosts')
    if (id === 'backup-timeline') return t('dashboard.tile.backupTimeline')
    if (tile.options) {
      const i = integrations.find((x) => x.id === tile.options!.integration_id)
      const m = i?.snapshot?.metrics?.find((x) => x.key === tile.options!.keys[0])
      return tile.options.label || (i && m ? metricTitle(i.type, m) : t('tile.metric'))
    }
    return integrations.find((x) => `integration:${x.id}` === id)?.name ?? id
  }
}

/** Kacheln eines Dashboards ein- und ausblenden (Alternative zum Ziehen). */
function TileSwitches({ d }: { d: Dashboard }) {
  const { t } = useTranslation()
  const save = useSaveDashboard()
  const label = useTileLabel()
  function toggle(tile: DashboardTile, on: boolean) {
    // eingeblendete Kachel unten anfügen, damit sie keine andere überdeckt
    const tiles = on ? appendTile(d.tiles, { ...tile, visible: true }) : d.tiles.map((x) => (x.id === tile.id ? { ...x, visible: false } : x))
    save.mutate({ ...d, tiles })
  }
  return (
    <ul className="divide-y border-t" aria-label={t('settings.dashboardTiles')}>
      {d.tiles.map((tile) => (
        <li key={tile.id} className={cn('flex items-center gap-3 px-3 py-2', !tile.visible && 'text-muted-foreground')} data-tile-setting={tile.id}>
          <Switch
            checked={tile.visible}
            aria-label={t('settings.tileVisible', { name: label(tile) })}
            onCheckedChange={(on) => toggle(tile, on)}
            disabled={save.isPending}
          />
          <span className="min-w-0 flex-1 truncate text-sm">{label(tile)}</span>
        </li>
      ))}
      {d.tiles.length === 0 && <li className="text-muted-foreground px-3 py-2 text-sm">{t('settings.noTiles')}</li>}
    </ul>
  )
}

/** Einstellungen → Dashboard: Dashboards anlegen, sortieren, löschen; Kacheln ein/aus. */
export function DashboardEditor() {
  const { t } = useTranslation()
  const dashboards = useDashboards()
  const create = useCreateDashboard()
  const order = useOrderDashboards()
  const remove = useDeleteDashboard()
  const [open, setOpen] = useState<string>()
  const list = dashboards.data ?? []
  const busy = create.isPending || order.isPending || remove.isPending
  const error = create.error ?? order.error ?? remove.error

  function move(from: number, to: number) {
    const ids = list.map((d) => d.id)
    const [id] = ids.splice(from, 1)
    ids.splice(to, 0, id)
    order.mutate(ids)
  }

  return (
    <Section icon={<LayoutDashboard className="size-5" />} title={t('settings.dashboard')} description={t('settings.dashboardHint')}>
      {dashboards.isPending ? (
        <Skeleton className="h-32" />
      ) : dashboards.isError ? (
        <p className="text-destructive text-sm">{dashboards.error.message}</p>
      ) : (
        <>
          <ul className="flex flex-col gap-2" aria-label={t('dashboard.tabs')}>
            {list.map((d, n) => {
              const name = d.name || t('dashboard.mainName')
              const shown = d.tiles.filter((x) => x.visible).length
              return (
                <li key={d.id} className="rounded-md border" data-dashboard-setting={d.id}>
                  <div className="flex flex-wrap items-center gap-2 px-3 py-2">
                    <button
                      type="button"
                      onClick={() => setOpen((o) => (o === d.id ? undefined : d.id))}
                      aria-expanded={open === d.id}
                      className="flex min-w-0 flex-1 items-center gap-2 text-left"
                    >
                      <ChevronDown className={cn('size-4 shrink-0 transition-transform', open === d.id && 'rotate-180')} aria-hidden />
                      <span className="truncate font-medium">{name}</span>
                      <span className="text-muted-foreground shrink-0 text-xs">{t('settings.tileCount', { count: shown })}</span>
                    </button>
                    <Button asChild variant="ghost" size="sm">
                      <Link to={d.id === 'main' ? '/' : `/d/${d.id}`}>
                        <Pencil className="size-4" />
                        {t('settings.arrange')}
                      </Link>
                    </Button>
                    <div className="flex">
                      <Button variant="ghost" size="icon" aria-label={t('settings.dashboardUp', { name })} disabled={n === 0 || busy} onClick={() => move(n, n - 1)}>
                        <ArrowUp />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t('settings.dashboardDown', { name })}
                        disabled={n === list.length - 1 || busy}
                        onClick={() => move(n, n + 1)}
                      >
                        <ArrowDown />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="text-destructive"
                        aria-label={t('settings.dashboardDelete', { name })}
                        disabled={list.length < 2 || busy}
                        onClick={() => remove.mutate(d.id)}
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  </div>
                  {open === d.id && <TileSwitches d={d} />}
                </li>
              )
            })}
          </ul>
          <Button
            variant="outline"
            className="self-start"
            disabled={busy}
            onClick={() => create.mutate({ name: t('dashboard.newName', { n: list.length + 1 }), icon: 'layout-dashboard' })}
          >
            <Plus className="size-4" />
            {t('dashboard.newDashboard')}
          </Button>
        </>
      )}
      {error && (
        <p className="text-destructive text-sm" role="alert">
          {error.message}
        </p>
      )}
    </Section>
  )
}
