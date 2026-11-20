import { Download, EyeOff, Network, Pencil, Plug } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { PageHeader } from '@/components/PageHeader'
import { TopologyEditor } from '@/components/topology/TopologyEditor'
import { ExposedPanel, LinkTable, NetworksPanel, Panel, Plan, ServiceChip } from '@/components/topology/TopologyView'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { useTopology } from '@/lib/queries'
import { RedactContext } from '@/lib/topology'

/** Bild des Plans (PNG oder SVG) herunterladen – Bibliothek erst beim Klick laden. */
async function exportImage(node: HTMLElement, kind: 'png' | 'svg') {
  const { toPng, toSvg } = await import('html-to-image')
  const bg = getComputedStyle(document.body).backgroundColor
  const opts = { backgroundColor: bg, pixelRatio: 2, cacheBust: true }
  const url = kind === 'png' ? await toPng(node, opts) : await toSvg(node, opts)
  const a = document.createElement('a')
  a.href = url
  a.download = `labdeck-netzplan-${new Date().toISOString().slice(0, 10)}.${kind}`
  a.click()
}

function Empty() {
  const { t } = useTranslation()
  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
        <span className="bg-muted grid size-12 place-items-center rounded-lg">
          <Network className="text-muted-foreground size-6" />
        </span>
        <p className="font-medium">{t('topology.emptyTitle')}</p>
        <p className="text-muted-foreground max-w-md text-sm">{t('topology.empty')}</p>
        <Link to="/einstellungen" className="text-primary flex items-center gap-1 text-sm font-medium hover:underline">
          <Plug className="size-4" /> {t('checks.toSettings')}
        </Link>
      </CardContent>
    </Card>
  )
}

/**
 * Netzplan: Geräte in Ebenen (Internet → Router/Firewall → Switches → Hosts),
 * Kabel mit Geschwindigkeit und VLAN, Ports mit VLAN-Balken, Gäste und Dienste
 * je Host – live aus den Integrationen, ergänzt um Gepflegtes.
 */
export function Topology() {
  const { t } = useTranslation()
  const topo = useTopology()
  const [editing, setEditing] = useState(false)
  const [redact, setRedact] = useState(false)
  const [busy, setBusy] = useState(false)
  const exportRef = useRef<HTMLDivElement>(null)

  async function onExport(kind: 'png' | 'svg') {
    if (!exportRef.current) return
    setBusy(true)
    try {
      await exportImage(exportRef.current, kind)
    } finally {
      setBusy(false)
    }
  }

  const data = topo.data
  return (
    <RedactContext.Provider value={redact}>
      <div className="flex flex-col gap-6">
        <PageHeader
          title={t('nav.network')}
          subtitle={t('topology.subtitle')}
          actions={
            data && (
              <div className="flex flex-wrap items-center gap-2">
                <label className="text-muted-foreground flex items-center gap-2 text-sm" title={t('topology.redactHint')}>
                  <Switch checked={redact} onCheckedChange={setRedact} aria-label={t('topology.redact')} />
                  <EyeOff className="size-4" aria-hidden />
                  {t('topology.redact')}
                </label>
                <Button variant="outline" size="sm" disabled={busy} onClick={() => onExport('png')} data-action="export-png">
                  <Download className="size-4" />
                  PNG
                </Button>
                <Button variant="outline" size="sm" disabled={busy} onClick={() => onExport('svg')} data-action="export-svg">
                  <Download className="size-4" />
                  SVG
                </Button>
                <Button variant="outline" size="sm" onClick={() => setEditing(true)} data-action="edit-topology">
                  <Pencil className="size-4" />
                  {t('topology.edit.open')}
                  {data.suggestions.length > 0 && (
                    <span className="bg-warning/20 text-warning rounded-full px-1.5 text-xs tabular-nums" title={t('topology.edit.suggestions')}>
                      {data.suggestions.length}
                    </span>
                  )}
                </Button>
              </div>
            )
          }
        />
        {topo.isPending ? (
          <div className="flex flex-col gap-4">
            <Skeleton className="h-40 rounded-xl" />
            <Skeleton className="h-72 rounded-xl" />
          </div>
        ) : !data || data.devices.length === 0 ? (
          <>
            <Empty />
            {data && (
              <Button variant="outline" className="w-fit" onClick={() => setEditing(true)}>
                <Pencil className="size-4" />
                {t('topology.edit.open')}
              </Button>
            )}
          </>
        ) : (
          <>
            <div ref={exportRef} className="bg-background grid gap-6 xl:grid-cols-[minmax(0,1fr)_18rem]" data-topology>
              <div className="min-w-0 overflow-x-auto pb-2">
                <Plan topo={data} />
              </div>
              <aside className="flex flex-col gap-4">
                <NetworksPanel vlans={data.vlans} />
                <ExposedPanel topo={data} />
                {data.services.length > 0 && (
                  <Panel title={t('topology.unplacedServices')}>
                    <p className="text-muted-foreground text-xs">{t('topology.unplacedHint')}</p>
                    <div className="flex flex-wrap gap-1">
                      {data.services.map((s) => (
                        <ServiceChip key={s.integration_id} s={s} />
                      ))}
                    </div>
                  </Panel>
                )}
              </aside>
            </div>
            <LinkTable topo={data} />
          </>
        )}
        {data && editing && <TopologyEditor topo={data} open={editing} onOpenChange={setEditing} />}
      </div>
    </RedactContext.Provider>
  )
}
