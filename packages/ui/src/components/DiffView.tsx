import { forwardRef } from "react";
import { cn } from "../utils";

export interface DiffLine {
  type: "add" | "remove" | "context";
  content: string;
  oldLineNumber?: number;
  newLineNumber?: number;
}

/**
 * Builds `DiffLine[]` from old→new file text for `DiffView` using an LCS line
 * diff (common prefix/suffix trimmed first, so typical single-hunk edits skip
 * the DP entirely). Inputs beyond the DP guard fall back to whole-block
 * remove/add for the divergent middle. Empty `oldText` yields a pure addition.
 */
export function diffLinesFromTexts(
  oldText: string,
  newText: string,
): DiffLine[] {
  const oldLines = oldText ? oldText.split("\n") : [];
  const newLines = newText.split("\n");

  // Trim the common prefix and suffix: they become context lines, and the DP
  // only ever runs on the divergent middle.
  let start = 0;
  while (
    start < oldLines.length &&
    start < newLines.length &&
    oldLines[start] === newLines[start]
  ) {
    start++;
  }
  let oldEnd = oldLines.length;
  let newEnd = newLines.length;
  while (
    oldEnd > start &&
    newEnd > start &&
    oldLines[oldEnd - 1] === newLines[newEnd - 1]
  ) {
    oldEnd--;
    newEnd--;
  }

  const lines: DiffLine[] = [];
  let oldNo = 1;
  let newNo = 1;
  const pushContext = (content: string) => {
    lines.push({
      type: "context",
      content,
      oldLineNumber: oldNo++,
      newLineNumber: newNo++,
    });
  };
  const pushRemove = (content: string) => {
    lines.push({ type: "remove", content, oldLineNumber: oldNo++ });
  };
  const pushAdd = (content: string) => {
    lines.push({ type: "add", content, newLineNumber: newNo++ });
  };

  for (let i = 0; i < start; i++) pushContext(oldLines[i]);

  const midOld = oldLines.slice(start, oldEnd);
  const midNew = newLines.slice(start, newEnd);
  emitMiddleDiff(midOld, midNew, pushContext, pushRemove, pushAdd);

  for (let i = oldEnd; i < oldLines.length; i++) pushContext(oldLines[i]);

  return lines;
}

// Guard for the O(n·m) LCS table; beyond it the middle degrades to whole-block
// remove/add, which is what this function always did before the LCS existed.
const LCS_CELL_LIMIT = 1_000_000;

function emitMiddleDiff(
  midOld: string[],
  midNew: string[],
  pushContext: (content: string) => void,
  pushRemove: (content: string) => void,
  pushAdd: (content: string) => void,
): void {
  if (midOld.length === 0 && midNew.length === 0) return;
  if (
    midOld.length === 0 ||
    midNew.length === 0 ||
    midOld.length * midNew.length > LCS_CELL_LIMIT
  ) {
    for (const content of midOld) pushRemove(content);
    for (const content of midNew) pushAdd(content);
    return;
  }

  // Standard LCS length table, then a backtrack that emits removes before
  // adds within each divergent run (conventional unified-diff ordering).
  const rows = midOld.length + 1;
  const cols = midNew.length + 1;
  const table = new Uint32Array(rows * cols);
  for (let i = midOld.length - 1; i >= 0; i--) {
    for (let j = midNew.length - 1; j >= 0; j--) {
      table[i * cols + j] =
        midOld[i] === midNew[j]
          ? table[(i + 1) * cols + j + 1] + 1
          : Math.max(table[(i + 1) * cols + j], table[i * cols + j + 1]);
    }
  }

  let i = 0;
  let j = 0;
  while (i < midOld.length && j < midNew.length) {
    if (midOld[i] === midNew[j]) {
      pushContext(midOld[i]);
      i++;
      j++;
    } else if (table[(i + 1) * cols + j] >= table[i * cols + j + 1]) {
      pushRemove(midOld[i]);
      i++;
    } else {
      pushAdd(midNew[j]);
      j++;
    }
  }
  while (i < midOld.length) pushRemove(midOld[i++]);
  while (j < midNew.length) pushAdd(midNew[j++]);
}

export interface DiffViewProps extends React.HTMLAttributes<HTMLDivElement> {
  /** The file path shown in the header. */
  filePath: string;
  /** Parsed diff lines. */
  lines: DiffLine[];
  /** Optional language hint (for future syntax highlighting). */
  language?: string;
}

const lineTypeStyles: Record<DiffLine["type"], string> = {
  add: "bg-success-50 dark:bg-dark-success-50 text-success-800 dark:text-dark-success-800",
  remove:
    "bg-error-50 dark:bg-dark-error-50 text-error-800 dark:text-dark-error-800",
  context: "text-text dark:text-dark-text",
};

const gutterStyles: Record<DiffLine["type"], string> = {
  add: "bg-success-100 dark:bg-dark-success-100 text-success-600 dark:text-dark-success-600",
  remove:
    "bg-error-100 dark:bg-dark-error-100 text-error-600 dark:text-dark-error-600",
  context:
    "bg-surface-100 dark:bg-dark-surface-300 text-text-muted dark:text-dark-text-muted",
};

const prefixChar: Record<DiffLine["type"], string> = {
  add: "+",
  remove: "-",
  context: " ",
};

export const DiffView = forwardRef<HTMLDivElement, DiffViewProps>(
  function DiffView({ className, filePath, lines, language, ...props }, ref) {
    return (
      <div
        ref={ref}
        className={cn(
          "overflow-hidden rounded-lg border",
          "border-surface-200 dark:border-dark-surface-500",
          "text-sm",
          className,
        )}
        {...props}
      >
        {/* File header */}
        <div
          className={cn(
            "flex items-center gap-2 border-b px-3 py-1.5",
            "bg-surface-100 dark:bg-dark-surface-300",
            "border-surface-200 dark:border-dark-surface-500",
          )}
        >
          <span className="font-mono text-xs font-medium text-text dark:text-dark-text">
            {filePath}
          </span>
          {language && (
            <span className="text-xs text-text-muted dark:text-dark-text-muted">
              {language}
            </span>
          )}
        </div>

        {/* Lines */}
        <div className="overflow-x-auto">
          <table className="w-full border-collapse font-mono text-xs leading-5">
            <tbody>
              {lines.map((line, i) => (
                <tr key={i} className={lineTypeStyles[line.type]}>
                  {/* Old line number */}
                  <td
                    className={cn(
                      "w-10 select-none px-2 text-right align-top",
                      gutterStyles[line.type],
                    )}
                  >
                    {line.type !== "add" ? (line.oldLineNumber ?? "") : ""}
                  </td>
                  {/* New line number */}
                  <td
                    className={cn(
                      "w-10 select-none px-2 text-right align-top",
                      gutterStyles[line.type],
                    )}
                  >
                    {line.type !== "remove" ? (line.newLineNumber ?? "") : ""}
                  </td>
                  {/* Prefix */}
                  <td className="w-4 select-none px-1 text-center align-top">
                    {prefixChar[line.type]}
                  </td>
                  {/* Content */}
                  <td className="whitespace-pre-wrap break-all px-2 align-top">
                    {line.content}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    );
  },
);
