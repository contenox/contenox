import { describe, expect, it } from 'vitest';
import type { AcpChatMessage } from '../../../hooks/acpSessionState';
import { shouldShowStreamingCaret, shouldShowStreamingPlaceholder } from './streamingPresentation';

function message(patch: Partial<AcpChatMessage>): AcpChatMessage {
  return { id: 'm1', role: 'assistant', text: '', ...patch };
}

describe('streamingPresentation: placeholder/caret switch', () => {
  it('shows the placeholder once a turn has started but no text has arrived yet', () => {
    expect(shouldShowStreamingPlaceholder(message({ text: '', streaming: true }))).toBe(true);
  });

  it('does not show the placeholder once text has started arriving, even mid-stream', () => {
    expect(shouldShowStreamingPlaceholder(message({ text: 'Sure, ', streaming: true }))).toBe(false);
  });

  it('does not show the placeholder for a settled empty message (no active turn)', () => {
    expect(shouldShowStreamingPlaceholder(message({ text: '', streaming: false }))).toBe(false);
    expect(shouldShowStreamingPlaceholder(message({ text: '' }))).toBe(false);
  });

  it('shows the caret only once text has arrived AND more chunks are expected', () => {
    expect(shouldShowStreamingCaret(message({ text: 'Sure, here', streaming: true }))).toBe(true);
  });

  it('hides the caret once the turn has ended (markdown renders as settled)', () => {
    expect(shouldShowStreamingCaret(message({ text: 'Sure, here you go.', streaming: false }))).toBe(false);
  });

  it('hides the caret while no text has arrived yet (placeholder owns that state instead)', () => {
    expect(shouldShowStreamingCaret(message({ text: '', streaming: true }))).toBe(false);
  });
});
