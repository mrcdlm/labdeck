import { CloudOff, HeartPulse, Info, Plug } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { LiveItem, LiveTableRow } from '@/components/LiveRow'
import { PageHeader } from '@/components/PageHeader'
import { Segmented } from '@/components/Segmented'
import { StatusBadge } from '@/components/StatusBadge'
import { RangeToggle, UptimeBar } from '@/components/UptimeBar'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { CheckItem, UptimeRange } from '@/lib/api'
import { anchorId, useHashTarget } from '@/lib/anchor'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { useChecks } from '@/lib/queries'
import { cn } from '@/lib/utils'

type GroupBy = 'project' | 'tag'

const rawStates = ['up', 'grace', 'down', 'paused', 'new'] as const

/** Status-Badge mit dem Begriff der Quelle (up, grace, down …). */
function CheckStatus({ check }: { check: CheckItem }) {
  const { t } = useTranslation()
  const raw = rawStates.find((r) => r === check.raw)
  return <StatusBadge status={check.status} label={raw ? t(`checks.raw.${raw}`) : undefined} />
}

function Time({ iso, now }: { iso?: string; now: number }) {
  if (!iso) return <span className="text-muted-foreground">—</span>
  return (
    <span title={absoluteTime(iso)} className="whitespace-nowrap">
      {relativeTime(iso, now)}
    </span>
  )
}

function History({ check, range }: { check: CheckItem; range: UptimeRange }) {
  const uptime = check.uptime?.[range]
  if (!uptime) return <span className="text-muted-foreground text-xs">—</span>
  return <UptimeBar uptime={uptime} range={range} compact />
}

function NameCell({ check }: { check: CheckItem }) {
  return (
    <div className="min-w-0">
      <div className="font-medium [overflow-wrap:anywhere]">{check.name}</div>
      {check.desc && <div className="text-muted-foreground mt-0.5 line-clamp-2 text-xs">{check.desc}</div>}
      {check.tags.length > 0 && (
        <div className="mt-1 flex flex-wrap gap-1">
          {check.tags.map((tag) => (
            <Badge key={tag} variant="outline" className="px-1.5 py-0 text-[11px] font-normal">
              {tag}
            </Badge>
          ))}
        </div>
      )}
    </div>
  )
}

interface Group {
  key: string
  title: string
  checks: CheckItem[]
  stale: boolean
}

function groupChecks(checks: CheckItem[], by: GroupBy, noTag: string): Group[] {
  const map = new Map<string, Group>()
  const add = (key: string, title: string, c: CheckItem) => {
    let g = map.get(key)
    if (!g) {
      g = { key, title, checks: [], stale: false }
      map.set(key, g)
    }
    g.checks.push(c)
    g.stale ||= !!c.stale
  }
  for (const c of checks) {
    if (by === 'project') add(`i${c.integration_id}`, c.integration_name, c)
    else if (c.tags.length === 0) add('', noTag, c)
    else for (const tag of c.tags) add(`t${tag}`, tag, c)
  }
  // „Ohne Tag“ ans Ende, sonst alphabetisch
  return [...map.values()].sort((a, b) =>
    a.key === '' ? 1 : b.key === '' ? -1 : a.title.localeCompare(b.title, undefined, { sensitivity: 'base' }),
  )
}

