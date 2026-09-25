import { test, expect, type Page } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';
import { discoverRoutes } from './routes';

const routes = discoverRoutes();
const filter = process.env.SCREENSHOT_FILTER;
const fullPage = !!process.env.SCREENSHOT_FULL;
const filteredRoutes = filter
  ? routes.filter((r) => r.path.includes(filter) || r.slug.includes(filter))
  : routes;

async function primeLazyContent(page: Page): Promise<void> {
  await page.evaluate(async () => {
    const step = window.innerHeight;
    for (let y = 0; y < document.body.scrollHeight; y += step) {
      window.scrollTo(0, y);
      await new Promise((resolve) => setTimeout(resolve, 60));
    }
    window.scrollTo(0, 0);
  });
}

async function captureScreenshot(page: Page, screenshotPath: string): Promise<Buffer> {
  for (let attempt = 1; attempt <= 3; attempt++) {
    try {
      return await page.screenshot({
        path: screenshotPath,
        fullPage,
        animations: 'disabled',
        timeout: fullPage ? 30_000 : 15_000,
      });
    } catch (err) {
      if (attempt === 3) throw err;
      await page.waitForTimeout(250);
    }
  }
  throw new Error('Screenshot capture failed');
}

test.describe('Website Visual & Health Tests', () => {
  for (const route of filteredRoutes) {
    test(`capture ${route.path}`, async ({ page }, testInfo) => {
      const response = await page.goto(route.path, {
        waitUntil: 'domcontentloaded',
      });

      expect(response?.status()).toBe(200);
      await expect(page.locator('body')).toBeVisible();
      await expect(page.locator('html')).toHaveAttribute(
        'lang',
        route.category === 'de' ? 'de' : 'en',
      );
      await expect(page.locator('link[rel="canonical"]')).toHaveCount(1);

      await page.waitForLoadState('networkidle');
      if (fullPage) await primeLazyContent(page);

      const expectedTheme = testInfo.project.metadata.theme;
      if (expectedTheme) {
        const resolvedTheme = await page.evaluate(() =>
          document.documentElement.classList.contains('dark') ? 'dark' : 'light',
        );
        expect(resolvedTheme, `${testInfo.project.name} theme`).toBe(expectedTheme);
      }

      const projectName = testInfo.project.name;
      const screenshotDir = path.resolve(__dirname, `../.screenshots/${projectName}`);
      fs.mkdirSync(screenshotDir, { recursive: true });

      const fileName = `${route.slug}${fullPage ? '-full' : ''}.png`;
      const screenshotPath = path.join(screenshotDir, fileName);

      const buffer = await captureScreenshot(page, screenshotPath);

      await testInfo.attach(`${projectName}-${route.slug}`, {
        body: buffer,
        contentType: 'image/png',
      });
    });
  }
});
