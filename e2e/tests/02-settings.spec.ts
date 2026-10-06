import { expect, test } from '@playwright/test'

import { api, login } from './helpers'

interface Settings {
  timezone: string
  raw_retention_days: number
  daily_retention_days: number
  limits: Record<string, number>
}

test('Einstellungen: Zeitzone und Aufbewahrung speichern, Verwerfen, Fehler', async ({ page }) => {
  await login(page)
  const before = (await api<Settings>(page, 'GET', '/api/settings')).json
  await page.goto('/einstellungen')

  const save = page.getByRole('button', { name: 'Speichern' })
  await expect(save).toBeDisabled()

  // Verwerfen stellt den gespeicherten Stand wieder her
  await page.getByLabel('Rohdaten aufbewahren').fill('7')
  await page.getByRole('button', { name: 'Verwerfen' }).click()
  await expect(page.getByLabel('Rohdaten aufbewahren')).toHaveValue(String(before.raw_retention_days))

  await page.getByLabel('Zeitzone').click()
  await page.getByRole('option', { name: 'America/New_York', exact: true }).click()
  await page.getByLabel('Rohdaten aufbewahren').fill('14')
  await save.click()
  await expect(page.getByText('Gespeichert', { exact: true })).toBeVisible()
  const saved = (await api<Settings>(page, 'GET', '/api/settings')).json
  expect(saved).toMatchObject({ timezone: 'America/New_York', raw_retention_days: 14 })

  // Nach dem Neuladen bleibt der Stand erhalten
  await page.reload()
  await expect(page.getByLabel('Zeitzone')).toHaveText('America/New_York')
  await expect(page.getByLabel('Rohdaten aufbewahren')).toHaveValue('14')

  // Server-Validierung erscheint im Formular (Browser-Prüfung umgehen)
  await page.locator('form').evaluate((f) => f.setAttribute('novalidate', ''))
  await page.getByLabel('Tageswerte aufbewahren').fill('5')
  await save.click()
  await expect(page.getByRole('alert')).toContainText('Aufbewahrung der Tageswerte muss zwischen 30 und 3650 Tagen liegen')

  await api(page, 'PUT', '/api/settings', before)
})

test('Einstellungen: Änderung in anderem Tab erscheint live', async ({ page }) => {
  await login(page)
  const before = (await api<Settings>(page, 'GET', '/api/settings')).json
  await page.goto('/einstellungen')
  await expect(page.getByText('Live', { exact: true })).toBeVisible()

  expect((await api(page, 'PUT', '/api/settings', { ...before, daily_retention_days: 730 })).status).toBe(200)
  await expect(page.getByLabel('Tageswerte aufbewahren')).toHaveValue('730')

  await api(page, 'PUT', '/api/settings', before)
})

test('Einstellungen: Systeminfo ohne Geheimnisse', async ({ page }) => {
  await login(page)
  await page.goto('/einstellungen')
  const row = page.locator('dl div').filter({ hasText: 'LABDECK_SECRET_KEY' })
  await expect(row.locator('dd')).toHaveText('gesetzt')
  await expect(page.getByText('AQEBAQEB')).toHaveCount(0)
})
