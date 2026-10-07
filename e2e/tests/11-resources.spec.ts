import { expect, test } from '@playwright/test'

import { api, expectNoHorizontalOverflow, login, MOCK } from './helpers'

interface Integration {
  id: number
  name: string
}

const create = (page: Parameters<typeof api>[0], body: Record<string, unknown>) =>
  api<Integration>(page, 'POST', '/api/integrations', {
    enabled: true, interval_seconds: 3600, timeout_seconds: 5, tls_verify: true, secrets: {}, clear_secrets: [], ...body,
  })

test('Proxmox VE und Glances: Ressourcen-Seite mit Hosts, VMs, Containern', async ({ page }) => {
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

  const glances = await create(page, { type: 'glances', name: 'Docker-Host', config: { base_url: `${MOCK}/glances` } })
  await api(page, 'POST', `/api/integrations/${glances.json.id}/poll`)

  await page.goto('/ressourcen')
  const pve1 = page.locator('[data-host="pve1"]')
  await expect(pve1).toContainText('5 von 6 laufen')
  await expect(pve1.getByRole('meter', { name: 'CPU' })).toHaveAttribute('aria-valuenow', '18')
  await expect(page.locator('tr[data-resource="nextcloud"]')).toContainText('VM 102')
  await expect(page.locator('tr[data-resource="win11"]')).toContainText('Gestoppt')
  await expect(page.locator('tr[data-resource="vaultwarden"]')).toContainText('Unhealthy')

  // Suche, Filter, Sortierung
  await page.getByLabel('Name, ID, Tag oder Image suchen').fill('media')
  await expect(page.locator('tr[data-resource]')).toHaveCount(1)
  await page.getByLabel('Name, ID, Tag oder Image suchen').fill('')
  await page.getByRole('group', { name: 'Zustand' }).getByRole('button', { name: 'Gestoppt' }).click()
  await expect(page.locator('tr[data-resource]')).toHaveCount(2) // win11, old-wiki
  await page.getByRole('group', { name: 'Zustand' }).getByRole('button', { name: 'Alle' }).click()
  await page.getByRole('group', { name: 'Sortieren' }).getByRole('button', { name: 'CPU' }).click()
  await expect(page.locator('[data-host="pve1"] + div tr[data-resource]').first()).toHaveAttribute('data-resource', 'immich')

  // Verlauf einer VM aufklappen
  await page.locator('tr[data-resource="immich"]').click()
  await expect(page.getByRole('group', { name: 'Kennzahl' })).toBeVisible()

  // Dashboard: Kachel und Probleme (SMART, ZFS, unhealthy Container, Dateisystem voll)
  await page.goto('/')
  await expect(page.locator('[data-tile="PVE"]')).toContainText('2 Nodes')
  const problems = page.getByRole('list', { name: 'Probleme' })
  await expect(problems.getByText('SMART', { exact: true })).toBeVisible()
  await expect(problems.getByText('ZFS-Zustand')).toBeVisible()
  await expect(problems.getByText('Container unhealthy')).toBeVisible()

  // Dienste: PVE-Karte verweist auf die Ressourcen
  await page.goto('/dienste')
  await expect(page.locator('[data-service="PVE"]').getByRole('link', { name: /Ressourcen ansehen/ })).toBeVisible()

  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/ressourcen')
  await expect(page.locator('li[data-resource="homeassistant"]')).toBeVisible()
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
  await expect(page.getByRole('list', { name: 'Probleme' }).getByText('Versorgung')).toBeVisible()

  await fetch(`${MOCK}/_nut/status?value=OL`, { method: 'POST' })
  await api(page, 'DELETE', `/api/integrations/${ups.json.id}`)
})
