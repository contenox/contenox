# OpenIDE Integration Blueprint

Target repository: `https://gitflic.ru/project/openide/openide.git`

OpenIDE is an IntelliJ Platform IDE, not a VS Code distribution. The current
Contenox VS Code extension cannot be installed directly as a VSIX into OpenIDE.
The practical integration path is a native IntelliJ Platform plugin that reuses
the existing Contenox runtime and stdio bridge concepts.

## Source Facts

- OpenIDE describes itself as the open-source part of the JetBrains IDEs
  codebase and as a basis for IntelliJ Platform development.
- OpenIDE source builds require the OpenIDE project checkout plus companion
  Android modules from `getPlugins.sh` or `getPlugins.bat`.
- OpenIDE build instructions target OpenIDE 2024.3 or newer and JetBrains
  Runtime 21 without JCEF.
- JetBrains recommends the IntelliJ Platform Gradle Plugin 2.x for building,
  testing, verifying, and publishing plugins for IntelliJ-based IDEs.

Useful links:

- OpenIDE project: https://gitflic.ru/project/openide/openide
- IntelliJ Platform plugin project docs:
  https://plugins.jetbrains.com/docs/intellij/creating-plugin-project.html
- IntelliJ Platform Gradle Plugin:
  https://plugins.jetbrains.com/docs/intellij/tools-intellij-platform-gradle-plugin.html
- Publishing IntelliJ Platform plugins:
  https://plugins.jetbrains.com/docs/intellij/publishing-plugin.html

## Product Boundary

The OpenIDE integration should expose the same user-facing Contenox value as the
VS Code extension:

- local-first setup and health checks
- codebase chat with selected editor context
- persisted sessions with meaningful titles
- diagnostics explanation and fixes
- workspace review and commit-message drafting
- model/provider selection backed by Contenox config
- HITL approvals for gated tool actions
- inspectable local output and telemetry

It should not fork the Contenox engine into Kotlin. The plugin should be an IDE
adapter around the Go runtime, the same way the VS Code extension is an IDE
adapter around `contenox`.

## Recommended Architecture

Create a new package:

```text
packages/openide/
```

Suggested modules:

```text
packages/openide/
  build.gradle.kts
  gradle.properties
  settings.gradle.kts
  src/main/kotlin/com/contenox/openide/
    ContenoxPlugin.kt
    bridge/
      BridgeProcess.kt
      JsonRpcFramer.kt
      Protocol.kt
    actions/
      OpenChatAction.kt
      AskSelectionAction.kt
      FixDiagnosticsAction.kt
      ReviewChangesAction.kt
      RunSetupAction.kt
    sessions/
      SessionToolWindow.kt
      SessionTreeModel.kt
    chat/
      ChatToolWindow.kt
      ChatController.kt
    config/
      ContenoxSettings.kt
      SettingsConfigurable.kt
    approvals/
      ApprovalDialog.kt
```

The plugin should launch a local Contenox process and speak a framed JSON-RPC
protocol over stdio. Initially, reuse the VS Code bridge protocol implemented by
`runtime/vscodeagent` to reduce backend work. After the API stabilizes, rename
the Go package and CLI command to a neutral name such as `ideagent` while
keeping compatibility aliases for the VS Code extension.

## Runtime Contract

The OpenIDE plugin should depend on the same runtime capabilities as the VS Code
extension:

- `initialize`
- `health`
- `getConfig`
- `setConfig`
- `listProviders`
- `listModels`
- `listCommands`
- `sessionCreate`
- `sessionList`
- `sessionLoad`
- `sessionDelete`
- `chatSend`
- `chatCancel`
- `approvalRespond`
- `listMCPServers`

Notifications to support:

- `chatStarted`
- `chatDelta`
- `chatCompleted`
- `chatCancelled`
- `chatFailed`
- `toolCall`
- `approvalRequested`
- `configChanged`

The bridge must preserve streaming text exactly as received from chat-completion
events. Internal route/classifier chunks must not be rendered as chat markdown.

## OpenIDE Plugin UX

Map Contenox features to native IntelliJ Platform surfaces:

