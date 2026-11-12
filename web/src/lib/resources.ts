import type { Resource } from '@/lib/api'

/** Läuft die Ressource (VM, Container, Host)? Unhealthy zählt als laufend. */
export const running = (r: Resource) => ['running', 'online', 'healthy', 'unhealthy'].includes(r.state)
