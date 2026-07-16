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
  hook?: HookCall;
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

export type TaskHandler =
  | 'route'
  | 'chat_completion'
  | 'execute_tool_calls'
  | 'tools'
  | 'hook'
  | 'prompt_to_string'
  | 'prompt_to_int'
  | 'raise_error'
  | 'noop';

export const HandleRoute: TaskHandler = 'route';
export const HandlePromptToString: TaskHandler = 'prompt_to_string';
export const HandleParseNumber: TaskHandler = 'prompt_to_int';
export const HandleRaiseError: TaskHandler = 'raise_error';
export const HandleChatCompletion: TaskHandler = 'chat_completion';
export const HandleExecuteToolCalls: TaskHandler = 'execute_tool_calls';
export const HandleNoop: TaskHandler = 'noop';
export const HandleTools: TaskHandler = 'tools';
export const HandleHook: TaskHandler = 'hook';
