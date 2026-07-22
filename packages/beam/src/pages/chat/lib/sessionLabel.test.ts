import { describe, expect, it } from 'vitest';
import type { SessionInfo } from '../../../lib/acp';
import { meaningfulTitle, workspaceLabel } from './sessionLabel';

describe('meaningfulTitle', () => {
  it('returns a real title', () => {
    expect(meaningfulTitle({ sessionId: 'abc', title: 'My chat' } as SessionInfo)).toBe('My chat');
  });

  it('treats a title that merely echoes the id as absent', () => {
    expect(meaningfulTitle({ sessionId: 'abc', title: 'abc' } as SessionInfo)).toBeNull();
    expect(meaningfulTitle({ sessionId: 'abc' } as SessionInfo)).toBeNull();
  });
});

describe('workspaceLabel', () => {
  it('returns the basename of the cwd', () => {
    expect(workspaceLabel('/home/naro/src/github.com/contenox/runtime')).toBe('runtime');
    expect(workspaceLabel('/home/naro/proj/')).toBe('proj'); // trailing slash tolerated
    expect(workspaceLabel('/single')).toBe('single');
  });

  it('returns null when there is nothing meaningful to show', () => {
    expect(workspaceLabel(undefined)).toBeNull();
    expect(workspaceLabel(null)).toBeNull();
    expect(workspaceLabel('')).toBeNull();
    expect(workspaceLabel('/')).toBeNull();
  });
});
