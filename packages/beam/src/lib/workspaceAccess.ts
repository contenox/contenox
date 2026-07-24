/**
 * The workspace access-preview transport: hand-mirrored TS types for the
 * `POST /workspace/access` batch verdict service plus the pure helpers the
 * `useWorkspaceAccess` hook composes (kept out of the hook so the request/URL
 * building, the path-union keying, and the response→map projection are unit-
 * testable without react-query or a DOM).
 *
 * Types mirror the Go DTOs one-for-one:
 *   - {@link DimensionVerdict} ← runtime/accessview.DimensionVerdict
 *   - {@link PathVerdict}      ← runtime/accessview.PathVerdict
 *   - {@link AccessEvaluateRequest}/{@link AccessEvaluateResponse}
 *                              ← runtime/internal/accessapi.EvaluateRequest/Response
 *
 * WHY a distinct verdict source (not `/files?filter=agent`): the tree lists the
 * filesystem (`/files`) and asks the policy engine for verdicts (`/workspace/access`)
 * as TWO independent calls, merged by path client-side. This decouples the raw
 * listing from the policy overlay — toggling agent view no longer re-lists files,
 * and both the lazy tree and the recursive-find filter feed the same batch verb.
 */
import { apiFetch } from './fetch';

/** One access dimension's decision. Mirrors hitlservice.Action as a plain string. */
export type AccessAction = 'allow' | 'approve' | 'deny';

/** Why the action was decided. Mirrors hitlservice.ReasonMatchedRule / ReasonDefaultAction. */
export type AccessReason = 'matched_rule' | 'default_action';

/**
 * The policy decision for one access dimension (read or write) of a path —
 * mirrors accessview.DimensionVerdict. `reason` is always populated by the
 * server (the whole point of this service over the quiet agent-view); `rule` is
 * present only when `reason === 'matched_rule'`.
 */
export interface DimensionVerdict {
  action: AccessAction;
  reason?: AccessReason;
  rule?: number;
}

/**
 * The evaluated access for one requested path — mirrors accessview.PathVerdict.
 * `read`/`write` are OMITTED when `reachable` is false (a path outside the
 * workspace boundary gets no policy evaluation, so there is no verdict).
 */
export interface PathVerdict {
  path: string;
  reachable: boolean;
  read?: DimensionVerdict;
  write?: DimensionVerdict;
}

/** POST body — mirrors accessapi.EvaluateRequest. */
export interface AccessEvaluateRequest {
  paths: string[];
}

/** Response — mirrors accessapi.EvaluateResponse (`policyName` is the RESOLVED policy). */
export interface AccessEvaluateResponse {
  policyName: string;
  verdicts: PathVerdict[];
}

/**
 * The per-session "use configured default" HITL policy sentinel (mirrors acpsvc's
 * `hitlPolicyDefaultValue`). The access call omits `policy=` for it so the server
 * default-resolves the configured fallback — the same policy a defaulting live
 * agent runs under. Canonical home for the constant (re-exported by
 * `useWorkspaceFiles` for its existing importers).
 */
export const HITL_POLICY_DEFAULT_VALUE = '__contenox_default__';

/**
 * De-dupes and sorts a path list into the stable set the batch request keys on.
 * The lazy tree grows its loaded-path union as directories expand; sorting makes
 * the react-query key order-independent (a re-render with the same set in a
 * different order does not refetch) while a genuinely larger union re-keys and
 * re-requests the whole loaded union.
 */
export function dedupeSortPaths(paths: readonly string[]): string[] {
  return Array.from(new Set(paths)).sort((a, b) => a.localeCompare(b));
}

/** Projects the ordered verdict list into a `path → verdict` lookup for the merge-by-path render. */
export function verdictsToMap(verdicts: readonly PathVerdict[]): Map<string, PathVerdict> {
  const map = new Map<string, PathVerdict>();
  for (const v of verdicts) map.set(v.path, v);
  return map;
}

/**
 * Builds `POST /workspace/access?root=&policy=`. For a CONCRETE policy selection
 * `policy=<name>` pins the evaluation to it (matching the policy the live agent
 * gates under); the Default sentinel (or empty/whitespace) omits `policy` so the
 * server default-resolves — mirroring `filesUrl`'s prior policy convention.
 */
export function accessUrl(root: string, policy?: string | null): string {
  const params = new URLSearchParams({ root });
  const p = policy?.trim();
  if (p && p !== HITL_POLICY_DEFAULT_VALUE) params.set('policy', p);
  return `/api/workspace/access?${params.toString()}`;
}

/** POSTs one batch and returns the raw response (verdicts in request order + resolved policy). */
export async function fetchWorkspaceAccess(
  root: string,
  policy: string | null | undefined,
  paths: string[],
): Promise<AccessEvaluateResponse> {
  const body: AccessEvaluateRequest = { paths };
  return apiFetch<AccessEvaluateResponse>(accessUrl(root, policy), {
    method: 'POST',
    body: JSON.stringify(body),
  });
}
