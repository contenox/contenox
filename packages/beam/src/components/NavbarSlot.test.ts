import { createElement, type ReactNode } from 'react';
import { describe, expect, it } from 'vitest';
import { releaseNavbarSlot } from './NavbarSlot';

/**
 * Pure coverage for the navbar slot's single-owner set/clear contract.
 * `@testing-library/react` isn't a dependency of this package (see
 * PermissionCard.test.tsx / AcpWorkspaceProvider.test.ts), and effects don't run
 * under `renderToStaticMarkup`, so — exactly like `createDeferredDisposer` — the
 * lifecycle guard is a pure function and is exercised directly. The model below
 * drives `releaseNavbarSlot` the same way `useNavbarSlot`'s effect does: claim
 * (overwrite) on mount, release (only if still ours) on unmount.
 */
describe('releaseNavbarSlot — navbar slot single-owner set/clear', () => {
  it('clears the slot when the unmounting claimant still holds its own node', () => {
    const node = createElement('span', null, 'badge');
    expect(releaseNavbarSlot(node, node)).toBeNull();
  });

  it('does NOT stomp a newer owner when a stale claimant releases', () => {
    const older = createElement('span', null, 'A');
    const newer = createElement('span', null, 'B');
    // Slot currently holds B (mounted after A). A's late unmount releases with
    // its own (older) node; since the slot no longer holds A, B is untouched.
    expect(releaseNavbarSlot(newer, older)).toBe(newer);
  });

  it('models mount → mount → unmount → unmount: last mounter wins, cleared when the last leaves', () => {
    let slot: ReactNode = null;
    const claim = (node: ReactNode) => {
      slot = node; // useNavbarSlot's mount: setNode(claimed) — last mounter wins
    };
    const release = (claimed: ReactNode) => {
      slot = releaseNavbarSlot(slot, claimed); // unmount: only clear if still ours
    };

    const a = createElement('span', null, 'A');
    const b = createElement('span', null, 'B');

    claim(a);
    expect(slot).toBe(a);
    claim(b); // B mounts after A — takes the slot
    expect(slot).toBe(b);
    release(a); // A unmounts after B — must not clear B's node
    expect(slot).toBe(b);
    release(b); // B, the current owner, unmounts — slot goes empty
    expect(slot).toBeNull();
  });
});
