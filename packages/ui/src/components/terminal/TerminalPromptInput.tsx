import {
  forwardRef,
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type Ref,
} from "react";
import { cn } from "../../utils";
import { TERMINAL_GLYPH } from "./glyphs";

export interface TerminalPromptInputProps {
  value: string;
  onChange: (value: string) => void;
  /** Consumer handles Enter/Tab/completion here. */
  onKeyDown?: (e: KeyboardEvent<HTMLTextAreaElement>) => void;
  disabled?: boolean;
  /** Prompt glyph. Defaults to ❯. */
  prompt?: React.ReactNode;
  /** Dim hint shown when empty and unfocused. */
  hint?: string;
  className?: string;
}

function assignRef<T>(ref: Ref<T> | undefined, value: T | null) {
  if (typeof ref === "function") ref(value);
  else if (ref && typeof ref === "object") (ref as { current: T | null }).current = value;
}

/**
 * A terminal prompt line — not a chat box. The input is flush with the
 * surrounding monospace grid: a prompt glyph, the text on the same cells, and
 * a blinking block cursor sitting in the text (tracked to the caret, correct
 * mid-string). Editing is keyboard-only via `onKeyDown`.
 *
 * Implementation: a transparent <textarea> captures input and selection while
 * an aria-hidden mirror renders the visible text + block cursor.
 */
export const TerminalPromptInput = forwardRef<HTMLTextAreaElement, TerminalPromptInputProps>(
  function TerminalPromptInput(
    { value, onChange, onKeyDown, disabled, prompt = TERMINAL_GLYPH.prompt, hint, className },
    forwardedRef,
  ) {
    const innerRef = useRef<HTMLTextAreaElement | null>(null);
    const [caret, setCaret] = useState(0);
    const [focused, setFocused] = useState(false);

    const setRef = useCallback(
      (el: HTMLTextAreaElement | null) => {
        innerRef.current = el;
        assignRef(forwardedRef, el);
      },
      [forwardedRef],
    );

    const syncCaret = useCallback(() => {
      const el = innerRef.current;
      if (el) setCaret(el.selectionStart ?? value.length);
    }, [value.length]);

    useEffect(() => {
      syncCaret();
    }, [value, syncCaret]);

    const before = value.slice(0, caret);
    const after = value.slice(caret);
    const showCursor = focused && !disabled;

    return (
      <div
        className={cn("relative flex cursor-text items-start gap-2 font-mono", className)}
        onClick={() => innerRef.current?.focus()}
      >
        <span className="select-none text-success dark:text-dark-success">{prompt}</span>
        <div className="relative min-w-0 flex-1">
          <div aria-hidden className="whitespace-pre-wrap break-words text-text dark:text-dark-text">
            {value.length === 0 && !showCursor ? (
              <span className="text-text-muted dark:text-dark-text-muted">{hint}</span>
            ) : (
              <>
                <span>{before}</span>
                {showCursor && (
                  <span className="contenox-terminal-cursor bg-success dark:bg-dark-success">
                    {" "}
                  </span>
                )}
                <span>{after}</span>
              </>
            )}
            {value.length === 0 && "​"}
          </div>
          <textarea
            ref={setRef}
            value={value}
            onChange={(e) => {
              onChange(e.target.value);
              requestAnimationFrame(syncCaret);
            }}
            onKeyDown={(e) => {
              onKeyDown?.(e);
              requestAnimationFrame(syncCaret);
            }}
            onKeyUp={syncCaret}
            onClick={syncCaret}
            onSelect={syncCaret}
            onFocus={() => setFocused(true)}
            onBlur={() => setFocused(false)}
            disabled={disabled}
            rows={1}
            spellCheck={false}
            autoComplete="off"
            className="absolute inset-0 h-full w-full resize-none overflow-hidden whitespace-pre-wrap break-words bg-transparent text-transparent caret-transparent outline-none"
          />
        </div>
      </div>
    );
  },
);
