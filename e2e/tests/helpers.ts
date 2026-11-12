import { expect, type Page } from '@playwright/test'

export async function login(page: Page) {
  await page.goto('/login')
  await page.fill('input[name=password]', 'test')
  await page.click('button[type=submit]')
  await expect(page).toHaveURL('/')
}

/** API-Aufruf im Kontext der Seite (mit Session-Cookie). */
export async function api<T = unknown>(page: Page, method: string, path: string, body?: unknown) {
  return page.evaluate(
    async ([m, p, b]) => {
      const r = await fetch(p as string, {
        method: m as string,
        headers: { 'Content-Type': 'application/json' },
        body: b === undefined ? undefined : JSON.stringify(b),
      })
      return { status: r.status, json: r.status === 204 ? null : await r.json() }
    },
    [method, path, body] as const,
  ) as Promise<{ status: number; json: T }>
}

/** Kein horizontales Scrollen der Seite (mobile Ansicht). */
export async function expectNoHorizontalOverflow(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
}

export const MOCK = 'http://localhost:8790'

// Steuerung der nachgebauten Dienste (cmd/mock)
export const mock = {
  hcFail: (status: number) => fetch(`${MOCK}/_healthchecks/fail?status=${status}`, { method: 'POST' }),
  hcStatus: (name: string, status: string) =>
    fetch(`${MOCK}/_healthchecks/status?name=${encodeURIComponent(name)}&status=${status}`, { method: 'POST' }),
}

/** Gesamtstatus oben auf der Übersicht („2 Probleme“, „Alles in Ordnung“). */
export const overall = (page: Page) => page.getByRole('region', { name: 'Gesamtstatus' })

/** Problemliste (Übersicht oder Detailseite eines Systems). */
export const problemList = (page: Page) => page.locator('[data-problem-list]').first()

/** Detailseite eines Systems über die Seite „Systeme“ öffnen. */
export async function openSystem(page: Page, name: string) {
  await page.goto('/systeme')
  await page.locator(`[data-system="${name}"]`).click()
  await expect(page.getByRole('heading', { level: 1, name })).toBeVisible()
}
