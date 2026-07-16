import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';
import { setupKeys } from '../lib/queryKeys';

/**
 * Full CLI config snapshot (all `contenox config` keys), independent from
 * useSetupStatus: setup-status is issue/readiness-oriented and only carries
 * the subset of defaults relevant to onboarding checks (model/provider/chain/
 * hitl-policy). The Settings page needs the full surface (alt model/provider,
 * autocomplete, max-tokens, think, telemetry, update-check) to show current
 * values for keys setup-status never reports.
 */
export function useCLIConfig(enabled: boolean) {
  return useQuery({
    queryKey: setupKeys.cliConfig(),
    queryFn: api.getCLIConfig,
    enabled,
    staleTime: 30_000,
  });
}
