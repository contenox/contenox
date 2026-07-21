import { Button, Form, FormField, H1, Input, P, Page, Select } from '@contenox/ui';
import { FormEvent, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { RootChip } from '../../../components/workspace/RootChip';
import { RootSelector } from '../../../components/workspace/RootSelector';
import { useAgents } from '../../../hooks/useAgents';
import { useDispatchMission } from '../../../hooks/useFleet';
import { useListPolicies } from '../../../hooks/usePolicies';
import { useWorkspaceRoots } from '../../../hooks/useWorkspaceRoots';
import type { TranslationKey } from '../../../i18n';
import { ApiError } from '../../../lib/fetch';
import { extractRefusedRoot, isWorkspaceRootRefusal } from '../../../lib/workspaceRoots';
import {
  canSubmitDispatchForm,
  type DispatchFormValues,
  validateDispatchForm,
} from './dispatchForm';

const EMPTY_FORM: DispatchFormValues = { agentName: '', intent: '', hitlPolicyName: '', cwd: '' };

/**
 * Fire-a-mission (docs/development/blueprints/acp/fleet-consolidation.md,
 * "Mission mode", slice M2): agent, intent, and envelope are required — a
 * mission with no envelope has no bounds, and the intent IS the prompt (see
 * DispatchRequest), so there is deliberately no separate prompt field. On
 * success the operator lands on the new mission's own detail page rather
 * than back on the list, since firing one is usually followed by wanting to
 * watch it.
 */
export default function MissionDispatchPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data: agents } = useAgents();
  const { data: policies } = useListPolicies();
  const { roots, defaultRoot, isAbsent: rootsAbsent } = useWorkspaceRoots();
  const dispatch = useDispatchMission();

  // A dispatch can be refused because the chosen cwd is outside the workspace
  // roots (the server does the real bounds check). Surface THAT legibly on the
  // working-directory field instead of the raw wire string, and suppress the
  // generic form-level error so the reason shows exactly once, where it belongs.
  const dispatchError = dispatch.error;
  const rootRefused =
    dispatch.isError &&
    dispatchError instanceof ApiError &&
    dispatchError.status === 422 &&
    isWorkspaceRootRefusal(dispatchError.message);

  // The command palette's agent action lands here with `?agent=<name>` so the
  // form opens with that agent already chosen — the "dispatch prefill" the
  // palette promises. Read once as the initial value; the Select stays freely
  // editable afterwards.
  const [searchParams] = useSearchParams();
  const [values, setValues] = useState<DispatchFormValues>(() => {
    const agent = searchParams.get('agent');
    return agent ? { ...EMPTY_FORM, agentName: agent } : EMPTY_FORM;
  });
  // Errors stay computed but hidden until a first submit attempt — a field
  // required error the instant the page loads (before anyone has typed
  // anything) would just be noise.
  const [touched, setTouched] = useState(false);

  const errors = validateDispatchForm(values);
  const fieldError = (key?: TranslationKey) => (touched && key ? t(key) : undefined);

  // Only-enabled filter mirrors AgentPicker.tsx's own judgment (see its doc
  // comment) — the server-side refusal (agentregistryservice.ResolveForSpawn)
  // is the real gate; this is belt-and-braces UX so a disabled agent is never
  // even offered.
  const enabledAgents = (agents ?? []).filter(a => a.enabled);
  const agentOptions = enabledAgents.map(a => ({ value: a.name, label: a.name }));
  const policyOptions = (policies ?? [])
    .slice()
    .sort((a, b) => a.localeCompare(b))
    .map(p => ({ value: p, label: p }));

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setTouched(true);
    if (!canSubmitDispatchForm(values)) return;
    const cwd = values.cwd.trim();
    dispatch.mutate(
      {
        agentName: values.agentName,
        intent: values.intent,
        hitlPolicyName: values.hitlPolicyName,
        ...(cwd ? { cwd } : {}),
      },
      { onSuccess: result => navigate(`/missions/${result.missionId}`) },
    );
  };

  return (
    <Page bodyScroll="auto">
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-8 p-4 md:p-6">
        <div>
          <H1 variant="page">{t('missions.form_title')}</H1>
          <P variant="muted" className="mt-2">
            {t('missions.form_description')}
          </P>
          {defaultRoot && (
            <div className="mt-3">
              <RootChip root={defaultRoot} />
            </div>
          )}
        </div>

        <Form
          onSubmit={onSubmit}
          error={
            dispatch.isError && !rootRefused
              ? `${t('missions.form_dispatch_error')} ${dispatch.error?.message ?? ''}`
              : undefined
          }
          actions={
            <Button
              type="submit"
              variant="primary"
              isLoading={dispatch.isPending}
              disabled={dispatch.isPending}>
              {dispatch.isPending ? t('missions.form_submitting') : t('missions.form_submit')}
            </Button>
          }>
          <FormField
            label={t('missions.form_agent_label')}
            required
            error={fieldError(errors.agentName)}>
            <Select
              className="w-full"
              value={values.agentName}
              onChange={e => setValues(v => ({ ...v, agentName: e.target.value }))}
              options={agentOptions}
              placeholder={
                enabledAgents.length === 0
                  ? t('missions.form_agent_empty')
                  : t('missions.form_agent_placeholder')
              }
            />
          </FormField>

          <FormField
            label={t('missions.form_intent_label')}
            required
            description={t('missions.form_intent_help')}
            error={fieldError(errors.intent)}>
            <Input
              type="text"
              value={values.intent}
              onChange={e => setValues(v => ({ ...v, intent: e.target.value }))}
              placeholder={t('missions.form_intent_placeholder')}
              error={!!fieldError(errors.intent)}
            />
          </FormField>

          <FormField
            label={t('missions.form_envelope_label')}
            required
            description={t('missions.form_envelope_help')}
            error={fieldError(errors.hitlPolicyName)}>
            <Select
              className="w-full"
              value={values.hitlPolicyName}
              onChange={e => setValues(v => ({ ...v, hitlPolicyName: e.target.value }))}
              options={policyOptions}
              placeholder={
                policyOptions.length === 0
                  ? t('missions.form_envelope_empty')
                  : t('missions.form_envelope_placeholder')
              }
            />
          </FormField>

          <FormField
            label={t('missions.form_cwd_label')}
            error={
              rootRefused
                ? t('roots.out_of_bounds_body_path', {
                    path: extractRefusedRoot(dispatch.error?.message) ?? values.cwd,
                  })
                : undefined
            }>
            <RootSelector
              value={values.cwd}
              onChange={cwd => setValues(v => ({ ...v, cwd }))}
              roots={roots}
              isAbsent={rootsAbsent}
            />
          </FormField>
        </Form>
      </div>
    </Page>
  );
}
