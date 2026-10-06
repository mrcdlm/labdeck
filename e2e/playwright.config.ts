import { defineConfig, devices } from '@playwright/test'

// Browser-Tests gegen einen echten labdeck-Server (start-server.sh).
// Alle Tests teilen sich einen Server → nacheinander ausführen.
export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  workers: 1,
  timeout: 60_000,
  expect: { timeout: 7_000 },
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: 'http://localhost:18080',
    ...devices['Desktop Chrome'],
    viewport: { width: 1280, height: 900 },
    // Die Tests prüfen deutsche Texte; Englisch testet 03-language-theme.spec.ts
    locale: 'de-DE',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  webServer: [
    {
      command: 'sh ./start-server.sh',
      url: 'http://localhost:18080/healthz',
      reuseExistingServer: false,
      timeout: 300_000,
      stdout: 'pipe',
    },
  ],
})
