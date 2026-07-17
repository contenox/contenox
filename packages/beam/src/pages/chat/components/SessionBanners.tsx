import { Button, Collapsible, InlineNotice, Span } from '@contenox/ui';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { classifyAcpExecutionError } from '../../../lib/acpFailureKind';

/** Auto-dismissing "reconnected" notice shown once a session's connection recovers. */
export function ResumedBanner() {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(true);
  useEffect(() => {
    const id = setTimeout(() => setVisible(false), 4000);
    return () => clearTimeout(id);
  }, []);
  if (!visible) return null;
  return <InlineNotice variant="info">{t('acp_chat.banner_resumed')}</InlineNotice>;
}

/**
 * The chain-failure banner. Classifies `message` via `classifyAcpExecutionError`
 * (see lib/acpFailureKind.ts) so a runtime-backend-unreachable failure, a
 * not-servable-default-model failure, and an unrelated chain failure each get
 * their own headline/description instead of one indistinguishable "Execution
 * failed" — the same three-way taxonomy `SetupRequiredState` uses for a
 * `/setup-status` blocking issue, so the two detection paths read as ONE
 * consistent state. The full raw error text stays behind a collapsed-by-default
 * disclosure.
 */
export function ExecutionErrorBanner({ message, onOpenSettings }: { message: string; onOpenSettings: () => void }) {
  const { t } = useTranslation();
  const kind = classifyAcpExecutionError(message);

  const headline =
    kind === 'backend_unreachable'
      ? t('acp_recovery.backend_unreachable_title')
      : kind === 'model_unavailable'
        ? t('acp_recovery.model_unavailable_title')
        : t('acp_chat.error_banner_headline');
  const hint =
    kind === 'backend_unreachable'
      ? t('acp_recovery.backend_unreachable_description')
      : kind === 'model_unavailable'
        ? t('acp_recovery.model_unavailable_description')
        : null;

  return (
    <InlineNotice variant="error">
      <div className="flex flex-col gap-1.5">
        <Span className="font-medium">{headline}</Span>
        {hint && <Span className="text-sm">{hint}</Span>}
        {kind === 'model_unavailable' && (
          <div>
            <Button type="button" variant="secondary" size="sm" onClick={onOpenSettings}>
              {t('acp_recovery.model_unavailable_action')}
            </Button>
          </div>
        )}
        <Collapsible defaultOpen={false} title={t('acp_chat.error_details_toggle')}>
          <p className="mt-1 text-xs whitespace-pre-wrap">{message}</p>
        </Collapsible>
      </div>
    </InlineNotice>
  );
}
