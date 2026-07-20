import { describe, expect, it } from 'vitest';
import {
  canSubmitDispatchForm,
  type DispatchFormValues,
  validateDispatchForm,
} from './dispatchForm';

const valid: DispatchFormValues = {
  agentName: 'researcher',
  intent: 'Investigate the flaky nightly test',
  hitlPolicyName: 'hitl-policy-dev.json',
  cwd: '',
};

describe('validateDispatchForm', () => {
  it('accepts a fully filled form with no errors', () => {
    expect(validateDispatchForm(valid)).toEqual({});
    expect(canSubmitDispatchForm(valid)).toBe(true);
  });

  it('requires an agent', () => {
    const errors = validateDispatchForm({ ...valid, agentName: '' });
    expect(errors.agentName).toBe('missions.form_error_agent');
    expect(canSubmitDispatchForm({ ...valid, agentName: '' })).toBe(false);
  });

  it('treats a whitespace-only agent the same as an empty one', () => {
    expect(validateDispatchForm({ ...valid, agentName: '   ' }).agentName).toBe(
      'missions.form_error_agent',
    );
  });

  it('requires an intent — it is the prompt, not an optional note', () => {
    const errors = validateDispatchForm({ ...valid, intent: '' });
    expect(errors.intent).toBe('missions.form_error_intent_required');
  });

  it('rejects a multi-line intent, mirroring missionservice.validate server-side', () => {
    const errors = validateDispatchForm({ ...valid, intent: 'line one\nline two' });
    expect(errors.intent).toBe('missions.form_error_intent_single_line');
  });

  it('rejects a carriage-return-only line break too', () => {
    const errors = validateDispatchForm({ ...valid, intent: 'line one\rline two' });
    expect(errors.intent).toBe('missions.form_error_intent_single_line');
  });

  it('requires an envelope — a mission with no HITL policy has no bounds', () => {
    const errors = validateDispatchForm({ ...valid, hitlPolicyName: '' });
    expect(errors.hitlPolicyName).toBe('missions.form_error_envelope');
  });

  it('never validates cwd — DispatchRequest marks it optional', () => {
    expect(validateDispatchForm({ ...valid, cwd: '' })).toEqual({});
    expect(validateDispatchForm({ ...valid, cwd: '   ' })).toEqual({});
  });

  it('reports every missing required field at once, not just the first', () => {
    const errors = validateDispatchForm({ agentName: '', intent: '', hitlPolicyName: '', cwd: '' });
    expect(errors.agentName).toBeDefined();
    expect(errors.intent).toBeDefined();
    expect(errors.hitlPolicyName).toBeDefined();
    expect(canSubmitDispatchForm({ agentName: '', intent: '', hitlPolicyName: '', cwd: '' })).toBe(
      false,
    );
  });
});
