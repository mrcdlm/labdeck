import { expect, test } from '@playwright/test'

import { api, expectNoHorizontalOverflow, login, mock, MOCK } from './helpers'

interface Integration {
  id: number
}

test('Backups, Checks und Gesamtstatus aus Healthchecks.io', async ({ page }) => {
  await login(page)
  const created = await api<Integration>(page, 'POST', '/api/integrations', {
    type: 'healthchecks', name: 'Homelab', enabled: true, interval_seconds: 3600, timeout_seconds: 5,
    tls_verify: true, config: { base_url: `${MOCK}/healthchecks` }, secrets: { api_key: 'hc-readonly' },
    clear_secrets: [],
  })
  expect(created.status).toBe(201)
  const id = created.json.id
  await api(page, 'POST', `/api/integrations/${id}/poll`)

  // Dashboard: Probleme ganz oben, Kacheln darunter
  await page.goto('/')
  await expect(page.getByText('2 Probleme')).toBeVisible()
  const problems = page.getByRole('list', { name: 'Probleme' })
  await expect(problems.locator('[data-problem="Proxmox vzdump"]')).toContainText('Überfällig')
  await expect(problems.locator('[data-problem="Borg Laptop"]')).toContainText('Bald fällig')
  await expect(page.getByRole('link', { name: /Backups 3 überwacht/ })).toBeVisible()
  // Healthchecks als Ampel: vzdump ist down (zählt hier mit, steht aber als Backup in der Liste)
  await expect(page.locator('[data-tile="checks"]')).toContainText('1 Check ausgefallen')

  // Backups: Probleme zuerst, Verlauf als Balken
  await page.getByRole('link', { name: /Backups 3 überwacht/ }).click()
  await expect(page).toHaveURL('/backups')
  const rows = page.locator('tr[data-backup]')
  await expect(rows).toHaveCount(3)
  await expect(rows.nth(0)).toHaveAttribute('data-backup', 'Proxmox vzdump')
  await expect(rows.nth(0)).toContainText('Überfällig')
  await expect(rows.nth(2)).toHaveAttribute('data-backup', 'Restic NAS → B2')
  await expect(rows.nth(2).getByRole('img', { name: /Uptime 7 Tage: \d/ })).toBeVisible()
  await page.getByRole('button', { name: '30 Tage' }).click()
  await expect(page.getByRole('columnheader', { name: 'Verlauf 30 Tage' })).toBeVisible()

  // Höchstalter je Backup: 72 h → vzdump ist „nur“ fehlgeschlagen (down, aber jünger)
  await rows.nth(0).getByRole('button', { name: 'Höchstalter von Proxmox vzdump ändern' }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByLabel('Höchstalter in Stunden')).toHaveValue('26')
  await dialog.getByLabel('Höchstalter in Stunden').fill('72')
  await expect(dialog.getByText('Höchstalter 3 Tage – Warnung ab 57,6 Stunden.')).toBeVisible()
  await dialog.getByRole('button', { name: 'Speichern' }).click()
  await expect(dialog).toBeHidden()
  const vz = page.locator('tr[data-backup="Proxmox vzdump"]')
  await expect(vz).toContainText('Fehlgeschlagen')
  await expect(vz).toContainText('3 Tage')
  await vz.getByRole('button', { name: 'Höchstalter von Proxmox vzdump ändern' }).click()
  await dialog.getByRole('button', { name: 'Standard verwenden (26 Stunden)' }).click()
  await expect(dialog).toBeHidden()
  await expect(vz).toContainText('Überfällig')

  // Checks: nach Projekt gruppiert, Tag-Filter, nach Tag gruppiert
  await page.goto('/checks')
  await expect(page.getByRole('heading', { name: 'Homelab' })).toBeVisible()
  const checks = page.locator('tr[data-check]')
  await expect(checks).toHaveCount(7)
  await expect(checks.nth(0)).toHaveAttribute('data-check', 'Proxmox vzdump')
  await expect(checks.nth(0)).toContainText('Down')
  await expect(page.locator('tr[data-check="Borg Laptop"]')).toContainText('Karenzzeit')
  await expect(page.locator('tr[data-check="DynDNS"]').getByRole('img', { name: /Uptime 24 h: 9\d/ })).toBeVisible()
  await page.getByRole('group', { name: 'Nach Tags filtern' }).getByRole('button', { name: 'cron' }).click()
  await expect(checks).toHaveCount(2)
  await page.getByRole('button', { name: 'Tag', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'dns' })).toBeVisible()
  await page.getByRole('button', { name: 'Filter zurücksetzen' }).click()
  await expect(page.getByRole('heading', { name: 'Ohne Tag' })).toBeVisible()

  // Ein Check ohne Backup-Tag fällt aus → Problem auf dem Dashboard (live)
  await page.goto('/')
  await mock.hcStatus('DynDNS', 'down')
  await api(page, 'POST', `/api/integrations/${id}/poll`)
  await expect(page.getByText('3 Probleme')).toBeVisible()
  // ausgefallene Checks je Projekt in einer Zeile
  const hc = problems.locator('[data-problem="Homelab"]')
  await expect(hc).toContainText('1 Check down')
  await expect(hc).toContainText('DynDNS')
  await expect(page.locator('[data-tile="checks"]')).toContainText('2 Checks ausgefallen')
  await mock.hcStatus('DynDNS', 'up')
  await api(page, 'POST', `/api/integrations/${id}/poll`)
  await expect(page.getByText('2 Probleme')).toBeVisible()

  // Standard-Höchstalter in den Einstellungen
  await page.goto('/einstellungen')
  await expect(page.getByLabel('Standard-Höchstalter')).toHaveValue('26')
  await expect(page.getByText('backup', { exact: true })).toBeVisible() // Backup-Tag der Integration
  await page.getByLabel('Standard-Höchstalter').fill('100')
  await page.getByRole('button', { name: 'Speichern' }).click()
  await expect(page.getByText('Gespeichert', { exact: true })).toBeVisible()
  await page.goto('/backups')
  await expect(vz).toContainText('Fehlgeschlagen')
  await expect(vz).toContainText('100 Stunden')
  const settings = (await api<Record<string, unknown>>(page, 'GET', '/api/settings')).json
  await api(page, 'PUT', '/api/settings', { ...settings, backup_max_age_hours: 26 })

  // Mobil: Karten statt Tabelle, kein horizontales Scrollen
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/backups')
  await expect(page.locator('li[data-backup="Proxmox vzdump"]')).toContainText('Überfällig')
  await expectNoHorizontalOverflow(page)
  await page.goto('/checks')
  await expect(page.locator('li[data-check="DynDNS"]')).toBeVisible()
  await expectNoHorizontalOverflow(page)

  await api(page, 'DELETE', `/api/integrations/${id}`)
})
