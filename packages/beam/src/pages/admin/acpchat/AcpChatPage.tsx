import {
  Badge,
  Button,
  ChatComposer,
  ChatMessage,
  ChatThread,
  ChatTypingIndicator,
  Dialog,
  EmptyState,
  H2,
  InlineNotice,
  Panel,
  Span,
  Spinner,
  ToolCallCard,
  useChatScroll,
  type ToolCallCardProps,
} from '@contenox/ui';
import { useCallback, useState, type FormEvent } from 'react';
import type { AcpToolCallState } from '../../../hooks/acpSessionState';
import { useAcpSession } from '../../../hooks/useAcpSession';
import type { PermissionOption } from '../../../lib/acp';

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

function permissionOptionVariant(kind: PermissionOption['kind']): 'primary' | 'danger' | 'outline' {
  if (kind === 'allow_once' || kind === 'allow_always') return 'primary';
  if (kind === 'reject_once' || kind === 'reject_always') return 'danger';
  return 'outline';
}

/**
 * A new, opt-in, minimal chat surface that talks to the runtime over ACP via
 * the `/acp` WebSocket endpoint — proving the browser -> /acp -> runtime ->
 * React path end to end. All ACP protocol/state logic lives in
 * `useAcpSession`; this page only renders it, coexisting with (not
 * replacing) the console at `pages/admin/console/`.
 */