function GroupCard({ group, range, now }: { group: Group; range: UptimeRange; now: number }) {
  const { t } = useTranslation()
  const problems = group.checks.filter((c) => c.status === 'error' || c.status === 'warning').length
  return (
    <Card className="gap-0 py-0">
      <div className="flex flex-wrap items-center gap-2 border-b px-4 py-3 md:px-6">
        <h2 className="font-semibold">{group.title}</h2>
        <span className="text-muted-foreground text-sm">{t('checks.count', { count: group.checks.length })}</span>
        {problems > 0 && <Badge variant="destructive">{t('checks.problems', { count: problems })}</Badge>}
        {group.stale && (
          <span className="text-warning ml-auto flex items-center gap-1 text-xs">
            <CloudOff className="size-3.5" aria-hidden />
            {t('checks.stale')}
          </span>
        )}
      </div>
      <div className="hidden md:block">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="pl-6">{t('checks.col.name')}</TableHead>
              <TableHead>{t('checks.col.status')}</TableHead>
              <TableHead>{t('checks.col.lastPing')}</TableHead>
              <TableHead className="hidden lg:table-cell">{t('checks.col.nextPing')}</TableHead>
              <TableHead className="pr-6">
                {t('checks.col.history', { range: t(`uptime.range.${range}`) })}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {group.checks.map((c, n) => (
              <LiveTableRow
                watch={c.status}
                index={n}
                key={`${c.integration_id}/${c.id}`}
                className={cn(c.status === 'paused' && 'opacity-60')}
                data-check={c.name}
                data-anchor={anchorId('check', c.integration_id, c.id)}
              >
                <TableCell className="max-w-[22rem] min-w-44 pl-6 whitespace-normal">
                  <NameCell check={c} />
                </TableCell>
                <TableCell>
                  <CheckStatus check={c} />
                </TableCell>
                <TableCell className="text-sm">
                  <Time iso={c.last_ping} now={now} />
                </TableCell>
                <TableCell className="hidden text-sm lg:table-cell">
                  <Time iso={c.next_ping} now={now} />
                </TableCell>
                <TableCell className="w-56 min-w-40 pr-6">
                  <History check={c} range={range} />
                </TableCell>
              </LiveTableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <ul className="divide-y md:hidden">
        {group.checks.map((c, n) => (
          <LiveItem
            watch={c.status}
            index={n}
            key={`${c.integration_id}/${c.id}`}
            className={cn('flex flex-col gap-2 p-4', c.status === 'paused' && 'opacity-60')}
            data-check={c.name}
            data-anchor={anchorId('check', c.integration_id, c.id)}
          >
            <div className="flex items-start justify-between gap-2">
              <NameCell check={c} />
              <CheckStatus check={c} />
            </div>
            <History check={c} range={range} />
            {c.last_ping && (
              <span className="text-muted-foreground text-xs" title={absoluteTime(c.last_ping)}>
                {t('checks.lastPingAgo', { time: relativeTime(c.last_ping, now) })}
              </span>
            )}
          </LiveItem>
        ))}
      </ul>
    </Card>
  )
}

function Empty() {
  const { t } = useTranslation()
  return (
    <Card>
      <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
        <span className="bg-muted grid size-12 place-items-center rounded-lg">
          <HeartPulse className="text-muted-foreground size-6" />
        </span>
        <p className="font-medium">{t('checks.emptyTitle')}</p>
        <p className="text-muted-foreground max-w-md text-sm">{t('checks.empty')}</p>
        <Link to="/einstellungen" className="text-primary flex items-center gap-1 text-sm font-medium hover:underline">
          <Plug className="size-4" /> {t('checks.toSettings')}
        </Link>
      </CardContent>
    </Card>
  )
}

export function Checks() {
  const { t } = useTranslation()
  const checks = useChecks()
  const now = useNow()
  const [range, setRange] = useState<UptimeRange>('24h')
  const [groupBy, setGroupBy] = useState<GroupBy>('project')
  const [selected, setSelected] = useState<string[]>([])
  useHashTarget(checks.isSuccess)

  const list = useMemo(() => checks.data?.checks ?? [], [checks.data])
  const tags = useMemo(() => [...new Set(list.flatMap((c) => c.tags))].sort(), [list])
  // Ausgewählte Tags, die es nicht mehr gibt, zählen nicht
  const active = selected.filter((s) => tags.includes(s))
  const filtered = active.length === 0 ? list : list.filter((c) => c.tags.some((tag) => active.includes(tag)))
  const groups = groupChecks(filtered, groupBy, t('checks.noTag'))

  function toggle(tag: string) {
    setSelected((s) => (s.includes(tag) ? s.filter((x) => x !== tag) : [...s, tag]))
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={t('nav.checks')}
        subtitle={t('checks.subtitle')}
        actions={<RangeToggle value={range} onChange={setRange} />}
      />

      {checks.isPending ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-10 rounded-xl" />
          <Skeleton className="h-48 rounded-xl" />
        </div>
      ) : checks.isError ? (
        <p className="text-destructive text-sm">{t('checks.loadError', { error: checks.error.message })}</p>
      ) : list.length === 0 ? (
        <Empty />
      ) : (
        <>
          <div className="flex flex-wrap items-center gap-3">
            <Segmented
              label={t('checks.groupBy')}
              value={groupBy}
              onChange={setGroupBy}
              options={[
                { value: 'project', label: t('checks.byProject') },
                { value: 'tag', label: t('checks.byTag') },
              ]}
            />
            {tags.length > 0 && (
              <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label={t('checks.filterTags')}>
                {tags.map((tag) => (
                  <button
                    key={tag}
                    type="button"
                    onClick={() => toggle(tag)}
                    aria-pressed={active.includes(tag)}
                    className={cn(
                      'rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors',
                      active.includes(tag)
                        ? 'bg-primary text-primary-foreground border-primary'
                        : 'text-muted-foreground hover:text-foreground hover:bg-accent',
                    )}
                  >
                    {tag}
                  </button>
                ))}
                {active.length > 0 && (
                  <button
                    type="button"
                    onClick={() => setSelected([])}
                    className="text-muted-foreground hover:text-foreground px-1 text-xs underline-offset-2 hover:underline"
                  >
                    {t('checks.clearFilter')}
                  </button>
                )}
              </div>
            )}
          </div>

          {groups.map((g) => (
            <GroupCard key={g.key} group={g} range={range} now={now} />
          ))}

          <div className="text-muted-foreground flex gap-3 text-sm">
            <Info className="mt-0.5 size-4 shrink-0" aria-hidden />
            <p>{t('checks.howItWorks')}</p>
          </div>
        </>
      )}
    </div>
  )
}
