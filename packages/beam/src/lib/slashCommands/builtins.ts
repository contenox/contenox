import { type SlashCommand, type SlashCommandContext } from './types';
import type { SlashCommandRegistry } from './registry';

/**
 * `/help` — lists every registered command with usage + description. Uses
 * `notify('info', ...)` so the composer's footer displays it until the user
 * starts typing again. Built lazily against the live registry so commands
 * added after mount are discoverable.
 */
export function createHelpCommand(registry: SlashCommandRegistry): SlashCommand {
  return {
    trigger: '/',
    name: 'help',
    description: 'List every command and @-mention available in this session.',
    usage: '/help',
    execute: (ctx: SlashCommandContext) => {
      const cmds = registry.list();
      if (cmds.length === 0) {
        ctx.notify('info', 'No commands registered.');
        return;
      }
      const lines = cmds.map((c) => {
        const trigger = c.trigger ?? '/';
        const usage = c.usage ?? `${trigger}${c.name}`;
        return `${usage} — ${c.description}`;
      });
      ctx.notify('info', `Available:\n${lines.join('\n')}`);
    },
  };
}

