import { expect, test, type Page } from '@playwright/test'

import { api, expectNoHorizontalOverflow, login, MOCK } from './helpers'

// Dashboard v2: mehrere Dashboards, freies Raster, Kennzahl-Kacheln, Live-Werte.

type Tile = { id: string; visible: boolean; x: number; y: number; w: number; h: number; options?: { keys: string[]; view: string } }
type D = { id: string; name: string; icon?: string; tiles: Tile[] }
const dashboards = async (page: Page) => (await api<{ dashboards: D[] }>(page, 'GET', '/api/dashboards')).json.dashboards

async function cleanup(page: Page) {
  for (const d of await dashboards(page)) if (d.id !== 'main') await api(page, 'DELETE', `/api/dashboards/${d.id}`)
  for (const i of (await api<{ id: number }[]>(page, 'GET', '/api/integrations')).json) await api(page, 'DELETE', `/api/integrations/${i.id}`)
}

test.afterEach(async ({ page }) => cleanup(page))

/** Pi-hole anlegen und abfragen (ohne Integration zeigt das Dashboard keine Kacheln). */
async function addPihole(page: Page) {
  const ph = await api<{ id: number }>(page, 'POST', '/api/integrations', {
    type: 'pihole', name: 'Pi-hole', enabled: true, interval_seconds: 3600, timeout_seconds: 5, tls_verify: true,
    config: { base_url: `${MOCK}/pihole` }, secrets: { password: 'pihole-app' }, clear_secrets: [],
  })
  await api(page, 'POST', `/api/integrations/${ph.json.id}/poll`)
  return ph.json.id
}

