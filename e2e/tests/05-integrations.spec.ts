import { expect, test } from '@playwright/test'

import { api, login, mock, MOCK } from './helpers'

interface Integration {
  id: number
  name: string
  status: string
  secrets: Record<string, { set: boolean }>
}

test('Healthchecks: anlegen, testen, abfragen, bearbeiten, deaktivieren, löschen', async ({ page }) => {
  await login(page)
  await page.goto('/einstellungen')
  const section = page.locator('[data-slot=card]').filter({ hasText: 'Integrationen' })
  await expect(section.getByText('Noch keine Integration eingerichtet.')).toBeVisible()

  // Nur ein Typ verfügbar → Formular erscheint direkt
  await section.getByRole('button', { name: 'Hinzufügen' }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('heading', { name: 'Healthchecks.io hinzufügen' })).toBeVisible()
  await expect(dialog.getByLabel('Adresse')).toHaveValue('https://healthchecks.io')
  await expect(dialog.getByLabel('Backup-Tag')).toHaveValue('backup')

  await dialog.getByLabel('Name').fill('Homelab')
  await dialog.getByLabel('Adresse').fill(`${MOCK}/healthchecks`)

  // Speichern ohne API-Key: Fehler vom Server
  await dialog.getByRole('button', { name: 'Speichern' }).click()
  await expect(dialog.getByRole('alert')).toContainText('API-Key: Pflichtfeld')

  // falscher Key → verständlicher Fehler
  await dialog.getByLabel('API-Key').fill('falsch')
  await dialog.getByRole('button', { name: 'Verbindung testen' }).click()
  await expect(dialog.getByRole('status')).toContainText('Zugriff verweigert (HTTP 401)')

  // richtiger Key → Überblick
  await dialog.getByLabel('API-Key').fill('hc-readonly')
  await dialog.getByRole('button', { name: 'Verbindung testen' }).click()
  await expect(dialog.getByRole('status')).toContainText('Verbindung erfolgreich – 7 Checks · 3 Backups')

  await dialog.getByRole('button', { name: 'Speichern' }).click()
  await expect(dialog).toBeHidden()

  // wird sofort abgefragt
  const item = section.locator('li[data-integration="Homelab"]')
  await expect(item.getByText('Verbunden')).toBeVisible()
  await expect(item.getByText(/Abgefragt .* · 7 Checks · 3 Backups/)).toBeVisible()

  // Secret verlässt nie den Server
  const list = await api<Integration[]>(page, 'GET', '/api/integrations')
  expect(JSON.stringify(list.json)).not.toContain('hc-readonly')
  expect(list.json[0].secrets.api_key.set).toBe(true)

  // Dashboard: Gesamtstatus
  await page.getByRole('link', { name: 'Dashboard' }).first().click()
  // Demo-Daten: vzdump überfällig, Borg bald fällig
  await expect(page.getByText('2 Probleme')).toBeVisible()
  await expect(page.getByText('1 Integration aktiv')).toBeVisible()

  // Ausfall der Quelle → Problem auf dem Dashboard (live)
  await mock.hcFail(500)
  await api(page, 'POST', `/api/integrations/${list.json[0].id}/poll`)
  await expect(page.getByText('3 Probleme')).toBeVisible()
  await expect(page.getByText('Dienst antwortet mit HTTP 500')).toBeVisible()
  await mock.hcFail(0)

  // Abfragen per Button → wieder in Ordnung
  await page.goto('/einstellungen')
  await expect(item.getByText('Fehler', { exact: true })).toBeVisible()
  await item.getByRole('button', { name: 'Abfragen' }).click()
  await expect(item.getByText('Verbunden')).toBeVisible()

  // Bearbeiten: Key bleibt, wenn das Feld leer bleibt
  await item.getByRole('button', { name: 'Homelab bearbeiten' }).click()
  await expect(dialog.getByLabel('API-Key')).toHaveValue('')
  await expect(dialog.getByLabel('API-Key')).toHaveAttribute('placeholder', /gespeichert/)
  await dialog.getByLabel('Name').fill('Homelab HC')
  await dialog.getByRole('button', { name: 'Verbindung testen' }).click()
  await expect(dialog.getByRole('status')).toContainText('Verbindung erfolgreich')
  await dialog.getByRole('button', { name: 'Speichern' }).click()
  await expect(dialog).toBeHidden()
  const renamed = section.locator('li[data-integration="Homelab HC"]')
  await expect(renamed.getByText('Verbunden')).toBeVisible()

  // Deaktivieren
  await renamed.getByRole('switch', { name: 'Homelab HC aktiv' }).click()
  await expect(renamed.getByText('Deaktiviert')).toBeVisible()
  await expect(renamed.getByRole('button', { name: 'Abfragen' })).toBeDisabled()

  // Löschen
  await renamed.getByRole('button', { name: 'Homelab HC löschen' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Löschen' }).click()
  await expect(section.getByText('Noch keine Integration eingerichtet.')).toBeVisible()
})

test('Verbindungsfehler verständlich melden', async ({ page }) => {
  await login(page)
  const base = {
    type: 'healthchecks', name: 'X', enabled: true, interval_seconds: 60, timeout_seconds: 2, tls_verify: true,
    secrets: { api_key: 'hc-readonly' }, clear_secrets: [],
  }
  // nicht erreichbarer Port
  const refused = await api<{ ok: boolean; error: string }>(page, 'POST', '/api/integrations/test', {
    ...base, config: { base_url: 'http://127.0.0.1:1' },
  })
  expect(refused.json.ok).toBe(false)
  expect(refused.json.error).toContain('Verbindung abgelehnt')

  // https gegen den http-Mock
  const noTLS = await api<{ ok: boolean; error: string }>(page, 'POST', '/api/integrations/test', {
    ...base, config: { base_url: MOCK.replace('http:', 'https:') + '/healthchecks' },
  })
  expect(noTLS.json.error).toContain('spricht kein HTTPS')
})
