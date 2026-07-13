import { useMemo } from 'react';
import { useSlashCommand } from '../../../lib/slashCommands/registry';
import type { SlashCommand } from '../../../lib/slashCommands/types';

type ConsoleCommandDeps = {
  chainPaths: string[];
  selectedChainId: string;
  setSelectedChainId: (id: string) => void;
  policyNames: string[];
  activePolicyName: string;
  setActivePolicy: (name: string) => void;
  newSession: () => void;
};

/**
 * Console action commands: the slash-command counterparts of the interim
 * status-line selectors. When these prove parity, the selectors retire.
 */
export function useConsoleCommands(deps: ConsoleCommandDeps): void {
  const chainCommand = useMemo<SlashCommand>(
    () => ({
      trigger: '/',
      name: 'chain',
      description: 'Show or switch the task chain for this session.',
      usage: '/chain [name]',
      execute: ctx => {
        const arg = ctx.rawArgs.trim();
        if (!arg) {
          const listing = deps.chainPaths
            .map(p => (p === deps.selectedChainId ? `${p} (current)` : p))
            .join(', ');
          ctx.notify('info', listing ? `Chains: ${listing}` : 'No chains available.');
          return;
        }
        const match =
          deps.chainPaths.find(p => p === arg) ??
          deps.chainPaths.find(p => p.startsWith(arg)) ??
          deps.chainPaths.find(p => p.includes(arg));
        if (!match) {
          throw new Error(`No chain matches "${arg}". Try /chain to list.`);
        }
        deps.setSelectedChainId(match);
        ctx.notify('info', `Chain set to ${match}.`);
      },
    }),
    [deps],
  );

  const policyCommand = useMemo<SlashCommand>(
    () => ({
      trigger: '/',
      name: 'policy',
      description: 'Show or switch the HITL approval policy.',
      usage: '/policy [name|none]',
      execute: ctx => {
        const arg = ctx.rawArgs.trim();
        if (!arg) {
          const listing = deps.policyNames
            .map(p => (p === deps.activePolicyName ? `${p} (active)` : p))
            .join(', ');
          ctx.notify('info', listing ? `Policies: ${listing}` : 'No policies available.');
          return;
        }
        if (arg === 'none') {
          deps.setActivePolicy('');
          ctx.notify('info', 'Policy cleared.');
          return;
        }
        const match =
          deps.policyNames.find(p => p === arg) ??
          deps.policyNames.find(p => p.startsWith(arg)) ??
          deps.policyNames.find(p => p.includes(arg));
        if (!match) {
          throw new Error(`No policy matches "${arg}". Try /policy to list.`);
        }
        deps.setActivePolicy(match);
        ctx.notify('info', `Policy set to ${match}.`);
      },
    }),
    [deps],
  );

  const newCommand = useMemo<SlashCommand>(
    () => ({
      trigger: '/',
      name: 'new',
      description: 'Start a new console session.',
      usage: '/new',
      execute: () => {
        deps.newSession();
      },
    }),
    [deps],
  );

  useSlashCommand(chainCommand);
  useSlashCommand(policyCommand);
  useSlashCommand(newCommand);
}
