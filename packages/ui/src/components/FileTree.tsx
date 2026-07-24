import { forwardRef, useCallback, useState, type ReactNode } from "react";
import { ChevronRight, File, Folder, FolderOpen } from "lucide-react";
import { cn } from "../utils";

/**
 * Severity of a single {@link FileTreeIndicator}, which tints its icon.
 * Consumer-defined semantics; the workspace file panel maps a HITL policy verdict
 * onto it — `allow` → a calm success green, `approve` → warning, `deny` → error,
 * `unreachable` → muted (paired with {@link FileTreeNode.dimmed} on the row).
 */
export type FileTreeIndicatorStatus = "allow" | "approve" | "deny" | "unreachable";

/**
 * A small trailing status icon on a row (e.g. a read or write access marker). The
 * icon element is CONSUMER-supplied so this component stays domain-agnostic — the
 * tree only tints it by {@link FileTreeIndicatorStatus} (via `currentColor`) and
 * wires its tooltip / accessible label. A node may carry several (rendered in
 * order, right-aligned), which is how the workspace panel shows the two
 * independent read + write axes.
 */
export interface FileTreeIndicator {
  /** Stable id within a node's indicator list (used as the React key). */
  key: string;
  /** Icon element to render, e.g. a lucide `<Eye />`. Sized/styled by the consumer; tinted here. */
  icon: ReactNode;
  /** Severity that tints the icon. */
  status: FileTreeIndicatorStatus;
  /** Per-indicator tooltip and accessible label (e.g. the localized policy reason). */
  title?: string;
}

export interface FileTreeNode {
  /** Unique id for this node. */
  id: string;
  /** Display name. */
  name: string;
  /** Full path (used for context, not rendering). */
  path?: string;
  /** Whether this node is a directory. */
  isDirectory?: boolean;
  /** Nested children (only for directories). */
  children?: FileTreeNode[];
  /** Trailing status indicators (e.g. read + write access), rendered after the name. */
  indicators?: FileTreeIndicator[];
  /** Dims the row to half opacity (e.g. an out-of-boundary / unreachable path). */
  dimmed?: boolean;
  /** Optional row tooltip. */
  title?: string;
}

/** Icon tint per severity. `allow` is a calm success green (a thin stroke, not a loud fill). */
const INDICATOR_TINT_CLASS: Record<FileTreeIndicatorStatus, string> = {
  allow: "text-success-500 dark:text-dark-success-500",
  approve: "text-warning-500 dark:text-dark-warning-500",
  deny: "text-error-500 dark:text-dark-error-500",
  unreachable: "text-text-muted dark:text-dark-text-muted",
};

/** The right-aligned run of trailing indicators (kept optional so plain trees are untouched). */
function IndicatorRow({ indicators }: { indicators: FileTreeIndicator[] }) {
  return (
    <span className="ml-auto flex shrink-0 items-center gap-1 pl-1.5">
      {indicators.map((ind) => (
        <span
          key={ind.key}
          className={cn("inline-flex shrink-0 items-center", INDICATOR_TINT_CLASS[ind.status])}
          title={ind.title}
          role={ind.title ? "img" : undefined}
          aria-label={ind.title || undefined}
          aria-hidden={ind.title ? undefined : true}
        >
          {ind.icon}
        </span>
      ))}
    </span>
  );
}

export interface FileTreeProps extends React.HTMLAttributes<HTMLDivElement> {
  /** The tree data. */
  nodes: FileTreeNode[];
  /** Currently selected node id. */
  selectedId?: string | null;
  /** Called when a file or folder is clicked. */
  onNodeSelect?: (node: FileTreeNode) => void;
  /**
   * `expand` (default): directory row toggles open/closed and fires `onNodeSelect`.
   * `navigate`: directory row only calls `onNodeSelect` (e.g. change cwd); use the chevron to expand/collapse when children exist.
   */
  directoryClickMode?: "expand" | "navigate";
  /** Set of node ids that are initially expanded. Defaults to all directories expanded. */
  defaultExpanded?: Set<string>;
  /** Depth indentation in px. */
  indent?: number;
}

export const FileTree = forwardRef<HTMLDivElement, FileTreeProps>(
  function FileTree(
    {
      className,
      nodes,
      selectedId,
      onNodeSelect,
      directoryClickMode = "expand",
      defaultExpanded,
      indent = 16,
      ...props
    },
    ref,
  ) {
    return (
      <div
        ref={ref}
        role="tree"
        className={cn("text-sm select-none", className)}
        {...props}
      >
        {nodes.map((node) => (
          <FileTreeItem
            key={node.id}
            node={node}
            depth={0}
            indent={indent}
            selectedId={selectedId}
            onNodeSelect={onNodeSelect}
            directoryClickMode={directoryClickMode}
            defaultExpanded={defaultExpanded}
          />
        ))}
      </div>
    );
  },
);

/* ------------------------------------------------------------------ */

