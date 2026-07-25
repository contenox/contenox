import {
  ChatMessage,
  ChatStreamingCaret,
  ChatStreamThinkingBox,
  chatTranscriptMarkdownComponents,
  ChatTranscriptStreamingPlaceholder,
  cn,
  Collapsible,
  diffLinesFromTexts,
  DiffView,
  InlineAttachments,
  InlineNotice,
  Span,
  TerminalOutput,
  ToolCallCard,
  type ToolCallCardProps,
} from '@contenox/ui';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import logoMarkLightUrl from '../../../assets/logo-mark-light.svg?url';
import logoMarkDarkUrl from '../../../assets/logo-mark.svg?url';
import type {
  AcpChatMessage,
  AcpErrorCard,
  AcpSessionState,
  AcpTerminalCard,
  AcpTimelineItem,
  AcpToolCallState,
} from '../../../hooks/acpSessionState';
import { acpFailureCopyKeys, classifyAcpExecutionError } from '../../../lib/acpFailureKind';
import { useTheme } from '../../../lib/ThemeProvider';
import {
  shouldShowStreamingCaret,
  shouldShowStreamingPlaceholder,
} from '../lib/streamingPresentation';
import { MissionAskCard } from './MissionAskCard';
import { PermissionCard } from './PermissionCard';

function toolCallCardStatus(
  status?: AcpToolCallState['status'],
): NonNullable<ToolCallCardProps['status']> {
  switch (status) {
    case 'in_progress':
      return 'running';
    case 'completed':
      return 'success';
    case 'failed':
      return 'error';
    case 'pending':
    default:
      return 'pending';
  }
}

/** Content-identity key for a diff entry — stable across a `tool_call_update` that replaces the whole `content` array wholesale, so React doesn't remount (and lose UI state on) an unchanged diff just because its array position shifted. Falls back to the positional index only when the path itself repeats within one call. */
function diffKey(path: string | undefined, indexOfKind: number): string {
  return `diff-${path ?? 'unnamed'}-${indexOfKind}`;
}

function locationKey(
  path: string | undefined,
  line: number | undefined,
  indexOfKind: number,
): string {
  return `loc-${path ?? 'unnamed'}-${line ?? ''}-${indexOfKind}`;
}

/**
 * The contenox agent's avatar mark, theme-paired exactly like `Layout.tsx`'s
 * header logo. Agent-name-gated (case-insensitive `contenox` match) so a
 * differently-named/fleet ACP agent still falls back to `ChatMessage`'s
 * default letter avatar instead of showing a mark that isn't its own.
 */
function useAssistantAvatar(agentName: string | null): ReactNode {
  const { theme } = useTheme();
  if (!agentName || !/contenox/i.test(agentName)) return undefined;
  const logoUrl = theme === 'dark' ? logoMarkDarkUrl : logoMarkLightUrl;
  return <img src={logoUrl} alt="" aria-hidden className="h-5 w-5" />;
}

function ThinkingHeader({ streaming }: { streaming: boolean | undefined }) {
  const { t } = useTranslation();
  return (
    <span className={cn('inline-flex items-center gap-1.5', streaming && 'animate-pulse')}>
      <span>
        {streaming ? t('acp_chat.thinking_streaming_label') : t('acp_chat.thinking_done_label')}
      </span>
    </span>
  );
}

