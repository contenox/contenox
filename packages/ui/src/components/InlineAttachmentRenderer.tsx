import { stripAnsi } from "../ansi";
import { CodeBlock } from "./CodeBlock";
import { Collapsible } from "./Collapsible";
import { Span } from "./Typography";
import { FileText, TerminalSquare, ListChecks, Workflow, Database } from "lucide-react";

/**
 * Discriminated union representing every typed attachment kind that can be
 * rendered inline adjacent to a chat message. Mirrors the Go-side
 * `taskengine.WidgetHint` / artifact shapes.
 */
export type InlineAttachment =
  | { kind: "file_view"; path: string; text: string; truncated?: boolean }
  | {
      kind: "terminal_excerpt";
      output: string;
      command?: string;
      sessionId?: string;
      capturedAt?: string;
    }
  | {
      kind: "plan_summary";
      planId: string;
      ordinal: number;
      description: string;
      status: string;
      summary?: string;
      failureClass?: string;
    }
  | { kind: "dag"; chainJSON: string; description?: string }
  | { kind: "state_unit"; name: string; data?: unknown };

/**
 * Overridable UI strings for attachment titles and captions. Every entry is
 * optional and falls back to the built-in English default.
 */
export type InlineAttachmentLabels = {
  /** Singular line-count unit (default: "line"). */
  line?: string;
  /** Plural line-count unit (default: "lines"). */
  lines?: string;
  /** Marker appended when a file view is truncated (default: "truncated"). */
  truncated?: string;
  /** Title for terminal excerpts (default: "Terminal output"). */
  terminalOutput?: string;
  /** Title prefix before the plan-step ordinal (default: "Plan step"). */
  planStep?: string;
  /** Caption above the plan-step description (default: "Description"). */
  description?: string;
  /** Caption above the plan-step summary (default: "Summary"). */
  summary?: string;
  /** Fallback title for DAG attachments (default: "Compiled chain DAG"). */
  compiledChainDag?: string;
  /** Title for state-unit attachments (default: "Captured state"). */
  capturedState?: string;
  /** Placeholder when a state unit has no data (default: "(no data)"). */
  noData?: string;
};

export type InlineAttachmentRendererProps = {
  attachment: InlineAttachment;
  labels?: InlineAttachmentLabels;
};

function FileViewAttachment({
  attachment,
  labels,
}: {
  attachment: Extract<InlineAttachment, { kind: "file_view" }>;
  labels?: InlineAttachmentLabels;
}) {
  const lineCount = attachment.text.split("\n").length;
  return (
    <Collapsible
      title={
        <span className="inline-flex items-center gap-1.5 text-xs">
          <FileText className="h-3.5 w-3.5" />
          <span className="font-mono">{attachment.path}</span>
          <Span variant="muted" className="text-[10px]">
            {lineCount}{" "}
            {lineCount === 1
              ? (labels?.line ?? "line")
              : (labels?.lines ?? "lines")}
            {attachment.truncated
              ? ` · ${labels?.truncated ?? "truncated"}`
              : ""}
          </Span>
        </span>
      }
      defaultExpanded={false}
      className="border-surface-300 dark:border-dark-surface-400 bg-surface-100 dark:bg-dark-surface-200 mt-2 rounded border"
    >
      <CodeBlock className="px-3 py-2 leading-relaxed">{attachment.text}</CodeBlock>
    </Collapsible>
  );
}

function TerminalExcerptAttachment({
  attachment,
  labels,
}: {
  attachment: Extract<InlineAttachment, { kind: "terminal_excerpt" }>;
  labels?: InlineAttachmentLabels;
}) {
  return (
    <Collapsible
      title={
        <span className="inline-flex items-center gap-1.5 text-xs">
          <TerminalSquare className="h-3.5 w-3.5" />
          <span>{labels?.terminalOutput ?? "Terminal output"}</span>
          {attachment.command && (
            <Span variant="muted" className="font-mono text-[10px]">
              $ {stripAnsi(attachment.command)}
            </Span>
          )}
        </span>
      }
      defaultExpanded={false}
      className="border-surface-300 dark:border-dark-surface-400 bg-surface-100 dark:bg-dark-surface-200 mt-2 rounded border"
    >
      <CodeBlock className="px-3 py-2 leading-relaxed">{stripAnsi(attachment.output)}</CodeBlock>
    </Collapsible>
  );
}

