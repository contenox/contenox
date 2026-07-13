import {
  Button,
  ChatMessage as ChatMessageUI,
  ChatStreamThinkingBox,
  ChatStreamingCaret,
  ChatTranscriptStreamingPlaceholder,
  chatTranscriptMarkdownComponents,
  ExecutionTimeline,
  InlineAttachments,
  ToolCallCard,
} from '@contenox/ui';
import { Terminal } from 'lucide-react';
import { t } from 'i18next';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { ChatMessage as ChatMessageModel } from '../../../../lib/types';

type ChatMessageProps = {
  message: ChatMessageModel;
  isLatest?: boolean;
  /** Task-event reasoning stream, shown only for the live assistant row. */
  streamThinking?: string;
  toolName?: string;
};

export const ChatMessage = ({ message, isLatest = false, streamThinking, toolName }: ChatMessageProps) => {
  const isUser = message.role === 'user';
  const isSystem = message.role === 'system';
  const isTool = message.role === 'tool';
  const isAssistant = !isUser && !isSystem && !isTool;

  // Hide intermediate assistant turns that only have tool calls and no content
  if (isAssistant && !message.content && !message.streaming && !message.error && !streamThinking) {
    return null;
  }

  if (isTool) {
    return (
      <ToolCallCard
        tool={toolName ?? "Tool Result"}
        title={message.toolCallId ? `ID: ${message.toolCallId}` : "Execution Result"}
        duration={new Date(message.sentAt).toLocaleTimeString()}
        status={message.error ? "error" : "success"}
        icon={<Terminal className="h-4 w-4" />}
        detail={
          <div className="flex flex-col gap-2">
            {message.content && (
              <ReactMarkdown remarkPlugins={[remarkGfm]} components={chatTranscriptMarkdownComponents}>
                {message.content}
              </ReactMarkdown>
            )}
            {message.error && (
              <div className="text-error dark:text-dark-error text-sm">{message.error}</div>
            )}
            <InlineAttachments attachments={message.attachments} />
            <ExecutionTimeline events={message.events} state={message.state} />
          </div>
        }
      />
    );
  }

  const roleLabel =
    isUser
      ? t('chat.role_user')
      : isSystem
        ? t('chat.role_system')
        : t('chat.role_assistant');

  return (
    <ChatMessageUI
      appearance="transcript"
      role={message.role}
      roleLabel={roleLabel}
      timestamp={new Date(message.sentAt).toLocaleTimeString()}
      timestampTooltip={new Date(message.sentAt).toLocaleString()}
      isLatest={isLatest}
      latestLabel={isLatest ? t('chat.latest') : undefined}
      defaultOpen={!(isSystem || isTool)}
      copyText={message.content}
      copyLabel={t('chat.copy')}
      copiedLabel={t('chat.copied', 'Copied!')}
      collapseToggleLabel={{
        open: t('chat.collapse', 'Hide'),
        closed: t('chat.expand', 'Show'),
      }}
      error={message.error}
      secondaryActions={
        <Button variant="ghost" size="sm" className="h-6 text-xs" type="button">
          {t('chat.share')}
        </Button>
      }
    >
      {message.streaming && streamThinking ? (
        <ChatStreamThinkingBox>{streamThinking}</ChatStreamThinkingBox>
      ) : null}
      {message.streaming && !message.content && !message.error && !streamThinking ? (
        <ChatTranscriptStreamingPlaceholder>
          {t('chat.streaming_placeholder')}
        </ChatTranscriptStreamingPlaceholder>
      ) : null}
      {message.content ? (
        <ReactMarkdown remarkPlugins={[remarkGfm]} components={chatTranscriptMarkdownComponents}>
          {message.content}
        </ReactMarkdown>
      ) : null}
      {message.streaming && message.content && !message.error ? <ChatStreamingCaret /> : null}
      <InlineAttachments attachments={message.attachments} />
      <ExecutionTimeline events={message.events} state={message.state} />
    </ChatMessageUI>
  );
};
