import type { CapturedStateUnit, InlineAttachment } from '@contenox/ui';
export type { InlineAttachment };

export type ModelDescriptor = {
  id?: string;
  name: string;
  sourceUrl: string;
  sizeBytes: number;
  curated: boolean;
};

export type ModelRegistryEntry = {
  id: string;
  name: string;
  sourceUrl: string;
  sizeBytes: number;
  createdAt?: string;
  updatedAt?: string;
};

export type Backend = {
  id: string;
  name: string;
  baseUrl: string;
  type: string;
  /** Runtime-observed model names for this backend. */
  models: string[];
  pulledModels: ObservedModel[];
  error: string;
  createdAt?: string;
  updatedAt?: string;
};

/** POST /api/backends/{id}/models/push response (see backendapi.pushModelResponse). */
export type PushModelResult = {
  name: string;
  alreadyPresent?: boolean;
  bytesWritten?: number;
};

/** GET /api/state — runtime-observed backend state (same shape as statetype.BackendRuntimeState JSON). */
export type BackendRuntimeState = {
  id: string;
  name: string;
  models: string[];
  pulledModels: ObservedModel[];
  backend: Backend;
  error?: string;
};

export type ModeldRuntimeConfig = {
  numCtx?: number;
  hotContextTokens?: number;
  plannerEffectiveContext?: number;
  numBatch?: number;
  numThreads?: number;
  numGpuLayers?: number;
  tensorSplit?: number[];
  flashAttn?: boolean;
  kvCacheType?: string;
  promptFormat?: string;
  promptTemplateDigest?: string;
  disableBOS?: boolean;
  reasoningFormat?: string;
};

export type ModeldAdapterInfo = {
  name?: string;
  digest?: string;
  scale?: number;
};

export type ModeldActiveModel = {
  modelName?: string;
  type?: string;
  digest?: string;
  adapters?: ModeldAdapterInfo[];
  config?: ModeldRuntimeConfig;
  generation: number;
};

export type ModeldSlotStatus = {
  ownerInstanceId?: string;
  backend?: string;
  state?: string;
  active?: ModeldActiveModel;
  busyOperation?: string;
  lastError?: string;
};

export type ModeldStatusResponse = {
  state: string;
  available: boolean;
  binary?: string;
  endpoint?: string;
  instance?: string;
  backend?: string;
  error?: string;
  runtimeProtocol: number;
  minRuntimeProtocol: number;
  slot?: ModeldSlotStatus;
};

export type ModeldLocalModel = {
  id: string;
  model: string;
  name?: string;
  backendId?: string;
  backendName?: string;
  backendType: string;
  digest?: string;
  contextLength?: number;
  maxOutputTokens?: number;
  canChat: boolean;
  canEmbed: boolean;
  canPrompt: boolean;
  canStream: boolean;
  canThink?: boolean;
};

export type ModeldCapacityDevice = {
  index: number;
  name?: string;
  description?: string;
  type?: string;
  memoryFree?: number;
  memoryTotal?: number;
};

export type ModeldCapacityInfo = {
  modelMaxContext: number;
  effectiveContext: number;
  memoryContextTokens?: number;
  hotContextTokens?: number;
  plannerEffectiveContext?: number;
  kvBytesPerToken?: number;
  freeBytes?: number;
  weightsBytes?: number;
  overheadBytes?: number;
  reservedBytes?: number;
  userLimitBytes?: number;
  minFreeBytes?: number;
  hostColdBudgetBytes?: number;
  usableBytes?: number;
  requiredBytes?: number;
  clamped?: boolean;
  reason?: string;
  deviceKind?: string;
  deviceId?: string;
  deviceTotalBytes?: number;
  sharedWithDisplay?: boolean;
  requestedGpuLayers?: number;
  resolvedGpuLayers?: number;
  sparseAttention?: boolean;
  slidingWindowAttentionTokens?: number;
  chatTemplateFormat?: string;
  chatTemplateThinkingStartTag?: string;
  chatTemplateReasoningFormat?: string;
  chatTemplateSupportsToolCalls?: boolean;
  chatTemplateSupportsThinking?: boolean;
  chatTemplateSupportsReasoningEffort?: boolean;
  runtimeName?: string;
  runtimeDigest?: string;
  runtimeSystemInfo?: string;
  supportsGpuOffload?: boolean;
  devices?: ModeldCapacityDevice[];
};

export type ModeldCapacityResponse = {
  model: ModeldLocalModel;
  info: ModeldCapacityInfo;
};

export type ModeldUnloadResponse = {
  unloaded: boolean;
  expectedGeneration: number;
};

export type ModeldLoadResponse = {
  loaded: boolean;
  expectedGeneration?: number;
  active: ModeldActiveModel;
};

