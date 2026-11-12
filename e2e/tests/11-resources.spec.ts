import { expect, test } from '@playwright/test'

import { api, expectNoHorizontalOverflow, login, MOCK, openSystem, problemList } from './helpers'

interface Integration {
  id: number
  name: string
}

const create = (page: Parameters<typeof api>[0], body: Record<string, unknown>) =>
  api<Integration>(page, 'POST', '/api/integrations', {
    enabled: true, interval_seconds: 3600, timeout_seconds: 5, tls_verify: true, secrets: {}, clear_secrets: [], ...body,
  })

test('Proxmox VE und Dockhand: Systeme, Detailseite mit Hosts, VMs, Containern, Suche', async ({ page }) => {
  await login(page)

  // PVE per Formular
  await page.goto('/einstellungen')
  const section = page.locator('[data-slot=card]').filter({ hasText: 'Integrationen' })
  await section.getByRole('button', { name: 'Hinzufügen' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: /^Proxmox VE/ }).click()
  await expect(dialog.getByText(/PVEAuditor/)).toBeVisible()
  await dialog.locator('#integration-name').fill('PVE')
  await dialog.getByLabel('Adresse').fill(`${MOCK}/pve`)
  await dialog.getByLabel('Token-ID').fill('labdeck@pve!dashboard')
  await dialog.getByLabel('Token-Secret').fill('pve-secret')
  await dialog.getByRole('button', { name: 'Verbindung testen' }).click()
  await expect(dialog.getByRole('status')).toContainText('Verbindung erfolgreich')
  await dialog.getByRole('button', { name: 'Speichern' }).click()
  await expect(dialog).toBeHidden()

  const dockhand = await create(page, {
    type: 'dockhand', name: 'Dockhand', config: { base_url: `${MOCK}/dockhand` }, secrets: { api_token: 'dh_mock' },
  })
  await api(page, 'POST', `/api/integrations/${dockhand.json.id}/poll`)

  // Systeme: beide unter „Server und Virtualisierung“
  await page.goto('/systeme')
  const compute = page.locator('[data-category="compute"]')
  await expect(compute.locator('[data-system="PVE"]')).toContainText('2 Nodes')
  await expect(compute.locator('[data-system="Dockhand"]')).toBeVisible()

  // Suche über alle Systeme findet Container und VMs und springt zur Zeile
  await page.getByLabel('Systeme, Hosts, VMs, Container oder Ports suchen').fill('vaultwarden')
  const hit = page.locator('[data-hit="vaultwarden"]')
  await expect(hit).toContainText('Dockhand')
  await hit.click()
  await expect(page).toHaveURL(/\/systeme\/\d+#res-container_/)
  await expect(page.locator('tr[data-resource="vaultwarden"]')).toContainText('Unhealthy')
  await expect(page.locator('tr[data-resource="vaultwarden"]')).toBeInViewport()

  // Detailseite PVE: Hosts mit Gästen
  await openSystem(page, 'PVE')
  const pve1 = page.locator('[data-host="pve1"]')
  await expect(pve1).toContainText('5 von 6 laufen')
  await expect(pve1.getByRole('meter', { name: 'CPU' })).toHaveAttribute('aria-valuenow', '18')
  await expect(page.locator('tr[data-resource="nextcloud"]')).toContainText('VM 102')
  await expect(page.locator('tr[data-resource="win11"]')).toContainText('Gestoppt')
  // Probleme dieser Integration oben auf der Seite
  await expect(problemList(page).getByText('SMART', { exact: true })).toBeVisible()
  await expect(problemList(page).getByText('ZFS-Zustand')).toBeVisible()

  // Suche, Filter, Sortierung
  await page.getByLabel('Name, ID, Tag oder Image suchen').fill('media')
  await expect(page.locator('tr[data-resource]')).toHaveCount(1)
  await page.getByLabel('Name, ID, Tag oder Image suchen').fill('')
  await page.getByRole('group', { name: 'Zustand' }).getByRole('button', { name: 'Gestoppt' }).click()
  await expect(page.locator('tr[data-resource]')).toHaveCount(1) // win11
  await page.getByRole('group', { name: 'Zustand' }).getByRole('button', { name: 'Alle' }).click()
  await page.getByRole('group', { name: 'Sortieren' }).getByRole('button', { name: 'CPU' }).click()
  await expect(page.locator('[data-host="pve1"] + div tr[data-resource]').first()).toHaveAttribute('data-resource', 'immich')

  // Verlauf einer VM aufklappen
  await page.locator('tr[data-resource="immich"]').click()
  await expect(page.getByRole('group', { name: 'Kennzahl' })).toBeVisible()

  // Dashboard: Kachel mit Anzahl, Probleme in der Liste (SMART, ZFS, unhealthy Container)
  await page.goto('/')
  await expect(page.locator('[data-tile="PVE"]')).toContainText('2 Nodes')
  const pveTile = page.locator('[data-tile="PVE"]')
  await expect(pveTile).toHaveAttribute('data-tone', 'error')
  await expect(pveTile.locator('[data-problem-count]')).toBeVisible()
  const unhealthy = problemList(page).getByRole('link', { name: /Container unhealthy/ })
  await expect(unhealthy).toContainText('Dockhand › vaultwarden')
  // Problem mit Ressource führt zur Zeile des Containers
  await unhealthy.click()
  await expect(page).toHaveURL(/\/systeme\/\d+#res-container_/)
  await expect(page.locator('tr[data-resource="vaultwarden"]')).toBeInViewport()
  await page.goto('/')
  // Hosts aller Quellen in einer Kachel
  const hosts = page.locator('[data-tile="hosts"]')
  await expect(hosts).toContainText('4 von 4 online')
  await expect(hosts.locator('[data-host="pve1"]').getByRole('meter', { name: 'CPU' })).toHaveAttribute('aria-valuenow', '18')
  await expect(hosts.locator('[data-host="docker01"]')).toBeVisible()
  await hosts.click()
  await expect(page).toHaveURL(/\/systeme$/)

  await page.setViewportSize({ width: 390, height: 844 })
  await openSystem(page, 'PVE')
  await expect(page.locator('li[data-resource="homeassistant"]')).toBeVisible()
  await expectNoHorizontalOverflow(page)
  await page.goto('/systeme')
  await expectNoHorizontalOverflow(page)

  const list = await api<Integration[]>(page, 'GET', '/api/integrations')
  for (const i of list.json) await api(page, 'DELETE', `/api/integrations/${i.id}`)
})

test('USV über NUT: Netzbetrieb, Stromausfall als Problem', async ({ page }) => {
  await login(page)
  const ups = await create(page, { type: 'nut', name: 'USV', config: { host: 'localhost', port: 8793 } })
  expect(ups.status).toBe(201)
  await api(page, 'POST', `/api/integrations/${ups.json.id}/poll`)

  await page.goto('/')
  const tile = page.locator('[data-tile="USV"]')
  await expect(tile).toContainText('Netzbetrieb')
  await expect(tile).toContainText('97 % Ladung')

  // Stromausfall
  await fetch(`${MOCK}/_nut/status?value=OB+DISCHRG`, { method: 'POST' })
  await api(page, 'POST', `/api/integrations/${ups.json.id}/poll`)
  await expect(tile).toContainText('Batterie')
  await expect(problemList(page).getByText('Versorgung')).toBeVisible()
  await expect(tile).toHaveAttribute('data-tone', 'warning')

  await fetch(`${MOCK}/_nut/status?value=OL`, { method: 'POST' })
  await api(page, 'DELETE', `/api/integrations/${ups.json.id}`)
})
