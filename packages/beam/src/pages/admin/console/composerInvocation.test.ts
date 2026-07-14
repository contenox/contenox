import { describe, expect, it, vi } from 'vitest';
import { SlashCommandRegistry, type SlashCommand } from '../../../lib/slashCommands';
import { runComposerInvocation, type ComposerInvocationDeps } from './composerInvocation';

function makeDeps(registry: SlashCommandRegistry): ComposerInvocationDeps & {
  onSend: ReturnType<typeof vi.fn>;
  onChange: ReturnType<typeof vi.fn>;
  notify: ReturnType<typeof vi.fn>;
  armArtifact: ReturnType<typeof vi.fn>;
} {
  return {
    registry,
    onSend: vi.fn(),
    onChange: vi.fn(),
    notify: vi.fn(),
    armArtifact: vi.fn(),
  };
}

describe('runComposerInvocation', () => {
  it('executes the command parsed from the given text, not from composer state', async () => {
    // Regression: clicking a /chain arg completion used to defer execution
    // behind a re-render and parse the stale "/chain " (empty rawArgs, the
    // listing branch) instead of the accepted "/chain <name>".
    const registry = new SlashCommandRegistry();
    const seenArgs: string[] = [];
    const chain: SlashCommand = {
      name: 'chain',
      description: 'select chain',
      execute: async ctx => {
        seenArgs.push(ctx.rawArgs);
      },
    };
    registry.register(chain);
    const deps = makeDeps(registry);

    await runComposerInvocation('/chain default-chain.json', deps);

    expect(seenArgs).toEqual(['default-chain.json']);
    expect(deps.onSend).not.toHaveBeenCalled();
    // The invocation body (empty here) replaces the composer text after dispatch.
    expect(deps.onChange).toHaveBeenCalledWith('');
  });

  it('falls through to onSend for non-command text', async () => {
    const deps = makeDeps(new SlashCommandRegistry());

    await runComposerInvocation('what is 2+2?', deps);

    expect(deps.onSend).toHaveBeenCalledTimes(1);
    expect(deps.onChange).not.toHaveBeenCalled();
    expect(deps.notify).not.toHaveBeenCalled();
  });

  it('notifies about unknown commands without sending or clearing', async () => {
    const deps = makeDeps(new SlashCommandRegistry());

    await runComposerInvocation('/nope', deps);

    expect(deps.notify).toHaveBeenCalledWith('error', 'unknown /nope — try /help');
    expect(deps.onSend).not.toHaveBeenCalled();
    expect(deps.onChange).not.toHaveBeenCalled();
  });

  it('surfaces execute failures via notify and keeps the composer text', async () => {
    const registry = new SlashCommandRegistry();
    registry.register({
      name: 'boom',
      description: 'always fails',
      execute: () => {
        throw new Error('no such chain');
      },
    });
    const deps = makeDeps(registry);

    await runComposerInvocation('/boom now', deps);

    expect(deps.notify).toHaveBeenCalledWith('error', '/boom: no such chain');
    expect(deps.onChange).not.toHaveBeenCalled();
  });
});
