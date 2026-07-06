# Blueprint: Beam Chat canvas

## Product thesis

Beam Chat should have two stable roles:

```text
Chat   = conversation, intent, feedback, steering
Canvas = the thing being inspected, previewed, or iterated on
```

The canvas is not an editor by default. It is not a filesystem explorer, not a
terminal-first workspace, and not a replacement for Zed/VS Code/ACP. It is the
place where the user can see the current artifact and give feedback.

That artifact can later be anything. The first milestone should create the slot
and interaction model, not pretend we already know every renderer.

## UX shape

Desktop:

```text
+--------------------------------------+------------------------------+
| Chat toolbar                         | Canvas toolbar               |
|--------------------------------------|------------------------------|
| Chat thread                          | Canvas renderer              |
|                                      |                              |
|                                      | Empty state at first         |
|--------------------------------------|                              |
| Composer                             |                              |
+--------------------------------------+------------------------------+
```

Mobile:

- Chat remains primary.
- Canvas opens as a full-screen overlay or bottom sheet.
- The toolbar exposes a compact canvas toggle.

The canvas must be:

- resizable on desktop
- collapsible
- state-persistent per browser
- renderer-agnostic
- safe to show as an empty placeholder before capabilities land

## Terminology

Use **canvas** as the product term.

Avoid:

- workspace, when the surface is not a general workspace
- terminal, as the main concept
- IDE/ADE language
- run log, as the primary second pane

The run log is useful telemetry, but it is not the user's main object of
attention. It can become a canvas renderer later or move behind a debug affordance.

## Canvas artifact model

Start with a small discriminated type on the Beam side:

```ts
type CanvasArtifact =
  | { kind: 'empty' }
  | { kind: 'message'; title?: string; body: string }
  | { kind: 'run'; requestId?: string }
  | { kind: 'url_preview'; url: string }
  | { kind: 'markdown'; title?: string; content: string }
  | { kind: 'image'; src: string; alt?: string }
  | { kind: 'video'; src: string }
  | { kind: 'diff'; files: unknown[] };
```

The first implementation only needs `empty` and perhaps `message`. The shape is
there so later work has somewhere to land.

This should remain distinct from chat context artifacts:

- `ChatContextArtifact` is what gets sent to the model.
- `CanvasArtifact` is what the human inspects.

Some future events may produce both. For example, a Vite preview URL may be a
canvas artifact, while the current screenshot or DOM summary becomes a context
artifact only when the user explicitly attaches it.

## Relationship to existing artifacts

The current Beam code already has an artifact registry and inline attachment
mapping. That is useful but not sufficient.

Inline attachments are local to a chat turn:

```text
"I attached this file/terminal output to my message."
```

Canvas is session-level:

```text
"This is the thing we are working on right now."
```

The first canvas slice should not overload the existing inline attachments.
Instead, introduce a small canvas state provider near `ChatPage`:

```ts
type CanvasState = {
  open: boolean;
  artifact: CanvasArtifact;
};
```

Future renderers can update that state from task events, slash commands, tool
outputs, or explicit user actions.

## What replaces the Run Log

Today `ChatRunLog` occupies the side rail. That makes the second pane internal
telemetry by default.

Replace that side rail with `CanvasPanel`.

Initial behavior:

- Canvas is closed or open based on persisted UI state.
- If open and no artifact exists, show an empty state:

```text
Canvas
No canvas yet.
Generated previews, documents, images, diffs, and run views will appear here.
```

- Keep run events available for development, but demote them:
  - temporary debug toggle inside canvas, or
  - a future `kind: 'run'` canvas renderer, or
  - a collapsible detail section below the chat thread.

Do not keep the run log as the default second pane.

## First implementation slice

Goal: create the durable canvas slot without adding a real renderer yet.

1. Add `CanvasPanel` component.
2. Add a `CanvasProvider` or local `ChatPage` state for:
   - open/closed
   - width
   - current artifact
3. Replace `ChatRunLog` side rail with the resizable canvas panel.
4. Add a toolbar toggle with a simple canvas icon/label.
5. Persist open/closed and width in local storage.
6. On mobile, open canvas as an overlay.
7. Render only the empty placeholder initially.

Acceptance criteria:

- Chat still works with canvas closed.
- Chat still works with canvas open.
- The right panel is resizable and persists size.
- The page no longer presents Run Log as the primary side pane.
- No Vite preview, file explorer, terminal, or renderer-specific work is required.

## Follow-up slices

### Slice 2: Run renderer

Move the old run log into a `CanvasArtifact` renderer:

```ts
{ kind: 'run', requestId }
```

This keeps telemetry available without making telemetry the product concept.

### Slice 3: URL preview renderer

Add:

```ts
{ kind: 'url_preview', url }
```

This can later host a Vite/local web preview, external link preview, or generated
web artifact. It should start as an iframe with conservative sandboxing and clear
origin display.

### Slice 4: Markdown/document renderer

Add:

```ts
{ kind: 'markdown', title, content }
```

This is the likely first non-developer work surface. It supports reports, specs,
plans, notes, generated docs, and user feedback without implying code editing.

### Slice 5: Image/video renderer

Add image and video renderers once the runtime has a way to persist or reference
generated media.

### Slice 6: Diff renderer

Diff is useful for developer workflows, but it should be one renderer among many,
not the reason the canvas exists.
