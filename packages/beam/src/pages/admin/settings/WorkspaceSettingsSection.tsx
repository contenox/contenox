import { Button, FormField, H2, InlineNotice, P, Panel, Select } from '@contenox/ui';
import { FormEvent, useContext, useEffect, useId, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useListChains } from '../../../hooks/useChains';
import { useListPolicies } from '../../../hooks/usePolicies';
import { usePutCLIConfig } from '../../../hooks/usePutCLIConfig';
import { useSetupStatus } from '../../../hooks/useSetupStatus';
import { AuthContext } from '../../../lib/authContext';
import type { CLIConfigUpdateRequest } from '../../../lib/types';

const uniqueSorted = (values: string[]) =>
  Array.from(new Set(values.map(value => value.trim()).filter(Boolean))).sort((a, b) =>
    a.localeCompare(b),
  );

export function WorkspaceSettingsSection() {
  const { t } = useTranslation();
  const { user } = useContext(AuthContext);
  const { data } = useSetupStatus(!!user);
  const { data: chains, error: chainsError } = useListChains();
  const { data: policies, error: policiesError } = useListPolicies();
  const putConfig = usePutCLIConfig();
  const formId = useId();

  const chainScope = data?.resolvedFrom?.defaultChain;
  const policyScope = data?.resolvedFrom?.hitlPolicyName;

  const [chain, setChain] = useState('');
  const [policy, setPolicy] = useState('');

  useEffect(() => {
    if (!data) return;
    setChain(chainScope === 'workspace' ? data.defaultChain || '' : '');
    setPolicy(policyScope === 'workspace' ? data.hitlPolicyName || '' : '');
  }, [chainScope, data, policyScope]);

  useEffect(() => {
    if (!putConfig.isSuccess) return;
    const timer = window.setTimeout(() => putConfig.reset(), 3000);
    return () => window.clearTimeout(timer);
  }, [putConfig.isSuccess, putConfig.reset]);

  const chainOptions = useMemo(() => {
    const values = uniqueSorted(chains ?? []);
    const current = chain.trim();
    if (current && !values.includes(current)) values.unshift(current);
    return [
      { value: '', label: t('settings.inherit_runtime_default') },
      ...values.map(value => ({ value, label: value })),
    ];
  }, [chain, chains, t]);

  const policyOptions = useMemo(() => {
    const values = uniqueSorted(policies ?? []);
    const current = policy.trim();
    if (current && !values.includes(current)) values.unshift(current);
    return [
      { value: '', label: t('settings.inherit_runtime_default') },
      ...values.map(value => ({ value, label: value })),
    ];
  }, [policies, policy, t]);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    putConfig.reset();
    const body: CLIConfigUpdateRequest = {
      'default-chain': chain.trim(),
      'hitl-policy-name': policy.trim(),
    };
    putConfig.mutate(body);
  };

  return (
    <Panel variant="surface">
      <div className="space-y-4">
        <div className="space-y-1">
          <H2>{t('settings.workspace_section_title')}</H2>
          <P variant="muted" className="text-sm">
            {t('settings.workspace_section_description')}
          </P>
        </div>

        <form id={formId} onSubmit={onSubmit} className="grid gap-4">
          <InlineNotice variant="info" className="rounded-lg">
            {t('settingsAdvanced.chain_scope_notice')}
          </InlineNotice>

          <FormField label={t('settings.default_chain_label')}>
            <Select
              name="default-chain"
              className="w-full"
              value={chain}
              onChange={e => setChain(e.target.value)}
              options={chainOptions}
            />
            {chainScope === 'global' && data?.defaultChain && (
              <P variant="muted" className="mt-1 text-xs">
                {t('settings.inherited_value', { value: data.defaultChain })}
              </P>
            )}
            {chainsError && (
              <P className="text-error mt-1 text-xs">
                {t('settings.chain_options_error', { message: chainsError.message })}
              </P>
            )}
          </FormField>

          <FormField
            label={t('settings.hitl_policy_label')}
            tooltip={t('settings.hitl_policy_tooltip')}>
            <Select
              name="hitl-policy-name"
              className="w-full"
              value={policy}
              onChange={e => setPolicy(e.target.value)}
              options={policyOptions}
            />
            {policyScope === 'global' && data?.hitlPolicyName && (
              <P variant="muted" className="mt-1 text-xs">
                {t('settings.inherited_value', { value: data.hitlPolicyName })}
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