function PlanSummaryAttachment({
  attachment,
  labels,
}: {
  attachment: Extract<InlineAttachment, { kind: "plan_summary" }>;
  labels?: InlineAttachmentLabels;
}) {
  const statusColor =
    attachment.status === "completed"
      ? "text-success"
      : attachment.status === "failed"
        ? "text-error"
        : "text-text-muted";
  return (
    <Collapsible
      title={
        <span className="inline-flex items-center gap-1.5 text-xs">
          <ListChecks className="h-3.5 w-3.5" />
          <span>
            {labels?.planStep ?? "Plan step"} {attachment.ordinal}
          </span>
          <Span variant="muted" className={`text-[10px] ${statusColor}`}>
            · {attachment.status}
            {attachment.failureClass ? ` (${attachment.failureClass})` : ""}
          </Span>
        </span>
      }
      defaultExpanded={false}
      className="border-surface-300 dark:border-dark-surface-400 bg-surface-100 dark:bg-dark-surface-200 mt-2 rounded border"
    >
      <div className="space-y-1.5 px-3 py-2 text-xs">
        <div>
          <Span variant="muted" className="text-[10px]">
            {labels?.description ?? "Description"}
          </Span>
          <div className="text-text dark:text-dark-text mt-0.5">{attachment.description}</div>
        </div>
        {attachment.summary && (
          <div>
            <Span variant="muted" className="text-[10px]">
              {labels?.summary ?? "Summary"}
            </Span>
            <CodeBlock className="mt-0.5 text-[11px] whitespace-pre-wrap">
              {attachment.summary}
            </CodeBlock>
          </div>
        )}
      </div>
    </Collapsible>
  );
}

function DAGAttachment({
  attachment,
  labels,
}: {
  attachment: Extract<InlineAttachment, { kind: "dag" }>;
  labels?: InlineAttachmentLabels;
}) {
  return (
    <Collapsible
      title={
        <span className="inline-flex items-center gap-1.5 text-xs">
          <Workflow className="h-3.5 w-3.5" />
          <span>
            {attachment.description ??
              labels?.compiledChainDag ??
              "Compiled chain DAG"}
          </span>
        </span>
      }
      defaultExpanded={false}
      className="border-surface-300 dark:border-dark-surface-400 bg-surface-100 dark:bg-dark-surface-200 mt-2 rounded border"
    >
      <CodeBlock className="px-3 py-2 text-[11px] leading-relaxed">{attachment.chainJSON}</CodeBlock>
    </Collapsible>
  );
}

function StateUnitAttachment({
  attachment,
  labels,
}: {
  attachment: Extract<InlineAttachment, { kind: "state_unit" }>;
  labels?: InlineAttachmentLabels;
}) {
  const data =
    attachment.data == null
      ? null
      : typeof attachment.data === "string"
        ? attachment.data
        : JSON.stringify(attachment.data, null, 2);
  return (
    <Collapsible
      title={
        <span className="inline-flex items-center gap-1.5 text-xs">
          <Database className="h-3.5 w-3.5" />
          <span>{labels?.capturedState ?? "Captured state"}</span>
          <Span variant="muted" className="text-[10px]">
            · {attachment.name}
          </Span>
        </span>
      }
      defaultExpanded={false}
      className="border-surface-300 dark:border-dark-surface-400 bg-surface-100 dark:bg-dark-surface-200 mt-2 rounded border"
    >
      <CodeBlock className="px-3 py-2 text-[11px] leading-relaxed">
        {data ?? labels?.noData ?? "(no data)"}
      </CodeBlock>
    </Collapsible>
  );
}

/**
 * Dispatch to the renderer for `attachment.kind`. Unknown kinds return null
 * so an experimental shape never breaks the thread.
 */
export function InlineAttachmentRenderer({
  attachment,
  labels,
}: InlineAttachmentRendererProps) {
  switch (attachment.kind) {
    case "file_view":
      return <FileViewAttachment attachment={attachment} labels={labels} />;
    case "terminal_excerpt":
      return (
        <TerminalExcerptAttachment attachment={attachment} labels={labels} />
      );
    case "plan_summary":
      return <PlanSummaryAttachment attachment={attachment} labels={labels} />;
    case "dag":
      return <DAGAttachment attachment={attachment} labels={labels} />;
    case "state_unit":
      return <StateUnitAttachment attachment={attachment} labels={labels} />;
    default: {
      const exhaustive: never = attachment;
      void exhaustive;
      return null;
    }
  }
}

/**
 * Render a list of attachments. Returns null when the list is empty so
 * callers can spread `<InlineAttachments />` unconditionally.
 */
export function InlineAttachments({
  attachments,
  labels,
}: {
  attachments?: InlineAttachment[];
  labels?: InlineAttachmentLabels;
}) {
  if (!attachments || attachments.length === 0) return null;
  return (
    <div className="mt-1 space-y-2">
      {attachments.map((a, i) => (
        <InlineAttachmentRenderer key={i} attachment={a} labels={labels} />
      ))}
    </div>
  );
}
