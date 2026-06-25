import type { TaskHandler } from './types';

export const CHAIN_HANDLER_OPTIONS: Array<{
  value: TaskHandler;
  label: string;
  hint?: string;
}> = [
  {
    value: 'route',
    label: 'Route',
    hint: 'Ask the model to pick one transition label',
  },
  {
    value: 'chat_completion',
    label: 'Chat Completion',
    hint: 'Run full chat (tools, etc.)',
  },
  {
    value: 'execute_tool_calls',
    label: 'Execute Tool Calls',
    hint: 'Run tools that previous model call asked for',
  },
  {
    value: 'tools',
    label: 'Tools',
    hint: 'Run a single configured tools-provider call',
  },
  {
    value: 'raise_error',
    label: 'Raise Error',
    hint: 'Fail the chain with the task input as message',
  },
  {
    value: 'noop',
    label: 'No Operation',
    hint: 'Pass input through unchanged',
  },
];
