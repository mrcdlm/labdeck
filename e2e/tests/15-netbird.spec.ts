import { expect, test } from '@playwright/test'

import { api, expectNoHorizontalOverflow, login, MOCK, openSystem, problemList } from './helpers'

interface Integration {
  id: number
  name: string
}

test('NetBird: Peers, überwachte Gruppe, Reverse Proxy, Systeme', async ({ page }) => {
  await login(page)

  // per Formular
  await page.goto('/einstellungen')
  const section = page.locator('[data-slot=card]').filter({ hasText: 'Integrationen' })
  await section.getByRole('button', { name: 'Hinzufügen' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: /^NetBird/ }).click()
  await expect(dialog.getByText(/Service Users/)).toBeVisible()
  await dialog.locator('#integration-name').fill('NetBird')
  await dialog.getByLabel('Management-Adresse').fill(`${MOCK}/netbird`)
  await dialog.getByLabel('Access Token').fill('nbp_mock')
  await dialog.getByLabel('Überwachte Gruppe').fill('server')
  await dialog.getByRole('button', { name: 'Verbindung testen' }).click()
  await expect(dialog.getByRole('status')).toContainText('Verbindung erfolgreich')
  await dialog.getByRole('button', { name: 'Speichern' }).click()
  await expect(dialog).toBeHidden()

  const list = await api<Integration[]>(page, 'GET', '/api/integrations')
  const nb = list.json.find((i) => i.name === 'NetBird')!
  await api(page, 'POST', `/api/integrations/${nb.id}/poll`)

  // Systeme: unter „Netzwerk“
  await page.goto('/systeme')
  await expect(page.locator('[data-category="network"] [data-system="NetBird"]')).toBeVisible()

  // Detailseite: überwachte Peers zuerst, pi-edge offline als Problem
  await openSystem(page, 'NetBird')
  const card = page.locator('[data-vpn="NetBird"]')
  await expect(card).toContainText('3 von 5 Peers online')
  await expect(card).toContainText('1 überwachter Peer mit Problem')
  await expect(card.locator('tr[data-peer]').first()).toHaveAttribute('data-peer', 'pi-edge')
  await expect(card.locator('tr[data-peer]')).toHaveCount(3) // nur überwachte
  await card.getByRole('group', { name: 'Peers' }).getByRole('button', { name: 'Alle' }).click()
  await expect(card.locator('tr[data-peer]')).toHaveCount(5)
  await expect(card.locator('tr[data-peer="marco-laptop"]')).toContainText('Offline')
  await expect(card.locator('[data-proxy="ha.example.org"]')).toContainText('Zertifikat fehlgeschlagen')
  await expect(card.locator('[data-proxy="photos.example.org"]')).toContainText('Aktiv')

  // Dashboard: Kachel mit Problemen, Problemzeile springt zum Peer
  await page.goto('/')
  const tile = page.locator('[data-tile="NetBird"]')
  await expect(tile).toHaveAttribute('data-tone', 'error')
  await expect(tile.locator('[data-problem-count]')).toBeVisible()
  const offline = problemList(page).getByRole('link', { name: /pi-edge/ })
  await expect(offline).toContainText('NetBird › pi-edge')
  await offline.click()
  await expect(page).toHaveURL(/\/systeme\/\d+#res-/)
  await expect(page.locator('tr[data-peer="pi-edge"]')).toBeInViewport()

  // Suche nach einem nicht überwachten Peer zeigt gleich alle Peers
  await page.goto('/systeme')
  await page.getByLabel('Systeme, Hosts, VMs, Container oder Ports suchen').fill('marco-laptop')
  await page.locator('[data-hit="marco-laptop"]').click()
  await expect(page.locator('tr[data-peer="marco-laptop"]')).toBeInViewport()

  // Peer kommt zurück → kein Problem mehr
  await fetch(`${MOCK}/_netbird/peer?name=pi-edge&connected=1`, { method: 'POST' })
  await api(page, 'POST', `/api/integrations/${nb.id}/poll`)
  await openSystem(page, 'NetBird')
  await expect(page.locator('[data-vpn="NetBird"]')).not.toContainText('überwachter Peer mit Problem')

  await page.setViewportSize({ width: 390, height: 844 })
  await openSystem(page, 'NetBird')
  await expect(page.locator('li[data-peer="docker01"]')).toBeVisible()
  await expectNoHorizontalOverflow(page)

  await fetch(`${MOCK}/_netbird/peer?name=pi-edge&connected=0`, { method: 'POST' })
  await api(page, 'DELETE', `/api/integrations/${nb.id}`)
})
