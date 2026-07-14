import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { TerminalPromptInput } from '@contenox/ui';
import {
  parseSlashInvocation,
  useSlashCommandRegistry,
  useSlashCommands,
  type SlashCommandContext,
} from '../../../../lib/slashCommands';
import { useArtifactRegistry, type ArtifactSource } from '../../../../lib/artifacts';
import type { ChatContextArtifact } from '../../../../lib/types';
import { TERM } from '../term';

type ConsoleComposerProps = {
  value: string;
  onChange: (v: string) => void;
  onSend: () => void;
  disabled?: boolean;
  hint?: string;
};

type Notice = { level: 'info' | 'error'; message: string } | null;

/** A row in the completion list. `execute` = accepting it runs the command. */
type Suggestion = { insert: string; label: string; hint?: string; execute: boolean };

/**
 * The console composer: a @contenox/ui TerminalPromptInput plus slash/mention
 * dispatch and a completion list that is keyboard-navigable (↑/↓/Tab/Enter)
 * AND clickable — the browser's affordance, not a real terminal's limitation.
 */
export function ConsoleComposer({ value, onChange, onSend, disabled, hint }: ConsoleComposerProps) {
  const registry = useSlashCommandRegistry();
  const commands = useSlashCommands();
  const artifacts = useArtifactRegistry();
  const [notice, setNotice] = useState<Notice>(null);
  const [selected, setSelected] = useState(0);
  const [dismissed, setDismissed] = useState(false);
  const taRef = useRef<HTMLTextAreaElement>(null);
  const armedUnregisters = useRef<Map<string, () => void>>(new Map());

  // Completion list: command names, or a command's argument values (chains).
  const suggestions = useMemo<Suggestion[]>(() => {
    if (dismissed) return [];
    const line = value.split('\n')[0] ?? '';

    // Arg mode: command name complete (space after it, or exact match).
    const full = /^([/@])([a-zA-Z][a-zA-Z0-9_-]*)(\s+(.*))?$/.exec(line);
    if (full) {
      const [, trigger, name, space, partial = ''] = full;
      const cmd = registry.get(trigger as '/' | '@', name.toLowerCase());
      const complete = space !== undefined || cmd?.name === name.toLowerCase();
      if (cmd?.argCompletions && complete) {
        return cmd
          .argCompletions(partial)
          .slice(0, 8)
          .map(c => ({
            insert: `${trigger}${cmd.name} ${c.value}`,
            label: c.value,
            hint: c.hint,
            execute: true,
          }));
      }
    }

    // Command mode: a bare "/" or a partial command token.
    const cmdMatch = /^([/@])([a-zA-Z0-9_-]*)$/.exec(line);
    if (cmdMatch) {
      const [, trigger, prefix] = cmdMatch;
      return commands
        .filter(c => (c.trigger ?? '/') === trigger && c.name.startsWith(prefix.toLowerCase()))
        .slice(0, 8)
        .map(c => ({
          insert: `${c.trigger ?? '/'}${c.name} `,
          label: `${c.trigger ?? '/'}${c.name}`,
          hint: c.description,
          execute: false,
        }));
    }
    return [];
  }, [value, commands, registry, dismissed]);

  const sugKey = suggestions.map(s => s.label).join('|');
  useEffect(() => setSelected(0), [sugKey]);

  const armArtifact = (sourceId: string, label: string, artifact: ChatContextArtifact) => {
    armedUnregisters.current.get(sourceId)?.();
    armedUnregisters.current.delete(sourceId);
    const source: ArtifactSource = {
      id: sourceId,
      label,
      collect: () => {
        queueMicrotask(() => {
          armedUnregisters.current.get(sourceId)?.();
          armedUnregisters.current.delete(sourceId);
        });
        return artifact;
      },
    };
    armedUnregisters.current.set(sourceId, artifacts.register(source));
  };

  const runInvocation = async () => {
    const inv = parseSlashInvocation(value);
    if (!inv) {
      onSend();
      return;
    }
    const cmd = registry.get(inv.trigger, inv.name);
    if (!cmd) {
      setNotice({ level: 'error', message: `unknown ${inv.trigger}${inv.name} — try /help` });
      return;
    }
    const ctx: SlashCommandContext = {
      commandName: inv.name,
      rawArgs: inv.rawArgs,
      armArtifact,
      notify: (level, message) => setNotice({ level, message }),
    };
    try {
      await cmd.execute(ctx);
    } catch (err) {
      setNotice({ level: 'error', message: `${inv.trigger}${inv.name}: ${err instanceof Error ? err.message : String(err)}` });
      return;
    }
    onChange(inv.body);
  };

  const accept = (s: Suggestion) => {
    onChange(s.insert);
    setDismissed(false);
    taRef.current?.focus();
    if (s.execute) {
      // Run on the next tick so `value` reflects the inserted text.
      setTimeout(() => void runInvocation(), 0);
    }
  };

  const submit = () => {
    if (disabled) return;
    if (suggestions.length > 0) {
      accept(suggestions[Math.min(selected, suggestions.length - 1)]);
      return;
    }
    void runInvocation();
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (suggestions.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setSelected(i => (i + 1) % suggestions.length);
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setSelected(i => (i - 1 + suggestions.length) % suggestions.length);
        return;
      }
      if (e.key === 'Tab') {
        e.preventDefault();
        accept(suggestions[Math.min(selected, suggestions.length - 1)]);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        setDismissed(true);
        return;
      }
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      submit();
    }
  };

  return (
    <div className={`${TERM.surface} shrink-0 border-t ${TERM.border} px-3 py-1.5 font-mono`}>
      {notice && (
        <div className={`pb-1 ${TERM.small} ${notice.level === 'error' ? TERM.err : TERM.dim}`}>{notice.message}</div>
      )}

      {suggestions.length > 0 && (
        <div className="pb-1">
          {suggestions.map((s, i) => (
            <button
              key={s.label}
              type="button"
              onMouseDown={e => {
                e.preventDefault();
                accept(s);
              }}
              onMouseEnter={() => setSelected(i)}
              className={`flex w-full items-baseline gap-2 px-1 text-left ${TERM.small} ${
                i === selected ? `${TERM.text} bg-success/10` : TERM.dim
              }`}>
              <span className={i === selected ? TERM.accent : TERM.text}>{s.label}</span>
              {s.hint && <span className={`${TERM.dim} truncate`}>{s.hint}</span>}
            </button>
          ))}
        </div>
      )}

      <TerminalPromptInput
        ref={taRef}
        value={value}
        onChange={v => {
          onChange(v);
          setDismissed(false);
          if (notice) setNotice(null);
        }}
        onKeyDown={onKeyDown}
        disabled={disabled}
        hint={hint}
      />
    </div>
  );
}
