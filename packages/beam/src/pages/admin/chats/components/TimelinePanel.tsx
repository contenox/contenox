import {
  Button,
  ExecutionTimeline,
  Span,
} from '@contenox/ui';
import { t } from 'i18next';
import { PanelLeftClose, PanelLeftOpen } from 'lucide-react';
import type { TaskEvent } from '../../../../lib/types';

interface TimelinePanelProps {
  open: boolean;
  onToggle: () => void;
  events: TaskEvent[];
  isProcessing: boolean;
  selectedEvent?: TaskEvent | null;
  onSelectEvent?: (event: TaskEvent | null) => void;
}

export function TimelinePanel({
  open,
  onToggle,
  events,
  isProcessing,
  selectedEvent,
  onSelectEvent,
}: TimelinePanelProps) {
  const count = events.length;

  if (!open) {
    return (
      <button
        type="button"
        onClick={onToggle}
        className="border-surface-300 dark:border-dark-surface-400 bg-surface-50 dark:bg-dark-surface-100 text-text-muted hover:text-text dark:hover:text-dark-text flex h-full w-8 flex-col items-center justify-start border-r py-2 text-xs"
        aria-label={t('chat.show_timeline')}
      >
        <PanelLeftOpen className="h-4 w-4" />
        {count > 0 && (
          <span className="mt-1 rotate-90 text-[10px] font-mono tracking-[1px]">
            {count}
          </span>
        )}
      </button>
    );
  }

  const handleEventClick = (ev: TaskEvent) => {
    if (onSelectEvent) {
      // Toggle selection
      if (selectedEvent && selectedEvent.timestamp === ev.timestamp && selectedEvent.kind === ev.kind) {
        onSelectEvent(null);
      } else {
        onSelectEvent(ev);
      }
    }
  };

  return (
    <div className="flex min-h-0 flex-col overflow-hidden border-b border-surface-300 dark:border-dark-surface-400 bg-surface-50 dark:bg-dark-surface-100">
      <div className="flex items-center justify-between border-b border-surface-200 px-2 py-1 dark:border-dark-surface-300">
        <div className="flex min-w-0 items-center gap-2">
          <Span className="text-text dark:text-dark-text truncate text-sm font-medium">
            {t('chat.execution_timeline', 'Timeline')}
          </Span>
          {count > 0 && (
            <span className="rounded bg-surface-200 px-1.5 py-0.5 font-mono text-[10px] dark:bg-dark-surface-300">
              {count}
            </span>
          )}
          {isProcessing && <span className="text-success ml-1 text-xs">● live</span>}
        </div>
        {onToggle && (
          <Button type="button" variant="ghost" size="sm" onClick={onToggle} aria-label={t('chat.hide_timeline')}>
            <PanelLeftClose className="h-4 w-4" />
          </Button>
        )}
      </div>

      <div className="min-h-0 flex-1 overflow-auto p-2 text-xs">
        {events.length > 0 ? (
          <div
            className="cursor-pointer"
            onClick={() => {
              if (onSelectEvent) onSelectEvent(null);
            }}
          >
            <ExecutionTimeline events={events} />
            <div className="mt-2 space-y-1 border-t border-surface-300 pt-2 dark:border-dark-surface-400">
              <Span variant="muted" className="mb-1 block px-1 text-xs font-medium">
                Steps
              </Span>
              {events.map((ev, idx) => {
                const isSelected =
                  selectedEvent &&
                  selectedEvent.timestamp === ev.timestamp &&
                  selectedEvent.kind === ev.kind;
                const label = `${ev.kind}${ev.task_handler ? ` · ${ev.task_handler}` : ''}`;
                return (
                  <button
                    key={`${ev.timestamp}-${idx}`}
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      handleEventClick(ev);
                    }}
                    className={`w-full rounded px-2 py-1 text-left text-xs font-mono transition ${
                      isSelected
                        ? 'bg-primary-100 text-primary-800 dark:bg-primary-900/40 dark:text-primary-200'
                        : 'hover:bg-surface-100 dark:hover:bg-dark-surface-200'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="truncate">{label}</span>
                      {ev.error && <span className="text-error">!</span>}
                    </div>
                    {ev.content && (
                      <div className="mt-0.5 line-clamp-2 text-[10px] text-text-muted opacity-70">
                        {ev.content.slice(0, 120)}
                      </div>
                    )}
                  </button>
                );
              })}
            </div>
          </div>
        ) : (
          <div className="text-text-muted p-3 text-xs">
            {isProcessing
              ? t('chat.timeline_waiting', 'Waiting for execution events…')
              : t('chat.timeline_empty', 'Run a chain to see the execution timeline here.')}
          </div>
        )}
      </div>
    </div>
  );
}