- Tool window: `Contenox`, containing chat and session list tabs.
- Actions: `Tools | Contenox | Open Chat`, `Ask About Selection`,
  `Fix Diagnostics`, `Review Workspace Changes`, `Draft Commit Message`,
  `Run Setup`, and `Show Status`.
- Editor popup actions for selection-aware chat and fixes.
- Inspection/intentions integration for diagnostics explanation and fixes.
- Settings page for runtime path, data directory, provider/model defaults,
  autocomplete settings, telemetry, and HITL policy.
- Notification balloons for bridge startup failures, setup-required status, and
  cancelled turns.
- Modal approvals for tool calls that cross policy boundaries.

Do not use a webview as the first implementation unless native Swing/IntelliJ UI
becomes a blocker. Native tool windows and actions will fit OpenIDE better and
avoid duplicating a browser app inside the IDE.

## Packaging And Distribution

Use the IntelliJ Platform Gradle Plugin 2.x.

Initial Gradle targets:

```sh
./gradlew runIde
./gradlew test
./gradlew verifyPlugin
./gradlew buildPlugin
```

Package strategy:

- Bundle one platform-specific `contenox` binary per plugin artifact, or require
  an external `contenox` path for early development builds.
- Prefer external runtime selection during the first prototype to avoid signing
  and cross-platform packaging work before the API is validated.
- Add bundled runtime artifacts only after the VS Code packaging matrix is
  stable.
- Keep the plugin ID distinct from the VS Code extension ID, for example
  `com.contenox.openide`.

Publishing options:

- JetBrains Marketplace for public OpenIDE/IntelliJ-compatible distribution.
- A custom plugin repository for internal or pre-release OpenIDE deployments.
- Manual ZIP install from `build/distributions/*.zip` for early smoke testing.

## Implementation Phases

1. Prototype bridge process management.
   Launch `contenox vscode-agent --stdio`, send `initialize`, `health`, and
   `shutdown`, and show bridge status in a tool window.
2. Add session and chat basics.
   Implement `sessionList`, `sessionCreate`, `sessionLoad`, `chatSend`, and
   streaming `chatDelta` rendering.
3. Add editor context.
   Attach selected text, active file content, language ID, and workspace path to
   chat requests.
4. Add actions and diagnostics.
   Wire selection actions, diagnostics explanation/fix actions, workspace review,
   and commit-message drafting.
5. Add approvals and tool-call UI.
   Render `toolCall` events and block on `approvalRequested` dialogs.
6. Add settings and setup.
   Expose runtime path, data directory, provider/model selection, autocomplete
   defaults, and HITL policy.
7. Harden packaging.
   Add plugin verification, sandbox smoke tests, platform runtime packaging, and
   marketplace/custom-repository release jobs.

## Acceptance Criteria

- OpenIDE can start the plugin without requiring VS Code or a VSIX install path.
- The plugin can launch or connect to a local `contenox` runtime.
- `health` reports setup-required versus configured status accurately.
- A user can open Contenox chat, send a selected-code prompt, and receive a
  streamed answer without route labels or glued internal output.
- Sessions persist through IDE restart and use meaningful titles derived from
  the prompt or slash command.
- Diagnostics actions include the relevant file, range, message, and language.
- Tool approvals block execution until the user accepts or denies the request.
- Plugin verification runs in CI against the selected OpenIDE/IntelliJ baseline.
- The plugin can be installed from a ZIP artifact into a clean OpenIDE profile.

## Open Questions

- Does OpenIDE publish stable product codes and downloadable IDE artifacts that
  the IntelliJ Platform Gradle Plugin can consume directly, or do we need a
  local IDE path for `runIde` and `verifyPlugin`?
- Should the first plugin target OpenIDE only, or all compatible IntelliJ-based
  IDEs?
- Should the runtime bridge be renamed from `vscodeagent` before OpenIDE work
  starts, or kept as a compatibility layer until both editor adapters are stable?
- Which marketplace is the preferred public channel for OpenIDE users:
  JetBrains Marketplace, an OpenIDE-specific plugin repository, or both?
