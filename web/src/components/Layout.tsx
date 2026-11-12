import {
  DatabaseBackup,
  HeartPulse,
  History,
  LayoutDashboard,
  LogOut,
  Server,
  Settings,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { NavLink, Outlet, useLocation } from 'react-router'

import { LiveProvider } from '@/components/LiveProvider'
import { Logo } from '@/components/Logo'
import { Preferences } from '@/components/Preferences'
import { Button } from '@/components/ui/button'
import { useLive } from '@/lib/live-context'
import { setDisplayTimeZone } from '@/lib/format'
import { useLogout, useOverview, useSettings } from '@/lib/queries'
import { cn } from '@/lib/utils'

type NavKey = 'dashboard' | 'backups' | 'checks' | 'systems' | 'history' | 'settings'

const nav: { to: string; key: NavKey; icon: LucideIcon }[] = [
  { to: '/', key: 'dashboard', icon: LayoutDashboard },
  { to: '/backups', key: 'backups', icon: DatabaseBackup },
  { to: '/checks', key: 'checks', icon: HeartPulse },
  { to: '/systeme', key: 'systems', icon: Server },
  { to: '/verlauf', key: 'history', icon: History },
  { to: '/einstellungen', key: 'settings', icon: Settings },
]

function NavEntry({ item, compact }: { item: (typeof nav)[number]; compact?: boolean }) {
  const { t } = useTranslation()
  const Icon = item.icon
  // weitere Dashboards (/d/…) gehören zum Dashboard
  const onDashboard = useLocation().pathname.startsWith('/d/') && item.to === '/'
  // Fehler als Zähler an der Übersicht – auch von anderen Seiten aus sichtbar
  const overview = useOverview()
  const errors =
    item.key === 'dashboard' ? (overview.data?.problems ?? []).filter((p) => p.status === 'error').length : 0
  return (
    <NavLink
      to={item.to}
      end={item.to === '/'}
      className={({ isActive }) =>
        cn(
          'flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors',
          compact && 'shrink-0 px-2.5 py-1.5',
          isActive || onDashboard
            ? 'bg-accent text-accent-foreground'
            : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground',
        )
      }
    >
      <Icon className="size-4" />
      {t(`nav.${item.key}`)}
      {errors > 0 && (
        <span
          className="bg-destructive/10 text-destructive ml-auto rounded-full px-1.5 text-xs leading-5 font-medium tabular-nums"
          title={t('dashboard.errorsCount', { count: errors })}
          data-nav-errors={errors}
        >
          {errors}
        </span>
      )}
    </NavLink>
  )
}

function LiveIndicator({ compact }: { compact?: boolean }) {
  const { t } = useTranslation()
  const live = useLive()
  return (
    <span
      className="text-muted-foreground flex items-center gap-2 text-xs"
      title={live ? t('live.onHint') : t('live.offHint')}
    >
      <span className="relative flex size-2" aria-hidden>
        {live && <span className="bg-success/60 animate-ring absolute inset-0 rounded-full [animation-duration:2.4s]" />}
        <span className={cn('relative size-2 rounded-full', live ? 'bg-success' : 'bg-muted-foreground/50')} />
      </span>
      {!compact && (live ? t('live.on') : t('live.off'))}
    </span>
  )
}

export function Layout() {
  return (
    <LiveProvider>
      <Shell />
    </LiveProvider>
  )
}

function Shell() {
  const { t } = useTranslation()
  const logout = useLogout()
  // Zeitzone der Anzeige (Einstellungen → Allgemein) für alle Zeitangaben
  setDisplayTimeZone(useSettings().data?.timezone ?? '')

  return (
    <div className="flex min-h-svh flex-col md:flex-row">
      {/* Desktop: Seitenleiste */}
      <aside className="bg-card/40 sticky top-0 hidden h-svh w-60 shrink-0 flex-col overflow-y-auto border-r p-4 md:flex">
        <Logo className="px-2 py-1" />
        <nav className="mt-6 flex flex-col gap-1">
          {nav.map((item) => (
            <NavEntry key={item.to} item={item} />
          ))}
        </nav>
        <div className="mt-auto flex items-center justify-between gap-2 px-3 py-2">
          <LiveIndicator />
          <Preferences />
        </div>
        <Button
          variant="ghost"
          className="text-muted-foreground justify-start"
          onClick={() => logout.mutate()}
          disabled={logout.isPending}
        >
          <LogOut />
          {t('auth.logout')}
        </Button>
      </aside>

      {/* Mobil: Kopfzeile mit horizontal scrollbarer Navigation */}
      <header className="bg-background/80 sticky top-0 z-10 border-b backdrop-blur md:hidden">
        <div className="flex items-center justify-between gap-2 px-4 py-3">
          <div className="flex items-center gap-3">
            <Logo />
            <LiveIndicator compact />
          </div>
          <div className="flex items-center gap-1">
            <Preferences />
            <Button
              variant="ghost"
              size="icon"
              aria-label={t('auth.logout')}
              onClick={() => logout.mutate()}
              disabled={logout.isPending}
            >
              <LogOut />
            </Button>
          </div>
        </div>
        <nav className="flex gap-1 overflow-x-auto px-3 pb-2">
          {nav.map((item) => (
            <NavEntry key={item.to} item={item} compact />
          ))}
        </nav>
      </header>

      <main className="min-w-0 flex-1 px-4 py-6 md:px-8 md:py-8">
        <div className="mx-auto max-w-5xl">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
