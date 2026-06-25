import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../lib/api';
import { setupKeys } from '../lib/queryKeys';
import { CLIConfigUpdateRequest, CLIConfigUpdateResponse } from '../lib/types';

export function usePutCLIConfig() {
  const queryClient = useQueryClient();
  return useMutation<
    CLIConfigUpdateResponse,
    Error,
    CLIConfigUpdateRequest
  >({
    mutationFn: body => api.putCLIConfig(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: setupKeys.status() });
    },
  });
}
