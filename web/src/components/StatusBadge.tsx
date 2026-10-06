import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import type { Status } from '@/lib/api'

// grün = ok, gelb = Warnung, rot = Fehler, grau = unbekannt/pausiert
const variant: Record<Status, 'success' | 'warning' | 'destructive' | 'secondary'> = {
  ok: 'success',
  warning: 'warning',
  error: 'destructive',
  unknown: 'secondary',
  paused: 'secondary',
}

export function StatusBadge({ status, label }: { status: Status; label?: string }) {
  const { t } = useTranslation()
  return <Badge variant={variant[status]}>{label ?? t(`status.${status}`)}</Badge>
}
