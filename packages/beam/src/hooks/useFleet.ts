import { useMutation, UseMutationResult, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../lib/api';
import { fleetKeys, missionKeys } from '../lib/queryKeys';
import { DispatchRequest, DispatchResult, FleetEntry } from '../lib/types';

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

/**
 * Stops an instance (`DELETE /api/fleet/{id}`), keyed by instance id.
 *
 * Deliberately NOT optimistic. Instance state is authoritative server-side —
 * the registry decides what is gone, not the browser — and the board already
 * re-reads it every 3s, so an optimistic removal could only ever disagree with
 * the very next poll. The invalidate is what makes the row disappear promptly;
 * if the stop failed, the row correctly stays and the caller surfaces `error`.
 */
export function useStopInstance(): UseMutationResult<string, Error, string, unknown> {
  const queryClient = useQueryClient();
  return useMutation<string, Error, string>({
    mutationFn: instanceId => api.stopInstance(instanceId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: fleetKeys.list() });
    },
  });
}

/**
 * Variables for {@link useCancelInstance}. Omitting `sessionId` is meaningful,
 * not lazy: the server cancels EVERY session attached to the instance when no
 * session is named, which is the board's "cancel all" affordance.
 */
export type CancelInstanceInput = { instanceId: string; sessionId?: string };

/**
 * Cancels in-flight turn(s) on an instance (`POST /api/fleet/{id}/cancel`).
 *
 * Same no-optimism reasoning as {@link useStopInstance}, with one extra reason:
 * cancel does not change the instance's own state at all — it interrupts a turn
 * — so there is nothing local to predict. Invalidating just pulls the next
 * session/viewer snapshot forward instead of waiting out the poll interval.
 */
export function useCancelInstance(): UseMutationResult<
  string,
  Error,
  CancelInstanceInput,
  unknown
> {
  const queryClient = useQueryClient();
  return useMutation<string, Error, CancelInstanceInput>({
    mutationFn: ({ instanceId, sessionId }) => api.cancelInstance(instanceId, sessionId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: fleetKeys.list() });
    },
  });
}

/**
 * Fires a mission (`POST /api/fleet/dispatch`): brings a declared agent up,
 * opens a session, and runs `intent` as its first turn, detached — see
 * DispatchRequest. Every dispatch is a mission; there is no separate "start
 * an instance with no mission" verb to call instead, so this is the one
 * mutation that creates both a fleet row and a mission record in the same
 * request.
 *
 * Invalidates both the fleet list and the mission list on success: a caller
 * watching either board sees the result immediately rather than waiting out
 * a poll (useFleet: 3s, useMissions: 5s). Deliberately NOT optimistic — the
 * server allocates the instance/session/mission ids, so there is nothing
 * meaningful to predict client-side before the response arrives.
 */
export function useDispatchMission(): UseMutationResult<
  DispatchResult,
  Error,
  DispatchRequest,
  unknown
> {
  const queryClient = useQueryClient();
  return useMutation<DispatchResult, Error, DispatchRequest>({
    mutationFn: req => api.dispatchMission(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: fleetKeys.list() });
      queryClient.invalidateQueries({ queryKey: missionKeys.list() });
    },
  });
}