export type ObservedModel = {
  name?: string;
  model: string;
  modifiedAt?: string;
  size?: number;
  digest?: string;
  contextLength: number;
  canChat: boolean;
  canEmbed: boolean;
  canPrompt: boolean;
  canStream: boolean;
};

export type LayoutDirection = 'horizontal' | 'vertical';

export type StateResponse = {
  response: string;
  state: CapturedStateUnit[];
  inputTokenCount: number;
  outputTokenCount: number;
  error?: string;
};

export type ChatContextArtifact = {
  kind: string;
  /** JSON value serialized per artifact */
  payload?: unknown;
};

// TaskEvent mirrors taskengine.TaskEvent on the Go side. The canonical client
// type lives in @contenox/ui (shared with ExecutionTimeline and TaskEventFeed);
// beam re-exports it so there is a single definition.
export type { TaskEvent, TaskEventKind } from '@contenox/ui';

export type SetupIssue = {
  code: string;
  severity: string;
  category?: string;
  message: string;
  fixPath?: string;
  cliCommand?: string;
};

export type SetupBackendCheck = {
  id: string;
  name: string;
  type: string;
  baseUrl: string;
  status: string;
  reachable: boolean;
  defaultProvider: boolean;
  modelCount: number;
  chatModelCount: number;
  chatModels?: string[];
  error?: string;
  hint?: string;
};

export type ConfigResolvedFrom = {
  defaultChain?: 'workspace' | 'global';
  hitlPolicyName?: 'workspace' | 'global';
};

export type SetupStatus = {
  defaultModel: string;
  defaultProvider: string;
  defaultChain: string;
  hitlPolicyName: string;
  backendCount: number;
  reachableBackendCount: number;
  issues: SetupIssue[];
  backendChecks: SetupBackendCheck[];
  resolvedFrom?: ConfigResolvedFrom;
};

export type CLIConfigUpdateResponse = {
  defaultModel: string;
  defaultProvider: string;
  defaultAltModel?: string;
  defaultAltProvider?: string;
  defaultAutocompleteModel?: string;
  defaultAutocompleteProvider?: string;
  defaultMaxTokens?: string;
  defaultThink?: string;
  defaultChain: string;
  hitlPolicyName: string;
  telemetryEnabled?: string;
  updateCheck?: string;
  resolvedFrom?: ConfigResolvedFrom;
};

/** Full CLI config snapshot — GET /api/cli-config, same shape PUT returns. */
export type CLIConfig = CLIConfigUpdateResponse;

export type CLIConfigUpdateRequest = {
  'default-model'?: string;
  'default-provider'?: string;
  'default-alt-model'?: string;
  'default-alt-provider'?: string;
  'default-autocomplete-model'?: string;
  'default-autocomplete-provider'?: string;
  'default-max-tokens'?: string;
  'default-think'?: string;
  'default-chain'?: string;
  'hitl-policy-name'?: string;
  'telemetry-enabled'?: string;
  'update-check'?: string;
};

export type HITLCondition = {
  key: string;
  op: 'eq' | 'glob';
  value: string;
};

export type HITLRule = {
  hook: string;
  tool: string;
  when?: HITLCondition[];
  action: 'allow' | 'approve' | 'deny';
  timeout_s?: number;
  on_timeout?: 'deny' | 'approve';
};

export type HITLPolicy = {
  default_action?: 'allow' | 'approve' | 'deny';
  rules: HITLRule[];
};

export type StatusResponse = {
  configured: boolean;
  provider: string;
  backendId?: string;
  backendName?: string;
  baseUrl?: string;
  secretSource?: string;
  secretConfigured?: boolean;
  secretPresent?: boolean;
  apiKeyEnv?: string;
  recommendedApiKeyEnv?: string;
  defaultProvider?: string;
  defaultModel?: string;
  updatedAt?: string;
};

export type CloudProviderType =
  | 'ollama'
  | 'openai'
  | 'openrouter'
  | 'anthropic'
  | 'gemini'
  | 'mistral'
  | 'bedrock'
  | 'vertex-google';

export type ConfigureProviderInput = {
  apiKey?: string;
  apiKeyEnv?: string;
  baseUrl?: string;
  defaultModel?: string;
  upsert: boolean;
  setDefault?: boolean;
};

export type SupportedProvider = {
  provider: string;
  defaultBaseUrl?: string;
  requiresBaseUrl: boolean;
  requiresSecretConfig: boolean;
  recommendedApiKeyEnv?: string;
};

export type ModelJob = {
  url: string;
  model: string;
};

export type Job = {
  id: string;
  taskType: string;
  modelJob: ModelJob | undefined;
  scheduledFor: number;
  validUntil: number;
  createdAt: Date;
};

// CapturedStateUnit mirrors taskengine.CapturedStateUnit on the Go side.
// The canonical client type lives in @contenox/ui (shared with
// ExecutionTimeline and StateVisualizer); beam re-exports it.
export type { CapturedStateUnit } from '@contenox/ui';

