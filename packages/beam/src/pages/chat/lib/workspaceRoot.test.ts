import { describe, expect, it } from 'vitest';
import { resolveWorkspaceRoot } from './workspaceRoot';

describe('resolveWorkspaceRoot', () => {
  it('uses the live session cwd once a session exists', () => {
    expect(
      resolveWorkspaceRoot({
        onEmptyChat: false,
        stagedRoot: undefined,
        defaultRoot: '/srv/default',
        activeSessionCwd: '/work/session-cwd',
      }),
    ).toBe('/work/session-cwd');
  });

  it('prefers the staged pick over the default on the empty chat', () => {
    expect(
      resolveWorkspaceRoot({
        onEmptyChat: true,
        stagedRoot: '/work/picked',
        defaultRoot: '/srv/default',
        activeSessionCwd: null,
      }),
    ).toBe('/work/picked');
  });

  it('falls back to the workspace default when nothing is staged', () => {
    expect(
      resolveWorkspaceRoot({
        onEmptyChat: true,
        stagedRoot: undefined,
        defaultRoot: '/srv/default',
        activeSessionCwd: null,
      }),
    ).toBe('/srv/default');
  });

  it('treats an empty staged pick as "none"', () => {
    expect(
      resolveWorkspaceRoot({
        onEmptyChat: true,
        stagedRoot: '',
        defaultRoot: '/srv/default',
        activeSessionCwd: null,
      }),
    ).toBe('/srv/default');
  });

  // Regression: an external-agent empty chat exposes no root PICKER (external
  // sessions advertise no native config options), so its stagedRoot is always
  // absent — it must resolve to the workspace default root, NOT null, so the
  // file panel and `@`-mention list work exactly as they do for a native chat.
  // (The runtime creates the external session under that same default cwd.)
  it('resolves an external empty chat (no staged pick) to the default root, not null', () => {
    expect(
      resolveWorkspaceRoot({
        onEmptyChat: true,
        stagedRoot: undefined,
        defaultRoot: '/srv/default',
        activeSessionCwd: null,
      }),
    ).toBe('/srv/default');
  });

  it('returns null when the empty chat has neither a staged pick nor a default', () => {
    expect(
      resolveWorkspaceRoot({
        onEmptyChat: true,
        stagedRoot: undefined,
        defaultRoot: undefined,
        activeSessionCwd: null,
      }),
    ).toBeNull();
  });
});
