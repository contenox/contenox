import type { ComponentPropsWithoutRef } from "react";
import type { Components } from "react-markdown";
import { cn } from "../../utils";

/**
 * ReactMarkdown component map for terminal-style surfaces (the console
 * result area). Dark-safe by construction: inline code uses opacity-on-
 * black/white so its contrast never depends on surface-scale tokens flipping
 * (the failure mode of the chat transcript map, whose `dark:bg-dark-surface-*`
 * loses the zero-specificity `dark:` cascade → invisible text). Terminal-
 * appropriate: tight spacing, framed code blocks, plain bold/headings.
 */
export const terminalMarkdownComponents: Components = {
  p: ({ children }) => <p className="my-1 whitespace-pre-wrap">{children}</p>,
  strong: ({ children }) => (
    <strong className="font-semibold text-text dark:text-dark-text">{children}</strong>
  ),
  em: ({ children }) => <em className="italic">{children}</em>,
  a: ({ children, href }) => (
    <a
      href={href}
      target="_blank"
      rel="noreferrer"
      className="text-success underline dark:text-dark-success"
    >
      {children}
    </a>
  ),
  h1: ({ children }) => (
    <div className="mt-2 mb-1 text-[14px] font-semibold text-text dark:text-dark-text">{children}</div>
  ),
  h2: ({ children }) => (
    <div className="mt-2 mb-1 text-[13px] font-semibold text-text dark:text-dark-text">{children}</div>
  ),
  h3: ({ children }) => (
    <div className="mt-1.5 mb-0.5 font-semibold text-text dark:text-dark-text">{children}</div>
  ),
  ul: ({ children }) => (
    <ul className="my-1 ml-4 list-disc space-y-0.5 marker:text-current/50">{children}</ul>
  ),
  ol: ({ children }) => (
    <ol className="my-1 ml-4 list-decimal space-y-0.5 marker:text-current/50">{children}</ol>
  ),
  li: ({ children }) => <li className="whitespace-pre-wrap">{children}</li>,
  blockquote: ({ children }) => (
    <blockquote className="my-1 border-l-2 border-surface-300 pl-2 text-text-muted dark:border-dark-surface-400 dark:text-dark-text-muted">
      {children}
    </blockquote>
  ),
  hr: () => <hr className="my-2 border-t border-surface-300 dark:border-dark-surface-400" />,
  pre: ({ children }) => (
    <pre className="my-1 overflow-auto rounded-sm border border-surface-300 bg-black/20 p-2 text-[12px] dark:border-dark-surface-400 dark:bg-black/40">
      {children}
    </pre>
  ),
  code: (props: ComponentPropsWithoutRef<"code"> & { node?: any }) => {
    const { className, children, node, ...rest } = props;
    const isBlock = /language-\w+/.test(className || "") || node?.parent?.tagName === "pre";
    if (isBlock) {
      return (
        <code className={className} {...rest}>
          {children}
        </code>
      );
    }
    return (
      <code
        className={cn(
          "rounded-sm bg-black/15 px-1 font-mono text-[0.92em] text-text dark:bg-white/15 dark:text-dark-text",
          className,
        )}
        {...rest}
      >
        {children}
      </code>
    );
  },
};
