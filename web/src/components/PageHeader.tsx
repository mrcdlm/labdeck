import type { ReactNode } from 'react'

/** Seitentitel mit grauer Unterzeile, optional Aktionen rechts. */
export function PageHeader({ title, subtitle, actions }: { title: string; subtitle: string; actions?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        <p className="text-muted-foreground text-sm">{subtitle}</p>
      </div>
      {actions}
    </div>
  )
}