export type ErrorState = {
  error: string | null;
};

export type QueueItem = {
  url: string;
  model: string;
  status: QueueProgressStatus;
};

export type QueueProgressStatus = {
  total: number;
  completed: number;
  status: string;
};

export type Model = {
  id: string;
  model: string;
  contextLength: number;
  canChat: boolean;
  canEmbed: boolean;
  canPrompt: boolean;
  canStream: boolean;
  createdAt?: string;
  updatedAt?: string;
};

export type Pool = {
  id: string;
  name: string;
  purposeType: string;
  createdAt?: string;
  updatedAt?: string;
};

export type AuthResponse = {
  user: User;
};

export type LocalHook = {
  name: string;
  description: string;
  type: string;
  /** Origin of this hook in the merged discovery list (from API). */
  source?: 'builtin' | 'mcp' | 'remote';
  tools: Tool[];
  /** Present when the server could not load tools (e.g. unreachable MCP). */
  unavailableReason?: string;
};

/**
 * Persisted declared-agent config; matches runtimetypes.Agent JSON. Read-only
 * over REST for now (registration stays with `contenox agent`). configJson is
 * the kind-specific config as a raw JSON value (e.g. an ExternalACPConfig for
 * kind "external_acp"), so it is intentionally untyped here.
 */
export type Agent = {
  id: string;
  name: string;
  kind: string;
  enabled: boolean;
  configJson: unknown;
  harnessId?: string;
  workspaceId?: string;
  source?: string;
  registryId?: string;
  registryVersion?: string;
  createdAt: string;
  updatedAt: string;
};

/**
 * Lifecycle state of a live agent instance; mirrors agentinstance's
 * `starting|running|stopped|error|warning` (runtime/agentinstance/instance.go).
 */
export type FleetInstanceState = 'starting' | 'running' | 'stopped' | 'error' | 'warning';

/**
 * A single running agent instance. Mirrors agentinstance.InstanceStatus JSON
 * (GET /api/fleet/{instanceID}, and the elements of a FleetEntry's instances).
 */
export type InstanceStatus = {
  id: string;
  agentId: string;
  agentName: string;
  kind: string;
  state: FleetInstanceState;
  sessions: number;
  viewers: number;
  startedAt: string;
  /** Ids of every session with at least one attached viewer, sorted. */
  sessionIds: string[];
};

/**
 * One declared agent annotated with its running instances — the config+runtime
 * join returned by GET /api/fleet. Mirrors agentinstance.FleetEntry JSON.
 * `instances` is null when the declared agent is idle (no live instances).
 */
export type FleetEntry = {
  agentId: string;
  agentName: string;
  kind: string;
  instances: InstanceStatus[] | null;
};

/**
 * Mission mode's lifecycle state; mirrors missionservice.Status
 * (runtime/missionservice/missionservice.go). A mission stays "open" for its
 * entire run — LastHeartbeat/LastError on {@link Mission}, not Status, carry
 * unattended liveness.
 */
export type MissionStatus = 'open' | 'landed' | 'derailed' | 'abandoned';

/**
 * A plan entry's lifecycle state; mirrors missionservice.PlanEntryStatus, whose
 * values are contracted byte-for-byte to libacp.PlanEntryStatus
 * (pending|in_progress|completed).
 */
export type MissionPlanEntryStatus = 'pending' | 'in_progress' | 'completed';

/** A plan entry's priority; mirrors missionservice.PlanEntryPriority (high|medium|low). */
export type MissionPlanEntryPriority = 'high' | 'medium' | 'low';

/**
 * One line of a mission's living plan; mirrors missionservice.PlanEntry. `id` is
 * server-assigned and stable across revisions (SetPlan diffs on it), so it is the
 * right React key.
 */
export type MissionPlanEntry = {
  id: string;
  content: string;
  status: MissionPlanEntryStatus;
  priority: MissionPlanEntryPriority;
};

/**
 * A mission's living plan; mirrors missionservice.Plan. `revision` counts
 * successful SetPlan calls — 0 with an empty/absent `entries` is the zero Plan
 * ("never planned"), which the UI renders as NO plan panel rather than an empty
 * shell. `explanation` is the agent's own "why it changed" line for the latest
 * revision. The field is always present on the wire (missionservice marshals a
 * never-planned mission as the zero Plan), so callers key on `entries` length,
 * not on the field's presence.
 */
export type MissionPlan = {
  entries: MissionPlanEntry[] | null;
  revision: number;
  explanation?: string;
};

/**
 * One durable entry in a mission's bounded plan-revision ring; mirrors
 * missionservice.PlanRevisionSummary. The "+2/−1 — why" line for a single past
 * SetPlan: `added`/`removed` are the entry delta, the per-status counts are the
 * snapshot after that revision, `explanation` is the agent's own account of the
 * change (absent when it gave none), and `at` is the wall-clock the revision was
 * stored. Surfaced additively on the mission GET as `planRevisions`
 * (oldest-first, newest is the final element) — absent on legacy or
 * never-planned missions, which is a real state, never an error.
 */
