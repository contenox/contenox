import {
  Bot,
  Database,
  Folder,
  FolderOpen,
  Inbox,
  MessageSquare,
  MessageSquarePlus,
  Radar,
  Rocket,
  Settings,
  type LucideIcon,
} from 'lucide-react';
import { createElement } from 'react';
import type { TranslationKey } from '../../i18n';
import { externalAgentFromMeta } from '../acp';
import { meaningfulTitle } from '../../pages/chat/lib/sessionLabel';
import type { PaletteItem, PaletteProvider, PaletteProviderContext } from './types';

/**
 * The built-in palette sources. Each is a pure `(ctx) => PaletteItem[]`; the
 * registry is just the array below, so a NEW source is one function appended
 * here — nothing else changes. Providers read only the snapshot data on `ctx`
 * (never fetch) and fold every non-title match term into `keywords`, in both
 * English and German, so the German UI matches German words too.
 */

const icon = (i: LucideIcon) => createElement(i, { className: 'h-4 w-4', 'aria-hidden': true });

/** Static navigation + verbs: every admin destination plus "fire a mission" / "new chat". */
const NAV_TARGETS: { path: string; labelKey: TranslationKey; icon: LucideIcon; keywords: string[] }[] = [
  { path: '/chat', labelKey: 'navbar.chat', icon: MessageSquare, keywords: ['chat', 'session', 'sitzung', 'beam'] },
  { path: '/fleet', labelKey: 'navbar.fleet', icon: Radar, keywords: ['fleet', 'flotte', 'board', 'instances', 'instanzen'] },
  { path: '/missions', labelKey: 'navbar.missions', icon: Rocket, keywords: ['missions', 'missionen'] },
  { path: '/inbox', labelKey: 'navbar.inbox', icon: Inbox, keywords: ['inbox', 'posteingang', 'approvals', 'freigaben', 'asks'] },
  { path: '/backends', labelKey: 'navbar.backends', icon: Database, keywords: ['backends', 'models', 'modelle', 'providers'] },
  { path: '/projects', labelKey: 'navbar.projects', icon: Folder, keywords: ['projects', 'projekte', 'workspace', 'workspaces', 'arbeitsbereich', 'roots', 'root'] },
  { path: '/settings', labelKey: 'navbar.settings', icon: Settings, keywords: ['settings', 'einstellungen', 'defaults', 'config'] },
];

export const actionsProvider: PaletteProvider = (ctx: PaletteProviderContext): PaletteItem[] => {
  const items: PaletteItem[] = [
    {
      id: 'action:new-mission',
      type: 'action',
      title: ctx.t('palette.action_new_mission'),
      subtitle: ctx.t('palette.subtitle_action'),
      icon: icon(Rocket),
      keywords: ['mission', 'neu', 'new', 'fire', 'dispatch', 'starten', 'feuern'],
      action: () => ctx.navigate('/missions/new'),
    },
    {
      id: 'action:new-chat',
      type: 'action',
      title: ctx.t('palette.action_new_chat'),
      subtitle: ctx.t('palette.subtitle_action'),
      icon: icon(MessageSquarePlus),
      keywords: ['chat', 'neu', 'new', 'session', 'sitzung'],
      action: () => ctx.navigate('/chat'),
    },
  ];

  for (const nav of NAV_TARGETS) {
    items.push({
      id: `nav:${nav.path}`,
      type: 'action',
      title: ctx.t(nav.labelKey),
      subtitle: ctx.t('palette.subtitle_navigate'),
      icon: icon(nav.icon),
      keywords: [nav.path, ...nav.keywords],
      action: () => ctx.navigate(nav.path),
    });
  }

  return items;
};

export const missionsProvider: PaletteProvider = (ctx): PaletteItem[] =>
  ctx.missions.map(m => {
    const statusLabel = ctx.t(`missions.status.${m.status}` as TranslationKey);
    return {
      id: `mission:${m.id}`,
      type: 'mission',
      title: m.intent,
      subtitle: `${m.agentName} · ${statusLabel}`,
      icon: icon(Rocket),
      keywords: [m.agentName, m.id, m.hitlPolicyName, statusLabel].filter(Boolean),
      action: () => ctx.navigate(`/missions/${m.id}`),
    };
  });

