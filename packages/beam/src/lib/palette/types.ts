import type { TFunction } from 'i18next';
import type { ReactNode } from 'react';
import type { Agent, FleetEntry, HITLApproval, Mission, WorkspaceRoot } from '../types';
import type { SessionInfo } from '../acp';

/**
 * The goto-anything palette's object taxonomy. Purely a display/ordering hint
 * (badge label, empty-query priority) — matching never keys off it.
 */
export type PaletteItemType =
  | 'action'
  | 'mission'
  | 'agent'
  | 'session'
  | 'fleet'
  | 'workspace'
  | 'inbox';

/**
 * THE palette item contract. Every source produces these and nothing else, so a
 * future source is added by writing one {@link PaletteProvider} that returns
 * them — no change to matching, ranking, frecency, or the overlay.
 *
 *  - `id`      stable + globally unique (namespaced by source, e.g. `mission:<id>`);
 *              it is also the frecency key, so it must be stable across renders.
 *  - `title`   the primary label — highest match weight, and the highlighted line.
 *  - `subtitle` optional secondary line (dimmed).
 *  - `icon`    a rendered node (a lucide icon element).
 *  - `keywords` extra match terms that are NOT displayed (agent name, ids, aliases,
 *              German/English synonyms) — mid match weight.
 *  - `action`  self-contained: run on select; it navigates / adopts / etc. The
 *              overlay records frecency and closes around it, so `action` never
 *              has to.
 */
export interface PaletteItem {
  id: string;
  type: PaletteItemType;
  title: string;
  subtitle?: string;
  icon: ReactNode;
  keywords?: string[];
  action: () => void;
}

/**
 * Everything a provider needs to turn cached/context data into items and wire
 * their actions. Assembled once per palette-open from read-only cache snapshots
 * and app context (see `components/palette/usePaletteSources.ts`); providers
 * never fetch.
 */
export interface PaletteProviderContext {
  t: TFunction;
  navigate: (to: string) => void;
  /** "Open this running session in chat" — the shared adopt entry point. */
  startAdopt: (ref: { instanceId: string; sessionId: string }) => void;

  missions: Mission[];
  fleet: FleetEntry[];
  agents: Agent[];
  approvals: HITLApproval[];
  sessions: SessionInfo[];
  workspaceRoots: WorkspaceRoot[];
}

/** A source of palette items: a pure function of the context. Register one by adding it to `paletteProviders`. */
export type PaletteProvider = (ctx: PaletteProviderContext) => PaletteItem[];
