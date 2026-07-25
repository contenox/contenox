import { Button, InlineNotice, P, Textarea } from '@contenox/ui';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useReplyToAsk } from '../../../hooks/useApprovals';
import type { MissionAsk } from '../../../lib/acp';

/**
 * The answer box under a mission unit's QUESTION, right in the transcript where
 * the question arrived.
 *
 * It exists because the alternative was a dead end that looked like a
 * conversation: the unit's question was delivered into this session, the operator
 * read it here, and then had to go find the inbox to reply — while the unit sat
 * parked on the tool call that asked. Answering where you are told is the whole
 * point; the inbox remains the queue for questions whose session nobody has open.
 *
 * The same mutation the inbox uses, so an answer given here and one given there
 * are the same act on the same durable ask.
 */
export function MissionAskCard({ ask }: { ask: MissionAsk }) {
  const { t } = useTranslation();
  const reply = useReplyToAsk();
  const [text, setText] = useState('');

  if (reply.isSuccess) {
    return (
      <InlineNotice variant="info" className="mt-3 rounded-lg">
        {t('acp_chat.mission_ask_answered')}
      </InlineNotice>
    );
  }

  return (
    <form
      className="border-warning-300 dark:border-warning-800 mt-3 flex flex-col gap-2 rounded-lg border p-3"
      onSubmit={e => {
        e.preventDefault();
        const answer = text.trim();
        if (!answer) return;
        reply.mutate({ id: ask.askId, answer });
      }}>
      <P variant="muted" className="text-xs">
        {ask.agentName
          ? t('acp_chat.mission_ask_hint_named', { agent: ask.agentName })
          : t('acp_chat.mission_ask_hint')}
      </P>
      <Textarea
        value={text}
        onChange={e => setText(e.target.value)}
        rows={2}
        placeholder={t('acp_chat.mission_ask_placeholder')}
        aria-label={t('acp_chat.mission_ask_aria', { question: ask.summary })}
        disabled={reply.isPending}
      />
      {reply.isError && <P className="text-error text-xs">{reply.error.message}</P>}
      <div className="flex justify-end">
        <Button
          type="submit"
          variant="primary"
          size="sm"
          isLoading={reply.isPending}
          disabled={reply.isPending || text.trim() === ''}>
          {t('acp_chat.mission_ask_send')}
        </Button>
      </div>
    </form>
  );
}
