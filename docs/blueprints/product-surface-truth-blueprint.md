# Blueprint: Product Surface Truth

Owner: runtime

Purpose: everything the product starts, surfaces, or promises must work
end-to-end, or it must not be surfaced. "The model decided" is not an
acceptable control boundary — and neither is "the README promised".

## Core Rule

A user-visible surface (CLI command, flag, curated model, provider type,
setup step, UI panel, documented syntax) exists only if an end-to-end test
exercises it on the product path. A surface without such a test is a removal
candidate, not a backlog item.

## Invariants

### Provider and backend advertisement

- Every provider type the CLI or docs advertise passes service validation and
  a live add-plus-chat test. A type that validation rejects is not advertised;
  a validation list that rejects an advertised type is a defect in one of the
  two.
- Setup and health output never present a dormant-but-healthy compiled-in
  backend as an error state.

### Curated catalog

- A curated model entry is loader-checked against the pinned runtime before it
  ships (see the capability-truth blueprint): a user must not be able to pull
  a multi-GB artifact that the linked runtime cannot open.
- Alias resolution is exact or certified: fuzzy/substring aliases must not
  route to unservable or uncertified entries.
- A curated entry that fails its quality smoke for the workloads it is
  curated for is removed or re-scoped, regardless of throughput.
- Advertised context (`model list` CTX, catalog, UI) is reachable context per
  the capability-truth definitions, never an unreachable ceiling.

### Platform claims

- Install and setup present a platform only when its build chain and packaged
  runtime are verified end-to-end on that platform.

### Chain and tool honesty

- A chain must not imply tools that are not attached: when no toolchain is
  bound, tool-assuming instructions are stripped or replaced. Narrated tool
  execution that never happened is a defect, not a model quirk.
- Failure-summarization paths bound their input so the summary cannot itself
  overflow and mask the original error.

### Documented syntax

- Every documented CLI syntax (input expansion, flags, config keys) has a test
  that uses it exactly as documented.

## Acceptance

The product surface is truthful when an inventory sweep finds, for every
surfaced item: the E2E test that covers it, or a removal. Anything else is a
violation of this blueprint, not a known issue.
