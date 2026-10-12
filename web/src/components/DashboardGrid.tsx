import { GripVertical, Settings2, X } from 'lucide-react'
import type { ReactNode } from 'react'
import ReactGridLayout, { useContainerWidth, verticalCompactor, type Layout } from 'react-grid-layout'
import 'react-grid-layout/css/styles.css'
import { useTranslation } from 'react-i18next'

import type { DashboardTile } from '@/lib/api'
import { rise } from '@/lib/motion'
import { GRID_COLS, GRID_MARGIN, MAX_TILE_HEIGHT, ROW_HEIGHT, stackOrder } from '@/lib/tiles'
import { cn } from '@/lib/utils'

/** Unter dieser Breite stehen die Kacheln untereinander (Handy). */
const STACK_BELOW = 640

const px = (h: number) => h * ROW_HEIGHT + (h - 1) * GRID_MARGIN

/** Leiste einer Kachel im Bearbeiten-Modus: Griff, Einstellungen, Entfernen. */
function EditOverlay({
  label,
  onRemove,
  onSettings,
  draggable,
}: {
  label: string
  onRemove: () => void
  onSettings?: () => void
  draggable: boolean
}) {
  const { t } = useTranslation()
  return (
    <div
      className={cn(
        'border-primary/50 bg-background/30 absolute inset-0 z-20 rounded-xl border-2 border-dashed',
        draggable && 'cursor-grab active:cursor-grabbing',
      )}
      data-edit-overlay={label}
    >
      <div className="tile-toolbar absolute top-2 right-2 flex gap-1">
        {onSettings && (
          <button
            type="button"
            onClick={onSettings}
            className="bg-background hover:bg-accent grid size-8 place-items-center rounded-md border shadow-sm"
            aria-label={t('tile.settings', { name: label })}
          >
            <Settings2 className="size-4" />
          </button>
        )}
        <button
          type="button"
          onClick={onRemove}
          className="bg-background hover:bg-destructive hover:text-destructive-foreground grid size-8 place-items-center rounded-md border shadow-sm"
          aria-label={t('tile.remove', { name: label })}
        >
          <X className="size-4" />
        </button>
      </div>
      {draggable && (
        <span className="bg-background text-muted-foreground absolute top-2 left-2 grid size-8 place-items-center rounded-md border shadow-sm" aria-hidden>
          <GripVertical className="size-4" />
        </span>
      )}
    </div>
  )
}

/**
 * Kacheln im Raster aus 12 Spalten: ansehen oder (editing) ziehen und in der
 * Größe ändern. Auf schmalen Bildschirmen untereinander – dort ohne Ziehen,
 * auf Wunsch Kacheln mit Problemen zuerst.
 */
export function DashboardGrid({
  tiles,
  render,
  label,
  editing,
  rank,
  onLayout,
  onRemove,
  onSettings,
}: {
  tiles: DashboardTile[]
  /** Inhalt einer Kachel; null = (noch) nichts anzuzeigen */
  render: (t: DashboardTile) => ReactNode
  label: (id: string) => string
  editing: boolean
  /** Rang für „Probleme zuerst“ in der gestapelten Ansicht (undefined = aus) */
  rank?: (id: string) => number
  onLayout: (layout: Layout) => void
  onRemove: (id: string) => void
  onSettings: (id: string) => void
}) {
  const { width, containerRef, mounted } = useContainerWidth()
  const shown = tiles.filter((t) => t.visible).map((t) => ({ t, content: render(t) }))
  const visible = shown.filter((x) => x.content !== null || editing)
  const stacked = mounted && width < STACK_BELOW

  const overlay = (t: DashboardTile, draggable: boolean) =>
    editing && (
      <EditOverlay
        label={label(t.id)}
        draggable={draggable}
        onRemove={() => onRemove(t.id)}
        onSettings={t.id.startsWith('metric:') ? () => onSettings(t.id) : undefined}
      />
    )

  return (
    <div ref={containerRef} className="min-w-0" data-grid={stacked ? 'stacked' : 'grid'}>
      {stacked ? (
        <div className="flex flex-col gap-4">
          {stackOrder(
            visible.map((x) => x.t),
            rank,
          ).map((t, n) => {
            const x = visible.find((v) => v.t.id === t.id)!
            const motion = rise(n)
            return (
              <div
                key={t.id}
                className={cn('relative min-w-0 [&>*:first-child]:h-full', motion.className)}
                style={{ ...motion.style, height: t.id.startsWith('metric:') ? px(t.h) : undefined }}
                data-grid-tile={t.id}
              >
                {x.content}
                {overlay(t, false)}
              </div>
            )
          })}
        </div>
      ) : (
        mounted && (
          <ReactGridLayout
            width={width}
            layout={visible.map(({ t }) => ({ i: t.id, x: t.x, y: t.y, w: t.w, h: t.h, minW: 2, minH: 2, maxH: MAX_TILE_HEIGHT }))}
            gridConfig={{ cols: GRID_COLS, rowHeight: ROW_HEIGHT, margin: [GRID_MARGIN, GRID_MARGIN], containerPadding: [0, 0] }}
            dragConfig={{ enabled: editing, cancel: '.tile-toolbar', threshold: 4 }}
            resizeConfig={{ enabled: editing, handles: ['se', 'e', 's'] }}
            compactor={verticalCompactor}
            onLayoutChange={(l) => editing && onLayout(l)}
            className={cn(editing && 'is-editing')}
          >
            {visible.map(({ t, content }, n) => {
              const motion = rise(n)
              return (
                <div key={t.id} className="min-w-0" data-grid-tile={t.id}>
                  <div className={cn('h-full [&>*:first-child]:h-full', motion.className)} style={motion.style}>
                    {content ?? <div className="bg-muted/40 h-full rounded-xl border border-dashed" />}
                  </div>
                  {overlay(t, true)}
                </div>
              )
            })}
          </ReactGridLayout>
        )
      )}
    </div>
  )
}
