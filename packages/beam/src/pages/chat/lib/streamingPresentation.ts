import type { AcpChatMessage } from '../../../hooks/acpSessionState';

/**
 * Pure predicates for the transcript's streaming-content presentation switch
 * (placeholder vs markdown vs trailing caret) — no React, so the logic is
 * unit-testable without mounting `TranscriptItems.tsx` (see
 * `streamingPresentation.test.ts`), and kept in its own module (matching
 * `configOptionMapping.ts`/`slashMenuState.ts`'s pattern in this directory)
 * instead of exported alongside the component, which would defeat React
 * Fast Refresh for that file.
 */

/** True once a turn has started for this message but no assistant text has arrived yet — the gap `ChatTranscriptStreamingPlaceholder` covers. */
export function shouldShowStreamingPlaceholder(message: AcpChatMessage): boolean {
  return !message.text && !!message.streaming;
}

/** True once assistant text has started arriving AND more chunks are still expected — gates the trailing `ChatStreamingCaret`. */
export function shouldShowStreamingCaret(message: AcpChatMessage): boolean {
  return !!message.text && !!message.streaming;
}
