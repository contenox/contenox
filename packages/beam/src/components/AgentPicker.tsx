import { Dropdown } from '@contenox/ui';
import type { MouseEventHandler, ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { useAgents } from '../hooks/useAgents';

/**
 * Geometry for the picker's popup panel. It lives inside the sessions rail,
 * whose ancestors clip overflow (`DesktopSidebar` is `overflow-hidden` /
 * `overflow-x-hidden`; `MobileSidebar` is a fixed full-width overlay), so the
 * panel MUST stay within both the rail (desktop) and the viewport (mobile):
 *
 *  - `right-0 left-auto` anchors the panel's RIGHT edge to the tiny caret
 *    trigger (itself near the rail's right edge), so the panel can never escape
 *    the right/viewport edge — it only ever grows leftward.
 *  - A FIXED, capped width `w-[min(14rem,calc(100vw-1.5rem))]` (was `w-max
 *    min-w-[12rem]`) stops the leftward growth: `w-max` grew with the widest
 *    label and, right-anchored, pushed the panel's left edge past the rail's
 *    clip bounds — options rendered "partially outside the screen" and could be
 *    clipped/occluded (so a click landed on nothing). 14rem sits inside the
 *    16rem (`w-64`) desktop rail with margin; the `calc(100vw-1.5rem)` cap keeps
 *    it inside narrow/mobile viewports.
 *  - `overflow-auto` + a height cap keep a long agent list (or a long label)
 *    scrolling INSIDE the panel rather than overflowing its box.
 *
 * Exported so the geometry decision is pinned by a regression test — the panel
 * is not otherwise reachable in the repo's DOM-less (node) test environment.
 */
export const AGENT_PICKER_CONTENT_CLASSNAME =
  'right-0 left-auto w-[min(14rem,calc(100vw-1.5rem))] max-h-[min(16rem,60vh)] overflow-auto';

export interface AgentPickerProps {
  /** The currently-selected agent name, or `null` for the native "contenox" chain. */
  value: string | null;
  /** Called with the chosen agent name, or `null` when the native chain is picked. */
  onSelect: (name: string | null) => void;
  /** The trigger element (the sidebar's compact agent button). */
  trigger: ReactElement<{ onClick?: MouseEventHandler }>;
  /**
   * Hide the whole control when there are no enabled external agents AND nothing
   * is currently selected — a workspace with no registered agents shows no
   * agent affordance at all. Defaults to `true`. Set `false` to always render
   * (so at least the native option is offered).
   */
  hideWhenEmpty?: boolean;
}

/**
 * Picks which agent a new chat talks to: the native "contenox" chain (top) or
 * any enabled registered external agent (from `useAgents`). Lives solely in the
 * sessions sidebar — an agent is bound at session creation and never switched
 * mid-conversation, so there is deliberately no in-chat variant. The choice
 * becomes the `session/new` `_meta` (see `AGENT_META_KEY`).
 *
 * The native option's wire value is the empty string; `onSelect` maps it back to
 * `null` so callers never special-case a sentinel.
 */
export function AgentPicker({ value, onSelect, trigger, hideWhenEmpty = true }: AgentPickerProps) {
  const { t } = useTranslation();
  const { data } = useAgents();

  // Enabled agents only; a registry entry literally named "contenox" would
  // duplicate the native option, so drop it.
  const agents = (data ?? []).filter(a => a.enabled && a.name.toLowerCase() !== 'contenox');

  if (hideWhenEmpty && agents.length === 0 && value == null) return null;

  const options = [
    { value: '', label: t('acp_chat.agent_native') },
    ...agents.map(a => ({ value: a.name, label: a.name })),
  ];

  return (
    <Dropdown
      trigger={trigger}
      options={options}
      value={value ?? ''}
      onChange={v => onSelect(v === '' ? null : v)}
      contentClassName={AGENT_PICKER_CONTENT_CLASSNAME}
    />
  );
}
