import {
  ChatDateSeparator,
  ChatProcessingBar,
  ChatScrollToLatest,
  ChatThread,
  ChatThreadSkeleton,
  EmptyState,
  InlineNotice,
  useChatScroll,
} from '@contenox/ui';
import { t } from 'i18next';
import type { ReactNode } from 'react';
import { memo } from 'react';
import { Sparkles } from 'lucide-react';
import type { ChatThreadItem } from '../chatThreadItems';
import { ChatMessage } from './ChatMessage';

function formatDateLabel(date: Date): string {
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const target = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const diffDays = Math.round((today.getTime() - target.getTime()) / 86400000);

  if (diffDays === 0) return t('chat.date_today', 'Today');
  if (diffDays === 1) return t('chat.date_yesterday', 'Yesterday');
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' });
}

function dateKey(iso: string): string {
  return iso.slice(0, 10);
}



export type ChatInterfaceProps = {
  threadItems: ChatThreadItem[];
  isLoading: boolean;
  error: Error | null;
  isProcessing?: boolean;
  processingBarLabel?: string;
  embedStreamThinkingInThread?: boolean;
  liveThinking?: string;
  canStop?: boolean;
  onStop?: () => void;
  streamScrollSignature?: string;
  liveStatus?: string;
  approvalContent?: ReactNode;
};

export const ChatInterface = memo(({
  threadItems,
  isLoading,
  error,
  isProcessing = false,
  processingBarLabel,
  embedStreamThinkingInThread = false,
  liveThinking,
  liveStatus,
  canStop = false,
  onStop,
  streamScrollSignature = '',

  approvalContent,
}: ChatInterfaceProps) => {
  const { containerRef, endRef, scrollToEnd, isNearBottom } = useChatScroll({
    deps: [threadItems, streamScrollSignature],
  });

  if (isLoading) {
    return <ChatThreadSkeleton />;
  }

  if (error) {
    return (
      <EmptyState
        title={t('chat.error_loading_messages')}
        description={error.message || t('chat.error_history')}
        icon="❌"
        variant="error"
        className="h-full"
      />
    );
  }

  const barLabel =
    processingBarLabel ?? (isProcessing ? liveStatus || t('chat.thinking') : '');



  let lastMessageIndex = -1;
  const toolNames = new Map<string, string>();
  for (let k = 0; k < threadItems.length; k++) {
    const item = threadItems[k];
    if (item.kind === 'message') {
      lastMessageIndex = k;
      if (item.message.callTools) {
        for (const call of item.message.callTools) {
          if (call.id && call.function?.name) {
            toolNames.set(call.id, call.function.name);
          }
        }
      }
    }
  }

  const threadBody = (
    <div className="relative min-h-0 flex-1">
      <ChatThread
        containerRef={containerRef}
        endRef={endRef}
        className="h-full"
        scrollClassName="flex-1 space-y-4 overflow-auto px-4 py-4 sm:px-5">
        <div className="mx-auto flex w-full max-w-4xl flex-col space-y-4 pb-6 pt-2">
          {!threadItems.length ? (
            <div className="flex flex-1 flex-col items-center justify-center py-20">
              <EmptyState
                title="Ready to execute"
                description="Select a toolchain from the top menu to define the agent's capabilities. Type your instruction below to begin."
                icon={<Sparkles className="h-8 w-8 opacity-80 shadow-primary/20 drop-shadow-md" />}
                iconSize="lg"
                orientation="vertical"
                className="bg-surface-200/50 dark:bg-dark-surface-300/50 ring-surface-300 dark:ring-dark-surface-400 max-w-lg shadow-sm ring-1"
              />
            </div>
          ) : (
            threadItems.map((item, index) => {
              const message = item.message;
              let prevMessageSentAt: string | undefined;
              for (let j = index - 1; j >= 0; j--) {
                const it = threadItems[j];
                if (it.kind === 'message') {
                  prevMessageSentAt = it.message.sentAt;
                  break;
                }
              }
              const prevDate = prevMessageSentAt != null ? dateKey(prevMessageSentAt) : null;
              const curDate = dateKey(message.sentAt);
              const showSeparator = prevDate == null || curDate !== prevDate;
              const isLatest = index === lastMessageIndex;

              return (
                <div
                  key={message.id ?? `${message.sentAt}-${index}`}
                  className="animate-in fade-in-0 duration-150 space-y-4">
                  {showSeparator && (
                    <ChatDateSeparator label={formatDateLabel(new Date(message.sentAt))} />
                  )}
                  <ChatMessage
                    message={message}
                    isLatest={isLatest}
                    streamThinking={message.streaming ? liveThinking : undefined}
                    toolName={message.toolCallId ? toolNames.get(message.toolCallId) : undefined}
                  />
                </div>
              );
            })
          )}
          {approvalContent && (
            <div className="animate-in fade-in-0 duration-150 pb-2">{approvalContent}</div>
          )}
        </div>
      </ChatThread>
      <ChatScrollToLatest
        visible={!isNearBottom}
        onClick={scrollToEnd}
        label={t('chat.scroll_to_latest')}
      />
    </div>
  );
  return (
    <div className="flex h-full min-h-0 flex-col">
      {isProcessing && (
        <>
          <ChatProcessingBar
            label={barLabel}
            onStop={canStop ? onStop : undefined}
            stopLabel={t('chat.stop')}
          />
          {liveThinking && !embedStreamThinkingInThread && (
            <InlineNotice variant="info" className="mx-4 mt-3">
              {liveThinking}
            </InlineNotice>
          )}
        </>
      )}
      {threadBody}
    </div>
  );
});
