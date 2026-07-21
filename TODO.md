# TODO — UX/design issues from live testing

## Fleet / missions / inbox (live-tested 2026-07-21, first real dispatch)

Observed driving a real mission (`chain-acp`, strict HITL envelope) from Beam
over LAN. The fleet surfaces are functionally wired but not operable yet:

- [ ] **Fleet board layout is unusable.** Declared-but-idle agents render as
      repetitive near-empty cards ("Deklariert, aber nicht laufend…" × N),
      and the one card that matters — a running instance with its
      status/sessions/viewers table — is crushed into the same small card
      format. Idle agents should be a compact secondary list; running units
      deserve the space. Redesign the page around "what is running now",
      not around the declared-agent grid.
- [ ] **Status vocabulary split reads as contradiction.** The board shows the
      instance state ("Läuft"/green) while the missions page shows the
      mission status ("Offen") plus "Noch nie gemeldet" for the heartbeat —
      three different truths about ONE unit with no reconciliation. The UI
      must present unit state + mission status + liveness as one composed
      picture (and once terminal statuses land, "open" must never read like
      a health indicator).
- [ ] **No session peek — the operator is blind.** There is no way to see
      what a dispatched unit is doing: no transcript view, no live stream,
      nothing. This is THE missing piece for trust (fleetservice's documented
      "no adoption into a beam chat session" v1 limitation, now confirmed as
      the first thing a real operator reaches for). The kernel already
      supports viewer attach + journal replay — Beam needs a read-only
      session viewer on the instance (attach as observer), reachable from
      board and mission detail.
- [ ] **Stuck-unit blindness (the trust collapse).** A unit that is stuck or
      waiting produced NOTHING in the inbox, and with no session peek and no
      heartbeat the operator cannot distinguish "working", "blocked on an
      unsurfaced ask", and "dead". Whatever the root cause of the missing
      ask (under investigation), the UX requirement stands: a unit in ANY
      wait state must be visible as such on the board/inbox, and "no signal
      for N minutes" is itself an attention-worthy inbox condition.
- [ ] **Fleet/mission/inbox pages ignore mobile and the house UI patterns.**
      None of the new pages work in a mobile viewport, and they diverge from
      the layout/component patterns the other admin pages use. Audit them
      against the existing pages' responsive patterns and bring them in line
      (same tokens, same list/card idioms, same breakpoint behavior).

# Archived — chat-path findings (2026-07-16)

## Chat path polish (the actual product surface)

The chat UI reads impressive at first glance but lacks every interaction that
makes an agent chat usable in practice:

- [x] **Live streaming render.** Backend streaming with tools landed
      2026-07-16 (taskexec + llama client Stream). Render-polish closed:
      `TranscriptItems.tsx` renders assistant/user text through ReactMarkdown
      (partial-markdown handling — see next item) and wires
      `ChatStreamingCaret` on the actively-streaming message plus
      `ChatTranscriptStreamingPlaceholder` for the gap after a turn starts but
      before the first chunk arrives (`chatTranscript.tsx`); `AcpChatPage.tsx`
      wires `ChatScrollToLatest` (gated on `!isNearBottom` from
      `useChatScroll`) so a user scrolled up isn't yanked to the bottom by
      incoming chunks. REMAINING: this was verified via `tsc`/`vitest`/`build`
      only — live end-to-end verification against a real streaming session
      (actual modeld + contenox serve) is pending; this slice was explicitly
      told not to drive the live app.
- [ ] **Waiting/elapsed feedback.** No "agent is responding…" state, no
      elapsed-time indicator ("responding for 12s"), no distinction between
      queued (model cold-loading, single slot busy) and generating. Local
      inference is slow — the UI must own that honestly.
- [x] **Tool cards.** Shipped: tool calls render through `ToolCallCard`
      (`@contenox/ui`) in `TranscriptItems.tsx` — tool title/kind,
      pending/running/succeeded/failed status, and a collapsible `ToolCallDetail`
      (target locations, `DiffView` for edits, raw output) rather than raw dumps.
- [x] **File peek.** Shipped: file edits render as a `DiffView` inside the
      tool-call card's detail; clicking a workspace file opens it as a read-only
      canvas tab (`fileCanvasTab` / `CanvasRegion`), and the `@`-mention menu
      shows a live `useFilePreview` of the highlighted file.
- [x] **@-mentions.** Shipped: typing `@` in the composer opens `MentionMenu`
      (`useMentionMenu` in `ChatSessionTab.tsx`) to attach workspace files, which
      ride the prompt as reference `resource_link` blocks (`activeMentions`).
- [ ] **Dark-mode shade/contrast audit.** Light mode reads noticeably more
      polished than dark mode (observed 2026-07-16): dark shades and contrast
      steps are flatter — surfaces, borders, and muted text blur together.
      Audit the dark palette tokens against the light ones (border visibility,
      elevation steps, muted-text contrast) and fix at the token level, not
      per-component.
