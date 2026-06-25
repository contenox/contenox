export type TaskEventKind =
  | "chain_started"
  | "step_started"
  | "step_chunk"
  | "step_completed"
  | "step_failed"
  | "chain_completed"
  | "chain_failed"
  | "approval_requested";

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
};

export type TaskErrorState = {
  error: string | null;
};

export type CapturedStateUnit = {
  taskID: string;
  taskType: string;
  inputType: string;
  outputType: string;
  transition: string;
  duration: number;
  error: TaskErrorState;
};
