// Beam product demo recording — real flows only, per scripts/demos/RECORDING.md.
// Story: seeded sidebar → new session in demo-project → prompt → read tool card
// → approval gate with diff → Allow (Y) → write lands → agent-view policy overlay.
//
// Deps: `npm i playwright-core` anywhere on NODE_PATH (no browser download
// needed — a Playwright-managed Chromium from ~/.cache/ms-playwright is
// reused). Set prep per RECORDING.md: `contenox serve <demo-dir>`, think off,
// seeded sidebar, fixtures reset. Run: node record-beam.mjs
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
const OUT = fileURLToPath(new URL('./video/', import.meta.url));
const BEAM = process.env.BEAM_URL ?? 'http://127.0.0.1:32123/#/chat';
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
  await page.goto(BEAM);
  await page.waitForLoadState('networkidle');
  await hold(400);

  // Opening shot: open the session drawer for a 2.5s pan, then close it again
  // so it can't intercept clicks on the chat surface.
  const toggle = page.getByRole('button', { name: /toggle sidebar/i }).first();
  const sessionsNav = page.locator('nav[aria-label="Sessions"], nav[aria-label="Sitzungen"]');
  if (!(await sessionsNav.isVisible().catch(() => false))) {
    await toggle.click();
    await hold(1800);
    await toggle.click();
    await hold(500);
  } else {
    await hold(1800);
  }

  // New session via the tab-bar affordance (not the drawer button).
  const newChat = page.getByRole('button', { name: /new chat|new session|neuer chat|neue sitzung/i }).last();
  await newChat.click();
  await hold(1200);

  // Pick the demo-project workspace — fail fast if it isn't offered.
  const wsSelect = page.locator('select').filter({ has: page.locator('option', { hasText: /demo-project/i }) }).last();
  await wsSelect.waitFor({ state: 'attached', timeout: 10000 });
  const val = await wsSelect.locator('option', { hasText: /demo-project/i }).first().getAttribute('value');
  await wsSelect.selectOption(val);
  await hold(1200);

  // Open the workspace files panel so the project tree (and the agent-view
  // toggle) are on camera for the whole take.
  const filesToggle = page.getByRole('button', { name: /workspace files|dateien/i }).last();
  await filesToggle.click();
  await hold(1500);

  // Type the prompt like a human
  const box = page.locator('textarea[placeholder], input[placeholder*="essage"], textarea').last();
  await box.click();
  await page.keyboard.type(
    'Read TODO.md and add a 0.2.0 entry to CHANGELOG.md for the completed items.',
    { delay: 34 },
  );
  await hold(600);
  await page.keyboard.press('Enter');

  // Wait for the approval gate (up to 60s), let the diff breathe, approve via Y
  const dialog = page.locator('[role="dialog"], [role="alertdialog"]');
  await dialog.waitFor({ state: 'visible', timeout: 60000 });
  await hold(3200);
  await page.keyboard.press('y');

  // Wait for the turn to finish (stop button gone), hold on the answer
  await page
    .getByRole('button', { name: /^stop$|^stopp$/i })
    .waitFor({ state: 'hidden', timeout: 90000 })
    .catch(() => {});
  await hold(2600);

  // Closer: agent-view policy overlay on the file tree (mandatory shot)
  const shield = page.getByRole('button', { name: /agent view|agenten-ansicht/i }).first();
  await shield.waitFor({ state: 'visible', timeout: 10000 });
  await shield.click();
  await hold(3500);

  await page.screenshot({ path: OUT + 'cover-frame.png' });
} finally {
  await ctx.close(); // flushes the video
  await browser.close();
}
console.log('done');
