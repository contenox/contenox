/**
 * i18n keys referenced in this file (namespace `acp_chat`; see i18n.ts):
 *   acp_chat.permission_default_kind   fallback tool-kind label
 *   acp_chat.permission_raw_input_label  "Raw input" section label
 *   acp_chat.approval_resolved_allowed / _denied / _stale  resolved banners
 *   acp_chat.approval_close            close-the-tab action
 */
import { Badge, Button, Collapsible, DiffView, diffLinesFromTexts, InlineNotice, Span } from '@contenox/ui';
import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import type { PermissionOption, RequestPermissionRequest } from '../../../lib/acp';
import {
  approvalResolutionForOption,
  approvalTabStatus,
  type ApprovalResolution,
} from '../lib/canvasTabs';
import { keyHintForOption, optionForKey, orderedPermissionOptions } from '../lib/permissionKeymap';

function optionVariant(kind: PermissionOption['kind']): 'primary' | 'danger' | 'outline' {
  if (kind === 'allow_once' || kind === 'allow_always') return 'primary';
  if (kind === 'reject_once' || kind === 'reject_always') return 'danger';
  return 'outline';
}

export interface ApprovalViewTabProps {
  /** The snapshotted request this tab renders (taken when the gate was maximized). */
  permission: RequestPermissionRequest;
  /** The session's live pending request (or null) — decides whether this tab is still live. */
  pendingPermission: RequestPermissionRequest | null;
  /** True when this is the active canvas tab: gates the y/a/n keymap so it only fires while focused. */
  active: boolean;
  /** The same responder the gate uses (`respondPermission`). */
  onRespond: (optionId: string) => void;
  /** Closes this canvas tab (called from the post-answer Close action). */
  onClose: () => void;
}

/**
 * A maximized permission approval hosted as a CANVAS tab — the full-size sibling
 * of `PermissionGate`. It renders the SAME payload (title, expanded diffs, raw
 * input) and the SAME Allow/Deny actions wired to the SAME `respondPermission`
 * callback, so a large diff or long command can be read in the full panel
 * instead of the cramped modal.
 *
 * Liveness is derived, not assumed: the tab renders live Allow/Deny only while
 * its snapshotted request is STILL the session's pending one (see
 * `approvalTabStatus`). The moment it resolves — answered here, answered in the
 * demoted modal after a restore, or the turn cancelled — the tab flips to a
 * read-only resolved banner rather than blanking out mid-read or, worse, leaving
 * a live Allow button on a dead request.
 *
 * The keymap is scoped to this element (a container `onKeyDown`, focused when the
 * tab is active and still pending) — never a global document listener — so y/a/n
 * only fire while the tab itself is focused.
 */
export function ApprovalViewTab({ permission, pendingPermission, active, onRespond, onClose }: ApprovalViewTabProps) {
  const { t } = useTranslation();
  const containerRef = useRef<HTMLDivElement>(null);
  // Set when the user answers IN this tab — lets the banner attribute the outcome
  // (allowed/denied) instead of the generic "resolved elsewhere".
  const [answered, setAnswered] = useState<ApprovalResolution | null>(null);

  const { toolCall } = permission;
  const status = approvalTabStatus(toolCall.toolCallId, pendingPermission?.toolCall.toolCallId ?? null, answered);
  const options = orderedPermissionOptions(permission.options);
  const diffs = (toolCall.content ?? []).filter(c => c.type === 'diff');

  const handleRespond = useCallback(
    (option: PermissionOption) => {
      setAnswered(approvalResolutionForOption(option.kind));
      onRespond(option.optionId);
    },
    [onRespond],
  );

  // Focus the panel when it becomes the active tab while still pending, so the
  // y/a/n keymap (a scoped onKeyDown below) has somewhere to land.
  useEffect(() => {
    if (active && status.state === 'pending') containerRef.current?.focus();
  }, [active, status.state]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLDivElement>) => {
      if (status.state !== 'pending') return;
      const byKind = optionForKey(permission.options, e.key);
      if (byKind) {
        e.preventDefault();
        handleRespond(byKind);
      }
    },
    [status.state, permission.options, handleRespond],
  );

  const resolvedNotice = () => {
    if (status.state !== 'resolved') return null;
    if (status.resolution === 'allowed') {
      return <InlineNotice variant="info">{t('acp_chat.approval_resolved_allowed')}</InlineNotice>;
    }
    if (status.resolution === 'denied') {
      return <InlineNotice variant="warning">{t('acp_chat.approval_resolved_denied')}</InlineNotice>;
    }
    return <InlineNotice variant="info">{t('acp_chat.approval_resolved_stale')}</InlineNotice>;
  };

  return (
    <div
      ref={containerRef}
      tabIndex={-1}
      onKeyDown={handleKeyDown}
      className="flex min-h-0 flex-1 flex-col gap-3 overflow-auto p-1 focus:outline-none">
      <div>
        <Span variant="status" className="text-text-muted dark:text-dark-text-muted">
          {toolCall.kind ?? t('acp_chat.permission_default_kind')}
        </Span>
        <p className="text-text dark:text-dark-text mt-1 text-sm font-medium">{toolCall.title ?? toolCall.toolCallId}</p>
      </div>

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
          <pre className="bg-surface-100 dark:bg-dark-surface-300 text-text dark:text-dark-text mt-2 overflow-auto rounded p-2 text-xs whitespace-pre-wrap">
            {JSON.stringify(toolCall.rawInput, null, 2)}
          </pre>
        </Collapsible>
      )}

      <div className="mt-auto">
        {status.state === 'pending' ? (
          <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:justify-end">
            {options.map(option => {
              const hint = keyHintForOption(option);
              return (
                <Button
                  key={option.optionId}
                  type="button"
                  variant={optionVariant(option.kind)}
                  className="w-full sm:w-auto"
                  onClick={() => handleRespond(option)}>
                  <span className="flex w-full items-center justify-between gap-2 sm:justify-center">
                    {option.name}
                    {hint && (
                      <Badge variant="outline" size="sm">
                        {hint}
                      </Badge>
                    )}
                  </span>
                </Button>
              );
            })}
          </div>
        ) : (
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            {resolvedNotice()}
            <Button type="button" variant="outline" className="w-full sm:w-auto" onClick={onClose}>
              {t('acp_chat.approval_close')}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
