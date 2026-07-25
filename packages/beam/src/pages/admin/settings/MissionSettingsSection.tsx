import { Button, FormField, H2, InlineNotice, P, Panel, Select } from '@contenox/ui';
import { FormEvent, useContext, useEffect, useId, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useAgents } from '../../../hooks/useAgents';
import { useCLIConfig } from '../../../hooks/useCLIConfig';
import { useListPolicies } from '../../../hooks/usePolicies';
import { usePutCLIConfig } from '../../../hooks/usePutCLIConfig';
import { AuthContext } from '../../../lib/authContext';
import type { CLIConfigUpdateRequest } from '../../../lib/types';

const uniqueSorted = (values: string[]) =>
  Array.from(new Set(values.map(value => value.trim()).filter(Boolean))).sort((a, b) =>
    a.localeCompare(b),
  );

/**
 * The two global keys that make `/mission <intent>` fireable: which declared
 * agent runs the mission, and the HITL policy that bounds it. Both were
 * CLI-only (`contenox config set default-mission-agent …`), which is what a
 * `/mission` in beam told you to go do — this section is that instruction made
 * clickable, with the same list endpoints the dispatch itself resolves against.
 *
 * Both are picked from lists rather than typed: a mission refuses to fire on an
 * agent or a policy that does not resolve, so a free-text field could only
 * produce a value that fails later, at the moment of firing.
 */
export function MissionSettingsSection() {
  const { t } = useTranslation();
  const { user } = useContext(AuthContext);
  // The mission pair lives only in the full CLI-config snapshot: setup-status
  // carries the onboarding-readiness subset, which these are not part of.
  const { data: cliConfig } = useCLIConfig(!!user);
  const { data: agents, error: agentsError } = useAgents();
  const { data: policies, error: policiesError } = useListPolicies();
  const putConfig = usePutCLIConfig();
  const formId = useId();

  const [agent, setAgent] = useState('');
  const [policy, setPolicy] = useState('');

  useEffect(() => {
    if (!cliConfig) return;
    setAgent(cliConfig.defaultMissionAgent || '');
    setPolicy(cliConfig.defaultMissionPolicy || '');
  }, [cliConfig]);

  useEffect(() => {
    if (!putConfig.isSuccess) return;
    const timer = window.setTimeout(() => putConfig.reset(), 3000);
    return () => window.clearTimeout(timer);
  }, [putConfig.isSuccess, putConfig.reset]);

  // Only ENABLED agents are offered: the dispatch resolves through
  // ResolveForSpawn, which refuses a disabled agent outright, so offering one
  // would be offering a default that cannot fire.
  const enabledAgentNames = useMemo(
    () => uniqueSorted((agents ?? []).filter(a => a.enabled).map(a => a.name)),
    [agents],
  );

  const agentOptions = useMemo(() => {
    const values = [...enabledAgentNames];
    const current = agent.trim();
    // A stored name that is no longer on the list (agent deleted, renamed, or
    // since disabled) is kept as an option so the field shows the truth rather
    // than silently resetting to "not set" — the warning below names the risk.
    if (current && !values.includes(current)) values.unshift(current);
    return [
      { value: '', label: t('settings.mission_not_set') },
      ...values.map(value => ({ value, label: value })),
    ];
  }, [agent, enabledAgentNames, t]);

  const policyOptions = useMemo(() => {
    const values = uniqueSorted(policies ?? []);
    const current = policy.trim();
    if (current && !values.includes(current)) values.unshift(current);
    return [
      { value: '', label: t('settings.mission_not_set') },
      ...values.map(value => ({ value, label: value })),
    ];
  }, [policies, policy, t]);

  const agentUnresolved = useMemo(() => {
    const current = agent.trim();
    if (!current || !agents) return false;
    return !enabledAgentNames.includes(current);
  }, [agent, agents, enabledAgentNames]);

  const policyUnresolved = useMemo(() => {
    const current = policy.trim();
    if (!current || !policies) return false;
    return !(policies ?? []).map(value => value.trim()).includes(current);
  }, [policies, policy]);

  // A mission needs BOTH halves; with either missing, /mission answers with the
  // error that sent the operator here. Say so up front instead of letting a
  // half-configured section look done.
  const incomplete = !agent.trim() || !policy.trim();

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    putConfig.reset();
    // Always send both, even empty: an empty value is how a default is CLEARED,
    // and an omitted key would leave a stale one in place.
    const body: CLIConfigUpdateRequest = {
      'default-mission-agent': agent.trim(),
      'default-mission-policy': policy.trim(),
    };
    putConfig.mutate(body);
  };

  return (
    <Panel variant="surface">
      <div className="space-y-4">
        <div className="space-y-1">
          <H2>{t('settings.mission_section_title')}</H2>
          <P variant="muted" className="text-sm">
            {t('settings.mission_section_description')}
          </P>
        </div>

        <form id={formId} onSubmit={onSubmit} className="grid gap-4">
          {incomplete && (
            <InlineNotice variant="warning" className="rounded-lg">
              {t('settings.mission_incomplete_notice')}
            </InlineNotice>
          )}

          <FormField
            label={t('settings.mission_agent_label')}
            tooltip={t('settings.mission_agent_tooltip')}>
            <Select
              name="default-mission-agent"
              className="w-full"
              value={agent}
              onChange={e => setAgent(e.target.value)}
              options={agentOptions}
            />
            {agentUnresolved && (
              <P className="text-warning mt-1 text-xs">
                {t('settings.mission_agent_unresolved', { name: agent.trim() })}
              </P>
            )}
            {enabledAgentNames.length === 0 && !agentsError && (
              <P variant="muted" className="mt-1 text-xs">
                {t('settings.mission_no_agents')}
              </P>
            )}
            {agentsError && (
              <P className="text-error mt-1 text-xs">
                {t('settings.mission_agent_options_error', { message: agentsError.message })}
              </P>
            )}
          </FormField>

          <FormField
            label={t('settings.mission_policy_label')}
            tooltip={t('settings.mission_policy_tooltip')}>
            <Select
              name="default-mission-policy"
              className="w-full"
              value={policy}
              onChange={e => setPolicy(e.target.value)}
              options={policyOptions}
            />
            {policyUnresolved && (
              <P className="text-warning mt-1 text-xs">
                {t('settings.mission_policy_unresolved', { name: policy.trim() })}
              </P>
            )}
            {policiesError && (
              <P className="text-error mt-1 text-xs">
                {t('settings.policy_options_error', { message: policiesError.message })}
              </P>
            )}
          </FormField>

          {putConfig.isError && <P className="text-error text-sm">{putConfig.error.message}</P>}
          {putConfig.isSuccess && <P className="text-text-muted text-sm">{t('settings.saved')}</P>}

          <div>
            <Button
              type="submit"
              form={formId}
              variant="primary"
              size="sm"
              disabled={putConfig.isPending}>
              {t('settings.save')}
            </Button>
          </div>
        </form>
      </div>
    </Panel>
  );
}
