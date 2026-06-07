import type { ChatMessage } from '../../../lib/types';

export type ChatThreadItem =
  | { kind: 'message'; message: ChatMessage };

export function buildChatThreadItems(options: {
  displayHistory: ChatMessage[];
}): ChatThreadItem[] {
  const { displayHistory } = options;
  return displayHistory.map(m => ({ kind: 'message', message: m }));
}
