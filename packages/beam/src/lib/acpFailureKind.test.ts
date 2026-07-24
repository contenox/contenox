import { describe, expect, it } from 'vitest';
import { acpFailureCopyKeys, classifyAcpExecutionError, classifySetupIssueCode } from './acpFailureKind';

describe('classifyAcpExecutionError', () => {
  it('classifies the runtime-state-empty resolver error as backend_unreachable', () => {
    expect(classifyAcpExecutionError('no models found in runtime state')).toBe('backend_unreachable');
  });

  it('classifies modeld-specific connectivity phrasings as backend_unreachable', () => {
    expect(classifyAcpExecutionError('backend error: modeld is not running')).toBe('backend_unreachable');
    expect(classifyAcpExecutionError('dial tcp 127.0.0.1:11434: connect: connection refused')).toBe(
      'backend_unreachable',
    );
  });

  it('classifies "no model matched"/"client resolution failed" as model_unavailable', () => {
    expect(classifyAcpExecutionError('prompt execute: client resolution failed: no model matched the requirements')).toBe(
      'model_unavailable',
    );
    expect(classifyAcpExecutionError('chat: client resolution failed: some wrapped error')).toBe('model_unavailable');
  });

  it('classifies a context-overflow message as model_unavailable (config problem, not connectivity)', () => {
    expect(
      classifyAcpExecutionError(
        'request needs 131072 tokens of context but the largest available model "qwen3-4b" provides only 32768; use a larger-context model or reduce the request size',
      ),
    ).toBe('model_unavailable');
  });

  it('classifies the agent host\'s fail-closed sandbox refusal as workspace_required, even wrapped deep in the acpsvc/agentinstance/agenthost chain', () => {
    expect(
      classifyAcpExecutionError(
        'acpsvc: start agent "claude" instance: agentinstance: start agent "claude": agenthost: sandbox external ACP agent "/home/naro/.contenox/claude-code-acp.sh": cwd is required to confine the agent (the wall needs a workspace; it will not default to the whole filesystem)',
      ),
    ).toBe('workspace_required');
  });

  it('falls back to generic for an unrelated chain failure', () => {
    expect(classifyAcpExecutionError('hook "mailing-service" returned HTTP 500')).toBe('generic');
  });

  it('falls back to generic for null/empty/undefined', () => {
    expect(classifyAcpExecutionError(null)).toBe('generic');
    expect(classifyAcpExecutionError(undefined)).toBe('generic');
    expect(classifyAcpExecutionError('')).toBe('generic');
  });
});

describe('acpFailureCopyKeys', () => {
  it('maps each specific kind to its own title + description i18n keys', () => {
    expect(acpFailureCopyKeys('backend_unreachable')).toEqual({
      titleKey: 'acp_recovery.backend_unreachable_title',
      descriptionKey: 'acp_recovery.backend_unreachable_description',
    });
    expect(acpFailureCopyKeys('model_unavailable')).toEqual({
      titleKey: 'acp_recovery.model_unavailable_title',
      descriptionKey: 'acp_recovery.model_unavailable_description',
    });
    expect(acpFailureCopyKeys('workspace_required')).toEqual({
      titleKey: 'acp_recovery.workspace_required_title',
      descriptionKey: 'acp_recovery.workspace_required_description',
    });
  });

  it('returns null keys for generic — each caller keeps its own generic fallback copy', () => {
    expect(acpFailureCopyKeys('generic')).toEqual({ titleKey: null, descriptionKey: null });
  });
});

describe('classifySetupIssueCode', () => {
  it('maps backend-connectivity issue codes to backend_unreachable', () => {
    expect(classifySetupIssueCode('runtime_state_empty')).toBe('backend_unreachable');
    expect(classifySetupIssueCode('all_backends_unreachable')).toBe('backend_unreachable');
    expect(classifySetupIssueCode('default_provider_unreachable')).toBe('backend_unreachable');
    expect(classifySetupIssueCode('default_provider_not_synced')).toBe('backend_unreachable');
  });

  it('maps default-model-selection issue codes to model_unavailable', () => {
    expect(classifySetupIssueCode('default_model_not_available')).toBe('model_unavailable');
    expect(classifySetupIssueCode('missing_default_model')).toBe('model_unavailable');
  });

  it('leaves unrelated issue codes (auth, missing backends, etc.) as generic', () => {
    expect(classifySetupIssueCode('default_provider_api_key_missing')).toBe('generic');
    expect(classifySetupIssueCode('default_provider_auth_failed')).toBe('generic');
    expect(classifySetupIssueCode('no_backends')).toBe('generic');
    expect(classifySetupIssueCode('no_chat_models')).toBe('generic');
    expect(classifySetupIssueCode(null)).toBe('generic');
    expect(classifySetupIssueCode(undefined)).toBe('generic');
  });
});
