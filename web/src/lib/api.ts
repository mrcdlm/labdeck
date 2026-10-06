import { currentLang } from '@/i18n'

/** Einheitlicher Status für Badges, Kacheln und Gesamtstatus. */
export type Status = 'ok' | 'warning' | 'error' | 'unknown' | 'paused'

export interface Settings {
  /** IANA-Zeitzone für die Anzeige; leer = Zeitzone des Browsers */
  timezone: string
  raw_retention_days: number
  daily_retention_days: number
  /** Grenzen der Eingaben (nur lesen) */
  limits: Record<string, number>
}

export interface Info {
  version: string
  data_dir: string
  secret_key_set: boolean
}

export interface Session {
  authenticated: boolean
}

export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { 'Accept-Language': currentLang() } // Server-Meldungen in UI-Sprache
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const res = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (res.status === 204) return undefined as T
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const data = (await res.json()) as { error?: string }
      if (data.error) msg = data.error
    } catch {
      // keine JSON-Antwort
    }
    throw new ApiError(res.status, msg)
  }
  return (await res.json()) as T
}

export const api = {
  session: () => request<Session>('GET', '/api/session'),
  login: (password: string) => request<Session>('POST', '/api/login', { password }),
  logout: () => request<Session>('POST', '/api/logout'),
  settings: () => request<Settings>('GET', '/api/settings'),
  saveSettings: (s: Settings) => request<Settings>('PUT', '/api/settings', s),
  info: () => request<Info>('GET', '/api/info'),
}

// --- Integrationen ---------------------------------------------------------

export type FieldType = 'string' | 'text' | 'url' | 'number' | 'bool' | 'select' | 'secret'

export interface SchemaField {
  key: string
  type: FieldType
  label: string
  help?: string
  placeholder?: string
  required: boolean
  default?: string | number | boolean
  options?: { value: string; label: string }[]
  min?: number
  max?: number
}

export interface IntegrationType {
  type: string
  title: string
  description: string
  /** lucide-Icon, z. B. "heart-pulse" */
  icon: string
  docs?: string
  fields: SchemaField[]
}

export interface IntegrationTypes {
  types: IntegrationType[]
  /** ohne LABDECK_SECRET_KEY nur ${SECRET_…}-Verweise möglich */
  secret_key_set: boolean
  limits: Record<string, number>
}

export type ConfigValue = string | number | boolean

export interface Metric {
  key: string
  name: string
  value: number
  unit?: string
  status: Status
}

export interface Check {
  id: string
  name: string
  group?: string
  tags?: string[]
  status: Status
  desc?: string
  last_ping?: string
  next_ping?: string
  raw?: string
}

export interface Backup {
  id: string
  name: string
  group?: string
  last_run?: string
  result: Status
  size_bytes?: number
  duration?: number
  description?: string
}

export interface Snapshot {
  metrics?: Metric[]
  checks?: Check[]
  backups?: Backup[]
}

export interface Integration {
  id: number
  type: string
  name: string
  enabled: boolean
  interval_seconds: number
  timeout_seconds: number
  tls_verify: boolean
  config: Record<string, ConfigValue>
  /** Zugangsdaten: nur ob gesetzt bzw. Verweis ${SECRET_…} */
  secrets: Record<string, { set: boolean; ref?: string }>
  created_at: string
  updated_at: string
  status: Status
  last_poll_at?: string
  last_success_at?: string
  last_error?: string
  last_error_code?: string
  last_duration_ms: number
  snapshot?: Snapshot
}

export interface IntegrationInput {
  type: string
  name: string
  enabled: boolean
  interval_seconds: number
  timeout_seconds: number
  tls_verify: boolean
  config: Record<string, ConfigValue>
  /** neue Werte; fehlend/leer = unverändert */
  secrets: Record<string, string>
  clear_secrets: string[]
}

export interface TestResult {
  ok: boolean
  error?: string
  duration_ms: number
  checks: number
  backups: number
  metrics: number
}

export const integrationApi = {
  types: () => request<IntegrationTypes>('GET', '/api/integration-types'),
  list: () => request<Integration[]>('GET', '/api/integrations'),
  create: (i: IntegrationInput) => request<Integration>('POST', '/api/integrations', i),
  update: (id: number, i: IntegrationInput) => request<Integration>('PUT', `/api/integrations/${id}`, i),
  remove: (id: number) => request<void>('DELETE', `/api/integrations/${id}`),
  /** id: gespeicherte Integration ergänzt leer gelassene Secrets */
  test: (i: IntegrationInput, id?: number) => request<TestResult>('POST', '/api/integrations/test', { ...i, id }),
  poll: (id: number) => request<Integration>('POST', `/api/integrations/${id}/poll`),
}
