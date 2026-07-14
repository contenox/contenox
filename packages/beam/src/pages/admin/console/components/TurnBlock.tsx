import {
  ApprovalCard,
  DiffView,
  diffLinesFromTexts,
  InlineAttachments,
  TerminalLine,
  TerminalMeta,
  TERMINAL_GLYPH,
  terminalMarkdownComponents,
} from '@contenox/ui';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { useExecutionEvents } from '../../../../hooks/useExecutionEvents';
import { useExecutionState } from '../../../../hooks/useExecutionState';
import { deriveApprovals } from '../../../../lib/approvals';
import { extractDiffs } from '../../../../lib/diffs';
import type { TaskEventViewState } from '../../../../lib/taskEvents';
import type { CapturedStateUnit, ChatMessage as ChatMessageModel } from '../../../../lib/types';
import { isFailureAnnotation, type ConsoleTurn } from '../consoleTurns';
import { summarizeArgs } from '../term';

type TurnBlockProps = {
  turn: ConsoleTurn;
  /** Retained run state for this turn (live or completed this session). */
  run?: TaskEventViewState | null;
  onRespondApproval?: (approvalId: string, approved: boolean) => void;
};

type WorkLine =
  | { kind: 'call'; id?: string; head: string }
  | { kind: 'result'; text: string; error?: boolean }
  | { kind: 'error'; text: string };

/** Flattens the turn's persisted tool interactions into terminal scrollback lines. */
function buildWorkLines(work: ChatMessageModel[]): WorkLine[] {
  const lines: WorkLine[] = [];
  for (const m of work) {
    if (m.role === 'assistant' && m.callTools && m.callTools.length > 0) {
      for (const call of m.callTools) {
        const name = call.function?.name ?? 'tool';
        const args = summarizeArgs(call.function?.arguments);
        lines.push({ kind: 'call', id: call.id, head: `${name}(${args})` });
      }
      continue;
    }
    if (m.role === 'tool') {
      lines.push({ kind: 'result', text: m.content || '(no output)', error: !!m.error });
      continue;
    }
    if (m.role === 'assistant' && isFailureAnnotation(m.content)) {
      lines.push({ kind: 'error', text: m.content });
    }
  }
  return lines;
}

/** Compact per-step summary line from hydrated captured state (no live events). */
function stateLines(state: CapturedStateUnit[]): string[] {
  return state
    .filter(u => u.taskHandler !== 'route' || u.error?.error)
    .map(u => {
      const ms = Math.round(u.duration / 1_000_000);
      const status = u.error?.error ? `✗ ${u.error.error}` : (u.transition || 'ok');
      return `${u.taskID} · ${status} · ${ms}ms`;
    });
}

/**
 * One console turn rendered as terminal scrollback: `❯ command`, a dim meta
 * line, glyph-led work lines, unified diffs, approval verdicts, then the
 * result. Line-oriented and dense — no card chrome.
 */
export function TurnBlock({ turn, run, onRespondApproval }: TurnBlockProps) {
  const retainedEvents = run?.events && run.events.length > 0 ? run.events : undefined;

  // Hydration precedence: retained live events → durable journal → state summary → note.
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

  const workLines = buildWorkLines(turn.work);
  const diffs = events ? extractDiffs(events) : [];
  const approvals = events
    ? deriveApprovals(events, run?.pendingApproval ?? null).filter(a => a.status !== 'pending')
    : [];
  const metaBits = [turn.chainRef, headerUnit?.modelName, tokenTotal > 0 ? `${tokenTotal.toLocaleString()} tok` : '']
    .filter(Boolean)
    .join('  ·  ');

  const resultIsError = turn.result ? isFailureAnnotation(turn.result.content) : false;

  return (
    <div className="py-1.5 text-[13px]">
      {/* Command line */}
      {turn.command && (
        <TerminalLine glyph={TERMINAL_GLYPH.prompt}>
          {turn.command.content}
          <InlineAttachments attachments={turn.command.attachments} />
        </TerminalLine>
      )}

      {metaBits && <TerminalMeta>{metaBits}</TerminalMeta>}

      {/* Work log */}
      {(workLines.length > 0 || approvals.length > 0 || diffs.length > 0) && (
        <div className="mt-1 space-y-0.5">
          {workLines.map((l, i) =>
            l.kind === 'call' ? (
              <TerminalLine key={i} glyph={TERMINAL_GLYPH.tool} indent={1}>
                {l.head}
              </TerminalLine>
            ) : l.kind === 'error' ? (
              <TerminalLine key={i} tone="error" indent={2}>
                {l.text}
              </TerminalLine>
            ) : (
              <TerminalLine
                key={i}
                glyph={TERMINAL_GLYPH.cont}
                glyphTone="muted"
                tone={l.error ? 'error' : 'muted'}
                indent={1}>
                {l.text.length > 600 ? l.text.slice(0, 600) + '…' : l.text}
              </TerminalLine>
            ),
          )}

          {diffs.map((d, i) => (
            <DiffView
              key={`${d.path}-${i}`}
              filePath={d.path}
              lines={diffLinesFromTexts(d.oldText, d.newText)}
              className="my-1 ml-3"
            />
          ))}

          {approvals.map(a => (
            <TerminalLine key={a.approvalId} glyph={TERMINAL_GLYPH.approval} glyphTone="muted" indent={1}>
              <span className="text-text-muted dark:text-dark-text-muted">
                {a.toolName || 'tool'} —{' '}
              </span>
              <span
                className={
                  a.status === 'denied'
                    ? 'text-error dark:text-dark-error'
                    : a.status === 'approved'
                      ? 'text-success dark:text-dark-success'
                      : 'text-text-muted dark:text-dark-text-muted'
                }>
                {a.status}
              </span>
            </TerminalLine>
          ))}
        </div>
      )}

      {/* Hydrated state summary (no journaled events available) */}
      {state && (
        <div className="mt-1">
          {stateLines(state).map((l, i) => (
            <TerminalMeta key={i}>{l}</TerminalMeta>
          ))}
        </div>
      )}
      {!events && !state && hydrating && (
        <TerminalMeta className="mt-1">loading execution log…</TerminalMeta>
      )}
      {!events && !state && hydrated && !turn.live && (
        <TerminalMeta className="mt-1">
          execution log unavailable (expired or pre-upgrade turn)
        </TerminalMeta>
      )}

      {/* Inline approval prompt (live) */}
      {turn.live && run?.pendingApproval && onRespondApproval && (
        <div className="mt-1.5 pl-3">
          <ApprovalCard
            approval={run.pendingApproval}
            onRespond={async approved => {
              if (!run.pendingApproval) return;
              onRespondApproval(run.pendingApproval.approvalId, approved);
            }}
          />
        </div>
      )}

      {/* Live running indicator */}
      {turn.live && !turn.result && !run?.pendingApproval && (
        <TerminalMeta className="mt-1 text-[12px]">
          <span className="animate-pulse">{TERMINAL_GLYPH.running}</span> {run?.status || 'working…'}
        </TerminalMeta>
      )}

      {/* Result */}
      {turn.result &&
        (resultIsError ? (
          <TerminalLine tone="error" indent={2} className="mt-1">
            {turn.result.content}
          </TerminalLine>
        ) : (
          <div className="mt-1 pl-5 font-mono text-[13px] text-text dark:text-dark-text">
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={terminalMarkdownComponents}>
              {turn.result.content}
            </ReactMarkdown>
            <InlineAttachments attachments={turn.result.attachments} />
          </div>
        ))}
    </div>
  );
}
