import { useEffect, useMemo, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";
import hljs from "highlight.js/lib/common";
import { cn } from "../../utils";
import { copyTextToClipboard } from "./clipboard";

/** Above this length we skip highlighting to keep streaming re-renders cheap. */
const HIGHLIGHT_MAX_CHARS = 20000;

function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

/** Synchronous so it stays flicker-free while assistant tokens stream in. */
function highlight(code: string, language?: string): string {
  if (code.length > HIGHLIGHT_MAX_CHARS) return escapeHtml(code);
  try {
    if (language && hljs.getLanguage(language)) {
      return hljs.highlight(code, { language, ignoreIllegals: true }).value;
    }
    return hljs.highlightAuto(code).value;
  } catch {
    return escapeHtml(code);
  }
}

export interface CodeBlockViewProps {
  code: string;
  language?: string;
  /** Accessible label for the copy control. */
  copyLabel?: string;
  /** Label shown briefly after a successful copy. */
  copiedLabel?: string;
}

/**
 * A fenced code block for the chat transcript: a header bar (language + copy)
 * over syntax-highlighted code. Highlighting mirrors VS Code's `vs` / `vs2015`
 * themes (see `index.css`) so blocks match the Monaco editors.
 */
export function CodeBlockView({
  code,
  language,
  copyLabel = "Copy",
  copiedLabel = "Copied",
}: CodeBlockViewProps) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  const html = useMemo(() => highlight(code, language), [code, language]);

  const onCopy = () => {
    void copyTextToClipboard(code).then((ok) => {
      if (!ok) return;
      setCopied(true);
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <div className="border-surface-200 dark:border-dark-surface-700 my-2 overflow-hidden rounded-lg border">
      <div className="border-surface-200 bg-surface-100 dark:border-dark-surface-700 dark:bg-dark-surface-800 flex items-center justify-between gap-2 border-b px-3 py-1.5">
        <span className="text-text-muted dark:text-dark-text-muted font-mono text-xs">
          {language || "text"}
        </span>
        <button
          type="button"
          onClick={onCopy}
          aria-label={copied ? copiedLabel : copyLabel}
          className="text-text-muted hover:text-text dark:text-dark-text-muted dark:hover:text-dark-text inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs transition-colors"
        >
          {copied ? (
            <Check className="h-3.5 w-3.5" aria-hidden />
          ) : (
            <Copy className="h-3.5 w-3.5" aria-hidden />
          )}
          <span>{copied ? copiedLabel : copyLabel}</span>
        </button>
      </div>
      <pre className="bg-surface-200 text-text dark:bg-dark-surface-700 dark:text-dark-text m-0 overflow-auto p-3 text-sm sm:p-4">
        <code
          className={cn("hljs", language && `language-${language}`)}
          dangerouslySetInnerHTML={{ __html: html }}
        />
      </pre>
    </div>
  );
}