export type PlanRevisionSummary = {
  revision: number;
  explanation?: string;
  added: number;
  removed: number;
  pending: number;
  inProgress: number;
  completed: number;
  at: string;
};

/**
 * The headless interaction model's durable record (see
 * docs/development/blueprints/acp/fleet-consolidation.md, "Mission mode"): a
 * one-line intent fired at a declared agent, bound to exactly one session and
 * one instance, and bounded by an envelope — a named HITL policy — that
 * governs what the unit may do while unattended. Mirrors missionservice.Mission
 * JSON (GET/POST /api/missions).
 *
 * LastHeartbeat and LastError are liveness facts, not status: they are how a
 * caller tells a unit that is quietly working apart from one that has gone
 * dark or is erroring, without attaching to its session. Both are absent
 * until the unit's first Heartbeat call — nothing calls it yet, it arrives
 * with the mission-tools slice — so a mission that has never reported and one
 * that reported long ago must render differently, not collapse into the same
 * "no time shown" blank.
 */
export type Mission = {
  id: string;
  intent: string;
  agentName: string;
  hitlPolicyName: string;
  sessionId?: string;
  instanceId?: string;
  parentSessionId?: string;
  status: MissionStatus;
  /**
   * The one line Finish attached when the mission reached a terminal status
   * (missionservice.Mission.StatusReason). Absent while open.
   */
  statusReason?: string;
  /**
   * The mission's living plan (missionservice.Mission.Plan). Always present on
   * the wire; a never-planned mission carries the zero Plan (revision 0, no
   * entries) — see {@link MissionPlan}. Optional here so an older serve that
   * omits the field entirely still decodes.
   */
  plan?: MissionPlan;
  /**
   * The mission's last-N plan-revision summaries, oldest-first (newest is the
   * final element), bounded server-side. Surfaced additively as `planRevisions`
   * (missionservice.Mission.PlanRevisions); absent on legacy or never-planned
   * missions — the revision feed simply renders nothing, never an empty shell.
   */
  planRevisions?: PlanRevisionSummary[];
  lastHeartbeat?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
};

/**
 * A changed file in a mission's aggregated diff (mirrors
 * missionchanges.ChangedFile). `score` is the Degree-of-Interest weight the
 * list is ordered by (DESC) — where the unit's attention concentrated, per the
 * attention layer; it is a ranking hint, never a gate. `path` is absolute (the
 * value the diff endpoint's `path` query parameter takes verbatim).
 */
export type MissionChangeStatus = 'added' | 'modified' | 'deleted';

export type MissionChangedFile = {
  path: string;
  status: MissionChangeStatus;
  score: number;
};

/**
 * The scope statistics of a mission's changes (mirrors missionchanges.ScopeStats)
 * — the attention layer's scope signal. `anomaly` flags that the unit's touched
 * paths diverged from its expected scope (an early-warning ADVICE flag, not a
 * verdict); `outsidePaths` names the offending paths when present.
 */
export type MissionChangeScope = {
  files: number;
  dirs: number;
  anomaly: boolean;
  outsidePaths?: string[];
};

/**
 * `GET /api/missions/{id}/changes` body (mirrors missionchanges.Changes).
 * `files` is non-nil and ordered by `score` DESC; `incomplete` is true when the
 * list was capped (100) so more changes exist than are shown.
 */
export type MissionChangesResponse = {
  files: MissionChangedFile[];
  incomplete: boolean;
  scope: MissionChangeScope;
};

/**
 * `GET /api/missions/{id}/changes/diff?path=<absolute>` body (mirrors
 * missionchanges.Diff). `truncated` is true when either side was capped at
 * 128 KiB. An added file has an empty `original`; a deleted file an empty
 * `modified`.
 */
export type MissionFileDiff = {
  original: string;
  modified: string;
  truncated?: boolean;
};

/**
 * One streamed match from `GET /api/workspace/search` (mirrors
 * localfileapi.searchMatch). `path` is root-relative (matching the /files
 * endpoints), `line` is 1-based, and `column`/`length` are 0-based BYTE offsets
 * of the matched substring within `preview` — so the client highlights it
 * without re-searching (see byteSlice in lib/workspaceSearch).
 */
export type WorkspaceSearchMatch = {
  path: string;
  line: number;
  column: number;
  length: number;
  preview: string;
};

/**
 * The terminal `done` frame of a workspace search (mirrors
 * localfileapi.searchDone). Always closes the stream; `truncated` is true when
 * the hard result cap stopped the scan early, so the UI can offer "refine your
 * search".
 */
