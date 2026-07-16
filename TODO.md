# TODO — UX/design issues from live testing (2026-07-16)

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
- [ ] **Tool cards.** Tool calls need proper cards: which tool, the key
      arguments, running/succeeded/failed state, collapsible result payload —
      not raw dumps or invisible execution.
- [ ] **File peek.** When a tool touches a file (read/write/edit), the
      transcript should offer an inline peek/diff of the file content, not
      just the path string.
- [ ] **@-mentions.** No way to @-reference files/resources in the prompt
      composer (ACP supports resource blocks; the composer should support
      typing @ to attach workspace files).
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
