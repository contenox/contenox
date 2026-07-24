import { Button, ErrorState, H2, InlineNotice, Input, LoadingState, P, Panel } from '@contenox/ui';
import { FormEvent, useContext, useEffect, useId, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePutShellEnv } from '../../../hooks/usePutShellEnv';
import { useShellEnv } from '../../../hooks/useShellEnv';
import { AuthContext } from '../../../lib/authContext';

// Variable names must match the backend contract: letters, digits and
// underscores, not starting with a digit.
const NAME_RE = /^[A-Za-z_][A-Za-z0-9_]*$/;

type EnvRow = { id: number; name: string; value: string };

export function ShellEnvSettingsSection() {
  const { t } = useTranslation();
  const { user } = useContext(AuthContext);
  const { data, isLoading, isError, error, refetch } = useShellEnv(!!user);
  const putEnv = usePutShellEnv();
  const formId = useId();

  // Rows are edited as an ordered list (not a Record keyed by name) so a
  // half-typed or duplicated name stays visible instead of collapsing onto
  // another row; the vars map is rebuilt from the rows on save.
  const [rows, setRows] = useState<EnvRow[]>([]);
  const idRef = useRef(0);
  const nextId = () => ++idRef.current;

  useEffect(() => {
    if (!data) return;
    setRows(
      Object.entries(data.vars ?? {}).map(([name, value]) => ({ id: nextId(), name, value })),
    );
  }, [data]);

  useEffect(() => {
    if (!putEnv.isSuccess) return;
    const timer = window.setTimeout(() => putEnv.reset(), 3000);
    return () => window.clearTimeout(timer);
  }, [putEnv.isSuccess, putEnv.reset]);

  const validation = useMemo(() => {
    const counts = new Map<string, number>();
    for (const row of rows) {
      const name = row.name.trim();
      if (name) counts.set(name, (counts.get(name) ?? 0) + 1);
    }
    const rowErrors = new Map<number, 'empty' | 'invalid' | 'duplicate'>();
    let hasEmpty = false;
    let hasInvalid = false;
    let hasDuplicate = false;
    for (const row of rows) {
      const name = row.name.trim();
      if (name === '') {
        hasEmpty = true;
        rowErrors.set(row.id, 'empty');
      } else if (!NAME_RE.test(name)) {
        hasInvalid = true;
        rowErrors.set(row.id, 'invalid');
      } else if ((counts.get(name) ?? 0) > 1) {
        hasDuplicate = true;
        rowErrors.set(row.id, 'duplicate');
      }
    }
    return {
      hasEmpty,
      hasInvalid,
      hasDuplicate,
      rowErrors,
      hasErrors: hasEmpty || hasInvalid || hasDuplicate,
    };
  }, [rows]);

  const addRow = () => setRows(prev => [...prev, { id: nextId(), name: '', value: '' }]);
  const removeRow = (id: number) => setRows(prev => prev.filter(row => row.id !== id));
  const updateName = (id: number, name: string) =>
    setRows(prev => prev.map(row => (row.id === id ? { ...row, name } : row)));
  const updateValue = (id: number, value: string) =>
    setRows(prev => prev.map(row => (row.id === id ? { ...row, value } : row)));

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (validation.hasErrors) return;
    putEnv.reset();
    // Full replacement: build the whole map from the current rows. An empty
    // rows list PUTs {} and clears every injected variable on the server.
    const vars: Record<string, string> = {};
    for (const row of rows) {
      const name = row.name.trim();
      if (name) vars[name] = row.value;
    }
    putEnv.mutate({ vars });
  };

  return (
    <Panel variant="surface">
      <div className="space-y-4">
        <div className="space-y-1">
          <H2>{t('settings.shell_env_section_title')}</H2>
          <P variant="muted" className="text-sm">
            {t('settings.shell_env_section_description')}
          </P>
        </div>

        {isLoading ? (
          <LoadingState message={t('settings.shell_env_loading')} />
        ) : isError ? (
          <ErrorState
            error={error ?? undefined}
            onRetry={refetch}
            title={t('settings.shell_env_load_error')}
          />
        ) : (
          <form id={formId} onSubmit={onSubmit} className="grid gap-4">
            {validation.hasErrors && (
              <InlineNotice variant="error" className="rounded-lg">
                <ul className="list-disc space-y-0.5 pl-4">
                  {validation.hasEmpty && <li>{t('settings.shell_env_error_empty_name')}</li>}
                  {validation.hasInvalid && <li>{t('settings.shell_env_error_invalid_name')}</li>}
                  {validation.hasDuplicate && (
                    <li>{t('settings.shell_env_error_duplicate_name')}</li>
                  )}
                </ul>
              </InlineNotice>
            )}

            {rows.length > 0 && (
              <div className="flex gap-2">
                <P variant="muted" className="flex-1 text-xs">
                  {t('settings.shell_env_name_label')}
                </P>
                <P variant="muted" className="flex-1 text-xs">
                  {t('settings.shell_env_value_label')}
                </P>
                <span className="w-[4.5rem] shrink-0" aria-hidden="true" />
              </div>
            )}

            <div className="space-y-2">
              {rows.map(row => (
                <div key={row.id} className="flex gap-2">
                  <Input
                    value={row.name}
                    onChange={e => updateName(row.id, e.target.value)}
                    placeholder={t('settings.shell_env_name_placeholder')}
                    className="flex-1"
                    aria-invalid={validation.rowErrors.has(row.id) || undefined}
                    disabled={putEnv.isPending}
                  />
                  <Input
                    value={row.value}
                    onChange={e => updateValue(row.id, e.target.value)}
                    placeholder={t('settings.shell_env_value_placeholder')}
                    className="flex-1"
                    disabled={putEnv.isPending}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="w-[4.5rem] shrink-0"
                    onClick={() => removeRow(row.id)}
                    disabled={putEnv.isPending}>
                    {t('common.remove')}
                  </Button>
                </div>
              ))}
              {rows.length === 0 && (
                <P variant="muted" className="text-xs">
                  {t('settings.shell_env_empty')}
                </P>
              )}
            </div>

            <div>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={addRow}
                disabled={putEnv.isPending}>
                {t('settings.shell_env_add')}
              </Button>
            </div>

            {putEnv.isError && <P className="text-error text-sm">{putEnv.error.message}</P>}
            {putEnv.isSuccess && <P className="text-text-muted text-sm">{t('settings.saved')}</P>}

            <div>
              <Button
                type="submit"
                form={formId}
                variant="primary"
                size="sm"
                disabled={putEnv.isPending || validation.hasErrors}>
                {t('settings.save')}
              </Button>
            </div>
          </form>
        )}
      </div>
    </Panel>
  );
}
