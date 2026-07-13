export type TaskEventKind =
  | "chain_started"
  | "step_started"
  | "step_chunk"
  | "step_completed"
  | "step_failed"
  | "chain_completed"
  | "chain_failed"
  | "approval_requested"
  | "hitl_decision"
  | "tool_call_pending"
  | "tool_call"
  | "print"
  | "token_usage";

export type TaskEvent = {
  kind: TaskEventKind;
  timestamp: string;
  request_id?: string;
  chain_id?: string;
  task_id?: string;
  task_handler?: string;
  retry?: number;
  model_name?: string;
  provider_type?: string;
  backend_id?: string;
  output_type?: string;
  transition?: string;
  content?: string;
  thinking?: string;
  error?: string;
  attachments?: Array<{ kind: string; payload?: unknown }>;
  approval_id?: string;
  hook_name?: string;
  tool_name?: string;
  approval_args?: Record<string, unknown>;
  approval_diff?: string;
  // hitl_decision fields
  hitl_action?: string;
  hitl_reason?: string;
  hitl_policy_name?: string;
  hitl_args_summary?: string;
  hitl_matched_rule?: number;
  hitl_timeout_s?: number;
  hitl_approval_requested?: boolean;
  // tool_call fields
  tool_diff_path?: string;
  tool_diff_old_text?: string;
  tool_diff_new_text?: string;
  token_used?: number;
  token_size?: number;
};

export type TaskErrorState = {
  error: string | null;
};

export type TokenUsage = {
  prompt: number;
  completion: number;
  total: number;
};

/** Mirrors taskengine.CapturedStateUnit (Go). `duration` is a Go time.Duration — nanoseconds. */
export type CapturedStateUnit = {
  taskID: string;
  taskHandler: string;
  inputType: string;
  outputType: string;
  transition: string;
  duration: number;
  error: TaskErrorState;
  input?: unknown;
  output?: unknown;
  inputVar?: string;
  retryIndex?: number;
  cancelled?: boolean;
  timedOut?: boolean;
  providerType?: string;
  modelName?: string;
  toolNames?: string[];
  tokenUsage?: TokenUsage;
};
