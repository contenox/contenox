import { Button, Collapsible, InlineNotice, Span } from '@contenox/ui';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { acpFailureCopyKeys, classifyAcpExecutionError } from '../../../lib/acpFailureKind';

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
 * not-servable-default-model failure, an external-agent-needs-a-workspace
 * failure, and an unrelated chain failure each get their own
 * headline/description (via `acpFailureCopyKeys`) instead of one
 * indistinguishable "Execution failed" — the same taxonomy `SetupRequiredState`
 * uses for a `/setup-status` blocking issue, so the two detection paths read as
 * ONE consistent state. The full raw error text stays behind a
 * collapsed-by-default disclosure.
 *
 * This is the ONLY place this exact failure renders while it is the session's
 * current error: `TranscriptItems`' `TranscriptError` deliberately skips the
 * matching in-transcript card for as long as `session.error` still points at
 * it (see that component's `latestErrorItemId` note) — otherwise the same
 * failure would show here AND in the transcript at once.
 */
export function ExecutionErrorBanner({ message, onOpenSettings }: { message: string; onOpenSettings: () => void }) {
  const { t } = useTranslation();
  // Loosened `t` for the dynamic keys acpFailureCopyKeys returns (mirrors WorkspacePanel's `tk`).
  const tk = t as (key: string) => string;
  const kind = classifyAcpExecutionError(message);
  const copy = acpFailureCopyKeys(kind);

  const headline = copy.titleKey ? tk(copy.titleKey) : t('acp_chat.error_banner_headline');
  const hint = copy.descriptionKey ? tk(copy.descriptionKey) : null;

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