interface FileTreeItemProps {
  node: FileTreeNode;
  depth: number;
  indent: number;
  selectedId?: string | null;
  onNodeSelect?: (node: FileTreeNode) => void;
  directoryClickMode: "expand" | "navigate";
  defaultExpanded?: Set<string>;
}

function FileTreeItem({
  node,
  depth,
  indent,
  selectedId,
  onNodeSelect,
  directoryClickMode,
  defaultExpanded,
}: FileTreeItemProps) {
  const [expanded, setExpanded] = useState(
    () => defaultExpanded?.has(node.id) ?? (node.isDirectory === true),
  );

  const isSelected = selectedId === node.id;
  const hasIndicators = !!node.indicators && node.indicators.length > 0;

  const toggleExpand = useCallback(() => {
    setExpanded((v) => !v);
  }, []);

  const handleRowClick = useCallback(() => {
    if (node.isDirectory && directoryClickMode === "expand") {
      setExpanded((v) => !v);
    }
    onNodeSelect?.(node);
  }, [node, onNodeSelect, directoryClickMode]);

  const handleChevronClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      toggleExpand();
    },
    [toggleExpand],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        handleRowClick();
      }
      if (node.isDirectory) {
        if (e.key === "ArrowRight" && !expanded) {
          e.preventDefault();
          setExpanded(true);
        }
        if (e.key === "ArrowLeft" && expanded) {
          e.preventDefault();
          setExpanded(false);
        }
      }
    },
    [handleRowClick, node.isDirectory, expanded],
  );

  const rowShellClass = cn(
    "flex w-full items-center gap-1.5 rounded px-2 py-1 text-left",
    "text-text dark:text-dark-text",
    "hover:bg-surface-100 dark:hover:bg-dark-surface-200",
    node.dimmed && "opacity-50",
    isSelected &&
      "bg-primary-50/50 text-primary-700 dark:bg-dark-primary-900/30 dark:text-dark-primary-400",
  );

  return (
    <div role="treeitem" aria-expanded={node.isDirectory ? expanded : undefined}>
      {node.isDirectory && directoryClickMode === "navigate" ? (
        <div className={rowShellClass} style={{ paddingLeft: depth * indent + 8 }} title={node.title}>
          <button
            type="button"
            className="text-text-muted dark:text-dark-text-muted hover:bg-surface-200 dark:hover:bg-dark-surface-300 inline-flex shrink-0 items-center justify-center rounded p-0.5"
            onClick={handleChevronClick}
            aria-expanded={expanded}
            aria-label={expanded ? "Collapse" : "Expand"}
          >
            <ChevronRight
              className={cn(
                "h-3.5 w-3.5 transition-transform",
                expanded && "rotate-90",
              )}
            />
          </button>
          <button
            type="button"
            onClick={() => onNodeSelect?.(node)}
            className="flex min-w-0 flex-1 items-center gap-1.5 rounded py-0.5 text-left hover:bg-transparent"
          >
            {expanded ? (
              <FolderOpen className="h-4 w-4 shrink-0 text-warning dark:text-dark-warning" />
            ) : (
              <Folder className="h-4 w-4 shrink-0 text-warning dark:text-dark-warning" />
            )}
            <span className="min-w-0 flex-1 truncate font-mono text-xs">{node.name}</span>
            {hasIndicators && <IndicatorRow indicators={node.indicators!} />}
          </button>
        </div>
      ) : (
        <button
          type="button"
          onClick={handleRowClick}
          onKeyDown={handleKeyDown}
          className={rowShellClass}
          style={{ paddingLeft: depth * indent + 8 }}
          title={node.title}
        >
          {node.isDirectory ? (
            <>
              <ChevronRight
                className={cn(
                  "h-3.5 w-3.5 shrink-0 transition-transform",
                  "text-text-muted dark:text-dark-text-muted",
                  expanded && "rotate-90",
                )}
              />
              {expanded ? (
                <FolderOpen className="h-4 w-4 shrink-0 text-warning dark:text-dark-warning" />
              ) : (
                <Folder className="h-4 w-4 shrink-0 text-warning dark:text-dark-warning" />
              )}
            </>
          ) : (
            <>
              <span className="w-3.5 shrink-0" />
              <File className="h-4 w-4 shrink-0 text-text-muted dark:text-dark-text-muted" />
            </>
          )}
          <span className="min-w-0 flex-1 truncate font-mono text-xs">{node.name}</span>
          {hasIndicators && <IndicatorRow indicators={node.indicators!} />}
        </button>
      )}

      {node.isDirectory && expanded && node.children && (
        <div role="group">
          {node.children.map((child) => (
            <FileTreeItem
              key={child.id}
              node={child}
              depth={depth + 1}
              indent={indent}
              selectedId={selectedId}
              onNodeSelect={onNodeSelect}
              directoryClickMode={directoryClickMode}
              defaultExpanded={defaultExpanded}
            />
          ))}
        </div>
      )}
    </div>
  );
}