- [x] **Proper previews.** Audited: message text was plain
      `whitespace-pre-wrap`, falling back for everything. Wired the
      already-built `packages/ui` chat components into
      `TranscriptItems.tsx`'s `TranscriptMessage`: text now renders through
      `ReactMarkdown` + `remark-gfm` + `chatTranscriptMarkdownComponents`
      (code blocks, inline code, blockquotes, GFM tables/strikethrough),
      same pattern as `ChatSurface.tsx` (the VS Code webview chat,
      `ChatSurface.tsx:1046-1050`/`1093-1096`). Thought blocks render through
      `ChatStreamThinkingBox` instead of a plain `<p>`. Diffs already render
      via `DiffView` in `ToolCallDetail` (pre-existing, unaffected). Raw image
      content blocks were not in this slice's harvest list (no ACP image
      content is currently produced) and remain unaddressed.

Deferred design issues observed while live-testing beam chat against a real
modeld + contenox serve. Infra/plumbing issues are NOT listed here — they are
being fixed directly.

## Error handling / recovery UX

- [x] **Double error screen when modeld is down.** When modeld is unreachable,
      the user gets two stacked/successive error surfaces instead of one clear
      "runtime backend unreachable" state with a retry affordance. Deduplicate
      into a single error state.
- [x] **Wrong redirect target for broken default model.** When modeld is up but
      the configured default model can't serve (e.g. default is an OpenVINO
      artifact while the llama backend is running), beam redirects to the
      **Backends** page — but the actual fix lives on the **Settings/Config**
      page (default model selection). Redirect (or at least deep-link) to where
      the user can fix it, and say *what* is misconfigured.
- [x] **Error states should name the failing component.** "modeld down",
      "default model not servable", "chain failed" are different failures with
      different fixes; the UI should distinguish them instead of generic error
      screens.

## Chat session controls

- [x] **No session controls on an empty chat.** Model/HITL/Think/Token-Limit
      controls are `session.configOptions`, built server-side per live session
      (`runtime/acpsvc/config_options.go:33`) and pushed via `session/update`
      after `session/new`. Sessions are minted lazily on first prompt submit,
      so on a fresh "Neue Sitzung" chat there is no session and therefore no
      controls — you cannot pick the model *before* the first turn, which is
      precisely the turn that fails when the default model is broken.
      Fix options: (a) session-less config surface (workspace-level
      config_options at initialize/connect, staged client-side, applied right
      after session/new in handleSubmit); (b) eager session mint on
      "Neue Sitzung" (needs empty-session pruning so the sidebar doesn't fill
      with husks). Option (a) is protocol/plumbing work, not just UI.
      RESOLVED via option (a). Server advertises workspace-level config options
      in the `initialize` response `_meta` under
      `contenox.workspaceConfigOptions` (`runtime/acpsvc/initialize.go`,
      `runtime/acpsvc/config_options.go` `workspaceConfigOptions`); a spec-safe
      extension conformant clients ignore. Lazy mint is kept. Client renders the
      controls on the empty chat from those options, stages picks locally, and
      flushes them via `set_config_option` right after `session/new` and before
      the first prompt (`packages/beam/.../acpWorkspaceController.ts`
      `applyConfigOptions`, `AcpChatPage.tsx` handleSubmit).

## Settings page (beam)

