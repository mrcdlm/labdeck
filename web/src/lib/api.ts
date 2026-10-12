import { currentLang } from '@/i18n'

/** Einheitlicher Status für Badges, Kacheln und Gesamtstatus. */
export type Status = 'ok' | 'warning' | 'error' | 'unknown' | 'paused'

export interface Settings {
  /** IANA-Zeitzone für die Anzeige; leer = Zeitzone des Browsers */
  timezone: string
  raw_retention_days: number
  daily_retention_days: number
  /** Standard-Höchstalter von Backups in Stunden */
  backup_max_age_hours: number
  /** Sprache der Benachrichtigungen */
  notify_language: 'de' | 'en'
  /** Aufbewahrung der Rohdaten einzelner VMs/Container in Tagen */
  resource_retention_days: number
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

export type FieldType = 'string' | 'text' | 'url' | 'number' | 'bool' | 'select' | 'secret' | 'list'

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
  /** Zahl: Höchstwert; Liste: höchstens so viele Einträge */
  max?: number
  /** halbe Breite im Formular */
  half?: boolean
  /** Unterfelder je Eintrag (nur list) */
  item?: SchemaField[]
  /** nur anzeigen, wenn Feld key einen der Werte hat */
  show_if?: { key: string; values: string[] }
}

export interface IntegrationType {
  type: string
  title: string
  description: string
  /** lucide-Icon, z. B. "heart-pulse" */
  icon: string
  docs?: string
  /** spricht kein HTTP(S): „TLS prüfen“ ausblenden */
  no_tls?: boolean
  fields: SchemaField[]
}

export interface IntegrationTypes {
  types: IntegrationType[]
  /** ohne LABDECK_SECRET_KEY nur ${SECRET_…}-Verweise möglich */
  secret_key_set: boolean
  limits: Record<string, number>
}

export type ScalarValue = string | number | boolean
export type ConfigValue = ScalarValue | Record<string, ScalarValue>[]

export interface Metric {
  key: string
  name: string
  /** fasst Kennzahlen zusammen, z. B. je Benutzer; leer = Hauptkennzahl */
  group?: string
  value: number
  unit?: string
  status: Status
  /** aus Checks/Backups abgeleitet */
  derived?: boolean
  /** gehört zu dieser Ressource (Ressourcen-Seite) */
  resource?: string
}

export type ResourceKind = 'node' | 'vm' | 'ct' | 'container' | 'switch' | 'port'

/** Host, VM oder Container mit Auslastung. */
export interface Resource {
  id: string
  name: string
  kind: ResourceKind
  /** Host/Node (leer bei Hosts) */
  parent?: string
  /** Zustand der Quelle: running, stopped, online, healthy … */
  state: string
  status: Status
  /** Prozent der zugewiesenen Kerne */
  cpu?: number
  cpus?: number
  mem_used?: number
  mem_total?: number
  disk_used?: number
  disk_total?: number
  /** Bytes seit Start bzw. Bytes/s (net_rate) */
  net_in?: number
  net_out?: number
  net_rate?: boolean
  uptime?: number
  tags?: string[]
  info?: Record<string, string>
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
  /** zusätzliche Angaben, z. B. version */
  info?: Record<string, string>
  resources?: Resource[]
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
  history: (id: number, key: string, range: MetricRange) =>
    request<MetricHistory>('GET', `/api/integrations/${id}/history?key=${encodeURIComponent(key)}&range=${range}`),
}

// --- Checks, Backups, Gesamtstatus -----------------------------------------

export type UptimeRange = '24h' | '7d' | '30d'
export const uptimeRanges: UptimeRange[] = ['24h', '7d', '30d']

export interface Uptime {
  /** Anteil erreichbarer an bekannter Zeit; null = nichts bekannt */
  percent: number | null
  /** Anteil bekannter Zeit am Zeitraum (0..1) */
  observed: number
  start: string
  bucket_seconds: number
  /** schlechtester Status je Abschnitt (unknown = keine Daten) */
  buckets: Status[]
}

/** Uptime je Zeitraum; null = die Quelle liefert keinen Verlauf */
export type Uptimes = Record<UptimeRange, Uptime> | null

export interface CheckItem {
  integration_id: number
  integration_name: string
  id: string
  name: string
  group?: string
  tags: string[]
  status: Status
  /** Status der Quelle, z. B. "grace" */
  raw?: string
  desc?: string
  last_ping?: string
  next_ping?: string
  /** Integration gerade nicht erreichbar – Daten vom letzten Abruf */
  stale?: boolean
  uptime: Uptimes
}

export type BackupState = 'ok' | 'due_soon' | 'overdue' | 'failed' | 'unknown' | 'paused'

export interface BackupItem {
  integration_id: number
  integration_name: string
  integration_type: string
  id: string
  name: string
  group?: string
  description?: string
  last_run?: string
  result: Status
  size_bytes?: number
  duration_seconds?: number
  /** Prüfung durch die Quelle (z. B. PBS-Verify); fehlt = unbekannt */
  verify?: 'ok' | 'failed' | 'none'
  state: BackupState
  status: Status
  max_age_hours: number
  max_age_custom: boolean
  due_at?: string
  stale?: boolean
  uptime: Uptimes
}

export interface BackupSummary {
  total: number
  states: Partial<Record<BackupState, number>>
  status: Status
}

export interface BackupsOverview {
  backups: BackupItem[]
  summary: BackupSummary
  default_max_age_hours: number
  warn_percent: number
  limits: Record<string, number>
}

