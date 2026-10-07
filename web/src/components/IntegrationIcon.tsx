import {
  BatteryCharging,
  Bell,
  Braces,
  Container,
  DatabaseBackup,
  Flame,
  Gauge,
  HeartPulse,
  Image,
  Plug,
  Server,
  type LucideIcon,
} from 'lucide-react'

import discord from '@/assets/brands/discord.svg'
import healthchecks from '@/assets/brands/healthchecks.svg'
import homeassistant from '@/assets/brands/home-assistant.svg'
import immich from '@/assets/brands/immich.svg'
import ntfy from '@/assets/brands/ntfy.svg'
import nut from '@/assets/brands/nut.svg'
import prometheus from '@/assets/brands/prometheus.svg'
import proxmox from '@/assets/brands/proxmox.svg'
import slack from '@/assets/brands/slack.svg'
import telegram from '@/assets/brands/telegram.svg'
import { cn } from '@/lib/utils'

// Symbole der Integrationstypen (Schema().Icon); unbekannte → Stecker.
const icons: Record<string, LucideIcon> = {
  'heart-pulse': HeartPulse,
  image: Image,
  'database-backup': DatabaseBackup,
  braces: Braces,
  flame: Flame,
  server: Server,
  gauge: Gauge,
  'battery-charging': BatteryCharging,
  container: Container,
  bell: Bell,
}

// Echte Logos (homarr-labs/dashboard-icons, siehe assets/brands/LICENSE.md).
// dark: Logo mit schwarzen Flächen – im Dark Mode auf hellem Grund.
const logos: Record<string, { src: string; dark?: boolean }> = {
  healthchecks: { src: healthchecks },
  immich: { src: immich },
  pve: { src: proxmox, dark: true },
  pbs: { src: proxmox, dark: true },
  prometheus: { src: prometheus },
  nut: { src: nut, dark: true },
  // Webhook-Vorlagen
  ntfy: { src: ntfy },
  discord: { src: discord },
  slack: { src: slack },
  telegram: { src: telegram },
  homeassistant: { src: homeassistant },
}

/**
 * Icon einer Integration bzw. Webhook-Vorlage: das Logo des Dienstes, wenn
 * vorhanden (type), sonst das lucide-Symbol aus dem Schema (name).
 */
export function IntegrationIcon({ name, type, className }: { name?: string; type?: string; className?: string }) {
  const logo = type ? logos[type] : undefined
  if (logo) {
    return (
      <img
        src={logo.src}
        alt=""
        aria-hidden
        draggable={false}
        className={cn('object-contain', logo.dark && 'dark:rounded-[3px] dark:bg-white dark:p-px', className)}
      />
    )
  }
  const Icon = (name && icons[name]) || Plug
  return <Icon className={className} />
}
