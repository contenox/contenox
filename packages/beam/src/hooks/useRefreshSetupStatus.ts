import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../lib/api';
import { setupKeys } from '../lib/queryKeys';
import type { SetupStatus } from '../lib/types';

export function useRefreshSetupStatus() {
  const queryClient = useQueryClient();
  return useMutation<SetupStatus, Error, void>({
    mutationFn: () => api.refreshSetupStatus(),
    onSuccess: status => {
      queryClient.setQueryData(setupKeys.status(), status);
    },
  });
}
