import { useTranslation } from 'react-i18next'

import { AnimatedNumber } from '@/components/AnimatedNumber'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { Metric } from '@/lib/api'
import { anchorId } from '@/lib/anchor'
import { formatMetric, isStateUnit, metricBase, metricLabel } from '@/lib/metrics'
import { cn } from '@/lib/utils'

const statusDot: Record<string, string> = { warning: 'bg-warning', error: 'bg-destructive' }

// Überschrift der Gruppen-Spalte je Integrationstyp
const groupTitle: Record<string, 'user' | 'datastore'> = { immich: 'user', pbs: 'datastore' }

/** Große Kennzahl; anklickbar, um sie im Diagramm zu zeigen. */
export function Stat({ m, label, selected, onSelect }: { m: Metric; label: string; selected: boolean; onSelect: () => void }) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className={cn(
        'hover:bg-accent/40 flex flex-col items-start gap-1 rounded-lg border p-3 text-left transition-colors',
        selected && 'border-ring bg-accent/40',
      )}
    >
      <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
        {statusDot[m.status] && <span className={cn('size-2 rounded-full', statusDot[m.status])} aria-hidden />}
        <span className="inline-block first-letter:uppercase">{label}</span>
      </span>
      <span className="text-xl font-semibold tabular-nums">
        {isStateUnit(m.unit) ? formatMetric(m.value, m.unit) : <AnimatedNumber value={m.value} format={(v) => formatMetric(v, m.unit)} />}
      </span>
    </button>
  )
}

// Bekannte Arten gruppierter Kennzahlen (Anfang des Schlüssels) bekommen je
// eine eigene Tabelle; alles andere landet gemeinsam in einer.
const sections = ['storage', 'disk', 'zfs', 'fs', 'ups', 'user'] as const
type Section = (typeof sections)[number] | 'other'

function sectionOf(key: string): Section {
  const prefix = key.slice(0, key.indexOf('/'))
  return sections.find((s) => s === prefix) ?? 'other'
}

/** Gruppierte Kennzahlen, je Art eine Tabelle. */
export function GroupTables({ type, metrics }: { type: string; metrics: Metric[] }) {
  const bySection = new Map<Section, Metric[]>()
  for (const m of metrics) {
    const s = sectionOf(m.key)
    bySection.set(s, [...(bySection.get(s) ?? []), m])
  }
  return [...bySection].map(([s, list]) => <Groups key={s} type={type} section={s} metrics={list} />)
}

function Groups({ type, section, metrics }: { type: string; section: Section; metrics: Metric[] }) {
  const { t } = useTranslation()
  const groups = [...new Set(metrics.map((m) => m.group!))]
  const columns = [...new Set(metrics.map((m) => metricBase(m.key)))]
  const cell = (g: string, c: string) => metrics.find((m) => m.group === g && metricBase(m.key) === c)
  return (
    <div className="overflow-x-auto rounded-md border">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead>
              {section === 'other'
                ? t(`services.groupTitle.${groupTitle[type] ?? 'group'}`)
                : t(`services.sections.${section}`)}
            </TableHead>
            {columns.map((c) => (
              <TableHead key={c} className="text-right">
                <span className="inline-block first-letter:uppercase">{metricLabel(type, metrics.find((m) => metricBase(m.key) === c)!)}</span>
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {groups.map((g) => (
            <TableRow key={g} data-anchor={anchorId('grp', g)}>
              <TableCell className="font-medium">{g}</TableCell>
              {columns.map((c) => {
                const m = cell(g, c)
                return (
                  <TableCell
                    key={c}
                    className={cn(
                      'text-right tabular-nums',
                      m?.status === 'warning' && 'text-warning',
                      m?.status === 'error' && 'text-destructive',
                    )}
                  >
                    {m ? formatMetric(m.value, m.unit) : '—'}
                  </TableCell>
                )
              })}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

