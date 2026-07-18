import { describe, expect, it } from 'vitest';
import { AGENT_PICKER_CONTENT_CLASSNAME } from './AgentPicker';

/**
 * The picker popup lives inside the sessions rail, whose ancestors clip overflow
 * (`DesktopSidebar` overflow-hidden; `MobileSidebar` a fixed full-width overlay).
 * These assertions pin the geometry that keeps it fully on-screen — the panel is
 * not otherwise reachable in the DOM-less (node) test env, and a regression here
 * silently re-introduces the "renders partially outside the screen" bug where a
 * clipped option can't be clicked.
 */
describe('AGENT_PICKER_CONTENT_CLASSNAME', () => {
  it('right-anchors so the panel never escapes the right/viewport edge', () => {
    expect(AGENT_PICKER_CONTENT_CLASSNAME).toContain('right-0');
    expect(AGENT_PICKER_CONTENT_CLASSNAME).toContain('left-auto');
  });

  it('drops the unbounded `w-max` that grew leftward past the rail clip bounds', () => {
    expect(AGENT_PICKER_CONTENT_CLASSNAME).not.toContain('w-max');
  });

  it('caps width to the viewport so it stays within narrow / mobile widths', () => {
    // A `calc(100vw-…)` cap guarantees the fixed width never exceeds the viewport.
    expect(AGENT_PICKER_CONTENT_CLASSNAME).toMatch(/calc\(100vw-[0-9.]+rem\)/);
  });

  it('scrolls long content inside the panel instead of overflowing its box', () => {
    expect(AGENT_PICKER_CONTENT_CLASSNAME).toContain('overflow-auto');
    expect(AGENT_PICKER_CONTENT_CLASSNAME).toMatch(/max-h-/);
  });
});
