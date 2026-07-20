import { Button, Form, FormField, GridLayout, H1, Input, P, Page, Select } from '@contenox/ui';
import { FormEvent, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useAgents } from '../../../hooks/useAgents';
import { useDispatchMission } from '../../../hooks/useFleet';
import { useListPolicies } from '../../../hooks/usePolicies';
import type { TranslationKey } from '../../../i18n';
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
  const dispatch = useDispatchMission();

  const [values, setValues] = useState<DispatchFormValues>(EMPTY_FORM);
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
      <GridLayout variant="body" minWidth="minmax(0, 1fr)" className="gap-8 pb-8">
        <div>
          <H1 variant="page">{t('missions.form_title')}</H1>
          <P variant="muted" className="mt-2">
            {t('missions.form_description')}
          </P>
        </div>

        <Form
          onSubmit={onSubmit}
          error={
            dispatch.isError
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

          <FormField label={t('missions.form_cwd_label')}>
            <Input
              type="text"
              value={values.cwd}
              onChange={e => setValues(v => ({ ...v, cwd: e.target.value }))}
              placeholder={t('missions.form_cwd_placeholder')}
            />
          </FormField>
        </Form>
      </GridLayout>
    </Page>
  );
}
