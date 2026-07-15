import { apiFetch } from './fetch';
import {
  AuthenticatedUser,
  Backend,
  BackendRuntimeState,
  ChainDefinition,
  CLIConfigUpdateRequest,
  CLIConfigUpdateResponse,
  CloudProviderType,
  ConfigureProviderInput,
  HITLPolicy,
  LocalHook,
  MCPServer,
  ModeldCapacityResponse,
  ModeldLoadResponse,
  ModelDescriptor,
  ModeldLocalModel,
  ModeldStatusResponse,
  ModeldUnloadResponse,
  ModelRegistryEntry,
  PushModelResult,
  RemoteHook,
  SetupStatus,
  StatusResponse,
  SupportedProvider,
  TaskExecutionRequest,
  TaskExecutionResponse,
} from './types';

type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';

interface ApiOptions {
  method?: HttpMethod;
  headers?: Record<string, string>;
  body?: string;
  credentials?: RequestCredentials;
}

const options = (method: HttpMethod, data?: unknown): ApiOptions => {
  const options: ApiOptions = {
    method,
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
  };

  if (data !== undefined) {
    options.body = JSON.stringify(data);
  }

  return options;
};

const localAuthenticatedUser: AuthenticatedUser = {
  id: 'local-user',
  subject: 'local-user',
  email: 'local@localhost',
  friendlyName: 'Local user',
  username: 'local-user',
};

const normalizeSetupStatus = (raw: SetupStatus): SetupStatus => ({
  ...raw,
  defaultModel: raw.defaultModel ?? '',
  defaultProvider: raw.defaultProvider ?? '',
  defaultChain: raw.defaultChain ?? '',
  hitlPolicyName: raw.hitlPolicyName ?? '',
  backendCount: raw.backendCount ?? 0,
  reachableBackendCount: raw.reachableBackendCount ?? 0,
  issues: Array.isArray(raw.issues) ? raw.issues : [],
  backendChecks: Array.isArray(raw.backendChecks) ? raw.backendChecks : [],
});

