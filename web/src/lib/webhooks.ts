import type { NotifyEventType, Webhook, WebhookInput } from './api'

// Beschriftungen: events.<typ>.label / .hint (locales)
export const eventTypes: NotifyEventType[] = [
  'check_down',
  'check_up',
  'backup_overdue',
  'backup_failed',
  'backup_recovered',
  'integration_down',
  'integration_recovered',
  'threshold',
]

/** Vorlagen; die Erklärung des Tokens steht unter webhooks.tokenFor.<id> */
export type PresetId = 'generic' | 'ntfy' | 'gotify' | 'discord' | 'slack' | 'telegram' | 'homeassistant'

export interface Preset {
  id: PresetId
  /** Anzeigename; Produktnamen werden nicht übersetzt ("generic" über locales) */
  label: string
  /** Vorlage verwendet ${TOKEN} */
  token: boolean
  webhook: Omit<WebhookInput, 'name' | 'enabled' | 'events'>
}

const json = 'application/json'

/**
 * Vorlagen sind nur Startpunkte (aus dnsdeck): technisch ist jeder Webhook
 * gleich (Methode, URL, Header, Body-Template). Das Geheimnis jeder Vorlage
 * steht im Feld Token (verschlüsselt) und wird als ${TOKEN} bzw. {{token}}
 * eingesetzt.
 */
export const presets: Preset[] = [
  {
    id: 'generic',
    label: '',
    token: false,
    webhook: { method: 'POST', url: 'https://example.com/hook', headers: [], content_type: json, body_template: '' },
  },
  {
    id: 'ntfy',
    label: 'ntfy',
    token: false,
    webhook: {
      method: 'POST',
      url: 'https://ntfy.sh',
      headers: [],
      content_type: json,
      body_template: `{
  "topic": "labdeck-mein-geheimes-topic",
  "title": {{json .Title}},
  "message": {{json .Message}},
  "priority": {{.Priority}}
}`,
    },
  },
  {
    id: 'gotify',
    label: 'Gotify',
    token: true,
    webhook: {
      method: 'POST',
      url: 'https://gotify.example.com/message',
      headers: [{ name: 'X-Gotify-Key', value: '${TOKEN}' }],
      content_type: json,
      // Gotify-Skala 0–10 (labdeck: 2–4)
      body_template: `{
  "title": {{json .Title}},
  "message": {{json .Message}},
  "priority": {{mul .Priority 2}}
}`,
    },
  },
  {
    id: 'discord',
    label: 'Discord',
    token: true,
    webhook: {
      method: 'POST',
      url: '${TOKEN}',
      headers: [],
      content_type: json,
      body_template: `{"content": {{json (printf "**%s**\\n%s" .Title .Message)}}}`,
    },
  },
  {
    id: 'slack',
    label: 'Slack',
    token: true,
    webhook: {
      method: 'POST',
      url: '${TOKEN}',
      headers: [],
      content_type: json,
      body_template: `{"text": {{json (printf "*%s*\\n%s" .Title .Message)}}}`,
    },
  },
  {
    id: 'telegram',
    label: 'Telegram',
    token: true,
    webhook: {
      method: 'POST',
      url: 'https://api.telegram.org/bot${TOKEN}/sendMessage',
      headers: [],
      content_type: json,
      body_template: `{
  "chat_id": "123456789",
  "text": {{json (printf "%s\\n%s" .Title .Message)}}
}`,
    },
  },
  {
    id: 'homeassistant',
    label: 'Home Assistant',
    token: true,
    webhook: {
      method: 'POST',
      url: 'http://homeassistant.local:8123/api/webhook/${TOKEN}',
      headers: [],
      content_type: json,
      body_template: '',
    },
  },
]

/** Bearbeitbare Felder eines gespeicherten Webhooks (Token bleibt unverändert). */
export function webhookInput(w: Webhook): WebhookInput {
  const { name, enabled, method, url, headers, content_type, body_template, events } = w
  return { name, enabled, method, url, headers, content_type, body_template, events }
}

export const contentTypes = ['application/json', 'text/plain; charset=utf-8', 'application/x-www-form-urlencoded']