- [ ] **Inline help boxes, including condition-triggered ones.** Tooltips
      (shipped 2026-07-16) are a good start but hide the guidance behind
      hover. The CLI goes further: it prints inline hints, many
      condition-triggered (`doctor` suggests the exact fix for the detected
      state, `setup` explains the next step, commands hint follow-ups on
      success/failure). The UI needs the equivalent: persistent inline help
      boxes per section, plus conditional ones driven by runtime state
      ("no backend registered — add one below", "default model not servable
      on the active engine — pick a servable one", "think unset: falls back
      to High"). Ties into the runtime-generated-UI blueprint: conditional
      help is server-known state and belongs in served specs, not hardcoded
      client logic.
- [x] **Settings changes have no observable effect.** INVESTIGATED: values
      *are* persisted (SQLite KV via `runtime/internal/clikv`, same store the
      CLI uses) and every form already showed Saved/error feedback in the
      current tree. The real gap was plumbing, and it's asymmetric:
      - `hitl-policy-name` is read live on every `session/update`
        (`runtime/acpsvc/config_options.go` `activeHITLPolicy` →
        `clikv.ReadHITLPolicy`) — already applies immediately, no fix needed.
      - `default-model`/`-provider`/`-alt-model`/`-alt-provider`/`-max-tokens`/
        `-think` are seeded into `acpsvc.Transport` once from `Deps` at
        `contenox serve` startup (`runtime/acpsvc/transport.go:92` "seed from
        Deps at construction; not read again") — genuinely frozen for the
        life of the beam/ACP chat connection. RESOLUTION: surfaced this
        explicitly instead of silence — a restart notice on
        GlobalSettingsSection/ResponseSettingsSection
        (`settingsAdvanced.restart_notice`) states these apply immediately to
        the OpenAI-compatible API / VS Code / task-chain runs (all read them
        live per-request via `stateservice.ResolveRuntimeDefaults`,
        confirmed in `runtime/internal/compatapi/openai_chat.go:33`) but need
        `contenox serve` restarted to reach the beam chat window.
      - `default-chain` (Workspace-Standards) doesn't affect beam/ACP chat
        *at all*, restart or not: the ACP chain is loaded once from
        `~/.contenox/default-acp-chain.json` (`runtime/acpsvc/chain.go`),
        entirely separate from the `default-chain` KV key. That key only
        drives the OpenAI-compatible API and the VS Code extension. Surfaced
        via `settingsAdvanced.chain_scope_notice` on WorkspaceSettingsSection
        rather than leaving it silently inert.
      Deep architectural rework to make the ACP connection itself hot-reload
      is out of scope for this slice (large, cross-cutting with concurrent
      Go taskengine/session-controls work); restart-required is the honest,
      shipped resolution per the task's own fallback instruction.
- [x] **Zero guidance on the Settings page.** Ported CLI help text
      (`runtime/contenoxcli/config_cmd.go` `validConfigKeys`) into tooltips
      across every field, in a new `settingsAdvanced` i18n namespace
      (`packages/beam/src/i18n.ts`, en+de). `default-think`'s tooltip states
      the "unset → High" runtime fallback explicitly and suggests Low/Medium
      on local GPUs; `default-alt-model`/`-provider`'s tooltip explains the
      chain router/recovery-step usage. "Inherit"/"Not set" options are
      labeled and explained inline.
- [x] **Settings page only covered 4 of 12 `contenox config` keys.** Added
      the missing 8: `default-alt-model`/`-alt-provider` (GlobalSettingsSection),
      `default-max-tokens`/`-think` (new ResponseSettingsSection),
      `default-autocomplete-model`/`-provider` (new AutocompleteSettingsSection),
      `telemetry-enabled`/`update-check` (new TelemetrySettingsSection, as
      switches). Backend: added `GET /api/cli-config` (full snapshot, all 12
      keys) and extended `PUT /api/cli-config` to accept `telemetry-enabled`/
      `update-check` (`runtime/internal/setupapi/routes.go`,
      `runtime/stateservice/stateservice.go`); `default-think` is now
      validated server-side the same way the CLI does
      (`reasoning.Normalize`). OpenAPI spec regenerated (`make openapi`).
- [x] **Einrichtungsassistent (setup wizard) entry appears dead.** Verified:
      already wired correctly in the current tree
      (`packages/beam/src/pages/admin/settings/SetupWizardSection.tsx` calls
      `openWizardManually()` from `lib/wizardDismissal.ts`; `Layout.tsx`'s
      `showWizard` honors the resulting `manualOpen` flag and renders the
      wizard full-screen). No further action needed here.
- [x] **Sidebar toggle inside the setup wizard did nothing.** The wizard
      replaces the whole main area and never renders `<Sidebar>`, but the
      navbar's `SidebarToggle` still rendered and toggled dead state.
      Hidden it while `showWizard` is true
      (`packages/beam/src/components/Layout.tsx`, navbar's toggle condition).

## Model provisioning page ("Modell bereitstellen")

- [ ] **Visual redesign — the list is unusable.** 39 flat rows rendering raw
      HuggingFace URLs as primary content, size crammed onto the URL string
      ("...Q4_K_M.gguf12723 MB"). Should be cards/rows built from structured
      fields: model name, parameter size, quant, backend badge (GGUF/OpenVINO),
      size in GB, curated badge — URL demoted to a details/tooltip affordance.
- [ ] **Group and dedupe variants.** Nearly every model appears twice
      (`qwen3-14b` + `qwen3-14b-ov`); group GGUF/OpenVINO variants of the same
      model into one entry with a backend picker, and group by family.
- [ ] **Fit-for-this-device indicator.** The registry knows the artifact size
      and modeld knows the device budget (weights + KV floor + reserves —
      see modeld/capacity). Show "fits fully / won't fit on your GPU" per
      model, so users don't download a 4.8GB model onto a 6GB card and hit
      degraded serving. This is the exact trap the default qwen3-8b setup
      walked into.
- [ ] **Search/filter/sort.** 39 entries need a filter box, backend filter,
      and size sort at minimum.
- [ ] **Download feedback.** "Die Runtime erkennt es automatisch" — but there
      is no visible progress/state for a multi-GB download.
- [ ] **"Modell registrieren" form** needs validation and help text (what URL
      shapes are accepted, GGUF file vs OpenVINO repo).

## Resource/feedback transparency

- [x] **No progress signal while the model is generating.** A reply to "hi" can
      take minutes (cold load + long reasoning chain) with no token streaming or
      progress indication, which reads as "the response never lands". Surface
      generation progress / streaming state in the transcript.
