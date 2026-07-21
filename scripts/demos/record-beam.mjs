// Beam product demo recording — real flows only, per scripts/demos/RECORDING.md.
// Story (external-ACP-agents wave): seeded sidebar → "New chat with an agent" →
// pick the registered agent → prompt that reads then writes a file → tool cards
// → INLINE permission card with diff → Allow → write lands → agent-view policy
// overlay closer.
//
// Deps: playwright-core on NODE_PATH (a Playwright-managed Chromium from
// ~/.cache/ms-playwright is reused — no browser download). Prep per RECORDING.md:
// serve FROM the demo project so the external agent's cwd is scoped to it
// (`cd <demo-dir> && contenox serve <demo-dir>`), register the demo agent,
// reset fixtures. Env: BEAM_URL, BEAM_TOKEN (LAN serve token), BEAM_AGENT.
// Run: NODE_PATH=<dir-with-playwright-core> node record-beam.mjs
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
const BASE = process.env.BEAM_URL ?? 'http://127.0.0.1:32123';
const TOKEN = process.env.BEAM_TOKEN ?? '';
const AGENT = process.env.BEAM_AGENT ?? 'claude';
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
  await page.goto(BASE + '/#/chat');
  await page.waitForLoadState('networkidle');

  // Auth: a token-gated (LAN) serve shows a login page; a loopback serve goes
  // straight to chat. Key on the Login BUTTON — any-visible-textbox is a false
  // positive on the chat composer.
  const loginButton = page.getByRole('button', { name: /^login$/i });
  if (await loginButton.isVisible().catch(() => false)) {
    await page.getByRole('textbox').first().fill(TOKEN);
    await loginButton.click();
    await page.waitForLoadState('networkidle');
  }
  await hold(1200);

  // A fresh recording context can see the one-time setup wizard (its dismissal
  // lives in localStorage only). Click it away before the story starts.
  const wizardDone = page.getByRole('button', { name: /start using contenox|skip setup/i }).last();
  if (await wizardDone.isVisible().catch(() => false)) {
    await wizardDone.click();
    await page.waitForLoadState('networkidle');
    await hold(600);
  }

  // Opening: the seeded sidebar on camera briefly.
  await hold(1600);

  // The headline beat: "New chat with an agent" → pick the registered agent.
  await page.getByRole('button', { name: /new chat with an agent/i }).first().click();
  await hold(700);
  await page.getByRole('option', { name: new RegExp(`^${AGENT}$`, 'i') }).click();
  await hold(1400);

  // Open the workspace files panel so the project tree (and the agent-view
  // toggle) are on camera. Scoped to the demo project because serve runs there.
  await page
    .getByRole('button', { name: /workspace files|dateien/i })
    .last()
    .click()
    .catch(() => {});
  await hold(1400);

  // Type the prompt like a human — a read-then-gated-write turn.
  const box = page.locator('textarea').last();
  await box.click();
  await page.keyboard.type(
    'Read TODO.md, then append a new line to it: "- [ ] Ship the external-agents launch demo".',
    { delay: 30 },
  );
  await hold(500);
  await page.keyboard.press('Enter');

  // Wait for the INLINE permission card (role="group", not a dialog), let the
  // diff breathe, then Allow via the button (no click-outside, no `y`).
  const card = page.getByRole('group', { name: /permission required/i }).first();
  await card.waitFor({ state: 'visible', timeout: 90000 });
  await hold(3200);
  await page.getByRole('button', { name: /^allow$/i }).first().click();

  // Wait for the turn to finish (Stop gone), hold on the answer.
  await page
    .getByRole('button', { name: /^stop$|^stopp$/i })
    .waitFor({ state: 'hidden', timeout: 120000 })
    .catch(() => {});
  await hold(2600);

  // Closer: agent-view policy overlay on the file tree (optional — skip cleanly
  // if the toggle isn't present for this session).
  const shield = page.getByRole('button', { name: /agent view|agenten-ansicht/i }).first();
  if (await shield.isVisible().catch(() => false)) {
    await shield.click();
    await hold(3500);
  } else {
    await hold(1500);
  }

  await page.screenshot({ path: OUT + 'cover-frame.png' });
} finally {
  await ctx.close(); // flushes the video
  await browser.close();
}
console.log('done');
