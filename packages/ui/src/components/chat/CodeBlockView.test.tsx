import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { describe, expect, it } from "vitest";
import { CodeBlockView } from "./CodeBlockView";
import { chatTranscriptMarkdownComponents } from "./chatTranscript";

describe("CodeBlockView", () => {
  it("syntax-highlights known languages and shows the language + copy affordance", () => {
    const html = renderToStaticMarkup(
      createElement(CodeBlockView, {
        code: "const answer = 42;",
        language: "ts",
      }),
    );
    // highlight.js tokenizes into hljs-* spans
    expect(html).toContain('class="hljs');
    expect(html).toContain("hljs-keyword"); // `const`
    expect(html).toContain("hljs-number"); // `42`
    // language label + copy control are present
    expect(html).toContain("ts");
    expect(html).toContain("Copy");
  });

  it("still renders when the language is unknown (no highlighter throw)", () => {
    const html = renderToStaticMarkup(
      createElement(CodeBlockView, {
        code: "just some prose text",
        language: "not-a-lang",
      }),
    );
    // Unknown language falls back to auto-detection; content still renders in a block.
    expect(html).toContain("just some prose");
    expect(html).toContain('class="hljs');
  });

  it("honors custom copy labels (for i18n)", () => {
    const html = renderToStaticMarkup(
      createElement(CodeBlockView, {
        code: "x",
        language: "ts",
        copyLabel: "Kopieren",
      }),
    );
    expect(html).toContain("Kopieren");
  });
});

describe("chatTranscriptMarkdownComponents: code fences", () => {
  it("routes fenced blocks through CodeBlockView (highlighted, with copy)", () => {
    const html = renderToStaticMarkup(
      createElement(ReactMarkdown, {
        remarkPlugins: [remarkGfm],
        components: chatTranscriptMarkdownComponents,
        children: "```ts\nconst x = 1;\n```",
      }),
    );
    expect(html).toContain("hljs");
    expect(html).toContain("hljs-keyword");
    expect(html).toContain("Copy");
    // No double frame: the pass-through `pre` must not wrap the card in a bare <pre><code> of raw text.
    expect(html).not.toContain(">const x = 1;\n</code>");
  });

  it("keeps inline code inline (not a CodeBlockView, no hljs)", () => {
    const html = renderToStaticMarkup(
      createElement(ReactMarkdown, {
        remarkPlugins: [remarkGfm],
        components: chatTranscriptMarkdownComponents,
        children: "use the `useMemo` hook",
      }),
    );
    expect(html).toContain("font-mono");
    expect(html).not.toContain("hljs");
    expect(html).not.toContain("Copy");
  });
});
