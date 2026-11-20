import { expect, test } from '@playwright/test'

import { api, expectNoHorizontalOverflow, login, MOCK } from './helpers'

const create = (page: Parameters<typeof api>[0], body: Record<string, unknown>) =>
  api<{ id: number }>(page, 'POST', '/api/integrations', {
    enabled: true, interval_seconds: 3600, timeout_seconds: 5, tls_verify: true, secrets: {}, clear_secrets: [], ...body,
  })

test('Netzplan: Geräte und Kabel aus SNMP/LLDP, Proxmox, Arcane; Pflege im Editor; Schwärzen', async ({ page }) => {
  await login(page)
  const sw = await create(page, { type: 'snmp', name: 'Core-Switch', config: { host: '127.0.0.1', port: 8794 }, secrets: { community: 'labdeck-ro' } })
  const pve = await create(page, {
    type: 'pve', name: 'Proxmox VE', config: { base_url: `${MOCK}/pve`, token_id: 'labdeck@pve!dashboard', backups: false },
    secrets: { token_secret: 'pve-secret' },
  })
  const arc = await create(page, { type: 'arcane', name: 'Arcane', config: { base_url: `${MOCK}/arcane` }, secrets: { api_key: 'arc_mock' } })
  for (const i of [sw, pve, arc]) await api(page, 'POST', `/api/integrations/${i.json.id}/poll`)

  await page.goto('/netz')
  await expect(page.getByRole('heading', { name: 'Netzplan', level: 1 })).toBeVisible()
  // Switch mit Ports, Hosts aus Proxmox und Arcane
  const swCard = page.locator('[data-device="core-switch"]')
  await expect(swCard.locator('[data-port-strip] [data-port]')).toHaveCount(28)
  await expect(page.locator('[data-device="pve1"]')).toContainText('homeassistant')
  await expect(page.locator('[data-device="docker01"]')).toContainText('jellyfin')
  // LLDP-Kabel in der Tabelle (und als Linie)
  const cable = page.locator('[data-cable]').filter({ hasText: 'pve1' })
  await expect(cable).toContainText('gi1/0/1')
  await expect(cable).toContainText('enp3s0')
  await expect(cable).toContainText('10 + 20, 40')
  await expect.poll(async () => Number(await page.locator('[data-links]').getAttribute('data-links'))).toBeGreaterThanOrEqual(4)
  // Netze vom Switch (Q-BRIDGE)
  await expect(page.locator('[data-vlan="20"]')).toContainText('DMZ')

  // Editor: Firewall anlegen (LLDP-Nachbar „opnsense“ passt per Name), AP aus Vorschlag anlegen
  await page.locator('[data-action="edit-topology"]').click()
  const dialog = page.getByRole('dialog')
  await dialog.locator('[data-action="add-device"]').click()
  await dialog.getByRole('textbox').first().fill('OPNsense')
  await dialog.getByRole('combobox', { name: 'Typ' }).first().click()
  await page.getByRole('option', { name: 'Firewall' }).click()
  await dialog.getByRole('button', { name: /^Zuordnung/ }).click()
  await expect(dialog).toContainText('ap-living')
  await dialog.getByRole('button', { name: 'Als Gerät anlegen' }).first().click()
  await dialog.locator('[data-action="save-topology"]').click()
  await expect(dialog).toBeHidden()

  const fw = page.locator('[data-device="OPNsense"]')
  await expect(fw).toContainText('Firewall')
  await expect(page.locator('[data-device="ap-living"]')).toBeVisible()
  // Uplink per LLDP zur Firewall, Kabel zum AP
  await expect(page.locator('[data-cable]').filter({ hasText: 'OPNsense' })).toContainText('igc1')
  await expect(page.locator('[data-cable]').filter({ hasText: 'ap-living' })).toContainText('gi1/0/8')
  const topo = await api<{ suggestions: unknown[]; config: { devices: { name: string }[] } }>(page, 'GET', '/api/topology')
  expect(topo.json.config.devices.map((d) => d.name).sort()).toEqual(['OPNsense', 'ap-living'])

  // Schwärzen: Subnetze und IPs unkenntlich
  const cfg = (await api<{ config: Record<string, unknown> }>(page, 'GET', '/api/topology')).json.config
  await api(page, 'PUT', '/api/topology', { ...cfg, vlans: [{ id: 10, subnet: '10.0.10.0/24' }] })
  await page.reload()
  const lan = page.locator('[data-vlan="10"]')
  await expect(lan).toContainText('10.0.10.0/24')
  await page.getByRole('switch', { name: 'Schwärzen' }).click()
  await expect(lan).not.toContainText('10.0.10.0')
  await expect(lan).toContainText('•••')

  // Handy: kein Querscrollen, Kabel als Tabelle
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.locator('[data-cable]').first()).toBeVisible()
  await expectNoHorizontalOverflow(page)

  await api(page, 'PUT', '/api/topology', { devices: [], links: [], vlans: [], aliases: {}, hidden: [] })
  for (const i of [sw, pve, arc]) await api(page, 'DELETE', `/api/integrations/${i.json.id}`)
})
