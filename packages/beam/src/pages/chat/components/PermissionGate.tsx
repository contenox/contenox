import { Badge, Button, Collapsible, DiffView, diffLinesFromTexts, Span } from '@contenox/ui';
import { Maximize2 } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import type { PermissionOption, RequestPermissionRequest } from '../../../lib/acp';
import { keyHintForOption, optionForKey, orderedPermissionOptions, safestRejectOption } from '../lib/permissionKeymap';

function optionVariant(kind: PermissionOption['kind']): 'primary' | 'danger' | 'outline' {
  if (kind === 'allow_once' || kind === 'allow_always') return 'primary';
  if (kind === 'reject_once' || kind === 'reject_always') return 'danger';
  return 'outline';
}

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

export interface PermissionGateProps {
  permission: RequestPermissionRequest | null;
  onRespond: (optionId: string) => void;
  /**
   * When provided, renders a maximize affordance that demotes the modal to a
   * full-size canvas tab (see ChatSessionTab). The request stays pending — the
   * caller suppresses the modal (passes `permission={null}`) while maximized and
   * shows a restore pill, so the gate is never silently lost.
   */
  onMaximize?: () => void;
}

/**
 * The permission gate — the signature ACP interaction. One layout, two looks
 * via Tailwind breakpoints (not two components): a centered dialog at `sm:`
 * and above, a bottom sheet with full-width stacked buttons below it (thumb
 * reachable at 375px). Keyboard mapping is by option KIND (see
 * lib/permissionKeymap.ts), not button position: y/a/n, Escape/backdrop to
 * the safest reject.
 *
 * Not built on the shared `Dialog` — that component hardcodes centered
 * desktop positioning on its outer wrapper with no responsive escape hatch —
 * but reimplements the same focus-trap / scroll-lock / Escape contract.
 */
export function PermissionGate({ permission, onRespond, onMaximize }: PermissionGateProps) {
  const { t } = useTranslation();
  const panelRef = useRef<HTMLDivElement>(null);
  const open = permission !== null;
  const options = useMemo(() => (permission ? orderedPermissionOptions(permission.options) : []), [permission]);
  const rejectOption = useMemo(() => (permission ? safestRejectOption(permission.options) : null), [permission]);

  const handleDismiss = useCallback(() => {
    // A permission gate shouldn't vanish unanswered: dismiss maps to the
    // safest reject when one was offered; otherwise it's a no-op and the
    // user must pick an option explicitly.
    if (rejectOption) onRespond(rejectOption.optionId);
  }, [rejectOption, onRespond]);

  useEffect(() => {
    if (!open) return;
    const previouslyFocused = document.activeElement as HTMLElement | null;
    panelRef.current?.focus();

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        handleDismiss();
        return;
      }
      const byKind = optionForKey(options, e.key);
      if (byKind) {
        e.preventDefault();
        onRespond(byKind.optionId);
        return;
      }
      if (e.key !== 'Tab') return;
      const panel = panelRef.current;
      if (!panel) return;
      const focusables = panel.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR);
      if (focusables.length === 0) {
        e.preventDefault();
        return;
      }
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      const active = document.activeElement;
      if (e.shiftKey) {
        if (active === first || active === panel) {
          e.preventDefault();
          last.focus();
        }
      } else if (active === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener('keydown', handleKeyDown, true);
    return () => {
      document.removeEventListener('keydown', handleKeyDown, true);
      previouslyFocused?.focus?.();
    };
  }, [open, options, onRespond, handleDismiss]);

  useEffect(() => {
    if (!open) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = previous;
    };
  }, [open]);

  if (!open || !permission) return null;

  const { toolCall } = permission;
  const diffs = (toolCall.content ?? []).filter(c => c.type === 'diff');

  return createPortal(
    <div className="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm" onClick={handleDismiss}>
      <div
        ref={panelRef}
        role="alertdialog"
        aria-modal="true"
        aria-label={t('acp_chat.permission_title')}
        tabIndex={-1}
        onClick={e => e.stopPropagation()}
        className="border-surface-200 bg-surface-50 dark:border-dark-surface-600 dark:bg-dark-surface-100 fixed inset-x-0 bottom-0 max-h-[85vh] overflow-auto rounded-t-2xl border-t p-4 shadow-xl focus:outline-none sm:top-1/2 sm:bottom-auto sm:left-1/2 sm:max-h-[80vh] sm:w-[520px] sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-2xl sm:border sm:p-6"
      >
        <div className="mb-4 flex items-start justify-between gap-2">
          <div className="min-w-0">
            <Span variant="status" className="text-text-muted dark:text-dark-text-muted">
              {toolCall.kind ?? t('acp_chat.permission_default_kind')}
            </Span>
            <p className="text-text dark:text-dark-text mt-1 text-sm font-medium">
              {toolCall.title ?? toolCall.toolCallId}
            </p>
          </div>
          {onMaximize && (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="shrink-0"
              aria-label={t('acp_chat.permission_maximize')}
              title={t('acp_chat.permission_maximize')}
              onClick={onMaximize}>
              <Maximize2 className="h-4 w-4" />
            </Button>
          )}
        </div>

        <div className="space-y-3">
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

        <div className="mt-5 flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:justify-end">
          {options.map(option => {
            const hint = keyHintForOption(option);
            return (
              <Button
                key={option.optionId}
                type="button"
                variant={optionVariant(option.kind)}
                className="w-full sm:w-auto"
                onClick={() => onRespond(option.optionId)}
              >
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
      </div>
    </div>,
    document.body,
  );
}
