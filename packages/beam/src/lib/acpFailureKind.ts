/**
 * Classifies a chat-page failure into the specific *component* that broke, so
 * the UI can say "backend unreachable" vs. "default model not servable" vs.
 * "chain failed" instead of one generic "execution failed" banner (see
 * TODO.md "Error handling / recovery UX").
 *
 * Two independent signals feed into the SAME three-way taxonomy here:
 *  - `classifyAcpExecutionError` reads the live `session.error` text from a
 *    failed `session/prompt` turn (see `acpWorkspaceController.ts`'s
 *    `sendPrompt`/`newSession`).
 *  - `classifySetupIssueCode` reads the `code` of a blocking issue from
 *    `GET /setup-status` (see `runtime/internal/setupcheck/setupcheck.go`).
 *
 * Both detection paths can fire for the SAME underlying problem (e.g. modeld
 * down): a prompt fails immediately with "no models found in runtime state"
 * while `/setup-status` is still catching up on its next poll. Converging
 * both paths onto one shared taxonomy — and therefore one shared set of
 * headline/description copy in AcpChatPage.tsx — is what turns two
 * differently-worded, successively-shown error surfaces into one consistent
 * state.
 */
export type AcpFailureKind = 'backend_unreachable' | 'model_unavailable' | 'generic';

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

export function classifyAcpExecutionError(message: string | null | undefined): AcpFailureKind {
  if (!message) return 'generic';
  if (BACKEND_UNREACHABLE_PATTERN.test(message)) return 'backend_unreachable';
  if (MODEL_UNAVAILABLE_PATTERN.test(message)) return 'model_unavailable';
  return 'generic';
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
