import { expect, test } from '@playwright/test'

import { api, login, MOCK } from './helpers'

interface Integration {
  id: number
  name: string
}

test('HTTP-JSON: Werte per Formular-Liste, Schwellwert, Zeitstempel als Backup', async ({ page }) => {
  await login(page)
  await page.goto('/einstellungen')
  const section = page.locator('[data-slot=card]').filter({ hasText: 'Integrationen' })
  await section.getByRole('button', { name: 'Hinzufügen' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: /^HTTP-JSON/ }).click()

  await dialog.locator('#integration-name').fill('Paperless')
  await dialog.getByLabel('Adresse').fill(`${MOCK}/json/stats`)
  // Body nur bei POST
  await expect(dialog.getByLabel('Body (nur POST)')).toBeHidden()
  await dialog.getByLabel('Header mit Zugangsdaten').fill('Authorization: Token json-token')

  // Wert 1 ist schon da
  const list = dialog.locator('[data-list=values]')
  const first = list.locator('[data-item="1"]')
  await first.getByLabel('Anzeigename').fill('Dokumente')
  await first.getByLabel('JSON-Pfad').fill('documents_total')
  await first.getByLabel('Warnung ab').fill('1000')
  // „Als Backup werten“ nur bei Zeitstempeln
  await expect(first.getByLabel('Als Backup werten')).toBeHidden()

  // falscher Pfad → klare Meldung beim Testen
  await list.getByRole('button', { name: 'Hinzufügen' }).click()
  const second = list.locator('[data-item="2"]')
  await second.getByLabel('Anzeigename').fill('Letztes Backup')
  await second.getByLabel('JSON-Pfad').fill('backup.gibt_es_nicht')
  await dialog.getByRole('button', { name: 'Verbindung testen' }).click()
  await expect(dialog.getByRole('status')).toContainText('Letztes Backup: Pfad backup.gibt_es_nicht nicht in der Antwort gefunden')

  await second.getByLabel('JSON-Pfad').fill('backup.last_success')
  await second.getByLabel('Art', { exact: true }).click()
  await page.getByRole('option', { name: 'Zeitstempel → Alter' }).click()
  await second.getByLabel('Als Backup werten').click()
  await dialog.getByRole('button', { name: 'Verbindung testen' }).click()
  await expect(dialog.getByRole('status')).toContainText('Verbindung erfolgreich – 1 Backup')

  // dritten Wert hinzufügen und wieder entfernen
  await list.getByRole('button', { name: 'Hinzufügen' }).click()
  await list.getByRole('button', { name: 'Werte 3 entfernen' }).click()
  await expect(list.locator('[data-item]')).toHaveCount(2)

  await dialog.getByRole('button', { name: 'Speichern' }).click()
  await expect(dialog).toBeHidden()
  await expect(section.locator('li[data-integration="Paperless"]').getByText('Verbunden')).toBeVisible()

  // Bearbeiten: Liste wird mit den gespeicherten Werten gefüllt
  await section.getByRole('button', { name: 'Paperless bearbeiten' }).click()
  await expect(list.locator('[data-item="2"]').getByLabel('JSON-Pfad')).toHaveValue('backup.last_success')
  await dialog.getByRole('button', { name: 'Abbrechen' }).click()

  // Schwellwert überschritten → Problem auf dem Dashboard
  await page.goto('/')
  const problem = page.locator('[data-problem="Dokumente"]')
  await expect(problem).toContainText('Warnung')
  await expect(problem).toContainText('Paperless · Wert 1.523')

  // Zeitstempel als Backup
  await page.goto('/backups')
  await expect(page.locator('tr[data-backup="Letztes Backup"]')).toContainText('OK')

  const all = await api<Integration[]>(page, 'GET', '/api/integrations')
  await api(page, 'DELETE', `/api/integrations/${all.json.find((i) => i.name === 'Paperless')!.id}`)
})

test('Prometheus: Abfragen mit Label je Ergebnis und Schwellwerten', async ({ page }) => {
  await login(page)
  const created = await api<Integration>(page, 'POST', '/api/integrations', {
    type: 'prometheus', name: 'Prometheus', enabled: true, interval_seconds: 3600, timeout_seconds: 5, tls_verify: true,
    clear_secrets: [], secrets: {},
    config: {
      base_url: `${MOCK}/prometheus`, auth: 'none',
      queries: [
        { name: 'CPU', query: '100 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100', unit: '%', warn: 80 },
        {
          name: 'Frei', unit: '%', label: 'instance', direction: 'below', warn: 20, error: 10,
          query: 'node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"} * 100',
        },
        { name: 'Restic', query: 'restic_backup_timestamp', kind: 'timestamp', label: 'client_hostname', backup: true },
      ],
    },
  })
  expect(created.status).toBe(201)
  await api(page, 'POST', `/api/integrations/${created.json.id}/poll`)

  await page.goto('/dienste')
  const card = page.locator('[data-service="Prometheus"]')
  await expect(card.getByRole('button', { name: /CPU\s*12,4 %/ })).toBeVisible()
  await expect(card.getByRole('row', { name: /pve:9100/ })).toContainText('9,6 %')

  // ungültige Abfrage → Meldung von Prometheus
  const test = await api<{ ok: boolean; error: string }>(page, 'POST', '/api/integrations/test', {
    type: 'prometheus', name: 'X', enabled: true, interval_seconds: 60, timeout_seconds: 5, tls_verify: true,
    secrets: {}, clear_secrets: [], config: { base_url: `${MOCK}/prometheus`, queries: [{ name: 'Kaputt', query: 'sum(' }] },
  })
  expect(test.json.ok).toBe(false)
  expect(test.json.error).toContain('Kaputt: Abfrage ungültig')

  await page.goto('/')
  await expect(page.locator('[data-problem="Frei"]')).toContainText('pve:9100')
  await page.goto('/backups')
  await expect(page.locator('tr[data-backup="Restic"]')).toHaveCount(2)

  await api(page, 'DELETE', `/api/integrations/${created.json.id}`)
})
