import {
  ApprovalCard,
  chatTranscriptMarkdownComponents,
  ExecutionTimeline,
  InlineAttachments,
  Span,
  Spinner,
} from '@contenox/ui';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { ChatMessage } from '../../chats/components/ChatMessage';
import { useExecutionEvents } from '../../../../hooks/useExecutionEvents';
import { useExecutionState } from '../../../../hooks/useExecutionState';
import { deriveApprovals } from '../../../../lib/approvals';
import { extractDiffs } from '../../../../lib/diffs';
import type { TaskEventViewState } from '../../../../lib/taskEvents';
import type { ChatMessage as ChatMessageModel } from '../../../../lib/types';
import { isFailureAnnotation, type ConsoleTurn } from '../consoleTurns';
import { DiffBlock } from './DiffBlock';

type TurnBlockProps = {
  turn: ConsoleTurn;
  /** Retained run state for this turn (live or completed this session). */
  run?: TaskEventViewState | null;
  onRespondApproval?: (approvalId: string, approved: boolean) => void;
};

/** toolCallId → function name, resolved from the turn's assistant tool-call requests. */
function buildToolNameLookup(work: ChatMessageModel[]): Map<string, string> {
  const map = new Map<string, string>();
  for (const m of work) {
    for (const call of m.callTools ?? []) {
      if (call.id && call.function?.name) {
        map.set(call.id, call.function.name);
      }
    }
  }
  return map;
}

/**
 * One console turn: command echo → work log → result.
 * The terminal-agent scrollback unit; work stays visible after completion.
 */
export function TurnBlock({ turn, run, onRespondApproval }: TurnBlockProps) {
  const toolNames = buildToolNameLookup(turn.work);
  const retainedEvents = run?.events && run.events.length > 0 ? run.events : undefined;

  // Hydration precedence for past turns: retained live events (this session)
  // → durable event journal (full work log: tool calls, diffs, approvals)
  // → captured state units (step summaries) → graceful absence note.
  const wantsHydration = !retainedEvents && !turn.live && !!turn.requestId;
  const { data: journalEvents, isSuccess: journalDone } = useExecutionEvents(
    turn.requestId,
    wantsHydration,
  );
  const hydratedEvents =
    !retainedEvents && journalEvents && journalEvents.length > 0 ? journalEvents : undefined;
  const events = retainedEvents ?? hydratedEvents;

  const {
    data: hydratedState,
    isLoading: hydrating,
    isSuccess: hydrated,
  } = useExecutionState(turn.requestId, wantsHydration && journalDone && !hydratedEvents);
  const state = !events && hydratedState && hydratedState.length > 0 ? hydratedState : undefined;
  const headerUnit = state?.find(u => u.modelName) ?? state?.[state.length - 1];
  const tokenTotal = state?.reduce((sum, u) => sum + (u.tokenUsage?.total ?? 0), 0) ?? 0;
  const diffs = events ? extractDiffs(events) : [];

  return (
    <div className="border-surface-200 dark:border-dark-surface-300 border-b py-3 first:pt-1 last:border-b-0">
      {turn.command && (
        <div className="flex items-start gap-2 font-mono text-sm">
          <Span variant="muted" className="select-none pt-px font-mono">
            &gt;
          </Span>
          <div className="min-w-0 flex-1 whitespace-pre-wrap break-words">
            {turn.command.content}
            <InlineAttachments attachments={turn.command.attachments} />
          </div>
        </div>
      )}

      {(turn.chainRef || headerUnit || tokenTotal > 0) && (
        <div className="mt-1 flex flex-wrap items-center gap-x-3 pl-4 font-mono text-[10px]">
          {turn.chainRef && <Span variant="muted">{turn.chainRef}</Span>}
          {headerUnit?.modelName && <Span variant="muted">{headerUnit.modelName}</Span>}
          {tokenTotal > 0 && <Span variant="muted">{tokenTotal.toLocaleString()} tok</Span>}
        </div>
      )}

      {(turn.work.length > 0 || events || state || hydrating) && (
        <div className="mt-2 space-y-2 pl-4">
          {turn.work.map((m, idx) =>
            m.role === 'assistant' && isFailureAnnotation(m.content) ? (
              <div
                key={m.id ?? idx}
                className="text-error dark:text-dark-error font-mono text-xs whitespace-pre-wrap">
                {m.content}
              </div>
            ) : (
              <ChatMessage
                key={m.id ?? idx}
                message={m}
                toolName={m.toolCallId ? toolNames.get(m.toolCallId) : undefined}
              />
            ),
          )}
          {events && <ExecutionTimeline events={events} />}
          {diffs.map((d, idx) => (
            <DiffBlock key={`${d.path}-${idx}`} diff={d} />
          ))}
          {events &&
            deriveApprovals(events, run?.pendingApproval ?? null)
              .filter(a => a.status !== 'pending')
              .map(a => (
                <div key={a.approvalId} className="font-mono text-[11px]">
                  <Span variant="muted">
                    ⏸ approval · {a.toolName || 'tool'} —{' '}
                  </Span>
                  <Span
                    variant={a.status === 'resolved' ? 'muted' : undefined}
                    className={
                      a.status === 'denied'
                        ? 'text-error dark:text-dark-error'
                        : a.status === 'approved'
                          ? 'text-success'
                          : undefined
                    }>
                    {a.status}
                  </Span>
                </div>
              ))}
          {state && <ExecutionTimeline state={state} />}
          {!events && !state && hydrating && (
            <Span variant="muted" className="font-mono text-[10px]">
              loading execution log…
            </Span>
          )}
          {!events && !state && hydrated && (
            <Span variant="muted" className="font-mono text-[10px]">
              execution log unavailable (expired or pre-upgrade turn)
            </Span>
          )}
        </div>
      )}

      {turn.live && run?.pendingApproval && onRespondApproval && (
        <div className="mt-2 pl-4">
          <ApprovalCard
            approval={run.pendingApproval}
            onRespond={async approved => {
              if (!run.pendingApproval) return;
              onRespondApproval(run.pendingApproval.approvalId, approved);
            }}
          />
        </div>
      )}

      {turn.live && !turn.result && (
        <div className="mt-2 flex items-center gap-2 pl-4">
          <Spinner size="sm" />
          <Span variant="muted" className="font-mono text-xs">
            {run?.status || '…'}
          </Span>
        </div>
      )}

      {turn.result && (
        <div className="mt-2 pl-4 text-sm">
          {isFailureAnnotation(turn.result.content) ? (
            <div className="text-error dark:text-dark-error font-mono text-xs whitespace-pre-wrap">
              {turn.result.content}
            </div>
          ) : (
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={chatTranscriptMarkdownComponents}>
              {turn.result.content}
            </ReactMarkdown>
          )}
          <InlineAttachments attachments={turn.result.attachments} />
        </div>
      )}
    </div>
  );
}