export interface Problem {
  kind: 'integration' | 'checks' | 'backup' | 'metric'
  status: Status
  integration_id: number
  integration_name: string
  integration_type: string
  item_id?: string
  name: string
  /** Kennzahl/Backup: Gruppe, z. B. Benutzer oder Host */
  group?: string
  /** Kennzahl: aktueller Wert */
  value?: number
  unit?: string
  /** Backup: overdue, failed, due_soon; Check: Status der Quelle (down, grace) */
  state?: string
  /** Fehlermeldung der Integration */
  detail?: string
  at?: string
  /** checks: Anzahl und Namen der ausgefallenen Checks */
  count?: number
  items?: string[]
}

export interface Overview {
  status: Status
  integrations: { total: number; active: number; error: number }
  checks: { total: number; statuses: Partial<Record<Status, number>>; status: Status }
  backups: BackupSummary
  problems: Problem[]
}

export const monitoringApi = {
  overview: () => request<Overview>('GET', '/api/overview'),
  checks: () => request<{ checks: CheckItem[] }>('GET', '/api/checks'),
  backups: () => request<BackupsOverview>('GET', '/api/backups'),
  /** maxAgeHours 0 = Standard */
  setBackupMaxAge: (integrationId: number, itemId: string, maxAgeHours: number) =>
    request<void>('PUT', '/api/backups/max-age', {
      integration_id: integrationId,
      item_id: itemId,
      max_age_hours: maxAgeHours,
    }),
}

// --- Verlauf von Kennzahlen --------------------------------------------------

export type MetricRange = '24h' | '7d' | '30d' | '1y'
export const metricRanges: MetricRange[] = ['24h', '7d', '30d', '1y']

export interface MetricHistory {
  key: string
  name: string
  group?: string
  unit?: string
  range: MetricRange
  /** 24 h/7 Tage: letzter Wert je Abschnitt; 30 Tage/1 Jahr: Tageswerte */
  points: { t: string; v: number; min?: number; max?: number }[]
}

// --- Benachrichtigungen (Webhooks wie dnsdeck) -------------------------------

export type NotifyEventType =
  | 'check_down'
  | 'check_up'
  | 'backup_overdue'
  | 'backup_failed'
  | 'backup_recovered'
  | 'integration_down'
  | 'integration_recovered'
  | 'threshold'

export type WebhookMethod = 'POST' | 'PUT' | 'PATCH' | 'GET'

export interface WebhookInput {
  name: string
  enabled: boolean
  method: WebhookMethod
  url: string
  headers: { name: string; value: string }[]
  content_type: string
  body_template: string
  events: NotifyEventType[]
  /** neuer Wert für ${TOKEN}; leer = unverändert */
  token?: string
  clear_token?: boolean
}

export interface Webhook extends Omit<WebhookInput, 'token' | 'clear_token'> {
  id: number
  last_sent_at?: string
  last_error?: string
  created_at: string
  updated_at: string
  /** verwendete, aber nicht gesetzte Env-Variablen (nur Namen) */
  missing_env: string[]
  /** Token gespeichert (der Wert wird nie ausgeliefert) */
  token_set: boolean
  /** ${TOKEN} wird verwendet, aber keins ist gespeichert */
  token_missing: boolean
}

export interface WebhookPreview {
  method: string
  url: string
  headers: Record<string, string[]>
  body: string
}

export interface WebhookTestResult {
  ok: boolean
  error?: string
}

export const webhookApi = {
  list: () => request<Webhook[]>('GET', '/api/webhooks'),
  create: (w: WebhookInput) => request<Webhook>('POST', '/api/webhooks', w),
  update: (id: number, w: WebhookInput) => request<Webhook>('PUT', `/api/webhooks/${id}`, w),
  remove: (id: number) => request<void>('DELETE', `/api/webhooks/${id}`),
  test: (id: number) => request<WebhookTestResult>('POST', `/api/webhooks/${id}/test`),
  preview: (w: WebhookInput, eventType: string) =>
    request<WebhookPreview>('POST', '/api/webhooks/preview', { ...w, event_type: eventType }),
}

// --- Verlauf und Dashboard ---------------------------------------------------

export type HistoryKind = 'integration' | 'check' | 'backup' | 'metric'

export interface HistoryEntry {
  id: number
  integration_id: number
  integration_name: string
  integration_type: string
  kind: HistoryKind
  item_id?: string
  item_name: string
  item_group?: string
  /** Status (ok, error …) bzw. bei Backups der Zustand (overdue, failed …) */
  from: string
  to: string
  message?: string
  at: string
}

export interface HistoryFilter {
  integration?: number
  kind?: HistoryKind
  problems?: boolean
}

export interface HistoryPage {
  entries: HistoryEntry[]
  more: boolean
}

export interface DashboardTile {
  /** backups, checks, hosts, backup-timeline, events, integration:<id> */
  id: string
  visible: boolean
  /** Spalten 1–3 */
  size: number
}

export interface DashboardLayout {
  tiles: DashboardTile[]
  /** Kacheln mit Problemen nach vorne (Standard: ja) */
  problems_first: boolean
}

export const historyApi = {
  list: (f: HistoryFilter, before?: number, limit = 50) => {
    const q = new URLSearchParams({ limit: String(limit) })
    if (f.integration) q.set('integration', String(f.integration))
    if (f.kind) q.set('kind', f.kind)
    if (f.problems) q.set('problems', '1')
    if (before) q.set('before', String(before))
    return request<HistoryPage>('GET', `/api/history?${q}`)
  },
  dashboard: () => request<DashboardLayout>('GET', '/api/dashboard'),
  saveDashboard: (l: DashboardLayout) => request<DashboardLayout>('PUT', '/api/dashboard', l),
}
