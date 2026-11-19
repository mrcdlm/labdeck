import { ExternalLink, Globe, Laptop } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IntegrationIcon } from '@/components/IntegrationIcon'
import { LiveItem, LiveTableRow } from '@/components/LiveRow'
import { Segmented } from '@/components/Segmented'
import { StatusBadge } from '@/components/StatusBadge'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { Integration, Resource } from '@/lib/api'
import { anchorId, useHash } from '@/lib/anchor'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { cn } from '@/lib/utils'

// Zustände von Peers und Reverse-Proxy-Diensten (NetBird) mit eigener Übersetzung
const states = [
  'online',
  'offline',
  'login_expired',
  'approval_required',
  'active',
  'pending',
  'certificate_pending',
  'certificate_failed',
  'tunnel_not_created',
  'error',
  'disabled',
] as const

function VpnState({ r }: { r: Resource }) {
  const { t } = useTranslation()
  const s = states.find((k) => k === r.state)
  return <StatusBadge status={r.status} label={s ? t(`vpn.state.${s}`) : r.state} />
}

const watched = (r: Resource) => r.info?.watched === '1'

// überwachte Probleme zuerst, dann überwachte, dann online, dann Name
const rank = (r: Resource) => (watched(r) ? (r.status === 'ok' ? 1 : 0) : r.state === 'online' ? 2 : 3)

function LastSeen({ r, now }: { r: Resource; now: number }) {
  const { t } = useTranslation()
  if (r.state === 'online') return <span className="text-muted-foreground">{t('vpn.now')}</span>
  if (!r.info?.last_seen) return <span className="text-muted-foreground">—</span>
  return <span title={absoluteTime(r.info.last_seen)}>{relativeTime(r.info.last_seen, now)}</span>
}