function TranscriptMessage({
  message,
  agentName,
  isLatest,
}: {
  message: AcpChatMessage;
  agentName: string | null;
  isLatest: boolean;
}) {
  const { t } = useTranslation();
  const isUser = message.role === 'user';
  const roleLabel = isUser ? t('acp_chat.role_user') : (agentName ?? t('acp_chat.role_agent'));
  const avatar = useAssistantAvatar(isUser ? null : agentName);

  return (
    <ChatMessage
      role={message.role}
      roleLabel={roleLabel}
      avatar={avatar}
      isLatest={isLatest}
      latestLabel={t('acp_chat.latest_label')}
      // This transcript surface doesn't collapse plain messages — only thought
      // blocks and tool detail collapse (see the Collapsible below and
      // ToolCallCard's own detail toggle).
      collapsible={false}
      copyText={message.text || undefined}
      copyLabel={t('acp_chat.copy')}
      copiedLabel={t('acp_chat.copied')}>
      {message.thinking && (
        <Collapsible
          defaultOpen={false}
          title={<ThinkingHeader streaming={message.thinkingStreaming} />}
          className="mb-2">
          <ChatStreamThinkingBox className="mt-1">{message.thinking}</ChatStreamThinkingBox>
        </Collapsible>
      )}
      {message.text ? (
        <>
          <ReactMarkdown remarkPlugins={[remarkGfm]} components={chatTranscriptMarkdownComponents}>
            {message.text}
          </ReactMarkdown>
          {shouldShowStreamingCaret(message) && <ChatStreamingCaret />}
        </>
      ) : shouldShowStreamingPlaceholder(message) ? (
        <ChatTranscriptStreamingPlaceholder>
          {t('acp_chat.typing_label')}
        </ChatTranscriptStreamingPlaceholder>
      ) : null}
      {/* A message that IS a unit's question gets its answer box here: the unit is
          parked on the call that asked, so replying in place unblocks it. */}
      {message.missionAsk && (
        <MissionAskCard key={message.missionAsk.askId} ask={message.missionAsk} />
      )}
      {message.images && message.images.length > 0 && (
        // Image parts render after the flattened text (see AcpChatMessage.images),
        // via the shared inline-attachment image kind: constrained thumbnail,
        // click-to-expand dialog.
        <InlineAttachments
          attachments={message.images.map(img => ({
            kind: 'image' as const,
            data: img.data,
            mimeType: img.mimeType,
          }))}
          labels={{
            imageAttachment: t('acp_chat.image_attachment_alt'),
            expandImage: t('acp_chat.image_expand'),
            closeImage: t('acp_chat.image_dialog_close'),
          }}
        />
      )}
    </ChatMessage>
  );
}

/** Best-effort `$ command arg1 arg2` line from a `local_shell`/exec tool call's raw input (`{command, args}`, see `runtime/acpsvc/events.go` `summarizeToolCallArgs`). */
function shellCommandLine(rawInput: unknown): string | null {
  if (rawInput == null || typeof rawInput !== 'object') return null;
  const obj = rawInput as Record<string, unknown>;
  const command = typeof obj.command === 'string' ? obj.command : null;
  if (!command) return null;
  const args = Array.isArray(obj.args) ? obj.args.filter((a): a is string => typeof a === 'string') : [];
  return ['$', command, ...args].join(' ');
}

/** Shell tool output is a plain string (`json.RawMessage(jsonString(ev.Content))` on the backend); split into terminal lines. */
function shellOutputLines(rawOutput: unknown): string[] | null {
  return typeof rawOutput === 'string' ? rawOutput.split('\n') : null;
}