export type WorkspaceSearchDone = {
  done: boolean;
  matches: number;
  truncated: boolean;
};

/**
 * The closed set a mission report's Kind is drawn from; mirrors
 * missionservice.ReportKind. Deliberately closed rather than free text — the
 * set IS the hint to an unattended agent about what is worth reporting at
 * all (see missionservice's package doc, "Reports").
 */
export type MissionReportKind = 'progress' | 'finding' | 'blocker' | 'result';

/**
 * A single dispatch from a unit on a mission, filed under a Kind that hints
 * how much it matters. Mirrors missionservice.Report JSON (GET/POST
 * /api/missions/{id}/reports). Refs is by reference only (paths, URLs) — a
 * report never carries artifact content.
 */
export type MissionReport = {
  id: string;
  missionId: string;
  kind: MissionReportKind;
  summary: string;
  detail?: string;
  refs?: string[];
  createdAt: string;
};

/**
 * POST /api/fleet/dispatch body — fire a mission: bring a declared agent up,
 * open a session, and run `intent` as its first turn, detached. Mirrors
 * fleetservice.DispatchRequest. Every dispatch is a mission (see
 * {@link Mission}), so intent and hitlPolicyName are both required — there is
 * no separate prompt field, and no dispatch with no envelope.
 */
export type DispatchRequest = {
  agentName: string;
  intent: string;
  hitlPolicyName: string;
  cwd?: string;
};

/**
 * The 202 body from POST /api/fleet/dispatch: the ids the dispatch created.
 * Mirrors fleetservice.DispatchResult. missionId is always present — every
 * dispatch is a mission.
 */
export type DispatchResult = {
  instanceId: string;
  sessionId: string;
  missionId: string;
};

/**
 * Lifecycle state of one durable human-in-the-loop approval ask; mirrors
 * runtimetypes.HITLApprovalState. `pending` is the only non-terminal state —
 * a row ends exactly once at approved/denied (a human's answer) or expired
 * (the sweeper). The inbox only ever lists `pending`.
 */
export type HITLApprovalState = 'pending' | 'approved' | 'denied' | 'expired';

/**
 * One durable approval ask (the attention inbox's rows). Mirrors
 * runtimetypes.HITLApproval JSON (GET /api/approvals). It is the ask an
 * unattended unit raised that a human must answer without attaching to the
 * session that raised it (see docs/development/blueprints/acp/
 * fleet-consolidation.md, slice C2).
 *
 * The attribution set — agentName, instanceId, sessionId, missionId — names WHO
 * is asking, which is what makes an inbox of many answerable: identical-looking
 * rows can be told apart, and an operator can name the policy (policyName +
 * matchedRule) that gated the action. All are best-effort: an ask raised by a
 * native chain turn with no fleet unit behind it carries none of them, so empty
 * means "not applicable", never "unknown but exists".
 */
export type HITLApproval = {
  id: string;
  toolsName: string;
  toolName: string;
  argsSummary?: string;
  diff?: string;
  policyName?: string;
  matchedRule?: number;
  onTimeout?: string;
  state: HITLApprovalState;
  /** Opaque JSON, present only once the ask is resolved. */
  resolution?: unknown;
  instanceId?: string;
  sessionId?: string;
  agentName?: string;
  missionId?: string;
  createdAt: string;
  expiresAt: string;
  resolvedAt?: string;
};

/**
 * Why a report landed in the operator inbox rather than reaching a live
 * supervising session; mirrors operatorinbox.Reason. `operator_fired`: the
 * mission carried no parent session, so an operator fired it directly and its
 * reports were always inbox-bound. `parent_gone`: the mission named a parent
 * session, but no live instance owned it when the report arrived — the
 * supervisor had ended.
 */
export type OperatorInboxReason = 'operator_fired' | 'parent_gone';

/**
 * One mission report that reached no live supervisor, plus the mission
 * attribution needed to render and act on it without a second read. Mirrors
 * operatorinbox.Item JSON (GET /api/operator-inbox) — the read sibling of
 * {@link HITLApproval}'s inbox for notices that need eyes rather than a
 * decision. `agentName`/`intent` are best-effort (present when the mission
 * carried them at write time); `parentSessionId` is present only for
 * `parent_gone` — the (now-unreachable) supervisor the report was meant for.
 */
export type OperatorInboxItem = {
  id: string;
  missionId: string;
  agentName?: string;
  intent?: string;
  parentSessionId?: string;
  reason: OperatorInboxReason;
  report: MissionReport;
  createdAt: string;
};

/** Persisted MCP server config; matches runtimetypes.MCPServer JSON. */
export type MCPServer = {
  id: string;
  name: string;
  transport: 'stdio' | 'sse' | 'http' | string;
  command?: string;
  args?: string[];
  url?: string;
  authType?: string;
  authToken?: string;
  authEnvKey?: string;
  connectTimeoutSeconds: number;
  headers?: Record<string, string>;
  injectParams?: Record<string, string>;
  createdAt?: string;
  updatedAt?: string;
};

