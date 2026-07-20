import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';
import { fleetKeys } from '../lib/queryKeys';
import { FleetEntry } from '../lib/types';

/**
 * Live fleet board feed. Polls `GET /api/fleet` on a short interval — the board
 * is polling-truthful by design: `Manager.List` is an in-memory config+runtime
 * join server-side (cheap to re-read), so there is no push channel in this
 * slice. See the fleet-manager blueprint's Telemetry Model.
 */
export function useFleet() {
  return useQuery<FleetEntry[]>({
    queryKey: fleetKeys.list(),
    queryFn: api.getFleet,
    refetchInterval: 3000,
  });
}
