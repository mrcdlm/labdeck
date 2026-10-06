import type { ConfigValue, Integration, IntegrationInput, IntegrationType, SchemaField } from './api'

/** Vorschläge für das Abfrageintervall (Sekunden). */
export const intervalPresets = [15, 30, 60, 120, 300, 600, 900, 1800, 3600, 6 * 3600, 12 * 3600, 86400]

/** Formularzustand: Zahlen als Text, damit leere Felder möglich sind. */
export type DraftValue = string | boolean
export type Draft = Record<string, DraftValue>

/** Startwerte eines Felds: gespeicherter Wert, sonst Default. */
export function initialDraft(type: IntegrationType, integration?: Integration): Draft {
  const out: Draft = {}
  for (const f of type.fields) {
    if (f.type === 'secret') {
      out[f.key] = ''
      continue
    }
    const v: ConfigValue | undefined = integration ? integration.config[f.key] : f.default
    if (f.type === 'bool') out[f.key] = v === true
    else out[f.key] = v === undefined || v === null ? '' : String(v)
  }
  return out
}

/** Formularzustand → Werte für die API (Secrets separat, leer = unverändert). */
export function draftToConfig(fields: SchemaField[], draft: Draft) {
  const config: Record<string, ConfigValue> = {}
  const secrets: Record<string, string> = {}
  for (const f of fields) {
    const v = draft[f.key]
    if (f.type === 'secret') {
      if (typeof v === 'string' && v.trim() !== '') secrets[f.key] = v.trim()
      continue
    }
    if (f.type === 'bool') {
      config[f.key] = v === true
      continue
    }
    if (typeof v !== 'string') continue
    if (f.type === 'number') {
      if (v.trim() === '') continue
      const n = Number(v.replace(',', '.'))
      config[f.key] = Number.isFinite(n) ? n : v
    } else {
      // leer mitschicken: ein geleertes Feld bekommt keinen Default
      config[f.key] = v
    }
  }
  return { config, secrets }
}

/** Eingabe, die eine gespeicherte Integration unverändert lässt (z. B. für den Aktiv-Schalter). */
export function inputFromIntegration(i: Integration, patch: Partial<IntegrationInput> = {}): IntegrationInput {
  return {
    type: i.type,
    name: i.name,
    enabled: i.enabled,
    interval_seconds: i.interval_seconds,
    timeout_seconds: i.timeout_seconds,
    tls_verify: i.tls_verify,
    config: i.config,
    secrets: {},
    clear_secrets: [],
    ...patch,
  }
}