export const fleetProvider: PaletteProvider = (ctx): PaletteItem[] => {
  const items: PaletteItem[] = [];
  for (const entry of ctx.fleet) {
    for (const inst of entry.instances ?? []) {
      const stateLabel = ctx.t(`fleet.state.${inst.state}` as TranslationKey);
      const sessionId = inst.sessionIds?.[0];
      // "Open session" only makes sense against a running instance with a live
      // downstream to adopt; otherwise the item just lands on the board.
      const canAdopt = inst.state === 'running' && !!sessionId;
      items.push({
        id: `fleet:${inst.id}`,
        type: 'fleet',
        title: entry.agentName,
        subtitle: canAdopt
          ? `${stateLabel} · ${ctx.t('palette.open_session')}`
          : `${stateLabel} · ${inst.id.slice(0, 8)}`,
        icon: icon(Radar),
        keywords: [inst.id, entry.kind, stateLabel, entry.agentName].filter(Boolean),
        action: canAdopt
          ? () => ctx.startAdopt({ instanceId: inst.id, sessionId: sessionId! })
          : () => ctx.navigate('/fleet'),
      });
    }
  }
  return items;
};

export const agentsProvider: PaletteProvider = (ctx): PaletteItem[] =>
  ctx.agents
    .filter(a => a.enabled)
    .map(a => ({
      id: `agent:${a.id}`,
      type: 'agent',
      title: a.name,
      subtitle: ctx.t('palette.subtitle_dispatch'),
      icon: icon(Bot),
      keywords: [a.kind, a.id, 'mission', 'dispatch', 'starten'].filter(Boolean),
      // Prefills the dispatch form's agent field via the query param
      // MissionDispatchPage reads on mount.
      action: () => ctx.navigate(`/missions/new?agent=${encodeURIComponent(a.name)}`),
    }));

export const sessionsProvider: PaletteProvider = (ctx): PaletteItem[] =>
  ctx.sessions.map(s => {
    const agentName = externalAgentFromMeta(s._meta);
    const title =
      meaningfulTitle(s) ?? ctx.t('palette.session_fallback', { shortId: s.sessionId.slice(0, 8) });
    return {
      id: `session:${s.sessionId}`,
      type: 'session',
      title,
      subtitle: agentName ?? undefined,
      icon: icon(MessageSquare),
      keywords: [s.sessionId, agentName ?? ''].filter(Boolean),
      action: () => ctx.navigate(`/chat/${s.sessionId}`),
    };
  });

export const workspaceProvider: PaletteProvider = (ctx): PaletteItem[] =>
  // When the workspace-roots API is absent (older serve 404) the snapshot is an
  // empty array, so this source simply yields nothing — never an error row.
  ctx.workspaceRoots.map(root => ({
    id: `workspace:${root.path}`,
    type: 'workspace',
    title: root.path,
    subtitle: root.default ? ctx.t('palette.workspace_default') : undefined,
    icon: icon(FolderOpen),
    keywords: [root.path, 'workspace', 'arbeitsbereich', 'root', 'explorer'],
    action: () => ctx.navigate('/chat'),
  }));

export const inboxProvider: PaletteProvider = (ctx): PaletteItem[] =>
  ctx.approvals.map(a => {
    const tool = [a.toolsName, a.toolName].map(p => p?.trim()).filter(Boolean).join(' / ');
    return {
      id: `inbox:${a.id}`,
      type: 'inbox',
      title: tool || ctx.t('palette.approval_fallback'),
      subtitle: a.agentName ?? a.policyName ?? ctx.t('palette.type_inbox'),
      icon: icon(Inbox),
      keywords: [a.agentName ?? '', a.missionId ?? '', a.policyName ?? '', a.toolName, a.toolsName].filter(
        Boolean,
      ),
      action: () => ctx.navigate('/inbox'),
    };
  });

/**
 * The provider registry. Order is irrelevant to ranking (fuzzy score + frecency
 * decide everything once a query is typed); it only affects the stable tie-break
 * for equally-scored items and the empty-query recents fallback.
 */
export const paletteProviders: PaletteProvider[] = [
  actionsProvider,
  inboxProvider,
  missionsProvider,
  fleetProvider,
  sessionsProvider,
  agentsProvider,
  workspaceProvider,
];

/** Runs every provider over the context and concatenates their items. */
export function buildPaletteItems(ctx: PaletteProviderContext): PaletteItem[] {
  return paletteProviders.flatMap(provider => provider(ctx));
}
