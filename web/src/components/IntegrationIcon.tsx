import {
  BatteryCharging,
  Braces,
  DatabaseBackup,
  Flame,
  Gauge,
  HeartPulse,
  Image,
  Plug,
  Server,
  type LucideIcon,
} from 'lucide-react'

// Icons der Integrationstypen (Schema().Icon); unbekannte → Stecker.
const icons: Record<string, LucideIcon> = {
  'heart-pulse': HeartPulse,
  image: Image,
  'database-backup': DatabaseBackup,
  braces: Braces,
  flame: Flame,
  server: Server,
  gauge: Gauge,
  'battery-charging': BatteryCharging,
}

export function IntegrationIcon({ name, className }: { name?: string; className?: string }) {
  const Icon = (name && icons[name]) || Plug
  return <Icon className={className} />
}