test('Neues Dashboard mit Integrations- und Kennzahl-Kachel, Reiter, Löschen', async ({ page }) => {
  await login(page)
  const ph = await api<{ id: number }>(page, 'POST', '/api/integrations', {
    type: 'pihole', name: 'Pi-hole', enabled: true, interval_seconds: 3600, timeout_seconds: 5, tls_verify: true,
    config: { base_url: `${MOCK}/pihole` }, secrets: { password: 'pihole-app' }, clear_secrets: [],
  })
  await api(page, 'POST', `/api/integrations/${ph.json.id}/poll`)
  await page.goto('/')
  // ein Dashboard: keine Reiter
  await expect(page.locator('[data-dashboard-tab]')).toHaveCount(0)

  await page.getByRole('button', { name: 'Bearbeiten' }).click()
  await page.getByRole('button', { name: 'Neues Dashboard' }).click()
  await expect(page).toHaveURL(/\/d\/d-/)
  await page.getByLabel('Name').fill('DNS')
  await page.getByRole('radio', { name: 'shield-check' }).click()

  await page.getByRole('button', { name: 'Kachel hinzufügen' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.locator(`[data-add-tile="integration:${ph.json.id}"]`).click()
  await expect(dialog).toHaveCount(0)

  await page.getByRole('button', { name: 'Kachel hinzufügen' }).click()
  await dialog.getByRole('button', { name: 'Eigene Kennzahl' }).click()
  await dialog.getByRole('combobox').click()
  await page.getByRole('option', { name: 'Pi-hole' }).click()
  await dialog.getByPlaceholder(/suchen/).fill('Clients')
  await dialog.locator('[data-metric="clients"]').click()
  await dialog.getByRole('button', { name: 'Tacho' }).click()
  await dialog.getByLabel('Skala bis').fill('50')
  await dialog.getByLabel('Überschrift (optional)').fill('Geräte im Netz')
  await expect(dialog.locator('[data-chart="gauge"]')).toBeVisible() // Vorschau
  await dialog.getByRole('button', { name: 'Kennzahl hinzufügen' }).click()
  await page.getByRole('button', { name: 'Fertig' }).click()
  await expect(page.locator('[data-edit-bar]')).toHaveCount(0)

  // Reiter, Kacheln, gespeichert
  await expect(page.locator('[data-dashboard-tab]')).toHaveCount(2)
  await expect(page.getByRole('heading', { name: 'DNS' })).toBeVisible()
  await expect(page.locator('[data-tile="Pi-hole"]')).toContainText('15,3 %')
  const gauge = page.locator('[data-tile^="metric:"]')
  await expect(gauge).toContainText('Geräte im Netz')
  await expect(gauge).toContainText('23')
  const dns = (await dashboards(page)).find((d) => d.name === 'DNS')!
  expect(dns.icon).toBe('shield-check')
  expect(dns.tiles.map((t) => t.id)).toEqual([`integration:${ph.json.id}`, expect.stringMatching(/^metric:/)])
  // neue Kacheln bleiben nebeneinander (höchste freie Stelle)
  expect(dns.tiles[1]).toMatchObject({ x: 4, y: 0 })
  // das Haupt-Dashboard ist unverändert
  await page.locator('[data-dashboard-tab="main"]').click()
  await expect(page).toHaveURL('/')
  await expect(page.locator('[data-tile^="metric:"]')).toHaveCount(0)

  // Kennzahl-Kachel einstellen und entfernen, Dashboard löschen
  await page.locator(`[data-dashboard-tab="${dns.id}"]`).click()
  await page.getByRole('button', { name: 'Bearbeiten' }).click()
  await page.getByRole('button', { name: 'Geräte im Netz einstellen' }).click()
  await dialog.getByRole('button', { name: 'Zahl', exact: true }).click()
  await dialog.getByRole('button', { name: 'Speichern' }).click()
  await page.getByRole('button', { name: 'Fertig' }).click()
  await expect.poll(async () => (await dashboards(page)).find((d) => d.id === dns.id)!.tiles[1].options!.view).toBe('number')

  await page.getByRole('button', { name: 'Bearbeiten' }).click()
  await page.getByRole('button', { name: 'Dashboard löschen' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Löschen' }).click()
  await expect(page).toHaveURL('/')
  await expect.poll(async () => (await dashboards(page)).length).toBe(1)
})

test('Kacheln ziehen und in der Größe ändern; Abbrechen verwirft', async ({ page }) => {
  await login(page)
  await addPihole(page)
  await page.goto('/')
  const before = (await dashboards(page))[0].tiles.find((t) => t.id === 'backups')!
  expect(before).toMatchObject({ x: 0, y: 0, w: 4 })

  await page.getByRole('button', { name: 'Bearbeiten' }).click()
  const backups = page.locator('[data-grid-tile="backups"]')
  const box = (await backups.boundingBox())!
  // an die rechte Kante ziehen
  await page.mouse.move(box.x + box.width / 2, box.y + 60)
  await page.mouse.down()
  await page.mouse.move(box.x + box.width / 2 + 300, box.y + 60, { steps: 10 })
  await page.mouse.move(box.x + box.width / 2 + 660, box.y + 60, { steps: 10 })
  await page.mouse.up()
  // Abbrechen: nichts gespeichert
  await page.getByRole('button', { name: 'Abbrechen' }).click()
  expect((await dashboards(page))[0].tiles.find((t) => t.id === 'backups')).toMatchObject({ x: 0, y: 0 })

  await page.getByRole('button', { name: 'Bearbeiten' }).click()
  const b2 = (await backups.boundingBox())!
  await page.mouse.move(b2.x + b2.width / 2, b2.y + 60)
  await page.mouse.down()
  await page.mouse.move(b2.x + b2.width / 2 + 300, b2.y + 60, { steps: 10 })
  await page.mouse.move(b2.x + b2.width / 2 + 660, b2.y + 60, { steps: 10 })
  await page.mouse.up()
  // Größe: Griff unten rechts nach rechts ziehen
  const b3 = (await backups.boundingBox())!
  const handle = backups.locator('.react-resizable-handle-se')
  const hb = (await handle.boundingBox())!
  await page.mouse.move(hb.x + hb.width / 2, hb.y + hb.height / 2)
  await page.mouse.down()
  await page.mouse.move(hb.x - b3.width / 2, hb.y + hb.height / 2 + 120, { steps: 10 })
  await page.mouse.up()
  await page.getByRole('button', { name: 'Fertig' }).click()
  await expect
    .poll(async () => (await dashboards(page))[0].tiles.find((t) => t.id === 'backups'))
    .toMatchObject({ x: expect.any(Number), h: expect.any(Number) })
  const after = (await dashboards(page))[0].tiles.find((t) => t.id === 'backups')!
  expect(after.x).toBeGreaterThan(0)
  expect(after.h).toBeGreaterThan(before.h)
  expect(after.w).toBeLessThan(before.w)
})

test('Live-Kachel: Wert ändert sich bei 5-s-Abfrage', async ({ page }) => {
  await login(page)
  const sw = await api<{ id: number }>(page, 'POST', '/api/integrations', {
    type: 'snmp', name: 'Core-Switch', enabled: true, interval_seconds: 5, timeout_seconds: 2, tls_verify: true,
    config: { host: '127.0.0.1', port: 8794 }, secrets: { community: 'labdeck-ro' }, clear_secrets: [],
  })
  expect(sw.status).toBe(201)
  // zweite Abfrage abwarten (erste liefert noch keine Raten)
  await expect
    .poll(async () => (await api<{ snapshot?: { metrics: { key: string }[] } }>(page, 'GET', `/api/integrations/${sw.json.id}`)).json.snapshot?.metrics.some((m) => m.key === 'traffic_in'), { timeout: 20_000 })
    .toBe(true)
  const main = (await dashboards(page))[0]
  await api(page, 'PUT', '/api/dashboards/main', {
    ...main,
    tiles: [...main.tiles, { id: 'metric:live', visible: true, x: 0, y: 0, w: 4, h: 3,
      options: { integration_id: sw.json.id, keys: ['traffic_in'], view: 'number', range: '1h' } }],
  })
  await page.goto('/')
  const tile = page.locator('[data-tile="metric:live"]')
  await expect(tile).toContainText('Live')
  await expect(tile).toContainText('bit/s')
  const first = await tile.locator('.tabular-nums').first().textContent()
  await expect.poll(async () => tile.locator('.tabular-nums').first().textContent(), { timeout: 20_000 }).not.toBe(first)
  // Verlauf der letzten Stunde aus dem Speicher: mehr Punkte als Minuten
  const h = await api<{ points: unknown[] }>(page, 'GET', `/api/integrations/${sw.json.id}/history?key=traffic_in&range=1h`)
  expect(h.json.points.length).toBeGreaterThanOrEqual(2)
})

test('Handy: Kacheln untereinander, ohne Querscrollen', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await login(page)
  await addPihole(page)
  await page.goto('/')
  await expect(page.locator('[data-grid="stacked"]')).toBeVisible()
  await expectNoHorizontalOverflow(page)
  await page.getByRole('button', { name: 'Bearbeiten' }).click()
  // ohne Griff (kein Ziehen), aber Entfernen geht
  await expect(page.locator('[data-edit-overlay]').first()).toBeVisible()
  await expectNoHorizontalOverflow(page)
})
