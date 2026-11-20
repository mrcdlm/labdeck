import { Eye, EyeOff, Plus, Trash2 } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Segmented } from '@/components/Segmented'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  topoDeviceTypes,
  type TopoConfig,
  type TopoDevice,
  type TopoDeviceType,
  type TopoManualLink,
  type Topology,
} from '@/lib/api'
import { useSaveTopology } from '@/lib/queries'
import { newTopoId, normName, VLAN_COLORS, VLAN_HEX } from '@/lib/topology'
import { cn } from '@/lib/utils'

type Tab = 'devices' | 'cables' | 'vlans' | 'assign'

const NONE = '__none__'
const speeds = [100, 1000, 2500, 5000, 10000, 25000]

function Row({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('grid items-end gap-2 rounded-lg border p-3', className)}>{children}</div>
}

function Field({ label, children, className }: { label: string; children: ReactNode; className?: string }) {
  return (
    <div className={cn('grid min-w-0 gap-1', className)}>
      <Label className="text-muted-foreground text-xs">{label}</Label>
      {children}
    </div>
  )
}

function DeviceSelect({
  devices,
  value,
  onChange,
  label,
  allowNone,
}: {
  devices: { id: string; name: string }[]
  value?: string
  onChange: (v: string) => void
  label: string
  allowNone?: boolean
}) {
  const { t } = useTranslation()
  return (
    <Select value={value || NONE} onValueChange={(v) => onChange(v === NONE ? '' : v)}>
      <SelectTrigger aria-label={label} className="w-full">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {allowNone !== false && <SelectItem value={NONE}>{t('topology.edit.none')}</SelectItem>}
        {devices.map((d) => (
          <SelectItem key={d.id} value={d.id}>
            {d.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

/**
 * Pflege des Netzplans: eigene Geräte, Kabel, VLANs und Zuordnungen. Arbeitet
 * auf einem Entwurf; erst „Speichern“ schreibt ihn.
 */
export function TopologyEditor({ topo, open, onOpenChange }: { topo: Topology; open: boolean; onOpenChange: (v: boolean) => void }) {
  const { t } = useTranslation()
  const save = useSaveTopology()
  const [tab, setTab] = useState<Tab>('devices')
  const [draft, setDraft] = useState<TopoConfig>(() => {
    const c = structuredClone(topo.config)
    return { devices: c.devices ?? [], links: c.links ?? [], vlans: c.vlans ?? [], aliases: c.aliases ?? {}, hidden: c.hidden ?? [] }
  })
  const set = (fn: (c: TopoConfig) => TopoConfig) => setDraft((c) => fn(structuredClone(c)))

  // Geräte für Auswahlfelder: alle sichtbaren plus eigene aus dem Entwurf
  const deviceList: { id: string; name: string; ports: string[] }[] = [
    ...topo.devices.filter((d) => !d.manual).map((d) => ({ id: d.id, name: d.name, ports: d.ports.map((p) => p.name) })),
    ...draft.devices.map((d) => ({
      id: `dev:${d.id}`,
      name: d.name || d.id,
      ports: Array.from({ length: d.ports ?? 0 }, (_, n) => String(n + 1)),
    })),
  ]
  const autoDevices = topo.devices.filter((d) => !d.manual)
  const hosts = topo.devices.filter((d) => ['hypervisor', 'docker', 'server', 'nas'].includes(d.type) || d.manual)
  const assignedServices = Object.entries(draft.aliases)
    .filter(([k]) => k.startsWith('int:'))
    .map(([k]) => Number(k.slice(4)))
  const services = [
    ...topo.services,
    ...topo.devices.flatMap((d) => d.services).filter((s) => assignedServices.includes(s.integration_id)),
  ]

  function onSave() {
    save.mutate(draft, { onSuccess: () => onOpenChange(false) })
  }

  function setAlias(key: string, target: string) {
    set((c) => {
      if (target) c.aliases[key] = target
      else delete c.aliases[key]
      return c
    })
  }

  function addLink(): TopoManualLink {
    return { id: newTopoId('kabel', draft.links.map((l) => l.id)), a: { device: '' }, b: { device: '' } }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl" data-topology-editor>
        <DialogHeader>
          <DialogTitle>{t('topology.edit.title')}</DialogTitle>
          <DialogDescription>{t('topology.edit.hint')}</DialogDescription>
        </DialogHeader>
        <Segmented
          label={t('topology.edit.title')}
          value={tab}
          onChange={setTab}
          options={(['devices', 'cables', 'vlans', 'assign'] as const).map((v) => ({
            value: v,
            label: t(`topology.edit.tab.${v}`) + (v === 'assign' && topo.suggestions.length ? ` (${topo.suggestions.length})` : ''),
          }))}
        />

        {tab === 'devices' && (
          <div className="flex flex-col gap-3">
            <p className="text-muted-foreground text-sm">{t('topology.edit.devicesHint')}</p>
            {draft.devices.map((d, n) => (
              <Row key={d.id} className="sm:grid-cols-[1.4fr_1fr_1fr_1fr_5rem_auto]" >
                <Field label={t('topology.edit.name')}>
                  <Input value={d.name} maxLength={80} onChange={(e) => set((c) => ((c.devices[n].name = e.target.value), c))} />
                </Field>
                <Field label={t('topology.edit.type')}>
                  <Select value={d.type} onValueChange={(v) => set((c) => ((c.devices[n].type = v as TopoDeviceType), c))}>
                    <SelectTrigger className="w-full" aria-label={t('topology.edit.type')}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {topoDeviceTypes.map((x) => (
                        <SelectItem key={x} value={x}>
                          {t(`topology.type.${x}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
                <Field label={t('topology.edit.model')}>
                  <Input value={d.model ?? ''} maxLength={80} onChange={(e) => set((c) => ((c.devices[n].model = e.target.value), c))} />
                </Field>
                <Field label={t('topology.edit.ip')}>
                  <Input value={d.ip ?? ''} maxLength={80} onChange={(e) => set((c) => ((c.devices[n].ip = e.target.value), c))} />
                </Field>
                <Field label={t('topology.edit.ports')}>
                  <Input
                    type="number"
                    min={0}
                    max={64}
                    value={d.ports ?? 0}
                    onChange={(e) => set((c) => ((c.devices[n].ports = Math.min(64, Math.max(0, Number(e.target.value) || 0))), c))}
                  />
                </Field>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t('topology.edit.remove', { name: d.name })}
                  onClick={() =>
                    set((c) => {
                      c.devices.splice(n, 1)
                      c.links = c.links.filter((l) => l.a.device !== `dev:${d.id}` && l.b.device !== `dev:${d.id}`)
                      for (const [k, v] of Object.entries(c.aliases)) if (v === `dev:${d.id}`) delete c.aliases[k]
                      return c
                    })
                  }
                >
                  <Trash2 className="size-4" />
                </Button>
              </Row>
            ))}
            <Button
              variant="outline"
              className="w-fit"
              onClick={() =>
                set((c) => {
                  const name = t('topology.edit.newDevice', { n: c.devices.length + 1 })
                  c.devices.push({ id: newTopoId(name, c.devices.map((x) => x.id)), name, type: 'router' })
                  return c
                })
              }
              data-action="add-device"
            >
              <Plus className="size-4" />
              {t('topology.edit.addDevice')}
            </Button>
            {(autoDevices.length > 0 || draft.hidden.length > 0) && (
              <div className="flex flex-col gap-2 border-t pt-3">
                <p className="text-sm font-medium">{t('topology.edit.visibility')}</p>
                <div className="flex flex-wrap gap-2">
                  {[...autoDevices.map((d) => ({ id: d.id, name: d.name })), ...draft.hidden.filter((h) => !autoDevices.some((d) => d.id === h)).map((h) => ({ id: h, name: h }))].map((d) => {
                    const hidden = draft.hidden.includes(d.id)
                    return (
                      <button
                        key={d.id}
                        type="button"
                        aria-pressed={!hidden}
                        onClick={() => set((c) => ((c.hidden = hidden ? c.hidden.filter((x) => x !== d.id) : [...c.hidden, d.id]), c))}
                        className={cn('flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm', hidden && 'text-muted-foreground line-through')}
                      >
                        {hidden ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
                        {d.name}
                      </button>
                    )
                  })}
                </div>
              </div>
            )}
          </div>
        )}

        {tab === 'cables' && (
          <div className="flex flex-col gap-3">
            <p className="text-muted-foreground text-sm">{t('topology.edit.cablesHint')}</p>
            {draft.links.map((l, n) => {
              const end = (side: 'a' | 'b') => {
                const dev = deviceList.find((d) => d.id === l[side].device)
                return (
                  <div className="grid grid-cols-[1fr_7rem] gap-2">
                    <Field label={t(side === 'a' ? 'topology.col.from' : 'topology.col.to')}>
                      <DeviceSelect
                        devices={deviceList}
                        value={l[side].device}
                        label={t(side === 'a' ? 'topology.col.from' : 'topology.col.to')}
                        onChange={(v) => set((c) => ((c.links[n][side] = { device: v }), c))}
                      />
                    </Field>
                    <Field label={t('topology.edit.port')}>
                      {dev && dev.ports.length > 0 ? (
                        <Select value={l[side].port || NONE} onValueChange={(v) => set((c) => ((c.links[n][side].port = v === NONE ? '' : v), c))}>
                          <SelectTrigger className="w-full" aria-label={t('topology.edit.port')}>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value={NONE}>—</SelectItem>
                            {dev.ports.map((p) => (
                              <SelectItem key={p} value={p}>
                                {p}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      ) : (
                        <Input
                          value={l[side].port ?? ''}
                          maxLength={80}
                          placeholder="eth0"
                          aria-label={t('topology.edit.port')}
                          onChange={(e) => set((c) => ((c.links[n][side].port = e.target.value), c))}
                        />
                      )}
                    </Field>
                  </div>
                )
              }
              return (
                <Row key={l.id} className="gap-3">
                  <div className="grid gap-3 sm:grid-cols-2">
                    {end('a')}
                    {end('b')}
                  </div>
                  <div className="grid grid-cols-[7rem_8rem_1fr_auto] items-end gap-2">
                    <Field label={t('topology.col.speed')}>
                      <Select value={String(l.speed ?? 0)} onValueChange={(v) => set((c) => ((c.links[n].speed = Number(v) || undefined), c))}>
                        <SelectTrigger className="w-full" aria-label={t('topology.col.speed')}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="0">{t('topology.edit.auto')}</SelectItem>
                          {speeds.map((s) => (
                            <SelectItem key={s} value={String(s)}>
                              {s >= 1000 ? `${s / 1000} Gbit/s` : `${s} Mbit/s`}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </Field>
                    <Field label={t('topology.edit.untagged')}>
                      <Select value={String(l.vlan ?? 0)} onValueChange={(v) => set((c) => ((c.links[n].vlan = Number(v) || undefined), c))}>
                        <SelectTrigger className="w-full" aria-label={t('topology.edit.untagged')}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="0">{t('topology.edit.auto')}</SelectItem>
                          {topo.vlans.concat(draft.vlans.filter((v) => !topo.vlans.some((x) => x.id === v.id))).map((v) => (
                            <SelectItem key={v.id} value={String(v.id)}>
                              {v.id} {v.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </Field>
                    <Field label={t('topology.edit.taggedList')}>
                      <Input
                        defaultValue={(l.tagged ?? []).join(', ')}
                        placeholder="10, 20"
                        onBlur={(e) =>
                          set((c) => {
                            const ids = e.target.value
                              .split(/[\s,;]+/)
                              .map(Number)
                              .filter((x) => Number.isInteger(x) && x >= 1 && x <= 4094)
                            c.links[n].tagged = ids.length ? [...new Set(ids)] : undefined
                            return c
                          })
                        }
                      />
                    </Field>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label={t('topology.edit.removeCable')}
                      onClick={() => set((c) => (c.links.splice(n, 1), c))}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                </Row>
              )
            })}
            <Button variant="outline" className="w-fit" onClick={() => set((c) => (c.links.push(addLink()), c))} data-action="add-cable">
              <Plus className="size-4" />
              {t('topology.edit.addCable')}
            </Button>
          </div>
        )}

        {tab === 'vlans' && (
          <div className="flex flex-col gap-3">
            <p className="text-muted-foreground text-sm">{t('topology.edit.vlansHint')}</p>
            {[...topo.vlans.filter((v) => !draft.vlans.some((x) => x.id === v.id)).map((v) => ({ ...v, auto: true })), ...draft.vlans.map((v) => ({ ...v, auto: false }))]
              .sort((a, b) => a.id - b.id)
              .map((v) => {
                const n = draft.vlans.findIndex((x) => x.id === v.id)
                const ensure = (c: TopoConfig) => {
                  let i = c.vlans.findIndex((x) => x.id === v.id)
                  if (i < 0) i = c.vlans.push({ id: v.id, name: '' }) - 1
                  return i
                }
                return (
                  <Row key={v.id} className="sm:grid-cols-[5rem_1fr_1fr_9rem_auto]">
                    <Field label="VLAN">
                      <Input value={v.id} readOnly className="font-mono" />
                    </Field>
                    <Field label={t('topology.edit.name')}>
                      <Input
                        value={n >= 0 ? draft.vlans[n].name : ''}
                        placeholder={v.auto ? v.name : ''}
                        maxLength={80}
                        onChange={(e) => set((c) => ((c.vlans[ensure(c)].name = e.target.value), c))}
                      />
                    </Field>
                    <Field label={t('topology.edit.subnet')}>
                      <Input
                        value={n >= 0 ? (draft.vlans[n].subnet ?? '') : ''}
                        placeholder="10.0.10.0/24"
                        maxLength={80}
                        onChange={(e) => set((c) => ((c.vlans[ensure(c)].subnet = e.target.value), c))}
                      />
                    </Field>
                    <Field label={t('topology.edit.color')}>
                      <Select
                        value={(n >= 0 && draft.vlans[n].color) || NONE}
                        onValueChange={(val) => set((c) => ((c.vlans[ensure(c)].color = val === NONE ? undefined : val), c))}
                      >
                        <SelectTrigger className="w-full" aria-label={t('topology.edit.color')}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value={NONE}>{t('topology.edit.auto')}</SelectItem>
                          {paletteHex.map((hex, i) => (
                            <SelectItem key={hex} value={hex}>
                              <span className="flex items-center gap-2">
                                <span className="size-3 rounded-sm" style={{ background: `var(--vlan-${i + 1})` }} />
                                {t('topology.edit.colorN', { n: i + 1 })}
                              </span>
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </Field>
                    <Button
                      variant="ghost"
                      size="icon"
                      disabled={n < 0}
                      aria-label={t('topology.edit.resetVlan', { id: v.id })}
                      onClick={() => set((c) => ((c.vlans = c.vlans.filter((x) => x.id !== v.id)), c))}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </Row>
                )
              })}
            <AddVlan
              taken={[...topo.vlans, ...draft.vlans].map((v) => v.id)}
              onAdd={(id) => set((c) => (c.vlans.push({ id, name: '' }), c))}
            />
          </div>
        )}

        {tab === 'assign' && (
          <div className="flex flex-col gap-4">
            <section className="flex flex-col gap-2">
              <h3 className="text-sm font-medium">{t('topology.edit.suggestions')}</h3>
              {topo.suggestions.length === 0 && Object.keys(draft.aliases).filter((k) => k.startsWith('lldp:')).length === 0 ? (
                <p className="text-muted-foreground text-sm">{t('topology.edit.noSuggestions')}</p>
              ) : (
                [...topo.suggestions.map((s) => ({ key: `lldp:${normName(s.name)}`, label: s.name, at: `${deviceList.find((d) => d.id === s.switch)?.name ?? s.switch} · ${s.port}` })),
                  ...Object.keys(draft.aliases)
                    .filter((k) => k.startsWith('lldp:') && !topo.suggestions.some((s) => `lldp:${normName(s.name)}` === k))
                    .map((k) => ({ key: k, label: k.slice(5), at: '' }))].map((s) => (
                  <Row key={s.key} className="sm:grid-cols-[1fr_1fr_auto]" >
                    <div className="text-sm">
                      <span className="font-mono">{s.label}</span>
                      {s.at && <span className="text-muted-foreground block text-xs">{t('topology.edit.seenAt', { at: s.at })}</span>}
                    </div>
                    <DeviceSelect devices={deviceList} value={draft.aliases[s.key]} label={t('topology.edit.assignTo')} onChange={(v) => setAlias(s.key, v)} />
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() =>
                        set((c) => {
                          const id = newTopoId(s.label, c.devices.map((x) => x.id))
                          c.devices.push({ id, name: s.label, type: 'other' })
                          c.aliases[s.key] = `dev:${id}`
                          return c
                        })
                      }
                    >
                      <Plus className="size-4" />
                      {t('topology.edit.createDevice')}
                    </Button>
                  </Row>
                ))
              )}
            </section>
            {services.length > 0 && (
              <section className="flex flex-col gap-2">
                <h3 className="text-sm font-medium">{t('topology.edit.servicesOn')}</h3>
                {services.map((s) => (
                  <Row key={s.integration_id} className="sm:grid-cols-[1fr_1fr]">
                    <span className="text-sm">{s.name}</span>
                    <DeviceSelect
                      devices={hosts}
                      value={draft.aliases[`int:${s.integration_id}`]}
                      label={t('topology.edit.runsOn', { name: s.name })}
                      onChange={(v) => setAlias(`int:${s.integration_id}`, v)}
                    />
                  </Row>
                ))}
              </section>
            )}
            {autoDevices.length > 1 && (
              <section className="flex flex-col gap-2">
                <h3 className="text-sm font-medium">{t('topology.edit.merge')}</h3>
                <p className="text-muted-foreground text-xs">{t('topology.edit.mergeHint')}</p>
                {autoDevices.map((d: TopoDevice) => (
                  <Row key={d.id} className="sm:grid-cols-[1fr_1fr]">
                    <span className="text-sm">{d.name}</span>
                    <DeviceSelect
                      devices={deviceList.filter((x) => x.id !== d.id)}
                      value={draft.aliases[d.id]}
                      label={t('topology.edit.sameAs', { name: d.name })}
                      onChange={(v) => setAlias(d.id, v)}
                    />
                  </Row>
                ))}
              </section>
            )}
          </div>
        )}

        {save.error && <p className="text-destructive text-sm">{save.error.message}</p>}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button onClick={onSave} disabled={save.isPending} data-action="save-topology">
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// Feste Farben für die Pflege: die validierten VLAN-Farben (Anzeige als Token)
const paletteHex = VLAN_HEX.slice(0, VLAN_COLORS)

function AddVlan({ taken, onAdd }: { taken: number[]; onAdd: (id: number) => void }) {
  const { t } = useTranslation()
  const [id, setId] = useState('')
  const n = Number(id)
  const valid = Number.isInteger(n) && n >= 1 && n <= 4094 && !taken.includes(n)
  return (
    <div className="flex items-end gap-2">
      <Field label={t('topology.edit.vlanId')} className="w-28">
        <Input inputMode="numeric" value={id} onChange={(e) => setId(e.target.value.replace(/\D/g, ''))} placeholder="30" />
      </Field>
      <Button
        variant="outline"
        disabled={!valid}
        onClick={() => {
          onAdd(n)
          setId('')
        }}
        data-action="add-vlan"
      >
        <Plus className="size-4" />
        {t('topology.edit.addVlan')}
      </Button>
    </div>
  )
}
