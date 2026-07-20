import type { TranslationKey } from '../../../i18n';

/**
 * Fire-a-mission form state. Mirrors DispatchRequest field-for-field, except
 * `cwd` stays a plain (possibly blank) string here — the caller trims it and
 * omits it from the wire request when blank, since DispatchRequest marks it
 * optional.
 */
export type DispatchFormValues = {
  agentName: string;
  intent: string;
  hitlPolicyName: string;
  cwd: string;
};

export type DispatchFormErrors = Partial<
  Record<keyof Omit<DispatchFormValues, 'cwd'>, TranslationKey>
>;

/**
 * Fire-a-mission's validation gate, as a pure function — mirrors FleetPage's
 * createStopFlow/stopConfirmCopy: the DOM-less test environment
 * (see PermissionCard.test.tsx) can't simulate a submit event, so the gate
 * that decides whether useDispatchMission ever gets called lives here, where
 * it is directly callable and testable.
 *
 * The rules mirror fleetservice.Dispatch and missionservice.validate exactly
 * (agentName, intent, and hitlPolicyName are all required; intent must be a
 * single line, since it becomes the unit's first turn verbatim) so a missing
 * or malformed field is caught here, before a round trip, not just echoed
 * back from a 400. `cwd` is the one DispatchRequest field marked optional and
 * is intentionally never validated.
 */
export function validateDispatchForm(values: DispatchFormValues): DispatchFormErrors {
  const errors: DispatchFormErrors = {};
  if (!values.agentName.trim()) {
    errors.agentName = 'missions.form_error_agent';
  }
  if (!values.intent.trim()) {
    errors.intent = 'missions.form_error_intent_required';
  } else if (/[\r\n]/.test(values.intent)) {
    errors.intent = 'missions.form_error_intent_single_line';
  }
  if (!values.hitlPolicyName.trim()) {
    errors.hitlPolicyName = 'missions.form_error_envelope';
  }
  return errors;
}

/** Whether the form has zero validation errors — the submit gate's boolean shorthand. */
export function canSubmitDispatchForm(values: DispatchFormValues): boolean {
  return Object.keys(validateDispatchForm(values)).length === 0;
}
