import {
  parseSlashInvocation,
  type SlashCommandContext,
  type SlashCommandRegistry,
} from '../../../lib/slashCommands';

/** The composer surface a dispatched invocation drives. */
export type ComposerInvocationDeps = {
  registry: Pick<SlashCommandRegistry, 'get'>;
  /** Send the composer's current content as a normal (non-command) message. */
  onSend: () => void;
  /** Replace the composer text (the invocation's body remains after dispatch). */
  onChange: (v: string) => void;
  notify: SlashCommandContext['notify'];
  armArtifact: SlashCommandContext['armArtifact'];
};

/**
 * Dispatch `text` as a slash/@ invocation, falling through to `onSend` when it
 * is not one. `text` is an explicit parameter — never read back from component
 * state — so accepting a completion can execute the inserted text directly
 * instead of waiting for a re-render to propagate it into the `value` prop.
 */
export async function runComposerInvocation(
  text: string,
  deps: ComposerInvocationDeps,
): Promise<void> {
  const inv = parseSlashInvocation(text);
  if (!inv) {
    deps.onSend();
    return;
  }
  const cmd = deps.registry.get(inv.trigger, inv.name);
  if (!cmd) {
    deps.notify('error', `unknown ${inv.trigger}${inv.name} — try /help`);
    return;
  }
  const ctx: SlashCommandContext = {
    commandName: inv.name,
    rawArgs: inv.rawArgs,
    armArtifact: deps.armArtifact,
    notify: deps.notify,
  };
  try {
    await cmd.execute(ctx);
  } catch (err) {
    deps.notify(
      'error',
      `${inv.trigger}${inv.name}: ${err instanceof Error ? err.message : String(err)}`,
    );
    return;
  }
  deps.onChange(inv.body);
}
