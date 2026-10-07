// Hilfsskript für scripts/screenshots.sh (kein Test).
//   node screenshots.mjs setup <labdeck-url> <mock-url>
//   node screenshots.mjs shoot <labdeck-url> <mock-url> <ziel>
import { chromium } from '@playwright/test'

const [mode, base, mock, out] = process.argv.slice(2)
const browser = await chromium.launch()

async function session(opts = {}) {
  const ctx = await browser.newContext({ locale: 'de-DE', viewport: { width: 1280, height: 860 }, ...opts })
  const page = await ctx.newPage()
  await page.goto(`${base}/login`)
  await page.fill('input[name=password]', 'demo')
  await page.click('button[type=submit]')
  await page.waitForURL(`${base}/`)
  return page
}

const api = (page, method, path, body) =>
  page.evaluate(
    async ([m, p, b]) => {
      const r = await fetch(p, { method: m, headers: { 'Content-Type': 'application/json' }, body: b && JSON.stringify(b) })
      return r.status === 204 ? null : r.json()
    },
    [method, path, body],
  )

const integrations = (m) => [
  { type: 'healthchecks', name: 'Healthchecks Homelab', config: { base_url: `${m}/healthchecks` }, secrets: { api_key: 'hc-readonly' } },
  { type: 'immich', name: 'Immich', config: { base_url: `${m}/immich` }, secrets: { api_key: 'immich-admin' } },
]

if (mode === 'setup') {
  const page = await session()
  for (const i of integrations(mock)) {
    const created = await api(page, 'POST', '/api/integrations', {
      enabled: true, interval_seconds: 3600, timeout_seconds: 10, tls_verify: true, clear_secrets: [], ...i,
    })
    await api(page, 'POST', `/api/integrations/${created.id}/poll`)
  }
} else {
  const shots = [
    ['dashboard', '/'],
    ['backups', '/backups'],
    ['checks', '/checks'],
    ['services', '/dienste'],
    ['settings', '/einstellungen'],
  ]
  for (const [scheme, suffix] of [['light', ''], ['dark', '-dark']]) {
    const page = await session({ colorScheme: scheme })
    for (const [name, path] of shots) {
      await page.goto(base + path)
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(600)
      await page.screenshot({ path: `${out}/${name}${suffix}.png`, fullPage: name !== 'settings' })
    }
  }
  const mobile = await session({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 })
  for (const [name, path] of [['dashboard', '/'], ['backups', '/backups']]) {
    await mobile.goto(base + path)
    await mobile.waitForLoadState('networkidle')
    await mobile.waitForTimeout(600)
    await mobile.screenshot({ path: `${out}/${name}-mobile.png` })
  }
}
await browser.close()