function ToolCallDetail({ toolCall }: { toolCall: AcpToolCallState }) {
  const { t } = useTranslation();
  const diffs = (toolCall.content ?? []).filter(c => c.type === 'diff');
  const other = (toolCall.content ?? []).filter(c => c.type !== 'diff');
  const hasRaw = toolCall.rawInput != null || toolCall.rawOutput != null || other.length > 0;

  // `execute`-kind calls (local_shell, run/exec tools) render like the actual
  // shell they ran in, not a JSON dump — same TerminalOutput component the
  // live `!`-passthrough terminal tab uses (packages/beam TerminalTab.tsx),
  // just fed this one call's command + output instead of a PTY stream.
  const shellLines =
    toolCall.kind === 'execute' && other.length === 0 ? shellOutputLines(toolCall.rawOutput) : null;
  if (shellLines) {
    const commandLine = shellCommandLine(toolCall.rawInput);
    return (
      <div className="space-y-3">
        <TerminalOutput
          lines={commandLine ? [commandLine, ...shellLines] : shellLines}
          maxHeight="15rem"
        />
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {diffs.map((d, i) => (
        <DiffView
          key={diffKey(d.path, i)}
          filePath={d.path ?? ''}
          lines={diffLinesFromTexts(d.oldText ?? '', d.newText ?? '')}
        />
      ))}
      {toolCall.locations && toolCall.locations.length > 0 && (
        <ul className="text-text-muted dark:text-dark-text-muted space-y-0.5">
          {toolCall.locations.map((loc, i) => (
            <li key={locationKey(loc.path, loc.line, i)} className="break-all">
              {loc.path}
              {loc.line ? `:${loc.line}` : ''}
            </li>
          ))}
        </ul>
      )}
      {hasRaw && (
        <Collapsible title={t('acp_chat.tool_raw_output')}>
          <pre className="mt-2 max-h-60 overflow-auto break-all whitespace-pre-wrap">
            {JSON.stringify(
              { input: toolCall.rawInput, output: toolCall.rawOutput, content: other },
              null,
              2,
            )}
          </pre>
        </Collapsible>
      )}
    </div>
  );
}

function TranscriptToolCall({ toolCall }: { toolCall: AcpToolCallState }) {
  const { t } = useTranslation();
  const diffs = (toolCall.content ?? []).filter(c => c.type === 'diff');
  const other = (toolCall.content ?? []).filter(c => c.type !== 'diff');
  const hasDetail =
    diffs.length > 0 ||
    other.length > 0 ||
    toolCall.rawInput != null ||
    toolCall.rawOutput != null ||
    (toolCall.locations?.length ?? 0) > 0;

  return (
    <ToolCallCard
      tool={toolCall.kind ?? 'tool'}
      title={toolCall.title ?? toolCall.toolCallId}
      status={toolCallCardStatus(toolCall.status)}
      statusLabels={{
        pending: t('acp_chat.tool_status_pending'),
        running: t('acp_chat.tool_status_running'),
        success: t('acp_chat.tool_status_success'),
        error: t('acp_chat.tool_status_error'),
      }}
      toggleDetailLabel={t('acp_chat.tool_toggle_detail')}
      detail={hasDetail ? <ToolCallDetail toolCall={toolCall} /> : undefined}
    />
  );
}

/**
 * A `!` passthrough line recorded in the transcript: a compact, collapsible
 * terminal-output excerpt (reuses the shared `terminal_excerpt` attachment). The
 * live/full stream lives in the terminal panel; this is the durable record.
 */
function TranscriptTerminal({ card }: { card: AcpTerminalCard }) {
  const { t } = useTranslation();
  return (
    <InlineAttachments
      attachments={[{ kind: 'terminal_excerpt', command: card.command, output: card.output }]}
      labels={{ terminalOutput: t('terminal.card_label') }}
    />
  );
}

/**
 * A failed turn, rendered inline in the transcript where it happened. Reuses
 * the same `classifyAcpExecutionError` taxonomy and `acpFailureCopyKeys`
 * mapping as the top recovery banner (see SessionBanners' `ExecutionErrorBanner`)
 * so a backend-unreachable / model-not-servable / workspace-required / generic
 * failure each gets a matching localized headline and plain-language body,
 * with the raw runtime error kept behind a collapsed disclosure. This is what
 * replaces the old silent dead-state: the chat can no longer just go quiet.
 */
function TranscriptError({ card }: { card: AcpErrorCard }) {
  const { t } = useTranslation();
  // Loosened `t` for the dynamic keys acpFailureCopyKeys returns (mirrors WorkspacePanel's `tk`).
  const tk = t as (key: string) => string;
  const kind = classifyAcpExecutionError(card.message);
  const copy = acpFailureCopyKeys(kind);
  const headline = copy.titleKey ? tk(copy.titleKey) : t('acp_chat.turn_failed_label');
  const hint = copy.descriptionKey ? tk(copy.descriptionKey) : null;
  return (
    <InlineNotice variant="error">
      <div className="flex flex-col gap-1.5">
        <Span className="font-medium">{headline}</Span>
        {hint && <Span className="text-sm">{hint}</Span>}
        {card.message && (
          <Collapsible defaultOpen={false} title={t('acp_chat.error_details_toggle')}>
            <p className="mt-1 max-h-40 overflow-y-auto text-xs [overflow-wrap:anywhere] break-words whitespace-pre-wrap">
              {card.message}
            </p>
          </Collapsible>
        )}
      </div>
    </InlineNotice>
  );
}

export interface TranscriptItemsProps {
  session: AcpSessionState;
  agentName: string | null;
  /** Answers this session's pending permission (see `PermissionCard`). The card is only rendered when `session.pendingPermission` is set. */
  onRespondPermission: (optionId: string) => void;
}

interface RenderItemContext {
  session: AcpSessionState;
  agentName: string | null;
  latestErrorItemId: string | null;
  pending: AcpSessionState['pendingPermission'];
  anchorId: string | null;
  onRespondPermission: (optionId: string) => void;
}

/** Renders ONE timeline item (plus its anchored permission card, if any). Shared by the flat path and the turn-grouped path below so both stay in sync. */
function renderItem(item: AcpTimelineItem, isLatest: boolean, ctx: RenderItemContext): ReactNode {
  const { session, agentName, latestErrorItemId, pending, anchorId, onRespondPermission } = ctx;
  let rendered: ReactNode = null;
  if (item.kind === 'message') {
    const message = session.messages[item.id];
    rendered = message ? (
      <TranscriptMessage key={`m-${item.id}`} message={message} agentName={agentName} isLatest={isLatest} />
    ) : null;
  } else if (item.kind === 'terminal') {
    const card = session.terminals[item.id];
    rendered = card ? <TranscriptTerminal key={`x-${item.id}`} card={card} /> : null;
  } else if (item.kind === 'error') {
    const card = session.errorCards[item.id];
    rendered =
      card && item.id !== latestErrorItemId ? <TranscriptError key={`e-${item.id}`} card={card} /> : null;
  } else {
    const toolCall = session.toolCalls[item.id];
    rendered = toolCall ? <TranscriptToolCall key={`t-${item.id}`} toolCall={toolCall} /> : null;
  }
  const anchorHere = pending && anchorId != null && item.kind === 'tool_call' && item.id === anchorId;
  if (!anchorHere) return rendered;
  // Return a keyed array (not a wrapper element) so the tool-call card keeps
  // its own stable key and is NOT remounted when the permission arrives or
  // resolves; the card is anchored as its immediate sibling.
  return [
    rendered,
    <PermissionCard key={`perm-${item.id}`} permission={pending} onRespond={onRespondPermission} />,
  ];
}

/** One maximal run of consecutive `session.items` sharing a defined `turnId`, or a lone untagged item. */
interface TimelineGroup {
  turnId: string | undefined;
  entries: Array<{ item: AcpTimelineItem; index: number }>;
}

function groupByTurn(items: AcpTimelineItem[]): TimelineGroup[] {
  const groups: TimelineGroup[] = [];
  for (const [index, item] of items.entries()) {
    const last = groups[groups.length - 1];
    if (last && item.turnId !== undefined && last.turnId === item.turnId) {
      last.entries.push({ item, index });
    } else {
      groups.push({ turnId: item.turnId, entries: [{ item, index }] });
    }
  }
  return groups;
}

/**
 * Renders `session.items` in arrival order (D4's unified timeline) —
 * messages via `ChatMessage`, tool calls via `ToolCallCard`. Overall order is
 * exactly `session.items`; the only derivation this component adds is
 * grouping ONE turn's `tool_call`/`terminal` steps into a collapsible trail
 * that sits above that turn's answer, instead of interleaving them as flat
 * siblings — the steps happen first chronologically, but the answer is what
 * the user actually reads, so it gets top billing once the steps are done.
 * Items with no `turnId` (session/load replay, user echoes, out-of-band
 * mission questions) render exactly as before, ungrouped.
 *
 * A pending permission request (`session.pendingPermission`) is rendered inline
 * as a `PermissionCard` anchored right after the tool-call item it belongs to
 * (matched by `toolCallId`, wherever that item ends up — flat or inside a
 * step trail), so the approve/deny surface lives chronologically where the
 * request happened instead of in a page-covering modal. When the pending
 * request references no tool-call item yet (it can arrive before its
 * `tool_call` update), the card falls back to the end of the transcript. The
 * card answers ONLY via its explicit buttons — there is no dismiss/deny-on-
 * outside-click path anywhere in this flow.
 */
export function TranscriptItems({ session, agentName, onRespondPermission }: TranscriptItemsProps) {
  const { t } = useTranslation();
  const pending = session.pendingPermission;
  const pendingToolCallId = pending?.toolCall.toolCallId ?? null;
  // Anchor the card after a real tool-call item only when one matches; otherwise
  // it renders once at the end (see the fallback below).
  const anchorId =
    pendingToolCallId != null &&
    session.items.some(it => it.kind === 'tool_call' && it.id === pendingToolCallId)
      ? pendingToolCallId
      : null;

  // The reducer's `prompt_error` case (acpSessionState.ts) sets `session.error`
  // AND appends this SAME failed turn's card to `errorCards` in one atomic
  // step, so for as long as `session.error` is non-null it is, by
  // construction, exactly this most-recently-added error item's message.
  // `ChatSessionTab` renders that live message once already, at the top, via
  // `ExecutionErrorBanner` — skipping its in-transcript twin here (by item id,
  // not by message text, so two DIFFERENT failed turns that happen to carry
  // identical wording still both get their own card) is what collapses the
  // "shown twice" bug into one presentation. `session.error` is cleared on the
  // next `prompt_start`/`session_reset`, at which point this card starts
  // rendering normally as part of the permanent history.
  const latestErrorItemId =
    session.error != null
      ? ([...session.items].reverse().find(it => it.kind === 'error')?.id ?? null)
      : null;

  const ctx: RenderItemContext = { session, agentName, latestErrorItemId, pending, anchorId, onRespondPermission };
  const render = (entry: { item: AcpTimelineItem; index: number }) =>
    renderItem(entry.item, entry.index === session.items.length - 1, ctx);

  return (
    <>
      {groupByTurn(session.items).map(group => {
        const steps = group.entries.filter(({ item }) => item.kind === 'tool_call' || item.kind === 'terminal');
        const rest = group.entries.filter(({ item }) => item.kind !== 'tool_call' && item.kind !== 'terminal');
        // Only worth a collapsible wrapper once there's both something that
        // happened (steps) AND something to read below it (the answer). A
        // steps-only group (still streaming, no text yet) or a message-only
        // group (no tool calls this turn) renders exactly like the flat path.
        if (group.turnId === undefined || steps.length === 0 || rest.length === 0) {
          return group.entries.map(render);
        }
        // Starts open while its own turn is still streaming (so the user can
        // watch it work), stays as the user last left it afterwards — no
        // forced auto-collapse animation, per the "emit, don't animate" rule.
        const stillStreaming = session.isPrompting && group.turnId === session.currentTurnId;
        return (
          <div key={`turn-${group.turnId}`} className="space-y-2">
            <Collapsible
              defaultOpen={stillStreaming}
              title={t('acp_chat.turn_steps_toggle', { count: steps.length })}>
              <div className="space-y-2">{steps.map(render)}</div>
            </Collapsible>
            {rest.map(render)}
          </div>
        );
      })}
      {pending && anchorId == null && (
        <PermissionCard key="perm-fallback" permission={pending} onRespond={onRespondPermission} />
      )}
    </>
  );
}
