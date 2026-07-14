import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import { describe, expect, it } from 'vitest';

/**
 * Design-token guard: components must use the semantic tokens from
 * @contenox/ui (text-error, bg-surface-100, ...), never raw Tailwind palette
 * classes — those don't follow the .dark theme flip and are exactly what made
 * beam look inconsistent. Also bans the invalid `@dark` at-rule (browsers
 * silently drop it; the theme flip is the unlayered `.dark { }` rule).
 */

const BEAM_SRC = join(__dirname, '..');
const UI_SRC = join(__dirname, '..', '..', '..', 'ui', 'src');

/** Theme bridges (hex-by-design) and intentionally garish debug UI. */
const ALLOWLIST = ['lib/monacoAppTheme.ts', 'lib/xtermTheme.ts', 'components/visualization/WorkflowVisualizer.tsx'];

const RAW_PALETTE =
  /\b(?:text|bg|border|ring|outline|fill|stroke|divide|decoration|caret|accent|shadow|from|via|to)-(?:red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|slate|gray|zinc|neutral|stone)-\d{2,3}\b/;

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name !== 'node_modules' && entry.name !== 'dist') walk(full, out);
    } else {
      out.push(full);
    }
  }
  return out;
}

function violations(root: string): string[] {
  const found: string[] = [];
  for (const file of walk(root)) {
    const rel = relative(root, file).replace(/\\/g, '/');
    if (ALLOWLIST.some(a => rel.endsWith(a))) continue;
    if (/\.(test|stories)\.(ts|tsx)$/.test(rel)) continue;
    if (/\.(ts|tsx)$/.test(rel)) {
      const lines = readFileSync(file, 'utf8').split('\n');
      lines.forEach((line, i) => {
        const m = line.match(RAW_PALETTE);
        if (m) found.push(`${rel}:${i + 1} uses raw palette class "${m[0]}"`);
      });
    } else if (rel.endsWith('.css')) {
      const lines = readFileSync(file, 'utf8').split('\n');
      lines.forEach((line, i) => {
        if (/@dark\b/.test(line)) found.push(`${rel}:${i + 1} uses the invalid @dark at-rule`);
      });
    }
  }
  return found;
}

describe('design token guard', () => {
  it('beam/src uses semantic tokens, not raw Tailwind palette classes', () => {
    expect(violations(BEAM_SRC)).toEqual([]);
  });

  it('ui/src uses semantic tokens, not raw Tailwind palette classes', () => {
    expect(violations(UI_SRC)).toEqual([]);
  });
});
