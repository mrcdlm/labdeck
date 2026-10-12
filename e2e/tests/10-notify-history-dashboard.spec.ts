import { expect, test } from '@playwright/test'

import { api, login, mock, MOCK } from './helpers'

interface Hook {
  method: string
  path: string
  headers: Record<string, string[]>
  body: string
}

const hooks = async () => (await (await fetch(`${MOCK}/_hooks`)).json()) as Hook[]
const clearHooks = () => fetch(`${MOCK}/_hooks`, { method: 'DELETE' })

test('Webhook mit Token: anlegen, testen, Benachrichtigung bei Check down', async ({ page }) => {
  await clearHooks()
  await login(page)
  await page.goto('/einstellungen')
  const section = page.locator('[data-slot=card]').filter({ hasText: 'Webhooks' })
  await section.getByRole('button', { name: 'Webhook hinzufügen' }).click()
  const dialog = page.getByRole('dialog')

  // Vorlage Gotify: Token-Header vorbelegt
  await dialog.getByLabel('Vorlage').click()
  await page.getByRole('option', { name: 'Gotify' }).click()
  await expect(dialog.getByLabel('Header 1 Wert')).toHaveValue('${TOKEN}')
  await expect(dialog.getByText('Diese Konfiguration verwendet ein Token')).toBeVisible()
  await dialog.locator('#wh-url').fill(`${MOCK}/hook/gotify`)
  await dialog.locator('#wh-token').fill('gt-123')
  // nur „Check down“
  for (const ev of ['Check wieder up', 'Backup überfällig', 'Backup fehlgeschlagen', 'Backup wieder OK',
    'Integration nicht erreichbar', 'Integration wieder erreichbar', 'Schwellwert überschritten']) {
    await dialog.getByLabel(ev, { exact: true }).click()
  }
  // Vorschau zeigt den Platzhalter, nie den Wert
  await expect(dialog.locator('pre')).toContainText('X-Gotify-Key: ${TOKEN}')
  await dialog.getByRole('button', { name: 'Hinzufügen' }).click()
  await expect(dialog).toBeHidden()

  const card = section.locator('li[data-webhook="Gotify"]')
  await card.getByRole('button', { name: 'Testen' }).click()
  await expect(card.getByText('Testnachricht zugestellt')).toBeVisible()
  let got = await hooks()
  expect(got).toHaveLength(1)
  expect(got[0].headers['X-Gotify-Key']).toEqual(['gt-123'])
  expect(got[0].body).toContain('labdeck: Testnachricht')

  // Check fällt aus → Benachrichtigung
  await clearHooks()
  const hc = await api<{ id: number }>(page, 'POST', '/api/integrations', {
    type: 'healthchecks', name: 'Homelab', enabled: true, interval_seconds: 3600, timeout_seconds: 5, tls_verify: true,
    config: { base_url: `${MOCK}/healthchecks` }, secrets: { api_key: 'hc-readonly' }, clear_secrets: [],
  })
  await api(page, 'POST', `/api/integrations/${hc.json.id}/poll`)
  await mock.hcStatus('DynDNS', 'down')
  await api(page, 'POST', `/api/integrations/${hc.json.id}/poll`)
  await expect.poll(async () => (await hooks()).length).toBe(1)
  got = await hooks()
  expect(got[0].body).toContain('Check down: DynDNS')
  await mock.hcStatus('DynDNS', 'up')
  await api(page, 'POST', `/api/integrations/${hc.json.id}/poll`)

  // Verlauf: Wechsel sichtbar, filterbar
  await page.goto('/verlauf')
  const dyn = page.locator('[data-entry="DynDNS"]')
  await expect(dyn.first()).toContainText('Fehler')
  await page.getByLabel('Nur Probleme').click()
  await page.getByLabel('Art').click()
  await page.getByRole('option', { name: 'Backups' }).click()
  await expect(page.locator('[data-entry="Proxmox vzdump"]')).toContainText('Überfällig')
  await expect(page.locator('[data-entry="DynDNS"]')).toHaveCount(0)

  // Löschen
  await page.goto('/einstellungen')
  await card.getByRole('button', { name: 'Gotify löschen' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Löschen' }).click()
  await expect(card).toHaveCount(0)
  await api(page, 'DELETE', `/api/integrations/${hc.json.id}`)
})

test('Einstellungen → Dashboard: Kacheln ein- und ausblenden; letzte Ereignisse', async ({ page }) => {
  await login(page)
  const hc = await api<{ id: number }>(page, 'POST', '/api/integrations', {
    type: 'healthchecks', name: 'Homelab', enabled: true, interval_seconds: 3600, timeout_seconds: 5, tls_verify: true,
    config: { base_url: `${MOCK}/healthchecks` }, secrets: { api_key: 'hc-readonly' }, clear_secrets: [],
  })
  await api(page, 'POST', `/api/integrations/${hc.json.id}/poll`)
  type D = { id: string; tiles: { id: string; visible: boolean }[] }
  const main = async () => (await api<{ dashboards: D[] }>(page, 'GET', '/api/dashboards')).json.dashboards.find((d) => d.id === 'main')!
  const before = await main()

  await page.goto('/')
  await expect(page.locator('[data-tile="events"]')).toContainText('Letzte Ereignisse')

  await page.goto('/einstellungen')
  const row = page.locator('[data-dashboard-setting="main"]')
  await row.locator('button[aria-expanded]').click()
  await row.getByRole('switch', { name: 'Checks anzeigen' }).click()
  await expect.poll(async () => (await main()).tiles.find((t) => t.id === 'checks')?.visible).toBe(false)

  await page.goto('/')
  await expect(page.locator('[data-tile="events"]')).toBeVisible()
  await expect(page.locator('[data-tile="checks"]')).toHaveCount(0)
  await expect(page.locator('[data-tile="backups"]')).toContainText('3 Backups überwacht')

  await api(page, 'PUT', '/api/dashboards/main', before)
  await api(page, 'DELETE', `/api/integrations/${hc.json.id}`)
})
