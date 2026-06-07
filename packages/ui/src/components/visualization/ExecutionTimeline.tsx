import { useMemo } from "react";
import { Activity, AlertCircle, CheckCircle2, Settings } from "lucide-react";
import { Badge } from "../Badge";
import { Collapsible } from "../Collapsible";
import { Table, TableCell, TableRow } from "../Table";
import { Span } from "../Typography";
import { cn } from "../../utils";
import type { TaskEvent, CapturedStateUnit } from "./taskTypes";

export interface ExecutionTimelineLabels {
  executionLog?: string;
  initializingPlan?: string;
  awaitingApproval?: string;
  showState?: (count: number) => string;
  taskId?: string;
  taskType?: string;
  transition?: string;
  duration?: string;
  error?: string;
}

const DEFAULT_LABELS: Required<ExecutionTimelineLabels> = {
  executionLog: "Execution Log",
  initializingPlan: "Initializing Plan",
  awaitingApproval: "Awaiting Approval",
  showState: (count) => `Show State Logs (${count})`,
  taskId: "Task",
  taskType: "Type",
  transition: "Transition",
  duration: "Duration",
  error: "Error",
};

export interface ExecutionTimelineProps {
  events?: TaskEvent[];
  state?: CapturedStateUnit[];
  labels?: ExecutionTimelineLabels;
}

export function ExecutionTimeline({ events, state, labels }: ExecutionTimelineProps) {
  if ((!events || events.length === 0) && (!state || state.length === 0)) {
    return null;
  }
  const l = { ...DEFAULT_LABELS, ...labels };

  return (
    <div className="flex flex-col gap-2 pt-3 border-t border-surface-300 dark:border-dark-surface-400">
      {events && events.length > 0 && <LiveTaskEvents events={events} l={l} />}
      {state && state.length > 0 && (!events || events.length === 0) && (
        <HistoricalState state={state} l={l} />
      )}
    </div>
  );
}

function LiveTaskEvents({
  events,
  l,
}: {
  events: TaskEvent[];
  l: Required<ExecutionTimelineLabels>;
}) {
  const steps = useMemo(() => {
    const groups: { id: string; events: TaskEvent[] }[] = [];
    let currentId: string | null = null;
    for (const e of events) {
      const stepId = e.task_id || e.task_handler || "system";
      if (stepId !== currentId) {
        currentId = stepId;
        groups.push({ id: stepId, events: [] });
      }
      groups[groups.length - 1].events.push(e);
    }
    return groups;
  }, [events]);

  return (
    <div className="flex flex-col gap-2 text-sm">
      <div className="flex items-center gap-2 text-text-muted font-medium px-1">
        <Activity size={14} />
        <Span>{l.executionLog}</Span>
      </div>
      {steps.map((group, idx) => (
        <StepCollapsible key={`${group.id}-${idx}`} group={group} l={l} />
      ))}
    </div>
  );
}

function StepCollapsible({
  group,
  l,
}: {
  group: { id: string; events: TaskEvent[] };
  l: Required<ExecutionTimelineLabels>;
}) {
  const events = group.events;
  const isError = events.some((e) => e.kind === "step_failed" || e.kind === "chain_failed");
  const isDone = events.some((e) => e.kind === "step_completed" || e.kind === "chain_completed");
  const transitionEvent = events.find((e) => !!e.transition);

  let title = group.id;
  if (title === "system" && events.some((e) => e.kind === "chain_started")) {
    title = l.initializingPlan;
  } else if (events.some((e) => e.kind === "approval_requested")) {
    title = l.awaitingApproval;
  }

  const TitleElement = (
    <div className="flex items-center gap-2">
      <span className="flex-shrink-0">
        {isError ? (
          <AlertCircle size={14} className="text-error" />
        ) : isDone ? (
          <CheckCircle2 size={14} className="text-success" />
        ) : (
          <Settings size={14} className="text-text-muted dark:text-dark-text-muted animate-spin-slow" />
        )}
      </span>
      <Span className="font-mono text-xs font-semibold">{title}</Span>
      {transitionEvent && transitionEvent.transition && (
        <Badge variant="outline" size="sm" className="text-[10px] py-0">
          {transitionEvent.transition}
        </Badge>
      )}
    </div>
  );

  return (
    <Collapsible title={TitleElement} className="bg-background">
      <div className="p-3 font-mono text-[11px] overflow-x-auto whitespace-pre bg-surface-50 dark:bg-dark-surface-50 rounded-b-md">
        {events.map((e, idx) => (
          <div key={idx} className="flex gap-2">
            <Span className="text-text-muted opacity-50 shrink-0">
              {new Date(e.timestamp).toLocaleTimeString([], { hour12: false })}
            </Span>
            <Span className={cn(e.error ? "text-error font-medium" : "text-text dark:text-dark-text")}>
              {e.kind}
              {e.task_handler && e.task_handler !== group.id ? ` [${e.task_handler}]` : ""}
              {e.error ? ` - ${e.error}` : ""}
            </Span>
          </div>
        ))}
      </div>
    </Collapsible>
  );
}

function HistoricalState({
  state,
  l,
}: {
  state: CapturedStateUnit[];
  l: Required<ExecutionTimelineLabels>;
}) {
  const formatDuration = (ms: number): string => {
    if (ms < 1000) return `${Math.round(ms)} ms`;
    return `${(ms / 1000).toFixed(2)} s`;
  };

  const TitleElement = (
    <div className="flex items-center gap-2">
      <Activity size={14} />
      <Span className="font-medium text-xs">{l.showState(state.length)}</Span>
    </div>
  );

  return (
    <Collapsible title={TitleElement} className="mt-1">
      <div className="border border-surface-300 dark:border-dark-surface-400 rounded-b-md overflow-x-auto">
        <Table
          columns={[l.taskId, l.taskType, l.transition, l.duration, l.error]}>
          {state.map((unit, index) => (
            <TableRow key={index}>
              <TableCell className="font-mono text-xs">{unit.taskID}</TableCell>
              <TableCell className="text-xs">{unit.taskType}</TableCell>
              <TableCell className="max-w-xs truncate text-xs">{unit.transition || "-"}</TableCell>
              <TableCell className="text-xs">{formatDuration(unit.duration)}</TableCell>
              <TableCell className="text-xs">
                {unit.error?.error ? (
                  <Badge variant="error" size="sm">
                    {unit.error.error}
                  </Badge>
                ) : (
                  "-"
                )}
              </TableCell>
            </TableRow>
          ))}
        </Table>
      </div>
    </Collapsible>
  );
}