export default function AcpChatPage() {
  const acp = useAcpSession();
  const [draft, setDraft] = useState('');
  const { containerRef, endRef } = useChatScroll({ deps: [acp.messages, acp.toolCallOrder, acp.pendingPermission] });

  const handleSubmit = useCallback(
    (e: FormEvent) => {
      e.preventDefault();
      const text = draft.trim();
      if (!text) return;
      acp.sendPrompt(text);
      setDraft('');
    },
    [acp, draft],
  );

  const pendingPermission = acp.pendingPermission;
  const rejectOption = pendingPermission?.options.find(o => o.kind.startsWith('reject'));
  const handleDialogClose = useCallback(() => {
    // A permission gate shouldn't vanish unanswered — Escape/backdrop map to
    // the safe "reject" choice when one is offered; otherwise it's a no-op
    // and the user must pick an option explicitly.
    if (rejectOption) acp.respondPermission(rejectOption.optionId);
  }, [acp, rejectOption]);

  const statusLabel =
    acp.status === 'connecting' ? 'Connecting…' : acp.status === 'ready' ? 'Connected' : 'Connection error';
  const statusVariant = acp.status === 'ready' ? 'success' : acp.status === 'error' ? 'error' : 'secondary';
  const hasContent = acp.messages.length > 0 || acp.toolCallOrder.length > 0;

  return (
    <div className="bg-surface dark:bg-dark-surface flex h-full min-h-0 flex-col">
      <header className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div className="flex flex-wrap items-center gap-2">
          <H2>ACP Chat</H2>
          <Badge variant={statusVariant} size="sm">
            {statusLabel}
          </Badge>
          {acp.agentName && (
            <Span variant="muted" className="text-sm">
              {acp.agentName}
            </Span>
          )}
        </div>
        <div className="flex items-center gap-3">
          {acp.usage && (
            <Span variant="muted" className="text-xs tabular-nums">
              {acp.usage.used.toLocaleString()} / {acp.usage.size.toLocaleString()} tokens
            </Span>
          )}
          {acp.isPrompting && (
            <Button type="button" variant="ghost" size="sm" onClick={acp.cancel}>
              Stop
            </Button>
          )}
        </div>
      </header>

      {acp.status === 'error' && acp.error && (
        <InlineNotice variant="error" className="m-3">
          {acp.error}
        </InlineNotice>
      )}

      {acp.plan.length > 0 && (
        <Panel variant="surface" className="mx-3 mt-3">
          <Span variant="status" className="mb-2 block">
            Plan
          </Span>
          <ol className="space-y-1 text-sm">
            {acp.plan.map((entry, i) => (
              <li key={i} className="flex items-center gap-2">
                <Badge
                  size="sm"
                  variant={
                    entry.status === 'completed' ? 'success' : entry.status === 'in_progress' ? 'primary' : 'secondary'
                  }
                >
                  {entry.status}
                </Badge>
                <span className="text-text dark:text-dark-text">{entry.content}</span>
              </li>
            ))}
          </ol>
        </Panel>
      )}

      {acp.status === 'connecting' ? (
        <div className="flex flex-1 items-center justify-center">
          <Spinner size="lg" />
        </div>
      ) : !hasContent ? (
        <div className="m-auto">
          <EmptyState
            title="No messages yet"
            description="Say hello — this talks to the runtime over ACP, live."
          />
        </div>
      ) : (
        <ChatThread containerRef={containerRef} endRef={endRef}>
          {acp.messages.map((message, i) => (
            <ChatMessage
              key={message.id}
              role={message.role}
              roleLabel={message.role === 'user' ? 'You' : (acp.agentName ?? 'Agent')}
              isLatest={i === acp.messages.length - 1}
            >
              {message.text ? (
                message.text
              ) : message.streaming ? (
                <ChatTypingIndicator aria-label="Assistant is typing" />
              ) : (
                ''
              )}
            </ChatMessage>
          ))}
          {acp.toolCallOrder.map(id => {
            const toolCall = acp.toolCalls[id];
            if (!toolCall) return null;
            const hasDetail = toolCall.rawInput != null || toolCall.rawOutput != null || toolCall.content != null;
            return (
              <ToolCallCard
                key={id}
                tool={toolCall.kind ?? 'tool'}
                title={toolCall.title ?? toolCall.toolCallId}
                status={toolCallCardStatus(toolCall.status)}
                detail={
                  hasDetail ? (
                    <pre className="whitespace-pre-wrap">
                      {JSON.stringify(
                        { input: toolCall.rawInput, output: toolCall.rawOutput, content: toolCall.content },
                        null,
                        2,
                      )}
                    </pre>
                  ) : undefined
                }
              />
            );
          })}
        </ChatThread>
      )}

      <ChatComposer
        value={draft}
        onChange={setDraft}
        onSubmit={handleSubmit}
        isPending={acp.isPrompting}
        disabled={acp.status !== 'ready'}
        placeholder={acp.status === 'ready' ? 'Message the agent…' : 'Connecting…'}
      />

      {/* The permission gate — the signature interaction: shows exactly what
          the agent wants to do and a button per option, keyboard-operable
          (Dialog traps focus and binds Escape). */}
      <Dialog open={pendingPermission !== null} onClose={handleDialogClose} title="Permission requested" className="w-[480px]">
        {pendingPermission && (
          <div className="space-y-4">
            <div>
              <Span variant="status" className="text-text-muted dark:text-dark-text-muted">
                {pendingPermission.toolCall.kind ?? 'action'}
              </Span>
              <p className="text-text dark:text-dark-text mt-1 text-sm font-medium">
                {pendingPermission.toolCall.title ?? pendingPermission.toolCall.toolCallId}
              </p>
            </div>

            {pendingPermission.toolCall.locations && pendingPermission.toolCall.locations.length > 0 && (
              <ul className="text-text-muted dark:text-dark-text-muted space-y-0.5 text-xs">
                {pendingPermission.toolCall.locations.map((loc, i) => (
                  <li key={i}>
                    {loc.path}
                    {loc.line ? `:${loc.line}` : ''}
                  </li>
                ))}
              </ul>
            )}

            {pendingPermission.toolCall.rawInput != null && (
              <pre className="bg-surface-100 dark:bg-dark-surface-300 text-text dark:text-dark-text max-h-40 overflow-auto rounded p-2 text-xs">
                {JSON.stringify(pendingPermission.toolCall.rawInput, null, 2)}
              </pre>
            )}

            <div className="flex flex-wrap justify-end gap-2">
              {pendingPermission.options.map(option => (
                <Button
                  key={option.optionId}
                  type="button"
                  variant={permissionOptionVariant(option.kind)}
                  onClick={() => acp.respondPermission(option.optionId)}
                >
                  {option.name}
                </Button>
              ))}
            </div>
          </div>
        )}
      </Dialog>
    </div>
  );
}
