import { Button, Collapsible, DiffView, diffLinesFromTexts, Span } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import type { PermissionOption, RequestPermissionRequest } from '../../../lib/acp';
import { orderedPermissionOptions } from '../lib/permissionKeymap';

function optionVariant(kind: PermissionOption['kind']): 'primary' | 'danger' | 'outline' {
  if (kind === 'allow_once' || kind === 'allow_always') return 'primary';
  if (kind === 'reject_once' || kind === 'reject_always') return 'danger';
  return 'outline';
}

export interface PermissionCardProps {
  /** The live pending permission request this card represents. */
  permission: RequestPermissionRequest;
  /** Answers the request with the chosen option id. The ONLY path that responds. */
  onRespond: (optionId: string) => void;
}

/**
 * The permission request rendered as an INLINE CARD in the session's transcript
 * flow — the replacement for the old modal `PermissionGate`. It is anchored
 * next to the pending tool call it belongs to (see `TranscriptItems`), so the
 * request lives chronologically where it happened instead of floating over the
 * page.
 *
 * Deliberately inert except for its option buttons: NO backdrop, NO portal, NO
 * document/Escape key listener, NO close/dismiss affordance, and NO effects.
 * The ONLY way to answer is an explicit click on one of the offered option
 * buttons — clicking elsewhere, pressing Escape, scrolling, switching tabs, or
 * unmounting the card can never send a response, so an unanswered request stays
 * pending (correct ACP semantics: the agent waits). This is what fixes the
 * "click-outside silently denies" bug. Several of these can be live at once,
 * one per session, each reading its own slice's `pendingPermission`.
 */
export function PermissionCard({ permission, onRespond }: PermissionCardProps) {
  const { t } = useTranslation();
  const { toolCall } = permission;
  const options = orderedPermissionOptions(permission.options);
  const diffs = (toolCall.content ?? []).filter(c => c.type === 'diff');

  return (
    <div
      role="group"
      aria-label={t('acp_chat.permission_card_title')}
      className="border-warning-300 bg-warning-50 dark:border-dark-surface-500 dark:bg-dark-surface-200 my-2 rounded-xl border p-4 shadow-sm">
      <Span variant="status" className="text-warning-800 dark:text-dark-text-muted">
        {t('acp_chat.permission_card_title')}
      </Span>

      <div className="mt-2 min-w-0">
        <Span variant="status" className="text-text-muted dark:text-dark-text-muted">
          {toolCall.kind ?? t('acp_chat.permission_default_kind')}
        </Span>
        <p className="text-text dark:text-dark-text mt-1 text-sm font-medium">{toolCall.title ?? toolCall.toolCallId}</p>
      </div>

      <div className="mt-3 space-y-3">
        {toolCall.locations && toolCall.locations.length > 0 && (
          <ul className="text-text-muted dark:text-dark-text-muted space-y-0.5 text-xs">
            {toolCall.locations.map((loc, i) => (
              <li key={i}>
                {loc.path}
                {loc.line ? `:${loc.line}` : ''}
              </li>
            ))}
          </ul>
        )}

        {diffs.map((d, i) => (
          <DiffView key={i} filePath={d.path ?? ''} lines={diffLinesFromTexts(d.oldText ?? '', d.newText ?? '')} />
        ))}

        {toolCall.rawInput != null && (
          <Collapsible title={t('acp_chat.permission_raw_input_label')}>
            <pre className="bg-surface-100 dark:bg-dark-surface-300 text-text dark:text-dark-text mt-2 max-h-40 overflow-auto rounded p-2 text-xs whitespace-pre-wrap">
              {JSON.stringify(toolCall.rawInput, null, 2)}
            </pre>
          </Collapsible>
        )}
      </div>

      <div className="mt-4 flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:justify-end">
        {options.map(option => (
          <Button
            key={option.optionId}
            type="button"
            variant={optionVariant(option.kind)}
            className="w-full sm:w-auto"
            onClick={() => onRespond(option.optionId)}>
            {option.name}
          </Button>
        ))}
      </div>
    </div>
  );
}
