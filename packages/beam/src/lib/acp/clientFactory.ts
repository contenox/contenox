/**
 * Capability-provider seam for `AcpClient` (see client.ts). `AcpClient` itself
 * implements no `fs/*`/`terminal/*` handling — it answers those agent->client
 * requests with `methodNotFound` (see `UNSUPPORTED_CLIENT_METHODS` in
 * client.ts). An `AcpCapabilityProvider` lets an embedder plug that support in
 * without `AcpClient` knowing anything about how a terminal or filesystem is
 * actually implemented. With no provider supplied, `createAcpClient` produces
 * a client whose behavior is byte-identical to `new AcpClient(transport)`
 * today — this module is purely additive.
 */
import { AcpClient, type AcpClientOptions } from './client';
import type { Transport } from './transport';
import type { ClientCapabilities } from './types';

/**
 * Supplies client-side capabilities beyond what `AcpClient` implements
 * itself.
 */
export interface AcpCapabilityProvider {
  /**
   * Merged into the `clientCapabilities` sent by `initialize()` (explicit
   * values passed to `initialize()` win over these per top-level key — see
   * `AcpClient.initialize`).
   */
  capabilities(): Partial<ClientCapabilities>;
  /**
   * Answers an agent -> client request whose method is in
   * `UNSUPPORTED_CLIENT_METHODS` (`fs/*`, `terminal/*`) before `AcpClient`
   * falls back to refusing it. Reject/throw to decline a specific method —
   * `AcpClient` then sends the same `methodNotFound` refusal it would with no
   * provider at all.
   */
  handleRequest(method: string, params: unknown): Promise<unknown>;
  /** Called when the client's transport closes, so the provider can release any resources it holds (e.g. running terminals). */
  dispose?(): void;
}

/** Constructs an `AcpClient`, optionally wired to a capability provider. */
export function createAcpClient(
  transport: Transport,
  opts: { capabilities?: AcpCapabilityProvider } = {},
): AcpClient {
  const clientOpts: AcpClientOptions = { capabilities: opts.capabilities };
  return new AcpClient(transport, clientOpts);
}
