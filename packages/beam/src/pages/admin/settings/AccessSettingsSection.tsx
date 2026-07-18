import { Button, H2, P, Panel } from '@contenox/ui';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useContext } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../../../lib/api';
import { AuthContext } from '../../../lib/authContext';
import { authKeys } from '../../../lib/queryKeys';

/**
 * Remote-access controls. Rendered only when the server requires a login token
 * (authRequired) — locally there is nothing to log out of. Logging out clears
 * the HttpOnly session cookie server-side, then re-queries /ui/auth-status so
 * App.tsx's AuthGate swaps back to the login page.
 */
export function AccessSettingsSection() {
  const { t } = useTranslation();
  const { authRequired, refresh } = useContext(AuthContext);
  const queryClient = useQueryClient();

  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: authKeys.status() });
      refresh();
    },
  });

  if (!authRequired) return null;

  return (
    <Panel variant="surface">
      <div className="space-y-4">
        <div className="space-y-1">
          <H2>{t('settings.access_section_title')}</H2>
          <P variant="muted" className="text-sm">
            {t('settings.access_section_description')}
          </P>
        </div>
        {logout.isError && <P className="text-error text-sm">{logout.error.message}</P>}
        <div>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            disabled={logout.isPending}
            onClick={() => logout.mutate()}>
            {logout.isPending ? t('settings.access_logging_out') : t('settings.access_logout')}
          </Button>
        </div>
      </div>
    </Panel>
  );
}
