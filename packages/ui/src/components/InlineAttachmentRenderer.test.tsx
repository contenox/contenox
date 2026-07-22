import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  InlineAttachmentRenderer,
  InlineAttachments,
  type InlineAttachment,
} from "./InlineAttachmentRenderer";
import { chatTranscriptMarkdownComponents } from "./chat/chatTranscript";

// A 1x1 transparent PNG — small but syntactically real base64.
const PNG_B64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==";

const imageAttachment: InlineAttachment = {
  kind: "image",
  mimeType: "image/png",
  data: PNG_B64,
};

describe("InlineAttachmentRenderer: image kind", () => {
  it("renders an <img> with a data: URI assembled from mimeType + raw base64 (no caller-side prefixing)", () => {
    const html = renderToStaticMarkup(
      createElement(InlineAttachmentRenderer, { attachment: imageAttachment }),
    );
    expect(html).toContain(`src="data:image/png;base64,${PNG_B64}"`);
    // Size-constrained so a large screenshot cannot blow out the thread column.
    expect(html).toContain("max-w-full");
    expect(html).toContain("max-h-64");
  });

  it("is directly visible (no Collapsible disclosure) with a click-to-expand affordance", () => {
    const html = renderToStaticMarkup(
      createElement(InlineAttachmentRenderer, { attachment: imageAttachment }),
    );
    // The thumbnail itself is a button that opens the full-size dialog…
    expect(html).toContain('aria-label="Show image full size"');
    // …and the collapsed-by-default dialog contributes no markup until opened.
    expect(html).not.toContain('role="dialog"');
  });

  it("uses the alt override when given, falling back to the imageAttachment label", () => {
    const withAlt = renderToStaticMarkup(
      createElement(InlineAttachmentRenderer, {
        attachment: {
          ...imageAttachment,
          alt: "Screenshot of the build failure",
        },
      }),
    );
    expect(withAlt).toContain('alt="Screenshot of the build failure"');

    const labeled = renderToStaticMarkup(
      createElement(InlineAttachmentRenderer, {
        attachment: imageAttachment,
        labels: {
          imageAttachment: "Angehängtes Bild",
          expandImage: "Bild vergrößern",
        },
      }),
    );
    expect(labeled).toContain('alt="Angehängtes Bild"');
    expect(labeled).toContain('aria-label="Bild vergrößern"');
  });

  it("renders inside an InlineAttachments list alongside other kinds", () => {
    const html = renderToStaticMarkup(
      createElement(InlineAttachments, {
        attachments: [
          { kind: "file_view", path: "a.txt", text: "hi" },
          imageAttachment,
        ],
      }),
    );
    expect(html).toContain("a.txt");
    expect(html).toContain(`data:image/png;base64,${PNG_B64}`);
  });
});

describe("chatTranscriptMarkdownComponents: img", () => {
  it("constrains markdown-embedded images (max-width, rounded) and keeps the src", () => {
    const Img = chatTranscriptMarkdownComponents.img as (
      props: React.ComponentPropsWithoutRef<"img">,
    ) => React.ReactElement;
    const html = renderToStaticMarkup(
      createElement(Img, {
        src: `data:image/png;base64,${PNG_B64}`,
        alt: "inline",
      }),
    );
    expect(html).toContain("max-w-full");
    expect(html).toContain("rounded-lg");
    expect(html).toContain(`src="data:image/png;base64,${PNG_B64}"`);
    expect(html).toContain('alt="inline"');
  });
});
