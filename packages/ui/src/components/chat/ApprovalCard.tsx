import { useState } from "react";
import { Button } from "../Button";
import { ButtonGroup } from "../ButtonGroup";
import { DiffView, type DiffLine } from "../DiffView";
import { KeyValue } from "../KeyValue";
import { Panel } from "../Panel";
import { Span } from "../Typography";
import type { PendingApproval } from "./types";

export interface ApprovalCardLabels {
  approvalRequired?: string;
  showDiff?: string;
  hideDiff?: string;
  approve?: string;
  deny?: string;
}

const DEFAULT_LABELS: Required<ApprovalCardLabels> = {
  approvalRequired: "Approval required:",
  showDiff: "▸ Show diff",
  hideDiff: "▾ Hide diff",
  approve: "Approve",
  deny: "Deny",
};

type Props = {
  approval: PendingApproval;
  onRespond: (approved: boolean) => void;
  labels?: ApprovalCardLabels;
};

function parsePatch(raw: string): { filePath: string; lines: DiffLine[] } {
  const rawLines = raw.split("\n");
  let filePath = "diff";
  const lines: DiffLine[] = [];
  let oldLine = 0;
  let newLine = 0;

  for (const text of rawLines) {
    if (text.startsWith("+++ ")) {
      filePath = text.slice(4).replace(/^b\//, "");
      continue;
    }
    if (text.startsWith("--- ")) continue;
    if (text.startsWith("@@ ")) {
      const m = text.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
      if (m) {
        oldLine = parseInt(m[1], 10);
        newLine = parseInt(m[2], 10);
      }
      lines.push({ type: "context", content: text });
      continue;
    }
    if (text.startsWith("+")) {
      lines.push({ type: "add", content: text.slice(1), newLineNumber: newLine++ });
    } else if (text.startsWith("-")) {
      lines.push({ type: "remove", content: text.slice(1), oldLineNumber: oldLine++ });
    } else {
      lines.push({
        type: "context",
        content: text.startsWith(" ") ? text.slice(1) : text,
        oldLineNumber: oldLine++,
        newLineNumber: newLine++,
      });
    }
  }

  return { filePath, lines };
}

export function ApprovalCard({ approval, onRespond, labels }: Props) {
  const [inflight, setInflight] = useState(false);
  const [diffExpanded, setDiffExpanded] = useState(false);
  const l = { ...DEFAULT_LABELS, ...labels };

  const handle = (approved: boolean) => {
    if (inflight) return;
    setInflight(true);
    onRespond(approved);
  };

  const argEntries = Object.entries(approval.args).filter(
    ([, v]) => v !== "" && v !== null && v !== undefined,
  );

  return (
    <Panel variant="warning">
      <div className="flex items-center gap-1.5 text-sm font-semibold">
        ⚠ {l.approvalRequired}{" "}
        <Span className="font-mono text-[0.9em]">
          {approval.hookName}.{approval.toolName}
        </Span>
      </div>

      {argEntries.length > 0 && (
        <div className="flex flex-col gap-0.5 text-xs">
          {argEntries.map(([k, v]) => (
            <KeyValue
              key={k}
              label={k}
              value={String(v)}
              labelClassName="text-text-muted dark:text-dark-text-muted pr-3 align-top"
              valueClassName="break-all font-mono"
            />
          ))}
        </div>
      )}

      {approval.diff && approval.diff !== "(no changes)" && (
        <div>
          <Button
            variant="ghost"
            size="sm"
            className="px-0 text-text-muted dark:text-dark-text-muted"
            onClick={() => setDiffExpanded((e) => !e)}>
            {diffExpanded ? l.hideDiff : l.showDiff}
          </Button>
          {diffExpanded &&
            (() => {
              const { filePath, lines } = parsePatch(approval.diff!);
              return (
                <DiffView filePath={filePath} lines={lines} className="max-h-80 overflow-auto" />
              );
            })()}
        </div>
      )}

      <ButtonGroup className="mt-1">
        <Button size="sm" variant="success" disabled={inflight} onClick={() => handle(true)}>
          {l.approve}
        </Button>
        <Button size="sm" variant="danger" disabled={inflight} onClick={() => handle(false)}>
          {l.deny}
        </Button>
      </ButtonGroup>
    </Panel>
  );
}
