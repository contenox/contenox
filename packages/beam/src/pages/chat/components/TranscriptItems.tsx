import {
  ChatMessage,
  ChatStreamingCaret,
  ChatStreamThinkingBox,
  ChatTranscriptStreamingPlaceholder,
  chatTranscriptMarkdownComponents,
  cn,
  Collapsible,
  DiffView,
  diffLinesFromTexts,
  InlineAttachments,
  InlineNotice,
  Span,
  ToolCallCard,
  type ToolCallCardProps,
} from '@contenox/ui';
import type { ReactNode } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { useTranslation } from 'react-i18next';
import logoMarkDarkUrl from '../../../assets/logo-mark.svg?url';
import logoMarkLightUrl from '../../../assets/logo-mark-light.svg?url';
import type { AcpChatMessage, AcpErrorCard, AcpSessionState, AcpTerminalCard, AcpToolCallState } from '../../../hooks/acpSessionState';
import { classifyAcpExecutionError } from '../../../lib/acpFailureKind';
import { useTheme } from '../../../lib/ThemeProvider';
import { shouldShowStreamingCaret, shouldShowStreamingPlaceholder } from '../lib/streamingPresentation';
import { PermissionCard } from './PermissionCard';

function toolCallCardStatus(status?: AcpToolCallState['status']): NonNullable<ToolCallCardProps['status']> {
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

function locationKey(path: string | undefined, line: number | undefined, indexOfKind: number): string {
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
      <span>{streaming ? t('acp_chat.thinking_streaming_label') : t('acp_chat.thinking_done_label')}</span>
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
      copiedLabel={t('acp_chat.copied')}
    >
      {message.thinking && (
        <Collapsible defaultOpen={false} title={<ThinkingHeader streaming={message.thinkingStreaming} />} className="mb-2">
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
        <ChatTranscriptStreamingPlaceholder>{t('acp_chat.typing_label')}</ChatTranscriptStreamingPlaceholder>
      ) : null}
      {message.images && message.images.length > 0 && (
        // Image parts render after the flattened text (see AcpChatMessage.images),
        // via the shared inline-attachment image kind: constrained thumbnail,
        // click-to-expand dialog.
        <InlineAttachments
          attachments={message.images.map(img => ({ kind: 'image' as const, data: img.data, mimeType: img.mimeType }))}
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

function ToolCallDetail({ toolCall }: { toolCall: AcpToolCallState }) {
  const { t } = useTranslation();
  const diffs = (toolCall.content ?? []).filter(c => c.type === 'diff');
  const other = (toolCall.content ?? []).filter(c => c.type !== 'diff');
  const hasRaw = toolCall.rawInput != null || toolCall.rawOutput != null || other.length > 0;

  return (
    <div className="space-y-3">
      {diffs.map((d, i) => (
        <DiffView key={diffKey(d.path, i)} filePath={d.path ?? ''} lines={diffLinesFromTexts(d.oldText ?? '', d.newText ?? '')} />
      ))}
      {toolCall.locations && toolCall.locations.length > 0 && (
        <ul className="text-text-muted dark:text-dark-text-muted space-y-0.5">
          {toolCall.locations.map((loc, i) => (
            <li key={locationKey(loc.path, loc.line, i)}>
              {loc.path}
              {loc.line ? `:${loc.line}` : ''}
            </li>
          ))}
        </ul>
      )}
      {hasRaw && (
        <Collapsible title={t('acp_chat.tool_raw_output')}>
          <pre className="mt-2 max-h-60 overflow-auto whitespace-pre-wrap break-all">
            {JSON.stringify({ input: toolCall.rawInput, output: toolCall.rawOutput, content: other }, null, 2)}
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
 * the same `classifyAcpExecutionError` taxonomy as the top recovery banner (see
 * SessionBanners' `ExecutionErrorBanner`) so a backend-unreachable /
 * model-not-servable / generic failure each gets a matching localized headline,
 * with the raw runtime error kept behind a collapsed disclosure. This is what
 * replaces the old silent dead-state: the chat can no longer just go quiet.
 */
function TranscriptError({ card }: { card: AcpErrorCard }) {
  const { t } = useTranslation();
  const kind = classifyAcpExecutionError(card.message);
  const headline =
    kind === 'backend_unreachable'
      ? t('acp_recovery.backend_unreachable_title')
      : kind === 'model_unavailable'
        ? t('acp_recovery.model_unavailable_title')
        : t('acp_chat.turn_failed_label');
  return (
    <InlineNotice variant="error">
      <div className="flex flex-col gap-1">
        <Span className="font-medium">{headline}</Span>
        {card.message && (
          <Collapsible defaultOpen={false} title={t('acp_chat.error_details_toggle')}>
            <p className="mt-1 text-xs whitespace-pre-wrap">{card.message}</p>
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

/**
 * Renders `session.items` in arrival order (D4's unified timeline) —
 * messages via `ChatMessage`, tool calls via `ToolCallCard`. Order is exactly
 * `session.items`; this component adds no derivation of its own.
 *
 * A pending permission request (`session.pendingPermission`) is rendered inline
 * as a `PermissionCard` anchored right after the tool-call item it belongs to
 * (matched by `toolCallId`), so the approve/deny surface lives chronologically
 * where the request happened instead of in a page-covering modal. When the
 * pending request references no tool-call item yet (it can arrive before its
 * `tool_call` update), the card falls back to the end of the transcript. The
 * card answers ONLY via its explicit buttons — there is no dismiss/deny-on-
 * outside-click path anywhere in this flow.
 */
export function TranscriptItems({ session, agentName, onRespondPermission }: TranscriptItemsProps) {
  const pending = session.pendingPermission;
  const pendingToolCallId = pending?.toolCall.toolCallId ?? null;
  // Anchor the card after a real tool-call item only when one matches; otherwise
  // it renders once at the end (see the fallback below).
  const anchorId =
    pendingToolCallId != null && session.items.some(it => it.kind === 'tool_call' && it.id === pendingToolCallId)
      ? pendingToolCallId
      : null;

  return (
    <>
      {session.items.map((item, i) => {
        const isLatest = i === session.items.length - 1;
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
          rendered = card ? <TranscriptError key={`e-${item.id}`} card={card} /> : null;
        } else {
          const toolCall = session.toolCalls[item.id];
          rendered = toolCall ? <TranscriptToolCall key={`t-${item.id}`} toolCall={toolCall} /> : null;
        }
        const anchorHere = pending && anchorId != null && item.kind === 'tool_call' && item.id === anchorId;
        if (!anchorHere) return rendered;
        // Return a keyed array (not a wrapper element) so the tool-call card keeps
        // its own stable key and is NOT remounted when the permission arrives or
        // resolves; the card is anchored as its immediate sibling.
        return [rendered, <PermissionCard key={`perm-${item.id}`} permission={pending} onRespond={onRespondPermission} />];
      })}
      {pending && anchorId == null && (
        <PermissionCard key="perm-fallback" permission={pending} onRespond={onRespondPermission} />
      )}
    </>
  );
}
