import { Check, Gauge, LayoutGrid, Plus, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { MetricTile } from '@/components/MetricTile'
import { Segmented } from '@/components/Segmented'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { tileRanges, tileViews, type Integration, type Metric, type TileOptions } from '@/lib/api'
import { formatMetric, isStateUnit, metricTitle } from '@/lib/metrics'
import { emptyOptions } from '@/lib/tiles'
import { cn } from '@/lib/utils'

/** Kennzahl-Kachel einstellen: Integration, 1–3 Kennzahlen, Darstellung, Zeitraum. */
function MetricForm({
  integrations,
  icons,
  value,
  onChange,
}: {
  integrations: Integration[]
  icons: Record<string, string | undefined>
  value: TileOptions
  onChange: (o: TileOptions) => void
}) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const sources = integrations.filter((i) => (i.snapshot?.metrics?.length ?? 0) > 0)
  const integration = sources.find((i) => i.id === value.integration_id)
  const metrics = integration?.snapshot?.metrics ?? []
  const chosen = value.keys.map((k) => metrics.find((m) => m.key === k)).filter((m): m is Metric => !!m)
  const unit = chosen[0]?.unit
  const q = query.trim().toLowerCase()
  const list = metrics
    .filter((m) => !q || `${metricTitle(integration!.type, m)} ${m.key}`.toLowerCase().includes(q))
    .slice(0, 200)
  const set = (p: Partial<TileOptions>) => onChange({ ...value, ...p })

  function toggle(m: Metric) {
    if (value.keys.includes(m.key)) set({ keys: value.keys.filter((k) => k !== m.key) })
    // mehrere Reihen nur mit gleicher Einheit (ein Diagramm, eine Achse)
    else if (value.keys.length === 0 || (value.keys.length < 3 && m.unit === unit && value.view !== 'gauge'))
      set({ keys: [...value.keys, m.key] })
    else set({ keys: [m.key] })
  }

  return (
    <div className="grid gap-4">
      <div className="grid gap-1.5">
        <Label htmlFor="tile-integration">{t('tile.integration')}</Label>
        <Select
          value={value.integration_id ? String(value.integration_id) : ''}
          onValueChange={(v) => set({ integration_id: Number(v), keys: [] })}
        >
          <SelectTrigger id="tile-integration" className="w-full">
            <SelectValue placeholder={t('tile.chooseIntegration')} />
          </SelectTrigger>
          <SelectContent>
            {sources.map((i) => (
              <SelectItem key={i.id} value={String(i.id)}>
                {i.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {integration && (
        <div className="grid gap-1.5">
          <Label htmlFor="tile-search">{t('tile.metrics')}</Label>
          <div className="relative">
            <Search className="text-muted-foreground absolute top-2.5 left-2.5 size-4" aria-hidden />
            <Input
              id="tile-search"
              className="pl-8"
              placeholder={t('tile.search')}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>
          <ul className="max-h-48 overflow-y-auto rounded-md border" aria-label={t('tile.metrics')}>
            {list.map((m) => {
              const on = value.keys.includes(m.key)
              return (
                <li key={m.key}>
                  <button
                    type="button"
                    onClick={() => toggle(m)}
                    aria-pressed={on}
                    className={cn('hover:bg-accent flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm', on && 'bg-accent')}
                    data-metric={m.key}
                  >
                    <span className={cn('grid size-4 shrink-0 place-items-center rounded-sm border', on && 'bg-primary text-primary-foreground border-primary')}>
                      {on && <Check className="size-3" />}
                    </span>
                    <span className="min-w-0 flex-1 truncate first-letter:uppercase">{metricTitle(integration.type, m)}</span>
                    <span className="text-muted-foreground shrink-0 text-xs tabular-nums">{formatMetric(m.value, m.unit)}</span>
                  </button>
                </li>
              )
            })}
            {list.length === 0 && <li className="text-muted-foreground px-3 py-2 text-sm">{t('tile.noMatch')}</li>}
          </ul>
          <p className="text-muted-foreground text-xs">{t('tile.metricsHint')}</p>
        </div>
      )}

      <div className="grid gap-3">
        <div className="grid gap-1.5">
          <span className="text-sm font-medium">{t('tile.view')}</span>
          <Segmented
            label={t('tile.view')}
            value={value.view}
            onChange={(v) => set({ view: v, keys: v === 'gauge' ? value.keys.slice(0, 1) : value.keys })}
            options={tileViews.map((v) => ({ value: v, label: t(`tile.views.${v}`) }))}
          />
        </div>
        <div className="grid gap-1.5">
          <span className="text-sm font-medium">{t('tile.rangeLabel')}</span>
          <Segmented
            label={t('tile.rangeLabel')}
            value={value.range}
            onChange={(v) => set({ range: v })}
            options={tileRanges.map((r) => ({ value: r, label: t(`tile.ranges.${r}`) }))}
          />
        </div>
      </div>

      {value.view === 'gauge' && (
        <div className="grid grid-cols-2 gap-3">
          {(['min', 'max'] as const).map((k) => (
            <div key={k} className="grid gap-1.5">
              <Label htmlFor={`tile-${k}`}>{t(`tile.${k}`)}</Label>
              <Input
                id={`tile-${k}`}
                type="number"
                inputMode="decimal"
                value={value[k] ?? ''}
                placeholder={k === 'min' ? '0' : unit === '%' ? '100' : t('tile.auto')}
                onChange={(e) => set({ [k]: e.target.value === '' ? undefined : Number(e.target.value) })}
              />
            </div>
          ))}
        </div>
      )}

      <div className="grid gap-1.5">
        <Label htmlFor="tile-label">{t('tile.label')}</Label>
        <Input
          id="tile-label"
          maxLength={80}
          value={value.label ?? ''}
          placeholder={chosen[0] && integration ? metricTitle(integration.type, chosen[0]) : ''}
          onChange={(e) => set({ label: e.target.value || undefined })}
        />
      </div>

      {integration && chosen.length > 0 && (
        <div className="grid gap-1.5">
          <span className="text-sm font-medium">{t('tile.preview')}</span>
          <div className="pointer-events-none h-52 [&>*]:h-full" aria-hidden>
            <MetricTile tile={{ id: 'metric:preview', visible: true, x: 0, y: 0, w: 4, h: 4, options: value }} integration={integration} icon={icons[integration.type]} />
          </div>
          {isStateUnit(unit) && value.view !== 'number' && <p className="text-muted-foreground text-xs">{t('tile.stateHint')}</p>}
        </div>
      )}
    </div>
  )
}

/**
 * Dialog „Kachel hinzufügen“: fertige Kacheln (eingebaut, je Integration) oder
 * eine eigene Kennzahl. Mit `edit` nur das Kennzahl-Formular (Einstellungen
 * einer bestehenden Kennzahl-Kachel).
 */
export function TileDialog({
  open,
  onOpenChange,
  available,
  label,
  integrations,
  icons,
  edit,
  onAdd,
  onSaveMetric,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** eingebaute/Integrations-Kacheln, die auf dem Dashboard fehlen */
  available: string[]
  label: (id: string) => string
  integrations: Integration[]
  icons: Record<string, string | undefined>
  edit?: TileOptions
  onAdd: (id: string) => void
  onSaveMetric: (o: TileOptions) => void
}) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<'tiles' | 'metric'>(edit ? 'metric' : available.length ? 'tiles' : 'metric')
  const [options, setOptions] = useState<TileOptions>(edit ?? emptyOptions())
  const valid = options.integration_id > 0 && options.keys.length > 0
  const sorted = useMemo(() => [...available].sort((a, b) => label(a).localeCompare(label(b))), [available, label])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{edit ? t('tile.editTitle') : t('tile.addTitle')}</DialogTitle>
          <DialogDescription>{edit ? t('tile.editHint') : t('tile.addHint')}</DialogDescription>
        </DialogHeader>
        {!edit && (
          <Segmented
            label={t('tile.addTitle')}
            value={tab}
            onChange={setTab}
            options={[
              { value: 'tiles', label: t('tile.tabTiles') },
              { value: 'metric', label: t('tile.tabMetric') },
            ]}
          />
        )}
        {tab === 'tiles' ? (
          sorted.length === 0 ? (
            <p className="text-muted-foreground py-6 text-center text-sm">{t('tile.allShown')}</p>
          ) : (
            <ul className="grid gap-2 sm:grid-cols-2">
              {sorted.map((id) => (
                <li key={id}>
                  <button
                    type="button"
                    onClick={() => onAdd(id)}
                    className="hover:bg-accent flex w-full items-center gap-2 rounded-lg border px-3 py-2 text-left text-sm"
                    data-add-tile={id}
                  >
                    <LayoutGrid className="text-muted-foreground size-4 shrink-0" aria-hidden />
                    <span className="min-w-0 flex-1 truncate">{label(id)}</span>
                    <Plus className="text-muted-foreground size-4" aria-hidden />
                  </button>
                </li>
              ))}
            </ul>
          )
        ) : (
          <MetricForm integrations={integrations} icons={icons} value={options} onChange={setOptions} />
        )}
        {tab === 'metric' && (
          <DialogFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {t('common.cancel')}
            </Button>
            <Button disabled={!valid} onClick={() => onSaveMetric(options)}>
              <Gauge className="size-4" />
              {edit ? t('common.save') : t('tile.addMetric')}
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  )
}