export const api = {
  // Remote Hooks
  getRemoteHooks: (params?: { limit?: number; cursor?: string }) => {
    const search = new URLSearchParams();
    if (params?.limit !== undefined) search.set('limit', params.limit.toString());
    if (params?.cursor) search.set('cursor', params.cursor);
    const qs = search.toString() ? `?${search.toString()}` : '';
    return apiFetch<RemoteHook[]>(`/api/tools/remote${qs}`);
  },

  getRemoteHook: (id: string) => apiFetch<RemoteHook>(`/api/tools/remote/${id}`),

  getRemoteHookByName: (name: string) => apiFetch<RemoteHook>(`/api/tools/remote/by-name/${name}`),

  createRemoteHook: (data: Partial<RemoteHook>) =>
    apiFetch<RemoteHook>('/api/tools/remote', options('POST', data)),

  updateRemoteHook: (id: string, data: Partial<RemoteHook>) =>
    apiFetch<RemoteHook>(`/api/tools/remote/${id}`, options('PUT', data)),

  deleteRemoteHook: (id: string) => apiFetch<string>(`/api/tools/remote/${id}`, options('DELETE')),
  getLocalHooks: () => apiFetch<LocalHook[]>('/api/tools/local'),

  getRemoteHookSchemas: () => apiFetch<Record<string, unknown>>('/api/tools/schemas'),

  // MCP servers (persisted configs; same DB as `contenox mcp`)
  getMcpServers: (params?: { limit?: number; cursor?: string }) => {
    const search = new URLSearchParams();
    if (params?.limit !== undefined) search.set('limit', params.limit.toString());
    if (params?.cursor) search.set('cursor', params.cursor);
    const qs = search.toString() ? `?${search.toString()}` : '';
    return apiFetch<MCPServer[]>(`/api/mcp-servers${qs}`);
  },
  getMcpServer: (id: string) => apiFetch<MCPServer>(`/api/mcp-servers/${id}`),
  getMcpServerByName: (name: string) =>
    apiFetch<MCPServer>(`/api/mcp-servers/by-name/${encodeURIComponent(name)}`),
  createMcpServer: (data: Partial<MCPServer>) =>
    apiFetch<MCPServer>('/api/mcp-servers', options('POST', data)),
  updateMcpServer: (id: string, data: Partial<MCPServer>) =>
    apiFetch<MCPServer>(`/api/mcp-servers/${id}`, options('PUT', data)),
  deleteMcpServer: (id: string) => apiFetch<string>(`/api/mcp-servers/${id}`, options('DELETE')),
  /** Starts OAuth 2.1 PKCE for an oauth-type MCP server; open authorizationUrl in the browser. */
  startMcpOAuth: (id: string, body: { redirectBase: string }) =>
    apiFetch<{ authorizationUrl: string }>(
      `/api/mcp-servers/${id}/oauth/start`,
      options('POST', body),
    ),
  // Backends
  getBackends: () => apiFetch<Backend[]>('/api/backends'),
  getBackend: (id: string) => apiFetch<Backend>(`/api/backends/${id}`),
  createBackend: (data: Partial<Backend>) =>
    apiFetch<Backend>('/api/backends', options('POST', data)),
  updateBackend: (id: string, data: Partial<Backend>) =>
    apiFetch<Backend>(`/api/backends/${id}`, options('PUT', data)),
  deleteBackend: (id: string) => apiFetch<void>(`/api/backends/${id}`, options('DELETE')),
  /**
   * Streams a local GGUF file to a modeld backend's model store (local or
   * remote) — the HTTP twin of `contenox model push`. The file is sent as
   * the raw request body (not multipart/form-data or JSON) so it streams
   * straight through instead of being buffered in memory; timeoutMs is
   * disabled since a multi-gigabyte upload can run far longer than the
   * default API timeout.
   */
  pushModel: (backendId: string, name: string, file: File) =>
    apiFetch<PushModelResult>(
      `/api/backends/${backendId}/models/push?${new URLSearchParams({ name }).toString()}`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/octet-stream' },
        body: file,
        timeoutMs: null,
      },
    ),

  getSetupStatus: async (): Promise<SetupStatus> =>
    normalizeSetupStatus(await apiFetch<SetupStatus>('/api/setup-status')),
  refreshSetupStatus: async (): Promise<SetupStatus> =>
    normalizeSetupStatus(await apiFetch<SetupStatus>('/api/setup/refresh', options('POST'))),
  putCLIConfig: (body: CLIConfigUpdateRequest) =>
    apiFetch<CLIConfigUpdateResponse>('/api/cli-config', options('PUT', body)),

  /** HITL policy presets stored alongside task chains. */
  listPolicies: () => apiFetch<string[]>('/api/hitl-policies/list'),
  getPolicy: (name: string) =>
    apiFetch<HITLPolicy>(`/api/hitl-policies?name=${encodeURIComponent(name)}`),
  createPolicy: (name: string, policy: HITLPolicy) =>
    apiFetch<HITLPolicy>(
      `/api/hitl-policies?name=${encodeURIComponent(name)}`,
      options('POST', policy),
    ),
  updatePolicy: (name: string, policy: HITLPolicy) =>
    apiFetch<HITLPolicy>(
      `/api/hitl-policies?name=${encodeURIComponent(name)}`,
      options('PUT', policy),
    ),
  deletePolicy: (name: string) =>
    apiFetch<string>(`/api/hitl-policies?name=${encodeURIComponent(name)}`, options('DELETE')),

  /** Runtime sync snapshot per backend (OSS backend refresh loop; not a managed download queue). */
  getRuntimeBackendState: () => apiFetch<BackendRuntimeState[]>('/api/state'),

  /** Local modeld daemon state exposed by contenox serve, not a direct browser daemon client. */
  getModeldStatus: () => apiFetch<ModeldStatusResponse>('/api/modeld/status'),
  getModeldModels: () => apiFetch<ModeldLocalModel[]>('/api/modeld/models'),
  getModeldCapacity: (model: string) =>
    apiFetch<ModeldCapacityResponse>(`/api/modeld/capacity?model=${encodeURIComponent(model)}`),
  loadModeld: (model: string, expectedGeneration?: number) =>
    apiFetch<ModeldLoadResponse>(
      '/api/modeld/load',
      options('POST', {
        model,
        ...(expectedGeneration === undefined ? {} : { expectedGeneration }),
      }),
    ),
  unloadModeld: (expectedGeneration: number) =>
    apiFetch<ModeldUnloadResponse>('/api/modeld/unload', options('POST', { expectedGeneration })),

  taskEvents(requestId: string): EventSource {
    // Must be root-absolute: on routes like /chat/:id, a relative "api/..." resolves to
    // /chat/api/... and hits the SPA shell instead of the API mux.
    return new EventSource(`/api/task-events?requestId=${encodeURIComponent(requestId)}`);
  },

  configureProvider: (provider: CloudProviderType, data: ConfigureProviderInput) =>
    apiFetch<StatusResponse>(`/api/providers/${provider}/configure`, options('POST', data)),

  getProviderStatus: (provider: CloudProviderType) =>
    apiFetch<StatusResponse>(`/api/providers/${provider}/status`),
  getSupportedProviders: () => apiFetch<SupportedProvider[]>('/api/providers/supported'),

  // Auth endpoints
  login: async (_data: { email?: string; password?: string }): Promise<AuthenticatedUser> => {
    void _data;
    return localAuthenticatedUser;
  },
  getCurrentUser: async (): Promise<AuthenticatedUser> => localAuthenticatedUser,

  // First-run account
  getAuthSetupStatus: async (): Promise<{ initialized: boolean }> => ({ initialized: true }),
  initAccount: async (_data: {
    username: string;
    password: string;
  }): Promise<{ initialized: boolean }> => {
    void _data;
    return { initialized: true };
  },

  executeTaskChain: (
    data: TaskExecutionRequest,
    opts?: { signal?: AbortSignal; requestId?: string },
  ) => {
    const requestOptions = options('POST', data);
    if (opts?.requestId) {
      requestOptions.headers = {
        ...requestOptions.headers,
        'X-Request-ID': opts.requestId,
      };
    }
    return apiFetch<TaskExecutionResponse>(`/api/tasks`, {
      ...requestOptions,
      signal: opts?.signal,
    });
  },

  /** Task chains are managed by the runtime task-chain service. */
  listChains: () => apiFetch<string[]>('/api/taskchains/list'),
  getChain: (path: string) =>
    apiFetch<ChainDefinition>(`/api/taskchains?path=${encodeURIComponent(path)}`),
  createChain: (path: string, data: Partial<ChainDefinition>) =>
    apiFetch<ChainDefinition>(
      `/api/taskchains?path=${encodeURIComponent(path)}`,
      options('POST', data),
    ),
  updateChain: (path: string, data: Partial<ChainDefinition>) =>
    apiFetch<ChainDefinition>(
      `/api/taskchains?path=${encodeURIComponent(path)}`,
      options('PUT', data),
    ),
  deleteChain: (path: string) =>
    apiFetch<void>(`/api/taskchains?path=${encodeURIComponent(path)}`, options('DELETE')),

  // ── Model Registry ───────────────────────────────────────────────
  listModelRegistry: () => apiFetch<ModelDescriptor[]>('/api/model-registry'),
  createModelRegistryEntry: (data: Omit<ModelRegistryEntry, 'id' | 'createdAt' | 'updatedAt'>) =>
    apiFetch<ModelRegistryEntry>('/api/model-registry', options('POST', data)),
  deleteModelRegistryEntry: (id: string) =>
    apiFetch<void>(`/api/model-registry/${encodeURIComponent(id)}`, options('DELETE')),
  downloadModel: (name: string) =>
    apiFetch<string>('/api/model-registry/download', {
      ...options('POST', { name }),
      timeoutMs: null,
    }),
};
