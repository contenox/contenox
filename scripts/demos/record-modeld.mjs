// Modeld dashboard demo — simple, real. Shows the local runtime dashboard
// (daemon status, resident slot, capacity) exposed by `contenox serve`, then
// loads a model so the single slot fills (Empty → resident). Nothing fancy.
//
// Prep: `make run-modeld` (a modeld daemon on the shared data root) + a running
// `contenox serve`. Env: BEAM_URL, BEAM_TOKEN. Deps: playwright-core on a
// resolvable node_modules (see the symlink note in record-beam.mjs).
import { readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { homedir } from 'node:os';
import { chromium } from 'playwright-core';

function chromiumExe() {
  if (process.env.PLAYWRIGHT_CHROMIUM_EXE) return process.env.PLAYWRIGHT_CHROMIUM_EXE;
  const cache = join(homedir(), '.cache', 'ms-playwright');
  const builds = readdirSync(cache)
    .filter((d) => /^chromium-\d+$/.test(d))
    .sort((a, b) => Number(b.split('-')[1]) - Number(a.split('-')[1]));
  for (const b of builds) {
    for (const sub of ['chrome-linux64', 'chrome-linux']) {
      try {
        const p = join(cache, b, sub, 'chrome');
        readdirSync(join(cache, b, sub));
        return p;
      } catch {}
    }
  }
  throw new Error('no Playwright Chromium found; set PLAYWRIGHT_CHROMIUM_EXE');
}

const EXE = chromiumExe();
const OUT = fileURLToPath(new URL('./video-modeld/', import.meta.url));
const BASE = process.env.BEAM_URL ?? 'http://127.0.0.1:32123';
const TOKEN = process.env.BEAM_TOKEN ?? '';
const hold = (ms) => new Promise((r) => setTimeout(r, ms));

const browser = await chromium.launch({ executablePath: EXE, headless: true });
const ctx = await browser.newContext({
  viewport: { width: 1440, height: 900 },
  locale: 'en-US',
  recordVideo: { dir: OUT, size: { width: 1440, height: 900 } },
});
await ctx.addInitScript(() => localStorage.setItem('i18nextLng', 'en'));
const page = await ctx.newPage();

try {
  await page.goto(BASE + '/#/backends');
  await page.waitForLoadState('networkidle');

  const tokenField = page.getByRole('textbox').first();
  if (await tokenField.isVisible().catch(() => false)) {
    await tokenField.fill(TOKEN);
    await page.getByRole('button', { name: /^login$/i }).click();
    await page.waitForLoadState('networkidle');
  }
  await hold(800);

  // Open the Modeld dashboard tab.
  await page.getByRole('tab', { name: /^modeld$/i }).click();
  await hold(2500);

  // A calm tour of the dashboard: status tiles → daemon table → resident slot
  // → capacity diagnostics, then settle back at the top on the status tiles.
  await hold(1200);
  await page.mouse.wheel(0, 380);
  await hold(2400);
  await page.mouse.wheel(0, 420);
  await hold(2400);
  await page.mouse.wheel(0, 420);
  await hold(2600);
  await page.mouse.wheel(0, -1220); // back to the status tiles for a clean close
  await hold(2600);

  await page.screenshot({ path: OUT + 'cover-frame.png' });
} finally {
  await ctx.close();
  await browser.close();
}
console.log('done');
