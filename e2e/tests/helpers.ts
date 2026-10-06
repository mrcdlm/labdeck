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
