import type { ComponentProps } from 'react'

import { TableRow } from '@/components/ui/table'
import { rise, useFlash } from '@/lib/motion'
import { cn } from '@/lib/utils'

type Live = {
  /** Zustand, dessen Änderung (Live-Update) die Zeile kurz aufleuchten lässt */
  watch: unknown
  /** Position für das gestaffelte Einblenden */
  index: number
}

/** Tabellenzeile, die gestaffelt einblendet und bei Änderungen aufleuchtet. */
export function LiveTableRow({ watch, index, className, style, ...props }: ComponentProps<typeof TableRow> & Live) {
  const flash = useFlash(watch)
  const motion = rise(index)
  return (
    <TableRow
      {...props}
      className={cn(motion.className, flash && 'animate-flash', className)}
      style={{ ...motion.style, ...style }}
    />
  )
}

/** Listeneintrag (mobile Karten, Verlauf) wie LiveTableRow. */
export function LiveItem({ watch, index, className, style, ...props }: ComponentProps<'li'> & Live) {
  const flash = useFlash(watch)
  const motion = rise(index)
  return (
    <li {...props} className={cn(motion.className, flash && 'animate-flash', className)} style={{ ...motion.style, ...style }} />
  )
}
