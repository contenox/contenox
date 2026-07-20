# Blueprint: The oversight layer — accepting work you did not watch

Owner: runtime

Status: DRAFT (2026-07-20). Strategic, deliberately **not** the next sprint.
It depends on the mission loop closing first
([`fleet-consolidation.md`](fleet-consolidation.md), M5 onward) and on units
producing work product worth judging. Written now because it decides what the
fleet is *for*, and that should shape the slices before it rather than be
retrofitted after.

## Purpose

A fleet's output outruns the capacity to review it. That is not a hypothetical:
public work in 2026 has converted codebases approaching a million lines with
tens of agents in parallel, in days. The instructive part is the shape of the
criticism. The test suite passed. Performance improved. And the work was still
judged unreviewable — the durable objection was a *ratio*: a safety-relevant
construct appearing tens of thousands of times where comparable hand-written
code had dozens.

Two lessons, and the second is the one that matters:

1. Correctness-by-test is not acceptance. A green suite says the thing does
   what it did before; it says nothing about what the change cost.
2. **Volume defeats review.** Nobody read those lines. Nobody could. So the
   deciding fact had to be something *computed about the result*, not something
   observed in the diff.

Running a fleet is becoming commodity capability. Being able to answer *what
changed and whether it is acceptable* is not. This layer is the difference
between contenox and a process supervisor with good manners.

## The core claim

**Oversight is not a diff viewer.** A textual diff of a large change is the
volume problem restated, not solved — beam already renders diffs and that
changes nothing at scale.

What is reviewable is a **property of the resulting state, compared to the
prior state**. "This construct went from 73 occurrences to 13,044" is
checkable in a second. The lines that caused it are not checkable at all. That
comparison — properties before versus after, not text before versus after — is
what "semantic state-diff" means here, and it is the product.

## The envelope is bidirectional

A mission already declares an envelope: a HITL policy saying what the unit
**may do**. The same declaration should say what must **remain true when it is
done**.

- *Before*: permission bounds. Crossing one costs the operator's attention.
- *After*: acceptance criteria. Violating one costs the operator's attention.

One declaration, two moments. Everything declared is then checked
mechanically, and human attention goes to exactly two places: criteria that
were violated, and changes no criterion covered. That is the fleet board's
"exceptions up, green silence" principle applied to **work product** instead of
process state — which is the argument that this is one system rather than two.

It also answers "how do you review six thousand commits." You do not. You
declare what must hold, the runtime checks, and you review the residue.

## Claim versus measurement

There are two possible sources of evidence about what a unit did, and they are
not equal:

- **What the unit reports.** It is the thing under review. Useful, never
  sufficient, and never trusted alone.
- **What the runtime measures independently.** Trustworthy, but only covers
  what someone thought to extract.

**Their disagreement is the highest-value signal in the system.** "The unit
reported adding tests; the measurement says no test file changed" is worth more
than either statement alone, and it is cheap to compute once both exist. A unit
that is wrong about its own work is the earliest reliable indicator that its
output needs a human.

## Verified current state

What this can be built from, rather than beside:

- **Chains** already compute things deterministically and are authored as
  reviewable files. A property extractor is a chain; this needs no new
  execution primitive.
- **HITL policy** is the envelope's first half and already exists.
- **Mission reports** (`runtime/missionservice`) are the evidence carrier:
  `Kind ∈ progress|finding|blocker|result`, `Summary`, `Detail`, `Refs`. Today
  `Refs` is a bare `[]string` and the payload is prose.
- **The taskengine inspector** already captures per-step execution state
  (`CapturedStateUnit`), and the journal already records what a unit did.
- **beam** renders diffs and tool calls today.

What is absent, and is the actual work:

- No notion of **state as a snapshot** that two points in time can be compared
  across.
- No **property extraction** — nothing computes a checkable fact about a
  workspace.
- No **acceptance act**. A mission ends `landed|derailed|abandoned`, all
  self-declared; there is no record of a human having judged it.
- No **aggregation**. Fifty reports are fifty reports.

## Invariants

- **A unit's self-report is never the sole basis for acceptance.** It is the
  subject of review, not a witness to it.
- **The layer must be able to say "I do not know."** An unchecked change must
  render as unchecked, never as approved. Silence meaning consent is the
  failure mode this layer exists to prevent, and it is easy to build by
  accident.
- **Absence of a criterion is itself a reportable fact.** "Nothing asserted
  anything about this change" is an oversight finding, not an empty result.
- **No new execution mechanism.** Properties are computed by chains, carried by
  mission reports, rendered by beam. A parallel analysis engine is
  reject-in-review.
- Acceptance is an explicit, recorded act with a stated basis — who, when, and
  against which criteria.
- The tracker stays passive and content-free; oversight evidence lives with the
  mission, not in the audit log.

## Slices

Sequenced so the earliest one is useful alone.

### O1 — Evidence-bearing reports

Give `Report` structure beyond prose: what changed (a reference the operator
can open — branch, patch, path set), what was checked, and what the check
found. Today `Refs []string` is untyped and nothing produces it.

Acceptance: a finished mission renders as "here is what changed and here is
what was verified," with no free-text summarization required to understand it.

### O2 — The acceptance gate

A mission gains a reviewed state distinct from its self-declared status:
accepted or rejected, by whom, when, and on what basis. Rejection records a
reason. This is small and it is the thing that makes the rest meaningful —
without an acceptance act there is nothing for evidence to feed.

Acceptance: `landed` stops meaning "the agent thinks it finished" and starts
being distinguishable from "a human agreed."

### O3 — Declared expectations (the envelope's second half)

Extend the mission declaration with criteria that must hold at completion, and
check them mechanically. Start with the obvious and verifiable — a command
that must still succeed, a path set that must not be touched.

Acceptance: a mission that violates a declared criterion escalates on its own;
one that satisfies all of them needs no human unless something uncovered
changed.

### O4 — Property extraction and the state-diff

Property extractors as chains, run before and after, with the comparison being
the report. This is where "semantic state-diff" becomes literal.

Acceptance: an operator can see that a change moved a measured property, and
by how much, without reading the change.

### O5 — Aggregation across missions

Roll up N missions into the facts that span them — the property deltas that
compound, the paths many units touched. Fleet-scale review is impossible
per-unit and tractable in aggregate.

### O6 — Claim-versus-measurement divergence

Compare what units reported against what was measured, and surface the gaps.
Depends on O1 and O4 both existing; cheap once they do.

## Decision points

RESERVED to the maintainer.

- **What is "state"?** A working tree, a branch, a content snapshot? This
  decides whether extraction runs in place or against something immutable, and
  it interacts directly with the deferred isolation primitive — a per-unit
  forked starting state would make before/after trivially well-defined.
- **Where do expectations live** — on the mission, or on the agent template
  that missions are fired from? Per-template is less typing and encodes
  institutional knowledge; per-mission is more precise.
- **Does rejection do anything**, or only record? Rolling back implies the
  runtime owns the work product, which is a much larger claim than observing
  it.
- **How much does the operator declare versus the system infer?** Declared
  criteria are honest but effortful; inferred ones risk the silence-means-
  consent failure the invariants forbid.

## Acceptance

The oversight layer exists when:

1. An operator can accept or reject a mission's work **without reading its
   diff**, and the basis for that decision is recorded.
2. A violated criterion reaches the operator and a satisfied one does not.
3. The system distinguishes *checked and fine* from *not checked* — and says
   which, unprompted.
4. A unit's own account of its work can be contradicted by measurement, and the
   contradiction is surfaced rather than averaged away.
5. None of it required a new execution engine, a second registry, or a parallel
   evidence store.
