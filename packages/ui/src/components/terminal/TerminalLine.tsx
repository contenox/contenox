import { forwardRef } from "react";
import { cn } from "../../utils";

/** Semantic tone for a terminal line's text or glyph. */
export type TerminalTone = "default" | "muted" | "accent" | "error" | "success";

const toneClass: Record<TerminalTone, string> = {
  default: "text-text dark:text-dark-text",
  muted: "text-text-muted dark:text-dark-text-muted",
  accent: "text-success dark:text-dark-success",
  error: "text-error dark:text-dark-error",
  success: "text-success dark:text-dark-success",
};

export interface TerminalLineProps extends React.HTMLAttributes<HTMLDivElement> {
  /** Leading glyph (e.g. ❯, ⏺, ⎿). Rendered in `glyphTone`. */
  glyph?: React.ReactNode;
  /** Tone of the glyph. Defaults to `accent`. */
  glyphTone?: TerminalTone;
  /** Tone of the line content. Defaults to `default`. */
  tone?: TerminalTone;
  /** Left indent in grid steps (each ≈ one glyph column). */
  indent?: 0 | 1 | 2 | 3;
}

const indentClass: Record<NonNullable<TerminalLineProps["indent"]>, string> = {
  0: "",
  1: "pl-3",
  2: "pl-5",
  3: "pl-7",
};

/**
 * One line of terminal scrollback: an optional leading glyph plus content, on
 * the monospace grid. The building block for command echoes, tool lines, and
 * result annotations. Purely presentational.
 */
export const TerminalLine = forwardRef<HTMLDivElement, TerminalLineProps>(
  function TerminalLine(
    { glyph, glyphTone = "accent", tone = "default", indent = 0, className, children, ...props },
    ref,
  ) {
    return (
      <div
        ref={ref}
        className={cn(
          "flex items-start gap-1.5 font-mono",
          indentClass[indent],
          toneClass[tone],
          className,
        )}
        {...props}
      >
        {glyph != null && (
          <span className={cn("select-none", toneClass[glyphTone])}>{glyph}</span>
        )}
        <span className="min-w-0 flex-1 whitespace-pre-wrap break-words">{children}</span>
      </div>
    );
  },
);

export interface TerminalMetaProps extends React.HTMLAttributes<HTMLDivElement> {
  /** Indent in grid steps. Defaults to 2 (aligns under a glyph line's content). */
  indent?: 0 | 1 | 2 | 3;
}

/** A dim metadata line (chain, model, tokens…) under a command. */
export const TerminalMeta = forwardRef<HTMLDivElement, TerminalMetaProps>(
  function TerminalMeta({ indent = 2, className, ...props }, ref) {
    return (
      <div
        ref={ref}
        className={cn(
          "font-mono text-[11px] text-text-muted dark:text-dark-text-muted",
          indentClass[indent],
          className,
        )}
        {...props}
      />
    );
  },
);
