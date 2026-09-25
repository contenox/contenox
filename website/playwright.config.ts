import { defineConfig, devices } from '@playwright/test';

const viewports = [
  {
    name: 'desktop',
    use: {
      ...devices['Desktop Chrome'],
      viewport: { width: 1920, height: 1080 },
    },
  },
  {
    name: 'mobile',
    use: {
      viewport: { width: 390, height: 844 },
      isMobile: true,
      hasTouch: true,
      deviceScaleFactor: 1,
    },
  },
];

const themes = ['light', 'dark'] as const;

export default defineConfig({
  testDir: './e2e',
  outputDir: './test-results',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 1,
  workers: process.env.CI ? 2 : undefined,
  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
  ],
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:4321',
    trace: 'on-first-retry',
  },
  projects: viewports.flatMap((viewport) =>
    themes.map((theme) => ({
      name: `${viewport.name}-${theme}`,
      metadata: { theme },
      use: { ...viewport.use, colorScheme: theme },
    })),
  ),
  webServer: {
    command: 'npm run dev',
    url: 'http://localhost:4321',
    reuseExistingServer: true,
    timeout: 60_000,
  },
});
