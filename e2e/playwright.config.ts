import { defineConfig, devices } from '@playwright/test'

// Browser-Tests gegen einen echten labdeck-Server (start-server.sh) und die
// nachgebauten APIs der Dienste (cmd/mock).
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
      // nachgebaute APIs (Healthchecks …), siehe cmd/mock
      command: 'go run ../cmd/mock -addr :8790',
      url: 'http://localhost:8790/_health',
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: 'sh ./start-server.sh',
      url: 'http://localhost:18080/healthz',
      reuseExistingServer: false,
      timeout: 300_000,
      stdout: 'pipe',
    },
  ],
})
