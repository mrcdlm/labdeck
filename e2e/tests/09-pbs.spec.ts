import { expect, test } from '@playwright/test'

import { api, login, MOCK } from './helpers'

interface Integration {
  id: number
  name: string
}

test('Proxmox Backup Server: Backups je VM/CT/Host, Verify, Datastore-Belegung', async ({ page }) => {
  await login(page)
  await page.goto('/einstellungen')
  const section = page.locator('[data-slot=card]').filter({ hasText: 'Integrationen' })
  await section.getByRole('button', { name: 'Hinzufügen' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: /^Proxmox Backup Server/ }).click()
  await expect(dialog.getByLabel('Adresse')).toHaveValue('https://pbs.lan:8007')
  await expect(dialog.getByText(/DatastoreAudit/)).toBeVisible()

  await dialog.locator('#integration-name').fill('PBS')
  await dialog.getByLabel('Adresse').fill(`${MOCK}/pbs`)
  await dialog.getByLabel('Token-ID').fill('labdeck@pbs!dashboard')
  await dialog.getByLabel('Token-Secret').fill('falsch')
  await dialog.getByRole('button', { name: 'Verbindung testen' }).click()
  await expect(dialog.getByRole('status')).toContainText('Zugriff verweigert (HTTP 401)')
  await dialog.getByLabel('Token-Secret').fill('pbs-secret')
  await dialog.getByRole('button', { name: 'Verbindung testen' }).click()
  await expect(dialog.getByRole('status')).toContainText('Verbindung erfolgreich – 6 Backups')
  await dialog.getByRole('button', { name: 'Speichern' }).click()
  await expect(dialog).toBeHidden()
  await expect(section.locator('li[data-integration="PBS"]').getByText('Verbunden')).toBeVisible()

  // Backups: Verify fehlgeschlagen, Host-Backup überfällig, Größe und Verlauf
  await page.goto('/backups')
  const nc = page.locator('tr[data-backup="vm/102 (nextcloud)"]')
  await expect(nc).toContainText('Fehlgeschlagen')
  await expect(nc).toContainText('Verify fehlgeschlagen')
  await expect(page.locator('tr[data-backup="host/nas"]')).toContainText('Überfällig')
  const ha = page.locator('tr[data-backup="vm/100 (homeassistant)"]').first()
  await expect(ha).toContainText('34 GB')
  await expect(ha).toContainText('verifiziert')
  await expect(ha.getByRole('img', { name: /Uptime 7 Tage: \d/ })).toBeVisible()
  await expect(page.locator('tr[data-backup="ct/200 (testbox)"]')).toContainText('local · lab')

  // Dienste: Belegung je Datastore
  await page.goto('/dienste')
  const card = page.locator('[data-service="PBS"]')
  await expect(card.getByRole('row', { name: /offsite/ })).toContainText('93,5 %')
  await expect(card.getByRole('row', { name: /^local/ })).toContainText('73 %')

  // Gesamtstatus: Datastore fast voll
  await page.goto('/')
  await expect(page.locator('[data-problem="used_pct"], [data-problem="Belegung"]').first()).toBeVisible()

  const list = await api<Integration[]>(page, 'GET', '/api/integrations')
  await api(page, 'DELETE', `/api/integrations/${list.json.find((i) => i.name === 'PBS')!.id}`)
})
