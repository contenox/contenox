import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';
import { setupKeys } from '../lib/queryKeys';

/**
 * Global shell environment variables (GET /api/shell-env) contenox injects
 * into the shells it spawns (local_shell, terminal). Independent from the CLI
 * config snapshot: these are plain config strings applied on top of the
 * environment scrub, not defaults for chat/task-chain execution.
 */
export function useShellEnv(enabled: boolean) {
  return useQuery({
    queryKey: setupKeys.shellEnv(),
    queryFn: api.getShellEnv,
    enabled,
    staleTime: 30_000,
  });
}
