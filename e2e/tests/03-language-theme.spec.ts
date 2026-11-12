import { expect, test, type Page } from '@playwright/test'

const isDark = (page: Page) => page.evaluate(() => document.documentElement.classList.contains('dark'))

test.describe('Sprache', () => {
  test('Umschalten auf Englisch: Oberfläche und Server-Meldungen, bleibt nach Neuladen', async ({ page }) => {
    await page.goto('/login')
    await expect(page.locator('[data-slot=card-title]')).toHaveText('Anmelden')
    await page.getByRole('group', { name: 'Sprache' }).getByRole('button', { name: 'en' }).click()
    await expect(page.locator('[data-slot=card-title]')).toHaveText('Sign in')
    expect(await page.evaluate(() => document.documentElement.lang)).toBe('en')

    // Server-Meldung in der gewählten Sprache
    await page.fill('input[name=password]', 'falsch')
    await page.click('button[type=submit]')
    await expect(page.getByRole('alert')).toContainText('Wrong password')

    await page.fill('input[name=password]', 'test')
    await page.click('button[type=submit]')
    await expect(page).toHaveURL('/')
    await expect(page.getByText('No integrations set up yet')).toBeVisible()

    await page.reload()
    await expect(page.getByRole('link', { name: 'Systems' })).toBeVisible()

    await page.locator('aside').getByRole('group', { name: 'Language' }).getByRole('button', { name: 'de' }).click()
    await expect(page.getByRole('link', { name: 'Systeme' })).toBeVisible()
  })

  test.describe('englischer Browser', () => {
    test.use({ locale: 'en-US' })
    test('Standard folgt der Browsersprache', async ({ page }) => {
      await page.goto('/login')
      await expect(page.locator('[data-slot=card-title]')).toHaveText('Sign in')
    })
  })
})

test.describe('Hell/Dunkel', () => {
  test.describe('helles System', () => {
    test.use({ colorScheme: 'light' })
    test('System folgt der Vorgabe, auch live', async ({ page }) => {
      await page.goto('/login')
      expect(await isDark(page)).toBe(false)
      await page.emulateMedia({ colorScheme: 'dark' })
      await expect.poll(() => isDark(page)).toBe(true)
      await page.emulateMedia({ colorScheme: 'light' })
      await expect.poll(() => isDark(page)).toBe(false)
    })
  })

  test.describe('dunkles System', () => {
    test.use({ colorScheme: 'dark' })
    test('manuelle Wahl bleibt nach Neuladen erhalten', async ({ page }) => {
      await page.goto('/login')
      expect(await isDark(page)).toBe(true)
      // System → Hell
      await page.getByRole('button', { name: 'Darstellung: System' }).click()
      await expect.poll(() => isDark(page)).toBe(false)
      await page.reload()
      expect(await isDark(page)).toBe(false)
      await expect(page.getByRole('button', { name: 'Darstellung: Hell' })).toBeVisible()
      // Hell → Dunkel → System
      await page.getByRole('button', { name: 'Darstellung: Hell' }).click()
      await expect.poll(() => isDark(page)).toBe(true)
      await page.getByRole('button', { name: 'Darstellung: Dunkel' }).click()
      await page.emulateMedia({ colorScheme: 'light' })
      await expect.poll(() => isDark(page)).toBe(false)
    })
  })
})
