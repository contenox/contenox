import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../lib/api';
import { setupKeys } from '../lib/queryKeys';
import { ShellEnvResponse, ShellEnvUpdateRequest } from '../lib/types';

export function usePutShellEnv() {
  const queryClient = useQueryClient();
  return useMutation<ShellEnvResponse, Error, ShellEnvUpdateRequest>({
    mutationFn: body => api.putShellEnv(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: setupKeys.shellEnv() });
    },
  });
}