export type Tool = {
  type: string;
  function: {
    name: string;
    description: string;
    parameters: Record<string, unknown>;
  };
};

export type User = {
  id: string;
  friendlyName: string;
  email: string;
  subject: string;
  password: string;
  createdAt?: string;
  updatedAt?: string;
};

export type DownloadStatus = {
  status: string;
  digest?: string;
  total?: number;
  completed?: number;
  model: string;
  baseUrl: string;
};

export type AccessEntry = {
  id: string;
  identity: string;
  resource: string;
  resourceType: string;
  permission: string;
  createdAt?: string;
  updatedAt?: string;
  identityDetails?: IdentityDetails;
  fileDetails?: filesDetails;
};

export type filesDetails = {
  id: string;
  path: string;
  type: string;
};

export type IdentityDetails = {
  id: string;
  friendlyName: string;
  email: string;
  subject: string;
};

export type UpdateUserRequest = {
  email?: string;
  subject?: string;
  friendlyName?: string;
  password?: string;
};

export type UpdateAccessEntryRequest = {
  identity?: string;
  resource?: string;
  permission?: string;
};

// Local Beam runs as a same-origin UI over the local HTTP API. There is no
// account service in OSS local mode; AuthProvider supplies a local identity.
export type AuthenticatedUser = {
  id: string;
  subject: string;
  email: string;
  friendlyName: string;
  username: string;
  expiresAt?: string;
};

/** Result of GET /ui/auth-status: whether remote-access login is required, and whether this browser is already authenticated via its session cookie. */
export type AuthStatus = {
  required: boolean;
  authenticated: boolean;
};

export type PendingJob = {
  id: string;
  taskType: string;
  operation: string;
  subject: string;
  entityId: string;
  scheduledFor: string;
  validUntil: string;
  retryCount: number;
  createdAt: string;
};

export type InProgressJob = PendingJob & {
  leaser: string;
  leaseExpiration: string;
};
export type Exec = {
  prompt: string;
};

export type ExecResp = {
  id: string;
  response: string;
};

export type TaskExecutionRequest = {
  input: unknown;
  inputType: string;
  chain: ChainDefinition;
  templateVars?: Record<string, string>;
};

export type TaskExecutionResponse = {
  output: unknown;
  outputType: string;
  state: CapturedStateUnit[];
};

export interface HookCall {
  name: string;
  tool_name?: string;
  args?: Record<string, string>;
}

export type ComposeStrategy = 'override' | 'merge_chat_histories' | 'append_string_to_chat_history';

export type OperatorTerm =
  | 'equals'
  | 'contains'
  | 'starts_with'
  | 'ends_with'
  | 'gt'
  | 'lt'
  | 'in_range'
  | 'default';

export interface TransitionBranch {
  operator?: OperatorTerm;
  when: string;
  goto: string;
  compose?: BranchCompose;
}

export interface TaskTransition {
  on_failure?: string;
  branches: TransitionBranch[];
}

export type FormTransition = TaskTransition;

export interface ChainTask {
  id: string;
  description: string;
  handler: TaskHandler;
  system_instruction?: string;
  execute_config?: ExecuteConfig;
  tools?: HookCall;
  print?: string;
  prompt_template: string;
  output_template?: string;
  input_var?: string;
  transition: TaskTransition;
  timeout?: string;
  retry_on_failure?: number;
}

// FormTask keeps partial but requires keys we edit frequently
export type FormTask = Partial<ChainTask> & {
  id: string;
  handler: TaskHandler;
  prompt_template: string;
  transition: TaskTransition;
};

// RetryPolicy mirrors taskengine/llmretry.RetryPolicy. Durations are expressed
// as Go duration strings ("500ms", "30s", "2m"); the backend also accepts
// nanoseconds as numbers but this editor writes strings for readability.
export interface RetryPolicy {
  max_attempts?: number;
  initial_backoff?: string;
  max_backoff?: string;
  jitter?: number;
  rate_limit_min_wait?: string;
  fallback_model_id?: string;
  fallback_after?: number;
}

// CompactPolicy mirrors taskengine/compact.Policy. Controls mid-run conversation
// compaction on the executor's chat_completion task.
export interface CompactPolicy {
  trigger_fraction?: number;
  keep_recent?: number;
  model?: string;
  provider?: string;
  max_failures?: number;
  min_replaced_messages?: number;
}

