import { expect, test } from '@playwright/test'

import { api, expectNoHorizontalOverflow, login, MOCK } from './helpers'

interface Integration {
  id: number
  name: string
}

test('Immich: anlegen, Kachel, Dienste-Seite mit Kennzahlen, Benutzern und Diagramm', async ({ page }) => {
  await login(page)
  await page.goto('/einstellungen')
  const section = page.locator('[data-slot=card]').filter({ hasText: 'Integrationen' })
  await section.getByRole('button', { name: 'Hinzufügen' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: /^Immich/ }).click()
  await expect(dialog.getByRole('heading', { name: 'Immich hinzufügen' })).toBeVisible()
  await expect(dialog.getByText(/server\.statistics/)).toBeVisible()

  await dialog.getByLabel('Name').fill('Fotos')
  await dialog.getByLabel('Adresse').fill(`${MOCK}/immich`)

  // Key ohne Berechtigung → konkreter Hinweis
  await dialog.getByLabel('API-Key').fill('immich-user')
  await dialog.getByRole('button', { name: 'Verbindung testen' }).click()
  await expect(dialog.getByRole('status')).toContainText('Berechtigung server.statistics')

  await dialog.getByLabel('API-Key').fill('immich-admin')
  await dialog.getByRole('button', { name: 'Verbindung testen' }).click()
  await expect(dialog.getByRole('status')).toContainText('Verbindung erfolgreich')
  await dialog.getByRole('button', { name: 'Speichern' }).click()
  await expect(dialog).toBeHidden()
  await expect(section.locator('li[data-integration="Fotos"]').getByText('Verbunden')).toBeVisible()

  // Dashboard-Kachel „48.213 Fotos · 1.204 Videos · 412 GB“
  await page.goto('/')
  const tile = page.locator('[data-tile="Fotos"]')
  await expect(tile).toContainText('48.213 Fotos')
  await expect(tile).toContainText('1.204 Videos')
  await expect(tile).toContainText('412 GB Speicher')

  // Systeme: Karte unter „Anwendungen“ mit den Kernzahlen
  await page.goto('/systeme')
  const sys = page.locator('[data-category="apps"] [data-system="Fotos"]')
  await expect(sys).toContainText('48.213 Fotos')
  await expect(sys).toContainText('Immich · v1.140.1')

  // Detailseite (von der Kachel aus): Kennzahlen, Version, je Benutzer
  await page.goto('/')
  await tile.click()
  await expect(page).toHaveURL(/\/systeme\/\d+$/)
  const card = page.locator('[data-service="Fotos"]')
  await expect(card).toContainText('Immich · v1.140.1')
  await expect(card.getByRole('button', { name: /Fotos\s*48\.213/ })).toHaveAttribute('aria-pressed', 'true')
  const anna = card.getByRole('row', { name: /Anna/ })
  await expect(anna).toContainText('16.711')
  await expect(anna).toContainText('51 %')

  // Diagramm: mit zwei Abfragen genug Punkte (Rohdaten 24 h)
  const list = await api<Integration[]>(page, 'GET', '/api/integrations')
  const id = list.json.find((i) => i.name === 'Fotos')!.id
  await fetch(`${MOCK}/_immich/photos?n=50`, { method: 'POST' })
  await card.getByRole('button', { name: '24 h' }).click()
  await expect(card.getByText(/Noch zu wenige Werte/)).toBeVisible()
  // ein Abschnitt je 15 Minuten → zweiter Punkt erst später; Tageswerte (30 Tage) ebenso nur einer
  const h = await api<{ points: unknown[] }>(page, 'GET', `/api/integrations/${id}/history?key=photos&range=24h`)
  expect(h.json.points.length).toBe(1)

  // Andere Kennzahl wählen
  await card.getByRole('button', { name: /^Videos/ }).click()
  await expect(card.getByText('Verlauf: Videos')).toBeVisible()

  // Mobil ohne horizontales Scrollen
  await page.setViewportSize({ width: 390, height: 844 })
  await expectNoHorizontalOverflow(page)

  await api(page, 'DELETE', `/api/integrations/${id}`)
})
