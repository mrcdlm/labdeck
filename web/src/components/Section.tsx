import type { ReactNode } from 'react'

import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

/** Karte mit Icon-Titel und Beschreibung, z. B. ein Abschnitt der Einstellungen. */
export function Section({
  icon,
  title,
  description,
  action,
  children,
}: {
  icon: ReactNode
  title: string
  description: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg">
          {icon}
          {title}
        </CardTitle>
        <CardDescription>{description}</CardDescription>
        {action && <CardAction>{action}</CardAction>}
      </CardHeader>
      <CardContent className="flex flex-col gap-5">{children}</CardContent>
    </Card>
  )
}
