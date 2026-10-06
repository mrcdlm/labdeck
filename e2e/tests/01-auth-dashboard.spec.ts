import { expect, test } from '@playwright/test'

import { api, expectNoHorizontalOverflow, login } from './helpers'

test('falsches Passwort zeigt Meldung und leert das Feld', async ({ page }) => {
  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await page.fill('input[name=password]', 'falsch')
  await page.click('button[type=submit]')
  await expect(page.getByRole('alert')).toContainText('Passwort falsch')
  await expect(page.locator('input[name=password]')).toHaveValue('')
})

test('Login, Reload, Abmelden, geschützte Seiten', async ({ page }) => {
  await login(page)
  await expect(page.getByText('Noch keine Integrationen eingerichtet')).toBeVisible()

  await page.reload()
  await expect(page).toHaveURL('/')
  await expect(page.getByText('Noch keine Integrationen eingerichtet')).toBeVisible()

  // unbekannte Route → Dashboard
  await page.goto('/gibtsnicht')
  await expect(page).toHaveURL('/')

  await page.getByRole('button', { name: 'Abmelden' }).first().click()
  await expect(page).toHaveURL(/\/login$/)
  await page.goto('/einstellungen')
  await expect(page).toHaveURL(/\/login$/)
  expect((await api(page, 'GET', '/api/settings')).status).toBe(401)
})

test('Navigation zu allen Seiten, Live-Verbindung', async ({ page }) => {
  await login(page)
  await expect(page.getByText('Live', { exact: true })).toBeVisible()

  const nav = page.locator('aside nav')
  for (const [link, url, heading] of [
    ['Backups', '/backups', 'Backups'],
    ['Checks', '/checks', 'Checks'],
    ['Dienste', '/dienste', 'Dienste'],
    ['Verlauf', '/verlauf', 'Verlauf'],
    ['Einstellungen', '/einstellungen', 'Einstellungen'],
    ['Dashboard', '/', 'Dashboard'],
  ]) {
    await nav.getByRole('link', { name: link }).click()
    await expect(page).toHaveURL(url)
    await expect(page.getByRole('heading', { level: 1, name: heading })).toBeVisible()
  }

  // Gesamtstatus führt zu den Einstellungen
  await page.getByRole('link', { name: /Noch keine Integrationen eingerichtet/ }).click()
  await expect(page).toHaveURL('/einstellungen')
})

test('Abmelden in einem Tab meldet andere Tabs über den Live-Stream ab', async ({ page, context }) => {
  test.slow() // Session-Prüfung im Heartbeat (25 s)
  await login(page)
  const other = await context.newPage()
  await other.goto('/')
  await other.getByRole('button', { name: 'Abmelden' }).first().click()
  await expect(page).toHaveURL(/\/login$/, { timeout: 45_000 })
})

test('Dashboard mobil ohne Überlauf, Navigation erreichbar', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await login(page)
  await expect(page.getByText('Noch keine Integrationen eingerichtet')).toBeVisible()
  await expectNoHorizontalOverflow(page)
  await page.locator('header nav').getByRole('link', { name: 'Einstellungen' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Einstellungen' })).toBeVisible()
  await expectNoHorizontalOverflow(page)
})
