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
