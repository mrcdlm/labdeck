import { expect, test } from '@playwright/test'

import { api, login, mock, MOCK, problemList } from './helpers'

interface Integration {
  id: number
}

const hc = (page: Parameters<typeof api>[0]) =>
  api<Integration>(page, 'POST', '/api/integrations', {
    type: 'healthchecks', name: 'Homelab', enabled: true, interval_seconds: 3600, timeout_seconds: 5, tls_verify: true,
    config: { base_url: `${MOCK}/healthchecks`, backup_tag: '' }, secrets: { api_key: 'hc-readonly' }, clear_secrets: [],
  })

test('Logos laden ohne CSP-Fehler, Checks-Kachel als Ampel (live grün → rot)', async ({ page }) => {
  const errors: string[] = []
  page.on('console', (m) => m.type() === 'error' && errors.push(m.text()))
  await login(page)
  await mock.hcStatus('Proxmox vzdump', 'up')
  await mock.hcStatus('Borg Laptop', 'up')
  const created = await hc(page)
  await api(page, 'POST', `/api/integrations/${created.json.id}/poll`)

  await page.goto('/')
  const tile = page.locator('[data-tile="checks"]')
  await expect(tile).toContainText('6 Checks aktiv')
  await expect(tile).toContainText('alle OK')
  await expect(tile).toHaveAttribute('data-tone', 'ok')
  // Logo statt Symbol, tatsächlich geladen
  const logo = tile.locator('img')
  await expect(logo).toHaveCount(1)
  expect(await logo.evaluate((img: HTMLImageElement) => img.complete && img.naturalWidth > 0)).toBe(true)

  // Check fällt aus → Kachel wird live rot, Ampel pulsiert
  await mock.hcStatus('DynDNS', 'down')
  await api(page, 'POST', `/api/integrations/${created.json.id}/poll`)
  await expect(tile).toContainText('1 Check ausgefallen')
  await expect(tile).toHaveAttribute('data-tone', 'error')
  await expect(problemList(page).locator('[data-problem="DynDNS"]')).toBeVisible()
  await expect(page.locator('[data-tone="error"] .animate-ring')).toBeVisible()

  // Systeme: Healthchecks als Karte, die zur Checks-Seite führt
  await page.goto('/systeme')
  const card = page.locator('[data-category="monitoring"] [data-system="Homelab"]')
  await expect(card).toHaveAttribute('data-tone', 'error')
  await expect(card.locator('[data-problem-count]')).toHaveText('1 Problem')
  await expect(card).toHaveAttribute('href', '/checks')

  expect(errors.filter((e) => /Content Security Policy|Refused/.test(e))).toEqual([])

  await mock.hcStatus('DynDNS', 'up')
  await mock.hcStatus('Proxmox vzdump', 'down')
  await mock.hcStatus('Borg Laptop', 'grace')
  await api(page, 'DELETE', `/api/integrations/${created.json.id}`)
})

test.describe('Bewegung reduzieren', () => {
  test.use({ reducedMotion: 'reduce' })

  test('Zahlen stehen sofort, keine Animationen', async ({ page }) => {
    await login(page)
    const im = await api<Integration>(page, 'POST', '/api/integrations', {
      type: 'immich', name: 'Fotos', enabled: true, interval_seconds: 3600, timeout_seconds: 5, tls_verify: true,
      config: { base_url: `${MOCK}/immich` }, secrets: { api_key: 'immich-admin' }, clear_secrets: [],
    })
    await api(page, 'POST', `/api/integrations/${im.json.id}/poll`)
    await page.goto('/')
    // ohne Hochzählen sofort der Endwert
    await expect(page.locator('[data-tile="Fotos"]')).toContainText(/48\.\d{3} Fotos/, { timeout: 1000 })
    const duration = await page
      .locator('[data-tile="Fotos"]')
      .evaluate((el) => getComputedStyle(el.parentElement!).animationDuration)
    expect(parseFloat(duration)).toBeLessThan(0.01)
    await api(page, 'DELETE', `/api/integrations/${im.json.id}`)
  })
})
