import {
  ChatMessage,
  ChatTypingIndicator,
  Collapsible,
  DiffView,
  diffLinesFromTexts,
  ToolCallCard,
  type ToolCallCardProps,
} from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import type { AcpChatMessage, AcpSessionState, AcpToolCallState } from '../../../hooks/acpSessionState';

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

  return (
    <ChatMessage
      role={message.role}
      roleLabel={roleLabel}
      isLatest={isLatest}
      latestLabel={t('acp_chat.latest_label')}
      copyText={message.text || undefined}
      copyLabel={t('acp_chat.copy')}
      copiedLabel={t('acp_chat.copied')}
    >
      {message.thinking && (
        <Collapsible defaultOpen={false} title={t('acp_chat.thinking_label')} className="mb-2">
          <p className="text-text-muted dark:text-dark-text-muted mt-1 text-xs whitespace-pre-wrap">
            {message.thinking}
          </p>
        </Collapsible>
      )}
      {message.text ? (
        <p className="whitespace-pre-wrap">{message.text}</p>
      ) : message.streaming ? (
        <ChatTypingIndicator aria-label={t('acp_chat.typing_label')} />
      ) : null}
      {message.text && message.streaming && <ChatTypingIndicator aria-label={t('acp_chat.typing_label')} className="mt-1 px-0" />}
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
        <DiffView key={i} filePath={d.path ?? ''} lines={diffLinesFromTexts(d.oldText ?? '', d.newText ?? '')} />
      ))}
      {toolCall.locations && toolCall.locations.length > 0 && (
        <ul className="text-text-muted dark:text-dark-text-muted space-y-0.5">
          {toolCall.locations.map((loc, i) => (
            <li key={i}>
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
      detail={hasDetail ? <ToolCallDetail toolCall={toolCall} /> : undefined}
    />
  );
}

export interface TranscriptItemsProps {
  session: AcpSessionState;
  agentName: string | null;
}

/**
 * Renders `session.items` in arrival order (D4's unified timeline) —
 * messages via `ChatMessage`, tool calls via `ToolCallCard`. Order is exactly
 * `session.items`; this component adds no derivation of its own.
 */
export function TranscriptItems({ session, agentName }: TranscriptItemsProps) {
  return (
    <>
      {session.items.map((item, i) => {
        const isLatest = i === session.items.length - 1;
        if (item.kind === 'message') {
          const message = session.messages[item.id];
          if (!message) return null;
          return <TranscriptMessage key={`m-${item.id}`} message={message} agentName={agentName} isLatest={isLatest} />;
        }
        const toolCall = session.toolCalls[item.id];
        if (!toolCall) return null;
        return <TranscriptToolCall key={`t-${item.id}`} toolCall={toolCall} />;
      })}
    </>
  );
}