export interface ExecuteConfig {
  model?: string;
  models?: string[];
  provider?: string;
  providers?: string[];
  temperature?: number;
  tools?: string[];
  hooks?: string[];
  hide_tools?: string[];
  pass_clients_tools?: boolean;
  // tools_policies maps tools-provider name -> policy key/value pairs.
  tools_policies?: Record<string, Record<string, string>>;
  // Legacy backup UI field. Read for compatibility, but new edits write tools_policies.
  hook_policies?: Record<string, Record<string, string>>;
  // think: "", "low", "medium", "high", or "true" / "false" — provider-gated.
  think?: string;
  // shift: allow the context window to slide on overflow instead of erroring.
  shift?: boolean;
  // retry_policy: classified retry/backoff + optional fallback model.
  // See taskengine/llmretry.RetryPolicy.
  retry_policy?: RetryPolicy;
  // compact_policy: mid-run conversation compaction.
  // See taskengine/compact.Policy.
  compact_policy?: CompactPolicy;
}

export interface ChainDefinition {
  id: string;
  description: string;
  tasks: ChainTask[];
  token_limit?: number;
  debug?: boolean;
}

export type ActivityLog = {
  id: string;
  operation: string;
  subject: string;
  start: string;
  end?: string;
  error?: string;
  entityID?: string;
  entityData?: undefined;
  durationMS?: number;
  metadata?: Record<string, string>;
  requestID?: string;
};

export type ActivityLogsResponse = ActivityLog[];

export type TrackedRequest = {
  id: string;
};

export type ActivityOperation = {
  operation: string;
  subject: string;
};

export type TrackedEvent = {
  id: string;
  operation: string;
  subject: string;
  start: string;
  end?: string;
  error?: string;
  entityID?: string;
  entityData?: unknown;
  durationMS?: number;
  metadata?: Record<string, string>;
  requestID?: string;
};

export type Operation = {
  operation: string;
  subject: string;
};

export type TrackedRequestsResponse = TrackedRequest[];
export type ActivityOperationsResponse = ActivityOperation[];

export type Alert = {
  id: string;
  requestID: string;
  metadata: unknown;
  message: string;
  timestamp: string;
};

export type ActivityAlertsResponse = Alert[];

export interface GitHubRepo {
  id: string;
  userID: string;
  botUserName: string;
  owner: string;
  repoName: string;
  accessToken: string;
  createdAt: string;
  updatedAt: string;
}

export interface PullRequest {
  id: number;
  number: number;
  title: string;
  state: string;
  url: string;
  createdAt: string;
  updatedAt: string;
  authorLogin: string;
}

export type TelegramFrontend = {
  id: string;
  userID: string;
  chatChain: string;
  description: string;
  botToken: string;
  syncInterval: number;
  status: string;
  lastOffset: number;
  lastHeartbeat?: string;
  lastError: string;
  createdAt?: string;
  updatedAt?: string;
};

export type Bot = {
  id: string;
  name: string;
  botType: string;
  jobType: string;
  taskChainID: string;
  createdAt: string;
  updatedAt: string;
};

export type InternalEvent = {
  id: string;
  nid: number;
  created_at: string;
  event_type: string;
  event_source: string;
  aggregate_id: string;
  aggregate_type: string;
  version: number;
  data: Record<string, unknown>;
  metadata: Record<string, unknown>;
};

export type RawEvent = {
  id: string;
  nid: number;
  received_at: string;
  path: string;
  headers: Record<string, string>;
  payload: Record<string, unknown>;
};

export type MappingConfig = {
  path: string;
  eventType: string;
  eventSource: string;
  aggregateType: string;
  aggregateIDField: string;
  aggregateTypeField: string;
  eventTypeField: string;
  eventSourceField: string;
  eventIDField: string;
  version: number;
  metadataMapping: Record<string, string>;
};

export type EventStreamMessage = {
  id: string;
  event_type: string;
  aggregate_type: string;
  aggregate_id: string;
  version: number;
  data: Record<string, unknown>;
  created_at: string;
};

// Executor and Task Types
export type TaskRequest = {
  prompt: string;
  modelName: string;
  modelProvider: string;
};

export type TaskResponse = {
  id: string;
  response: string;
};

export type DataType = 'string' | 'json';

export type ExecutionHistory = {
  taskID: string;
  taskType: string;
  inputType: string;
  outputType: string;
  transition: string;
  duration: number;
  error?: string;
}[];

// Pagination Types
export type PaginationParams = {
  limit?: number;
  cursor?: string;
};

export type PaginatedResponse<T> = {
  data: T[];
  nextCursor?: string;
  hasMore: boolean;
};

// Validation Error Types
export type ValidationError = {
  field: string;
  message: string;
  code: string;
};

export type ApiError = {
  message: string;
  code: string;
  details?: ValidationError[];
};

// Webhook and Integration Types
export type WebhookPayload = {
  headers: Record<string, string>;
  body: Record<string, unknown>;
  query: Record<string, string>;
  method: string;
  path: string;
};

export interface BranchCompose {
  with_var?: string;
  strategy?: string;
}

