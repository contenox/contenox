import { Span } from "../Typography";
import { TerminalOutput } from "../TerminalOutput";
import type { TaskEvent } from "./taskTypes";

const TAIL = 40;

function extrasLine(e: TaskEvent): string | null {
  const parts: string[] = [];
  if (e.request_id) parts.push(`request=${e.request_id}`);
  if (e.chain_id) parts.push(`chain=${e.chain_id}`);
  if (e.task_handler) parts.push(`handler=${e.task_handler}`);
  if (e.model_name) parts.push(`model=${e.model_name}`);
  if (e.transition) parts.push(`transition=${e.transition}`);
  if (e.content && e.kind === "step_chunk") {
    const c = e.content.trim().replace(/\s+/g, " ");
    parts.push(c.length > 120 ? `${c.slice(0, 120)}…` : c);
  }
  return parts.length ? parts.join(" ") : null;
}

function eventToLines(e: TaskEvent): string[] {
  const base = `[${e.timestamp}] ${e.kind}${e.task_id ? ` ${e.task_id}` : ""}`;
  const lines = [base];
  const extra = extrasLine(e);
  if (extra) lines.push(`  ${extra}`);
  if (e.error) lines.push(`  error: ${e.error}`);
  return lines;
}

export interface TaskEventFeedProps {
  events: TaskEvent[];
  omittedLabel?: (count: number) => string;
}

const DEFAULT_OMITTED_LABEL = (count: number) =>
  `… ${count} earlier event${count === 1 ? "" : "s"} omitted`;

export function TaskEventFeed({ events, omittedLabel = DEFAULT_OMITTED_LABEL }: TaskEventFeedProps) {
  if (!events.length) return null;
  const tail = events.length > TAIL ? events.slice(-TAIL) : events;
  const omitted = events.length - tail.length;
  const lines = tail.flatMap(eventToLines);

  return (
    <div className="flex flex-col gap-1">
      {omitted > 0 ? (
        <Span variant="muted" className="text-[10px]">
          {omittedLabel(omitted)}
        </Span>
      ) : null}
      <TerminalOutput lines={lines} maxHeight="min(320px, 40vh)" className="min-h-[120px]" />
    </div>
  );
}
