import { expect, test } from '@playwright/test'

import { api, expectNoHorizontalOverflow, login, MOCK } from './helpers'

// Switch (SNMP), Server-Management (Redfish/iLO) und Pi-hole gegen cmd/mock:
// SNMP-Agenten auf UDP 8794 (Cisco C1300) und 8795 (8-Port-Switch).

const base = { enabled: true, interval_seconds: 3600, timeout_seconds: 5, tls_verify: true, clear_secrets: [] }

test('Switch per SNMP: Formular, Verbindung testen, Front mit Ports', async ({ page }) => {
  await login(page)
  await page.goto('/einstellungen')
  const section = page.locator('[data-slot=card]').filter({ hasText: 'Integrationen' })
  await section.getByRole('button', { name: 'Hinzufügen' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: /^Switch \(SNMP\)/ }).click()
  // v3-Felder erst bei Version v3; TLS gibt es bei SNMP nicht
  await expect(dialog.getByLabel('Benutzer')).toHaveCount(0)
  await dialog.getByLabel('Name').fill('Core-Switch')
  await dialog.getByLabel('Host').fill('127.0.0.1')
  await dialog.getByLabel('Port (UDP)').fill('8794')
  await dialog.getByLabel('Community').fill('falsch')
  await dialog.getByRole('button', { name: 'Verbindung testen' }).click()
  // falsche Community → der Agent schweigt; Meldung erst nach dem Timeout (10 s / 3 je Versuch, 2 Versuche)
  await expect(dialog.getByRole('status')).toContainText('Keine Antwort per SNMP', { timeout: 15_000 })
  await dialog.getByLabel('Community').fill('labdeck-ro')
  await dialog.getByRole('button', { name: 'Verbindung testen' }).click()
  await expect(dialog.getByRole('status')).toContainText('Verbindung erfolgreich')
  await dialog.getByRole('button', { name: 'Erweitert' }).click()
  await expect(dialog.getByLabel('TLS-Zertifikat prüfen')).toHaveCount(0)
  await dialog.getByRole('button', { name: 'Speichern' }).click()
  await expect(dialog).toHaveCount(0)

  // zweiter Switch ohne ENTITY-MIB (Namen „Port 1…8“), per API
  const small = await api<{ id: number }>(page, 'POST', '/api/integrations', {
    ...base, type: 'snmp', name: 'Büro-Switch', config: { host: '127.0.0.1', port: 8795, monitor: 'list', monitor_ports: '1-3, 7' },
    secrets: { community: 'labdeck-ro' },
  })
  await api(page, 'POST', `/api/integrations/${small.json.id}/poll`)

  await page.goto('/ressourcen')
  const core = page.locator('[data-switch="core-switch"]')
  await expect(core).toContainText('C1300-24P-4G')
  await expect(core.locator('[data-port]')).toHaveCount(28)
  await expect(core.locator('[data-media="sfp"] [data-port]')).toHaveCount(4)
  await expect(core.locator('[data-port="gi1/0/12"]')).toHaveAttribute('data-tone', 'error') // Drucker, überwacht
  await expect(core.locator('[data-port="gi1/0/23"]')).toHaveAttribute('data-tone', 'disabled')
  await expect(core).toContainText('14 / 28')
  // Port anklicken: Details
  await core.locator('[data-port="gi1/0/1"]').click()
  await expect(core.locator('[data-port-detail="gi1/0/1"]')).toContainText('pve-01')

  const office = page.locator('[data-switch="office-switch"]')
  await expect(office.locator('[data-port]')).toHaveCount(8)
  await expect(office.locator('[data-port="Port 7"]')).toHaveAttribute('data-tone', 'error')

  // Problem auf dem Dashboard
  await page.goto('/')
  await expect(page.locator('[data-tile="Core-Switch"]')).toContainText('gi1/0/12')
  await expect(page.locator('[data-tile="Core-Switch"]')).toHaveAttribute('data-tone', 'error')

  for (const i of (await api<{ id: number; type: string }[]>(page, 'GET', '/api/integrations')).json) {
    if (i.type === 'snmp') await api(page, 'DELETE', `/api/integrations/${i.id}`)
  }
})

test('Server per Redfish (iLO 5): Hardware-Karte, Netzteil fällt aus', async ({ page }) => {
  await login(page)
  const ilo = await api<{ id: number }>(page, 'POST', '/api/integrations', {
    ...base, type: 'redfish', name: 'iLO pve-01', config: { base_url: `${MOCK}/ilo`, username: 'labdeck' },
    secrets: { password: 'ilo-pass' },
  })
  await api(page, 'POST', `/api/integrations/${ilo.json.id}/poll`)
  await page.goto('/ressourcen')
  const card = page.locator('[data-server="pve-01"]')
  await expect(card).toContainText('ProLiant DL380 Gen10')
  await expect(card).toContainText('212 W')
  await expect(card.locator('[data-component="power_supplies"]')).toHaveAttribute('data-status', 'ok')
  await expect(card.locator('[data-sensor="01-Inlet Ambient"]')).toContainText('23 °C')
  await expect(card.locator('[data-item^="1I:1:1"]')).toContainText('93 % Rest')

  await fetch(`${MOCK}/_ilo/psu?n=1&health=Critical`, { method: 'POST' })
  await api(page, 'POST', `/api/integrations/${ilo.json.id}/poll`)
  await expect(card.locator('[data-component="power_supplies"]')).toHaveAttribute('data-status', 'error')
  await expect(card.locator('[data-item="PSU 2"]')).toContainText('Fehler')
  await page.goto('/')
  await expect(page.locator('[data-tile="iLO pve-01"]')).toHaveAttribute('data-tone', 'error')
  await expect(page.locator('[data-tile="iLO pve-01"]')).toContainText('Netzteil-Redundanz')

  await fetch(`${MOCK}/_ilo/psu?n=1&health=OK`, { method: 'POST' })
  await api(page, 'DELETE', `/api/integrations/${ilo.json.id}`)
})

test('Pi-hole: Kennzahlen, Blocken aus als Warnung, mobil', async ({ page }) => {
  await login(page)
  const ph = await api<{ id: number }>(page, 'POST', '/api/integrations', {
    ...base, type: 'pihole', name: 'Pi-hole', config: { base_url: `${MOCK}/pihole` }, secrets: { password: 'pihole-app' },
  })
  await api(page, 'POST', `/api/integrations/${ph.json.id}/poll`)
  await page.goto('/dienste')
  const card = page.locator('[data-service="Pi-hole"]')
  await expect(card).toContainText('15,3 %')
  await expect(card).toContainText('48.213')

  await fetch(`${MOCK}/_pihole/blocking?value=disabled`, { method: 'POST' })
  await api(page, 'POST', `/api/integrations/${ph.json.id}/poll`)
  await page.goto('/')
  const tile = page.locator('[data-tile="Pi-hole"]')
  await expect(tile).toHaveAttribute('data-tone', 'warning')
  await expect(tile).toContainText('Aus')

  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/ressourcen')
  await expectNoHorizontalOverflow(page)

  await fetch(`${MOCK}/_pihole/blocking?value=enabled`, { method: 'POST' })
  await api(page, 'DELETE', `/api/integrations/${ph.json.id}`)
})