export type IntegrationConfig = {
  id: string;
  name: string;
  type: 'webhook' | 'api' | 'event';
  config: Record<string, unknown>;
  enabled: boolean;
  createdAt?: string;
  updatedAt?: string;
};

// System and Health Types
export type SystemHealth = {
  status: 'healthy' | 'degraded' | 'unhealthy';
  components: {
    database: HealthStatus;
    cache: HealthStatus;
    executor: HealthStatus;
    eventBus: HealthStatus;
  };
  timestamp: string;
};

export type HealthStatus = {
  status: 'up' | 'down' | 'degraded';
  latency?: number;
  error?: string;
  lastChecked: string;
};

export interface BackendApiError {
  error: {
    message: string;
    type?: string;
    code?: string;
    param?: string;
  };
}

export type RemoteHook = {
  id: string;
  name: string;
  endpointUrl: string;
  timeoutMs: number;
  headers?: Record<string, string>;
  properties: InjectionArg;
  createdAt?: string;
  updatedAt?: string;
};

export type InjectionArg = {
  name: string;
  value: unknown;
  in: 'path' | 'query' | 'body';
};

// For listing with pagination
export type RemoteHookListResponse = {
  hooks: RemoteHook[];
  nextCursor?: string; // RFC3339Nano timestamp
};

export interface DragResult {
  draggableId: string;
  source: {
    index: number;
    droppableId: string;
  };
  destination: {
    index: number;
    droppableId: string;
  } | null;
}

export interface DroppableProvided {
  innerRef: (element: HTMLElement | null) => void;
  droppableProps: {
    [key: string]: unknown;
  };
  placeholder?: React.ReactNode;
}

export interface DraggableProvided {
  innerRef: (element: HTMLElement | null) => void;
  draggableProps: {
    [key: string]: unknown;
    style?: React.CSSProperties;
  };
  dragHandleProps: {
    [key: string]: unknown;
  } | null;
}

// Matches the engine's closed handler set exactly (runtime/taskengine/tasktype.go:15-24).
// An unknown handler is hard-rejected at chain validation, so this union must never
// declare a value the engine does not also accept.
export type TaskHandler =
  | 'raise_error'
  | 'route'
  | 'chat_completion'
  | 'execute_tool_calls'
  | 'noop'
  | 'tools';

export const HandleRaiseError: TaskHandler = 'raise_error';
export const HandleRoute: TaskHandler = 'route';
export const HandleChatCompletion: TaskHandler = 'chat_completion';
export const HandleExecuteToolCalls: TaskHandler = 'execute_tool_calls';
export const HandleNoop: TaskHandler = 'noop';
export const HandleTools: TaskHandler = 'tools';

/**
 * One allowlisted workspace root reported by `GET /workspace/roots`. Mirrors
 * localfileapi.workspaceRoot: `default` marks the single root that the empty
 * value and "/" resolve to (see vfs.Factory.Default). The list is the legible
 * boundary a client offers as a folder picker instead of discovering it by
 * probing paths and reading the 422 the per-request `root` check returns.
 *
 * `name` is the root's EXPLICIT project name (its marker's name) — empty/absent
 * for a structural or unnamed root, which is how a client tells a real
 * registered project apart (display fallback to the path basename is the
 * client's job, see `projectName`). `managed` distinguishes a runtime grant
 * (operator-forgettable) from a launch root. Both are optional so a serve
 * predating them — and older fixtures/tests — still typecheck against the
 * earlier `{ path, default }` shape.
 */
export type WorkspaceRoot = {
  path: string;
  default: boolean;
  name?: string;
  managed?: boolean;
};

/**
 * `GET /workspace/roots` body. Mirrors localfileapi.workspaceRootsResponse.
 * The whole route is nil-gated server-side: when serve has no workspace-root
 * allowlist configured it 404s, which the client reads as "feature absent"
 * (hide the picker affordances), never as an error to surface.
 */
export type WorkspaceRootsResponse = {
  roots: WorkspaceRoot[];
};

/**
 * `POST /api/terminal/sessions` body. Mirrors terminalapi.createSessionRequest.
 * `cwd` is validated server-side against the workspace-root allowlist (empty →
 * default root); `cols`/`rows` seed the PTY size before the first fit/resize
 * frame. `shell` is optional (server default when blank).
 */
export type TerminalSessionRequest = {
  cwd?: string;
  cols: number;
  rows: number;
  shell?: string;
};

/**
 * The 201 body from `POST /api/terminal/sessions`. Mirrors
 * terminalapi.createSessionResponse. `wsPath` is the server-authoritative,
 * already-`/api`-prefixed path of the binary-frame WebSocket to attach to
 * (`/api/terminal/sessions/{id}/ws`) — the client turns it into a `ws(s)://`
 * URL rather than reconstructing the path itself.
 */
export type TerminalSession = {
  id: string;
  wsPath: string;
};