function PeerTable({ peers }: { peers: Resource[] }) {
  const { t } = useTranslation()
  const now = useNow()
  return (
    <>
      <div className="hidden md:block">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="pl-6">{t('resources.col.name')}</TableHead>
              <TableHead>{t('resources.col.state')}</TableHead>
              <TableHead>{t('vpn.col.address')}</TableHead>
              <TableHead className="hidden lg:table-cell">{t('vpn.col.os')}</TableHead>
              <TableHead className="hidden xl:table-cell">{t('vpn.col.version')}</TableHead>
              <TableHead className="pr-6">{t('vpn.col.lastSeen')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {peers.map((r, n) => (
              <LiveTableRow
                key={r.id}
                watch={r.state}
                index={n}
                className={cn(r.state !== 'online' && r.status !== 'error' && 'opacity-60')}
                data-peer={r.name}
                data-anchor={anchorId('res', r.id)}
              >
                <TableCell className="max-w-[18rem] min-w-40 pl-6 whitespace-normal">
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span className="font-medium [overflow-wrap:anywhere]">{r.name}</span>
                    {watched(r) && (
                      <Badge variant="outline" className="px-1.5 py-0 text-[11px] font-normal">
                        {t('vpn.watched')}
                      </Badge>
                    )}
                  </div>
                  {r.tags && r.tags.length > 0 && (
                    <div className="text-muted-foreground mt-0.5 truncate text-xs">{r.tags.join(' · ')}</div>
                  )}
                </TableCell>
                <TableCell>
                  <VpnState r={r} />
                </TableCell>
                <TableCell className="text-sm">
                  <div className="tabular-nums">{r.info?.ip ?? '—'}</div>
                  {r.info?.dns && <div className="text-muted-foreground truncate text-xs">{r.info.dns}</div>}
                </TableCell>
                <TableCell className="hidden max-w-[16rem] truncate text-sm lg:table-cell">{r.info?.os ?? '—'}</TableCell>
                <TableCell className="hidden text-sm tabular-nums xl:table-cell">{r.info?.version ?? '—'}</TableCell>
                <TableCell className="pr-6 text-sm whitespace-nowrap">
                  <LastSeen r={r} now={now} />
                </TableCell>
              </LiveTableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <ul className="divide-y md:hidden">
        {peers.map((r, n) => (
          <LiveItem
            watch={r.state}
            index={n}
            key={r.id}
            className={cn('flex flex-col gap-1 p-4', r.state !== 'online' && r.status !== 'error' && 'opacity-60')}
            data-peer={r.name}
            data-anchor={anchorId('res', r.id)}
          >
            <div className="flex items-start justify-between gap-2">
              <span className="flex flex-wrap items-center gap-1.5 font-medium">
                {r.name}
                {watched(r) && (
                  <Badge variant="outline" className="px-1.5 py-0 text-[11px] font-normal">
                    {t('vpn.watched')}
                  </Badge>
                )}
              </span>
              <VpnState r={r} />
            </div>
            <div className="text-muted-foreground flex flex-wrap gap-x-3 text-xs">
              <span className="tabular-nums">{r.info?.ip}</span>
              <LastSeen r={r} now={now} />
            </div>
          </LiveItem>
        ))}
      </ul>
    </>
  )
}

function ProxyTable({ services }: { services: Resource[] }) {
  const { t } = useTranslation()
  const now = useNow()
  return (
    <ul className="divide-y">
      {services.map((r, n) => (
        <LiveItem
          watch={r.state}
          index={n}
          key={r.id}
          className={cn('flex flex-wrap items-center gap-x-4 gap-y-1 px-6 py-3', r.status === 'paused' && 'opacity-60')}
          data-proxy={r.name}
          data-anchor={anchorId('res', r.id)}
        >
          <a
            href={`https://${r.name}`}
            target="_blank"
            rel="noreferrer"
            className="flex min-w-0 items-center gap-1.5 font-medium hover:underline [overflow-wrap:anywhere]"
          >
            {r.name}
            <ExternalLink className="text-muted-foreground size-3.5 shrink-0" aria-hidden />
          </a>
          <VpnState r={r} />
          <span className="text-muted-foreground min-w-0 text-sm [overflow-wrap:anywhere]">
            {r.info?.targets ? `→ ${r.info.targets}` : ''}
          </span>
          {r.info?.cert_issued && (
            <span className="text-muted-foreground ml-auto text-xs whitespace-nowrap" title={absoluteTime(r.info.cert_issued)}>
              {t('vpn.certIssued', { time: relativeTime(r.info.cert_issued, now) })}
            </span>
          )}
        </LiveItem>
      ))}
    </ul>
  )
}

type PeerFilter = 'watched' | 'all'

/** Eine NetBird-Instanz: Peers (überwachte zuerst) und Reverse-Proxy-Dienste. */
export function VpnCard({ integration: i }: { integration: Integration }) {
  const { t } = useTranslation()
  const resources = i.snapshot?.resources ?? []
  const all = resources.filter((r) => r.kind === 'peer')
  const services = resources.filter((r) => r.kind === 'proxy')
  const hasWatched = all.some(watched)
  // Sprung (Suche, Problemliste) zu einem nicht überwachten Peer: gleich alle zeigen
  const hash = useHash()
  const target = all.find((r) => anchorId('res', r.id) === hash)
  const [filter, setFilter] = useState<PeerFilter>(hasWatched && (!target || watched(target)) ? 'watched' : 'all')
  const peers = (filter === 'watched' && hasWatched ? all.filter(watched) : all).sort(
    (a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name, undefined, { numeric: true }),
  )
  const online = all.filter((r) => r.state === 'online').length
  const problems = all.filter((r) => watched(r) && r.status !== 'ok').length

  return (
    <Card className="gap-0 py-0" data-vpn={i.name}>
      <CardContent className="flex flex-wrap items-center gap-3 px-6 py-5">
        <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg">
          <IntegrationIcon type={i.type} name="network" className="size-5" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="text-lg font-semibold">{i.name}</h3>
          <p className="text-muted-foreground text-sm">
            {t('vpn.online', { count: online, total: all.length })}
            {problems > 0 && (
              <span className="text-destructive"> · {t('vpn.watchedProblems', { count: problems })}</span>
            )}
          </p>
        </div>
        {hasWatched && (
          <Segmented
            label={t('vpn.filter')}
            value={filter}
            onChange={setFilter}
            options={(['watched', 'all'] as const).map((v) => ({ value: v, label: t(`vpn.filters.${v}`) }))}
          />
        )}
      </CardContent>
      {peers.length > 0 && (
        <div className="border-t">
          <h4 className="text-muted-foreground flex items-center gap-2 px-6 pt-3 text-xs font-medium tracking-wide uppercase">
            <Laptop className="size-3.5" aria-hidden />
            {t('vpn.peers')}
          </h4>
          <PeerTable peers={peers} />
        </div>
      )}
      {services.length > 0 && (
        <div className="border-t">
          <h4 className="text-muted-foreground flex items-center gap-2 px-6 pt-3 text-xs font-medium tracking-wide uppercase">
            <Globe className="size-3.5" aria-hidden />
            {t('vpn.proxies')}
          </h4>
          <ProxyTable services={services} />
        </div>
      )}
    </Card>
  )
}
