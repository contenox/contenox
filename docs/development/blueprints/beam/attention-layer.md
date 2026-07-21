# The attention layer — what Autobookmarks means for contenox

Date: 2026-07-21
Status: blueprint (conclusion + staged design). Origin: the maintainer's 2021
thesis ("On the impact of automatic bookmarks for predicting navigation cost
through sourcecode", Uni Bremen) — instrument all IDE interactions (MIMESIS),
compute a per-line Degree-of-Interest from weighted, decaying attention
events, auto-set ranked bookmarks. Its findings, not just its mechanism, bind
this design.

## The conclusion

Contenox is the thesis generalized: from *lines a developer attends in an
editor* to *the work itself* — what agents attend, what operators must attend.
The runtime already rebuilt the MIMESIS half without naming it: journals,
task events, tool calls, reports, telemetry record every interaction. What
was never rebuilt is the Autobookmarks half — **the relevance computation
over those recordings**. That is the attention layer, and it is the unifying
design behind the Beam criterion: "semantic state-diff oversight" is
autobookmarks over agent work.

Three doctrines carry over from the thesis as law:

1. **The blind-spot doctrine** (Guzzi's paradox: developers call bookmarks
   useful and never set them): every artifact of orientation must be an
   automatic by-product of work. Contenox already lives this — agents
   declared by filename, heartbeats riding tool calls, missions as records —
   and the attention layer must too: no curation, no tagging, no "mark as
   important". Ranked relevance is computed, never asked for.
2. **The metric lesson** (the thesis's own bolded null-result finding:
   navigation COUNT is not a tenable metric; the real signal was SCOPE —
   autobookmark users worked in fewer files and abandoned misleading paths
   faster): step/token counts are the wrong efficiency metric for agents
   too. **Scope is the signal**: how few paths a unit touched to land, how
   fast it abandoned a wrong path. Corollary: **derailment is a scope
   anomaly before it is anything else** — the first real derailed unit
   (wandering $HOME instead of the repo) was detectable from its first two
   tool calls.
3. **The falsifiability lesson** (the thesis reported its null honestly):
   do not assume DOI-ranked oversight helps — measure it. Beam-over-LAN is
   an instrumented remote environment; the thesis's own study design
   (recorded sessions, cohort comparison, automated scoring) is the template
   the tool-eval harness already adopts. Rank-vs-flat review is A/B-able.

## What it becomes, concretely (staged)

**Stage 0 — the inward face: harness guidance counters** (maintainer's
addition, 2026-07-21: "a tracker of some kind in the tool harness that would
allow the LLM to be guided — like a simple counter"). The same attention
signal, fed back to the AGENT live, through the tool-result envelope — the
one channel every model reads. Deterministic per-session counters over
(tool, path, args-fingerprint) yield: a repeat-call marker on identical calls
("[harness] 3rd identical list_dir of '.' this session"), a periodic scope
line every N calls ("[harness] scope: 12 files in 4 dirs; mission cwd is …"),
a revisit hint on heavy re-reads. The thesis's premise applies to models
verbatim — they cannot judge navigation value and will not curate their own
navigation memory; the harness derives it. Rules: terse, fixed, clearly
marked envelope (model-noise risk is real on weak models — the envelope is a
tool-hardening surface and a future ModelProfile dial); NEVER a gate; counts
reset per session; implemented as a ToolsRepo decorator so it wraps any tool
provider without touching them.

**Stage 1 — DOI over a mission's work** (rides the diff-review arc,
`ide-workflows.md` Arc 1): per-path interest scores from the unit's already-
journaled interactions — reads, edits, tool dwell — weighted (edit > read >
list), decayed, masked (one anchor per neighborhood, the thesis's masking
step). Surfaces: the changed-files list ordered by where the unit's attention
concentrated (review starts at the hot spot, not alphabetically);
"concentrated in N files" badges on board rows.

**Stage 2 — scope-anomaly detection** (the derailment early-warning): a
unit's touched-path set diverges from its mission's workspace/plan expectation
→ an attention-worthy condition in `UnitStatus` and the inbox, BEFORE failure
or silence. Cheap: set arithmetic over the same aggregation. This converts
the thesis's deepest finding into the fleet's most valuable alarm.

**Stage 3 — DOI over the fleet for the operator**: the overnight-skim
navigation-cost problem is the thesis problem squared (20 units producing
activity nobody can pre-judge). Rank inbox groups by accumulated relevance
(blocker recency, plan-revision magnitude, scope anomaly, handover presence);
anchors into long transcripts (jump to where attention/failure concentrated).

**Stage 4 — landed-vs-planned, measured** (closes the loop with the plan
engine): the plan gives intent, the attention layer gives where work actually
went; their diff IS the semantic state-diff review. The thesis Fazit's own
proposed follow-up — mine failed participants' traces against successful
ones — becomes: diff derailed missions' traces against landed ones,
automatically, as a fleet feature.

## Design notes

- All inputs exist today (kernel journals, task events, tool-call events,
  reports); the layer is a consumer, never a new recording duty.
- Scores follow the thesis mechanics: weighted event types, additive
  accumulation, decay per round, masking, relevance tiers — parameters are
  tunable and MUST be treated as hypotheses (lesson 3), not constants.
- Nothing here gates anything. The attention layer ranks and flags; envelopes
  gate. Rank is advice; the operator's eyes stay the judge (exoskeleton, not
  autopilot).
- First implementation slice: the Stage-1 aggregation inside the diff-review
  arc's Go endpoint (order-by-interest is one SQL/fold away from
  order-by-path), with edit-count ordering as the stub and the full DOI
  weights behind it.
