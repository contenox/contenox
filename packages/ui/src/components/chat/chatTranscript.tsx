import React from "react";
import type { Components } from "react-markdown";
import { cn } from "../../utils";
import { CodeBlockView } from "./CodeBlockView";

/** Flatten a markdown node's children down to its raw text (for code fences). */
function nodeText(children: React.ReactNode): string {
  if (children == null || typeof children === "boolean") return "";
  if (typeof children === "string" || typeof children === "number")
    return String(children);
  if (Array.isArray(children)) return children.map(nodeText).join("");
  if (React.isValidElement(children)) {
    return nodeText(
      (children.props as { children?: React.ReactNode }).children,
    );
  }
  return "";
}

/** Default ReactMarkdown `components` map for assistant/user transcript content. */
export const chatTranscriptMarkdownComponents: Components = {
  // Block code is rendered entirely by the `code` handler (into a CodeBlockView);
  // this keeps the `pre` wrapper from adding a second frame around it.
  pre: ({ children }: React.ComponentPropsWithoutRef<"pre">) => <>{children}</>,

  code: (props: React.ComponentPropsWithoutRef<"code"> & { node?: any }) => {
    const { className, children, node, ...rest } = props;
    const match = /language-(\w+)/.exec(className || "");
    const isBlock =
      Boolean(match) ||
      (node &&
        node.parent &&
        node.parent.type === "element" &&
        node.parent.tagName === "pre");

    if (isBlock) {
      return <CodeBlockView code={nodeText(children)} language={match?.[1]} />;
    }

    // Inline code styling
    return (
      <code
        className={cn(
          "bg-surface-200 text-text dark:bg-dark-surface-700 dark:text-dark-text rounded px-1.5 py-0.5 font-mono text-xs",
          className,
        )}
        {...rest}
      >
        {children}
      </code>
    );
  },

  // Tailwind's preflight strips default heading/list styling, so the transcript
  // must restate it with semantic tokens — otherwise markdown reads as flat text.
  h1: ({ children, ...props }: React.ComponentPropsWithoutRef<"h1">) => (
    <h1
      className="text-text dark:text-dark-text mt-4 mb-2 text-xl font-semibold first:mt-0"
      {...props}
    >
      {children}
    </h1>
  ),
  h2: ({ children, ...props }: React.ComponentPropsWithoutRef<"h2">) => (
    <h2
      className="text-text dark:text-dark-text mt-4 mb-2 text-lg font-semibold first:mt-0"
      {...props}
    >
      {children}
    </h2>
  ),
  h3: ({ children, ...props }: React.ComponentPropsWithoutRef<"h3">) => (
    <h3
      className="text-text dark:text-dark-text mt-3 mb-1.5 text-base font-semibold first:mt-0"
      {...props}
    >
      {children}
    </h3>
  ),
  h4: ({ children, ...props }: React.ComponentPropsWithoutRef<"h4">) => (
    <h4
      className="text-text dark:text-dark-text mt-3 mb-1.5 text-sm font-semibold first:mt-0"
      {...props}
    >
      {children}
    </h4>
  ),

  p: ({ children, ...props }: React.ComponentPropsWithoutRef<"p">) => (
    <p className="my-2 leading-relaxed first:mt-0 last:mb-0" {...props}>
      {children}
    </p>
  ),

  ul: ({ children, ...props }: React.ComponentPropsWithoutRef<"ul">) => (
    <ul
      className="marker:text-text-muted dark:marker:text-dark-text-muted my-2 list-disc space-y-1 pl-6"
      {...props}
    >
      {children}
    </ul>
  ),
  ol: ({ children, ...props }: React.ComponentPropsWithoutRef<"ol">) => (
    <ol
      className="marker:text-text-muted dark:marker:text-dark-text-muted my-2 list-decimal space-y-1 pl-6"
      {...props}
    >
      {children}
    </ol>
  ),
  li: ({ children, ...props }: React.ComponentPropsWithoutRef<"li">) => (
    <li className="leading-relaxed" {...props}>
      {children}
    </li>
  ),

  a: ({ children, href, ...props }: React.ComponentPropsWithoutRef<"a">) => {
    const external = /^https?:\/\//i.test(href || "");
    return (
      <a
        href={href}
        className="text-text-accent dark:text-dark-text-accent underline decoration-1 underline-offset-2 hover:opacity-80"
        {...(external ? { target: "_blank", rel: "noreferrer noopener" } : {})}
        {...props}
      >
        {children}
      </a>
    );
  },

  strong: ({
    children,
    ...props
  }: React.ComponentPropsWithoutRef<"strong">) => (
    <strong className="font-semibold" {...props}>
      {children}
    </strong>
  ),
  em: ({ children, ...props }: React.ComponentPropsWithoutRef<"em">) => (
    <em className="italic" {...props}>
      {children}
    </em>
  ),
  hr: (props: React.ComponentPropsWithoutRef<"hr">) => (
    <hr
      className="border-surface-200 dark:border-dark-surface-700 my-4"
      {...props}
    />
  ),

  // GFM tables: scroll horizontally inside the bubble instead of overflowing it.
  table: ({ children, ...props }: React.ComponentPropsWithoutRef<"table">) => (
    <div className="my-3 overflow-x-auto">
      <table className="w-full border-collapse text-sm" {...props}>
        {children}
      </table>
    </div>
  ),
  thead: ({ children, ...props }: React.ComponentPropsWithoutRef<"thead">) => (
    <thead
      className="border-surface-200 dark:border-dark-surface-700 border-b"
      {...props}
    >
      {children}
    </thead>
  ),
  th: ({ children, ...props }: React.ComponentPropsWithoutRef<"th">) => (
    <th className="px-3 py-1.5 text-left font-semibold" {...props}>
      {children}
    </th>
  ),
  td: ({ children, ...props }: React.ComponentPropsWithoutRef<"td">) => (
    <td
      className="border-surface-200 dark:border-dark-surface-700 border-t px-3 py-1.5 align-top"
      {...props}
    >
      {children}
    </td>
  ),

  // Markdown-embedded images must degrade nicely inside a chat column: never
  // overflow the bubble, keep the transcript's rounded/bordered look.
  img: ({ className, ...props }: React.ComponentPropsWithoutRef<"img">) => (
    <img
      loading="lazy"
      className={cn(
        "border-surface-200 dark:border-dark-surface-700 my-2 h-auto max-w-full rounded-lg border",
        className,
      )}
      {...props}
    />
  ),

  blockquote: ({
    children,
    ...props
  }: React.ComponentPropsWithoutRef<"blockquote">) => (
    <blockquote
      className="border-primary-400 dark:border-dark-primary-500 bg-surface-50/50 text-text dark:bg-dark-surface-300/20 dark:text-dark-text rounded-r-lg border-l-4 py-2 pl-4"
      {...props}
    >
      {children}
    </blockquote>
  ),
};

export function ChatStreamThinkingBox({
  className,
  children,
}: {
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div
      className={cn(
        "border-primary-300/50 bg-surface-50/60 dark:border-dark-primary-600/40 dark:bg-dark-surface-300/30 text-text-muted dark:text-dark-text-muted max-h-48 overflow-auto rounded-md border px-3 py-2 font-mono text-xs whitespace-pre-wrap",
        className,
      )}
    >
      {children}
    </div>
  );
}

export function ChatTranscriptStreamingPlaceholder({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <p className="text-text-muted dark:text-dark-text-muted text-sm italic">
      {children}
    </p>
  );
}

export function ChatStreamingCaret({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "bg-primary-500 ml-0.5 inline-block h-3 w-1.5 animate-pulse rounded-sm align-middle",
        className,
      )}
      aria-hidden
    />
  );
}
