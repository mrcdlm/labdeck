import type { ConfigValue, Integration, IntegrationInput, IntegrationType, ScalarValue, SchemaField } from './api'

/** Vorschläge für das Abfrageintervall (Sekunden). */
export const intervalPresets = [15, 30, 60, 120, 300, 600, 900, 1800, 3600, 6 * 3600, 12 * 3600, 86400]

/** Formularzustand: Zahlen als Text, damit leere Felder möglich sind; Listen als Einträge. */
export type DraftValue = string | boolean
export type DraftItem = Record<string, DraftValue>
export type Draft = Record<string, DraftValue | DraftItem[]>

function scalarDraft(f: SchemaField, v: ScalarValue | undefined): DraftValue {
  if (f.type === 'bool') return v === true
  return v === undefined || v === null ? '' : String(v)
}

/** Neuer Listeneintrag mit den Defaults der Unterfelder. */
export function newItem(f: SchemaField): DraftItem {
  const out: DraftItem = {}
  for (const sub of f.item ?? []) out[sub.key] = scalarDraft(sub, sub.default)
  return out
}

/** Startwerte eines Felds: gespeicherter Wert, sonst Default. */
export function initialDraft(type: IntegrationType, integration?: Integration): Draft {
  const out: Draft = {}
  for (const f of type.fields) {
    if (f.type === 'secret') {
      out[f.key] = ''
      continue
    }
    const v: ConfigValue | undefined = integration ? integration.config[f.key] : f.default
    if (f.type === 'list') {
      const items = Array.isArray(v) ? v : []
      out[f.key] = items.length
        ? items.map((it) => Object.fromEntries((f.item ?? []).map((sub) => [sub.key, scalarDraft(sub, it[sub.key])])))
        : integration
          ? []
          : [newItem(f)]
      continue
    }
    out[f.key] = scalarDraft(f, Array.isArray(v) ? undefined : v)
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
    if (f.type === 'list') {
      const items = Array.isArray(v) ? v : []
      config[f.key] = items.map((it) => draftToConfig(f.item ?? [], it).config as Record<string, ScalarValue>)
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

/** Feld sichtbar? (show_if: anderes Feld hat einen der Werte) */
export function visible(f: SchemaField, values: Record<string, unknown>): boolean {
  if (!f.show_if) return true
  return f.show_if.values.includes(String(values[f.show_if.key] ?? ''))
}
