import { Button, Span, Spinner } from '@contenox/ui';
import { t } from 'i18next';
import { MessageSquarePlus } from 'lucide-react';
import { Link, useMatch, useNavigate } from 'react-router-dom';
import { useChats, useCreateChat } from '../../hooks/useChats';
import { ChatSession } from '../../lib/types';

const getPreviewText = (content: string): string => {
  const trimmed = content.trim();
  if (trimmed.length === 0) return '';
  if (trimmed.length <= 60) return trimmed;
  return trimmed.slice(0, 57) + '…';
};

// Sessions created without an explicit name get an auto-generated "session-<8hex>"
// placeholder (see sessionservice.go) or a transient "New session" name from the UI.
// We treat these as unnamed so the sidebar falls back to the first user message (subject),
// last message preview, model name, or "Untitled chat".
const isPlaceholderName = (name: string): boolean => {
	const n = name.trim();
	if (/^session-[0-9a-f]{8}$/i.test(n)) return true;
	const lower = n.toLowerCase();
	if (lower === 'new session' || lower === 'new chat' || lower === 'untitled' || lower === 'untitled chat') {
		return true;
	}
	// Numbered variants like "New session (2)" before the server has renamed them.
	if (/^new (session|chat) \(\d+\)$/i.test(n)) return true;
	return false;
};

export function ChatSessionSidebar({ setIsOpen }: { setIsOpen: (open: boolean) => void }) {
  const navigate = useNavigate();
  const chatMatch = useMatch('/chat/:chatId');
  const activeChatId = chatMatch?.params.chatId;
  const createChatMutation = useCreateChat();
  const { data: chats, isLoading, error } = useChats();

  const handleNewChat = () => {
    createChatMutation.mutate(
      {},
      {
        onSuccess: (data: Partial<ChatSession>) => {
          if (data?.id) {
            navigate(`/chat/${data.id}`);
            setIsOpen(false);
          }
        },
      },
    );
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="border-surface-300 dark:border-dark-surface-700 shrink-0 border-b p-3">
        <Button
          variant="primary"
          size="sm"
          className="w-full gap-2"
          disabled={createChatMutation.isPending}
          onClick={handleNewChat}>
          {createChatMutation.isPending ? (
            <Spinner size="sm" />
          ) : (
            <MessageSquarePlus className="h-4 w-4 shrink-0" aria-hidden />
          )}
          <span className="truncate">{t('chat.start_new_chat')}</span>
        </Button>
      </div>
      <nav
        className="min-h-0 flex-1 space-y-1 overflow-y-auto p-3"
        aria-label={t('chat.personal_chat_list_title')}>
        {isLoading ? (
          <div className="flex items-center justify-center gap-2 py-8">
            <Spinner size="md" />
            <Span className="text-text-muted text-sm">{t('chat.loading_chats')}</Span>
          </div>
        ) : error ? (
          <Span className="text-error text-sm">{error.message || t('chat.list_error')}</Span>
        ) : !chats?.length ? (
          <Span className="text-text-muted text-sm">{t('chat.sidebar_empty_hint')}</Span>
        ) : (
          chats.map(chat => {
            const isActive = activeChatId === chat.id;
            // Title precedence: a user-assigned name (most intentional) → the user's
            // first message (what the chat is about) → the last message → "Untitled chat".
            // Auto-generated "session-<hex>" placeholders and the raw id carry no meaning.
            const name =
              chat.name && !isPlaceholderName(chat.name) ? chat.name.trim() : null;
            const subject = chat.subject ? getPreviewText(chat.subject) : null;
            const lastPreview = chat.lastMessage?.content
              ? getPreviewText(chat.lastMessage.content)
              : null;
            const shortId = chat.id.slice(0, 8);
            const displayText =
              name || subject || lastPreview || chat.model || t('chat.untitled_session', 'Untitled chat');
            // Always carry the short id as a disambiguator, except when it is already
            // the title itself.
            const subtitle = displayText === shortId ? null : shortId;
            return (
              <Link
                key={chat.id}
                to={`/chat/${chat.id}`}
                onClick={() => setIsOpen(false)}
                className={`block rounded-lg border p-4 transition-colors duration-150 ${
                  isActive
                    ? 'bg-surface-200 dark:bg-dark-surface-200 border-surface-400 dark:border-dark-surface-600'
                    : 'bg-surface-100 dark:bg-dark-surface-100 border-surface-200 dark:border-dark-surface-700 hover:bg-surface-200 dark:hover:bg-dark-surface-200'
                }`}>
                <Span className="text-text dark:text-dark-text line-clamp-2 text-xs">
                  {displayText}
                </Span>
                {subtitle && (
                  <Span className="text-text-muted dark:text-dark-text-muted mt-1 block text-xs">
                    {subtitle}
                  </Span>
                )}
              </Link>
            );
          })
        )}
      </nav>
    </div>
  );
}
