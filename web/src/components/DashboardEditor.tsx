import { ArrowDown, ArrowUp, GripVertical, LayoutDashboard } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Section } from '@/components/Section'
import { Segmented } from '@/components/Segmented'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import type { DashboardTile, Integration } from '@/lib/api'
import { useDashboardLayout, useIntegrations, useSaveDashboardLayout } from '@/lib/queries'
import { cn } from '@/lib/utils'

/** Anzeigename einer Kachel. */
function useTileLabel() {
  const { t } = useTranslation()
  const integrations = useIntegrations()
  return (id: string) => {
    if (id === 'backups') return t('nav.backups')
    if (id === 'checks') return t('nav.checks')
    if (id === 'events') return t('dashboard.events')
    if (id === 'hosts') return t('dashboard.tile.hosts')
    if (id === 'backup-timeline') return t('dashboard.tile.backupTimeline')
    const i: Integration | undefined = integrations.data?.find((x) => `integration:${x.id}` === id)
    return i?.name ?? id
  }
}

/** Einstellungen → Dashboard: Kacheln anordnen, ein-/ausblenden, Größe. */
export function DashboardEditor() {
  const { t } = useTranslation()
  const layout = useDashboardLayout()
  const save = useSaveDashboardLayout()
  const label = useTileLabel()
  const [dragging, setDragging] = useState<number>()
  const tiles = layout.data?.tiles ?? []
  const problemsFirst = layout.data?.problems_first ?? true

  function commit(next: DashboardTile[], first = problemsFirst) {
    save.mutate({ tiles: next, problems_first: first })
  }
  function move(from: number, to: number) {
    if (to < 0 || to >= tiles.length || from === to) return
    const next = [...tiles]
    const [item] = next.splice(from, 1)
    next.splice(to, 0, item)
    commit(next)
  }
  const patch = (i: number, p: Partial<DashboardTile>) => commit(tiles.map((x, n) => (n === i ? { ...x, ...p } : x)))

  return (
    <Section
      icon={<LayoutDashboard className="size-5" />}
      title={t('settings.dashboard')}
      description={t('settings.dashboardHint')}
    >
      {layout.isPending ? (
        <Skeleton className="h-32" />
      ) : layout.isError ? (
        <p className="text-destructive text-sm">{layout.error.message}</p>
      ) : (
        <>
          <label className="flex items-start gap-3">
            <Switch
              checked={problemsFirst}
              onCheckedChange={(on) => commit(tiles, on)}
              disabled={save.isPending}
              className="mt-0.5"
            />
            <span className="flex flex-col gap-0.5">
              <span className="text-sm font-medium">{t('dashboard.problemsFirst')}</span>
              <span className="text-muted-foreground text-xs">{t('dashboard.problemsFirstHint')}</span>
            </span>
          </label>
          <ul className="divide-y rounded-md border" aria-label={t('settings.dashboardTiles')}>
            {tiles.map((tile, i) => (
              <li
                key={tile.id}
                draggable
                onDragStart={(e) => {
                  setDragging(i)
                  e.dataTransfer.effectAllowed = 'move'
                }}
                onDragOver={(e) => e.preventDefault()}
                onDrop={(e) => {
                  e.preventDefault()
                  if (dragging !== undefined) move(dragging, i)
                  setDragging(undefined)
                }}
                onDragEnd={() => setDragging(undefined)}
                className={cn(
                  'flex flex-wrap items-center gap-3 px-3 py-2.5',
                  dragging === i && 'opacity-50',
                  !tile.visible && 'text-muted-foreground',
                )}
                data-tile-setting={tile.id}
              >
                <GripVertical className="text-muted-foreground size-4 shrink-0 cursor-grab" aria-hidden />
                <Switch
                  checked={tile.visible}
                  aria-label={t('settings.tileVisible', { name: label(tile.id) })}
                  onCheckedChange={(on) => patch(i, { visible: on })}
                  disabled={save.isPending}
                />
                <span className="min-w-0 flex-1 font-medium">{label(tile.id)}</span>
                <Segmented
                  label={t('settings.tileSize', { name: label(tile.id) })}
                  value={String(tile.size)}
                  onChange={(v) => patch(i, { size: Number(v) })}
                  options={[1, 2, 3].map((n) => ({ value: String(n), label: t('settings.columns', { count: n }) }))}
                />
                <div className="flex">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={t('settings.tileUp', { name: label(tile.id) })}
                    disabled={i === 0 || save.isPending}
                    onClick={() => move(i, i - 1)}
                  >
                    <ArrowUp />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={t('settings.tileDown', { name: label(tile.id) })}
                    disabled={i === tiles.length - 1 || save.isPending}
                    onClick={() => move(i, i + 1)}
                  >
                    <ArrowDown />
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
      {save.isError && (
        <p className="text-destructive text-sm" role="alert">
          {save.error.message}
        </p>
      )}
    </Section>
  )
}
