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

export type InlineAttachmentRendererProps = { attachment: InlineAttachment };

function FileViewAttachment({
  attachment,
}: {
  attachment: Extract<InlineAttachment, { kind: "file_view" }>;
}) {
  const lineCount = attachment.text.split("\n").length;
  return (
    <Collapsible
      title={
        <span className="inline-flex items-center gap-1.5 text-xs">
          <FileText className="h-3.5 w-3.5" />
          <span className="font-mono">{attachment.path}</span>
          <Span variant="muted" className="text-[10px]">
            {lineCount} {lineCount === 1 ? "line" : "lines"}
            {attachment.truncated ? " · truncated" : ""}
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
}: {
  attachment: Extract<InlineAttachment, { kind: "terminal_excerpt" }>;
}) {
  return (
    <Collapsible
      title={
        <span className="inline-flex items-center gap-1.5 text-xs">
          <TerminalSquare className="h-3.5 w-3.5" />
          <span>Terminal output</span>
          {attachment.command && (
            <Span variant="muted" className="font-mono text-[10px]">
              $ {attachment.command}
            </Span>
          )}
        </span>
      }
      defaultExpanded={false}
      className="border-surface-300 dark:border-dark-surface-400 bg-surface-100 dark:bg-dark-surface-200 mt-2 rounded border"
    >
      <CodeBlock className="px-3 py-2 leading-relaxed">{attachment.output}</CodeBlock>
    </Collapsible>
  );
}

function PlanSummaryAttachment({
  attachment,
}: {
  attachment: Extract<InlineAttachment, { kind: "plan_summary" }>;
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
          <span>Plan step {attachment.ordinal}</span>
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
            Description
          </Span>
          <div className="text-text dark:text-dark-text mt-0.5">{attachment.description}</div>
        </div>
        {attachment.summary && (
          <div>
            <Span variant="muted" className="text-[10px]">
              Summary
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
}: {
  attachment: Extract<InlineAttachment, { kind: "dag" }>;
}) {
  return (
    <Collapsible
      title={
        <span className="inline-flex items-center gap-1.5 text-xs">
          <Workflow className="h-3.5 w-3.5" />
          <span>{attachment.description ?? "Compiled chain DAG"}</span>
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
}: {
  attachment: Extract<InlineAttachment, { kind: "state_unit" }>;
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
          <span>Captured state</span>
          <Span variant="muted" className="text-[10px]">
            · {attachment.name}
          </Span>
        </span>
      }
      defaultExpanded={false}
      className="border-surface-300 dark:border-dark-surface-400 bg-surface-100 dark:bg-dark-surface-200 mt-2 rounded border"
    >
      <CodeBlock className="px-3 py-2 text-[11px] leading-relaxed">
        {data ?? "(no data)"}
      </CodeBlock>
    </Collapsible>
  );
}

/**
 * Dispatch to the renderer for `attachment.kind`. Unknown kinds return null
 * so an experimental shape never breaks the thread.
 */
export function InlineAttachmentRenderer({ attachment }: InlineAttachmentRendererProps) {
  switch (attachment.kind) {
    case "file_view":
      return <FileViewAttachment attachment={attachment} />;
    case "terminal_excerpt":
      return <TerminalExcerptAttachment attachment={attachment} />;
    case "plan_summary":
      return <PlanSummaryAttachment attachment={attachment} />;
    case "dag":
      return <DAGAttachment attachment={attachment} />;
    case "state_unit":
      return <StateUnitAttachment attachment={attachment} />;
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
export function InlineAttachments({ attachments }: { attachments?: InlineAttachment[] }) {
  if (!attachments || attachments.length === 0) return null;
  return (
    <div className="mt-1 space-y-2">
      {attachments.map((a, i) => (
        <InlineAttachmentRenderer key={i} attachment={a} />
      ))}
    </div>
  );
}
