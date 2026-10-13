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
  {
    type: 'pbs', name: 'Proxmox Backup', config: { base_url: `${m}/pbs`, token_id: 'labdeck@pbs!dashboard' },
    secrets: { token_secret: 'pbs-secret' },
  },
  {
    type: 'pve', name: 'Proxmox VE', config: { base_url: `${m}/pve`, token_id: 'labdeck@pve!dashboard', backups: false },
    secrets: { token_secret: 'pve-secret' },
  },
  { type: 'dockhand', name: 'Dockhand', config: { base_url: `${m}/dockhand` }, secrets: { api_token: 'dh_mock' } },
  { type: 'nut', name: 'USV', config: { host: new URL(m).hostname, port: 8794 }, secrets: {} },
  {
    type: 'snmp', name: 'Core-Switch', config: { host: new URL(m).hostname, port: 8796, lags: true },
    secrets: { community: 'labdeck-ro' },
  },
  { type: 'redfish', name: 'iLO pve-01', config: { base_url: `${m}/ilo`, username: 'labdeck' }, secrets: { password: 'ilo-pass' } },
  { type: 'pihole', name: 'Pi-hole', config: { base_url: `${m}/pihole` }, secrets: { password: 'pihole-app' } },
  {
    type: 'prometheus', name: 'Prometheus', secrets: {},
    config: {
      base_url: `${m}/prometheus`,
      queries: [
        { name: 'Freier Speicher', unit: '%', label: 'instance', direction: 'below', warn: 15, error: 5,
          query: 'node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"} * 100' },
        { name: 'CPU', unit: '%', warn: 80, query: '100 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100' },
      ],
    },
  },
]

if (mode === 'setup') {
  const page = await session()
  for (const i of integrations(mock)) {
    const created = await api(page, 'POST', '/api/integrations', {
      enabled: true, interval_seconds: 3600, timeout_seconds: 10, tls_verify: true, clear_secrets: [], ...i,
    })
    await api(page, 'POST', `/api/integrations/${created.id}/poll`)
    // Switch: Raten erst ab der zweiten Abfrage
    if (i.type === 'snmp') {
      await page.waitForTimeout(3000)
      await api(page, 'POST', `/api/integrations/${created.id}/poll`)
    }
  }
  // zweites Dashboard „Netzwerk & Hardware“ mit Kennzahl-Kacheln
  const all = await api(page, 'GET', '/api/integrations')
  const id = (type) => all.find((x) => x.type === type).id
  const res = await api(page, 'POST', '/api/dashboards', { name: 'Netzwerk & Hardware', icon: 'network' })
  const d = res.dashboards.at(-1)
  const metric = (key, x, y, w, h, options) => ({ id: `metric:${key}`, visible: true, x, y, w, h, options })
  await api(page, 'PUT', `/api/dashboards/${d.id}`, {
    ...d,
    tiles: [
      { id: `integration:${id('snmp')}`, visible: true, x: 0, y: 0, w: 6, h: 5 },
      metric('traffic', 6, 0, 6, 5, { integration_id: id('snmp'), keys: ['traffic_in', 'traffic_out'], view: 'area', range: '24h', label: 'Traffic' }),
      { id: `integration:${id('redfish')}`, visible: true, x: 0, y: 5, w: 4, h: 5 },
      metric('watt', 4, 5, 4, 5, { integration_id: id('redfish'), keys: ['power'], view: 'sparkline', range: '24h' }),
      metric('blocked', 8, 5, 4, 5, { integration_id: id('pihole'), keys: ['blocked_pct'], view: 'gauge', range: '24h', max: 50, label: 'Pi-hole geblockt' }),
    ],
  })
} else {
  const shots = [
    ['dashboard', '/'],
    ['backups', '/backups'],
    ['checks', '/checks'],
    ['services', '/dienste'],
    ['resources', '/ressourcen'],
    ['history', '/verlauf'],
    ['settings', '/einstellungen'],
  ]
  const shoot = async (page, name, path, file, full = true) => {
    await page.goto(base + path)
    await page.waitForLoadState('networkidle')
    await page.waitForTimeout(2000) // Diagramme animieren per JS (animations: 'disabled' greift nicht)
    await page.screenshot({ path: `${out}/${file}.png`, fullPage: full, animations: 'disabled' })
  }
  // je Sprache alle Seiten hell, Dashboard zusätzlich dunkel und mobil
  for (const lang of ['de', 'en']) {
    const locale = lang === 'de' ? 'de-DE' : 'en-US'
    const page = await session({ locale })
    for (const [name, path] of shots) await shoot(page, name, path, `${name}.${lang}`, name !== 'settings')
    const net = (await api(page, 'GET', '/api/dashboards')).dashboards.find((d) => d.id !== 'main')
    await shoot(page, 'dashboard', `/d/${net.id}`, `dashboard-network.${lang}`)
    const dark = await session({ locale, colorScheme: 'dark' })
    await shoot(dark, 'dashboard', '/', `dashboard-dark.${lang}`)
    const mobile = await session({ locale, viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 })
    await shoot(mobile, 'dashboard', '/', `dashboard-mobile.${lang}`, false)
  }
}
await browser.close()
