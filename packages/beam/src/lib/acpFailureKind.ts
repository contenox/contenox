/**
 * Classifies a chat-page failure into the specific *component* that broke, so
 * the UI can say "backend unreachable" vs. "default model not servable" vs.
 * "this agent needs a workspace" vs. "chain failed" instead of one generic
 * "execution failed" banner (see TODO.md "Error handling / recovery UX").
 *
 * Two independent signals feed into the SAME taxonomy here:
 *  - `classifyAcpExecutionError` reads the live `session.error` text from a
 *    failed `session/prompt` turn (see `acpWorkspaceController.ts`'s
 *    `sendPrompt`/`newSession`) — this is also where `workspace_required` is
 *    detected (an external ACP agent's fail-closed sandbox refusal; see
 *    `WORKSPACE_REQUIRED_PATTERN` below).
 *  - `classifySetupIssueCode` reads the `code` of a blocking issue from
 *    `GET /setup-status` (see `runtime/internal/setupcheck/setupcheck.go`) —
 *    scoped to backend/model issue codes only; there is no setup-status code
 *    for a per-session missing workspace, so that path never returns
 *    `workspace_required`.
 *
 * Both detection paths can fire for the SAME underlying backend/model problem
 * (e.g. modeld down): a prompt fails immediately with "no models found in
 * runtime state" while `/setup-status` is still catching up on its next poll.
 * Converging both paths onto one shared taxonomy — and therefore one shared
 * set of headline/description copy (`acpFailureCopyKeys` below) — is what
 * turns two differently-worded, successively-shown error surfaces into one
 * consistent state.
 */
export type AcpFailureKind = 'backend_unreachable' | 'model_unavailable' | 'workspace_required' | 'generic';

/**
 * Matches `llmresolver.ErrNoAvailableModels` ("no models found in runtime
 * state", requestresolver.go) and the connectivity-flavored substrings
 * `setupcheck.go`'s `classifyBackendError` already treats as "unreachable"
 * (connection refused, dial tcp, no such host, context deadline exceeded,
 * the modeld-specific phrasings). This is a backend/runtime-daemon problem —
 * fixed by starting/reaching the backend, not by picking a different model.
 */
const BACKEND_UNREACHABLE_PATTERN =
  /no models found in runtime state|modeld (?:is )?not (?:running|available)|modeld unavailable|requires a running modeld|connection refused|dial tcp|no such host|context deadline exceeded|network is unreachable/i;

/**
 * Matches `llmresolver.ErrNoSatisfactoryModel` ("no model matched..."), the
 * `llmrepo.go` "client resolution failed" wrapper, and the context-overflow
 * message from `requestresolver.go` ("request needs N tokens of context but
 * the largest available model ... provides only M"). All three mean the
 * BACKEND is fine but the *configured default model* can't serve this
 * request — fixed on the Settings page (default model selection), not by
 * restarting anything.
 */
const MODEL_UNAVAILABLE_PATTERN =
  /no model matched|client resolution failed|tokens? of context but the largest available model|provides only \d+/i;

/**
 * Matches the agent host's fail-closed sandbox refusal (`agenthost.go`'s
 * `sandbox external ACP agent ...: cwd is required to confine the agent`,
 * deeply wrapped by `acpsvc`/`agentinstance` above it) — an external ACP
 * agent (e.g. a registered "claude" agent) started in a session with no
 * workspace/cwd. This is neither a backend nor a model problem: the agent
 * host refuses to run an external agent unconfined, so the fix is opening
 * the agent from a session that HAS a workspace, not retrying or changing
 * Settings. Matched on the stable substring rather than the full wrapped
 * chain (`acpsvc: ... agentinstance: ... agenthost: ...`) so a legible
 * notice can replace that raw nested wire string, mirroring
 * `workspaceRoots.ts`'s `isWorkspaceRootRefusal`.
 */
const WORKSPACE_REQUIRED_PATTERN = /cwd is required to confine the agent/i;

export function classifyAcpExecutionError(message: string | null | undefined): AcpFailureKind {
  if (!message) return 'generic';
  if (BACKEND_UNREACHABLE_PATTERN.test(message)) return 'backend_unreachable';
  if (MODEL_UNAVAILABLE_PATTERN.test(message)) return 'model_unavailable';
  if (WORKSPACE_REQUIRED_PATTERN.test(message)) return 'workspace_required';
  return 'generic';
}

/** The i18n keys for a failure kind's headline + plain-language body, shared
 * between the top recovery banner (`SessionBanners`' `ExecutionErrorBanner`)
 * and the in-transcript failed-turn card (`TranscriptItems`' `TranscriptError`)
 * so both read from ONE mapping instead of two hand-kept ternary chains.
 * `'generic'` returns null keys — each caller keeps its own generic fallback
 * copy (`acp_chat.error_banner_headline` vs. `acp_chat.turn_failed_label`),
 * since those two callers deliberately use different generic wording. */
export interface AcpFailureCopyKeys {
  titleKey: string | null;
  descriptionKey: string | null;
}

export function acpFailureCopyKeys(kind: AcpFailureKind): AcpFailureCopyKeys {
  switch (kind) {
    case 'backend_unreachable':
      return {
        titleKey: 'acp_recovery.backend_unreachable_title',
        descriptionKey: 'acp_recovery.backend_unreachable_description',
      };
    case 'model_unavailable':
      return {
        titleKey: 'acp_recovery.model_unavailable_title',
        descriptionKey: 'acp_recovery.model_unavailable_description',
      };
    case 'workspace_required':
      return {
        titleKey: 'acp_recovery.workspace_required_title',
        descriptionKey: 'acp_recovery.workspace_required_description',
      };
    case 'generic':
    default:
      return { titleKey: null, descriptionKey: null };
  }
}

/**
 * Mirrors the same taxonomy for a `setup-status` blocking issue code (see
 * `setupcheck.go`'s `Evaluate`/`addDefaultProviderIssues`). Codes not listed
 * here (auth/API-key failures, no backends registered, etc.) are genuinely
 * different fixes and stay `'generic'` — they keep the existing
 * `SetupRequiredState` treatment.
 */
export function classifySetupIssueCode(code: string | null | undefined): AcpFailureKind {
  switch (code) {
    case 'runtime_state_empty':
    case 'all_backends_unreachable':
    case 'default_provider_unreachable':
    case 'default_provider_not_synced':
      return 'backend_unreachable';
    case 'default_model_not_available':
    case 'missing_default_model':
      return 'model_unavailable';
    default:
      return 'generic';
  }
}
