# Blueprint: Hosting Foreign ACP Agents — The Wall + Complete Telemetry

> Status: security-architecture blueprint, drafted 2026-07-23. A design record
> for how contenox hosts a *foreign* agent (Claude Code, Goose, or any ACP
> binary) as a subprocess without handing it the operator's machine. Nothing here
> is landed: today the spawn is unsandboxed (see §1). It decides the *isolation
> architecture* — the invariant, the necessity carve-outs, the telemetry, and the
> enforcement tiers. It is explicitly **not** a permission/HITL policy system
> (see §6).

## The invariant

**An agent effects the world only through editor-provided tools. Every other
path is absent by construction. There is no bypassing.**

That is the whole design. A hosted agent is given tools over ACP; its real work
and every side effect flow through them. Whatever gates those tools — approval
prompts, allow/deny, human-in-the-loop — is the **tool layer's** business, at the
altitude where intent is legible (`git push` arrives as a structured call). This
blueprint is about the *other* surface: the code the agent runs **inside its own
process** — its Bash, and everything its toolchain drags in. `npm install` runs
third-party `postinstall` scripts with the agent's permissions; that is the most
actively exploited gap in the ecosystem (secret-scanning worms hunting `~/.ssh`,
`~/.aws`, `~/.npmrc`, `~/.claude`, env, then exfiltrating). The tool gate cannot
see it, because it is not a tool call.

So the sandbox does one dumb thing: it makes those bypass paths **not exist**. It
is not a policy engine, it makes no per-operation decisions, and it **knows
nothing about HITL policies** (§6). The only holes in the wall are *functional
necessities* — a thing the agent needs to boot or authenticate at all — and
every attempt to hit the wall (to do a side effect by a path other than the
tools) is **recorded**. That recording is how you learn whether an agent cheats,
and how.

Three properties, and that is all:

- **No bypass** — side effects go through tools; all other paths are absent.
- **Necessity carve-outs only** — the wall has holes solely where the agent
  breaks without them, minimized and justified one by one.
- **Every wall-hit is telemetry** — blocked attempts are first-class events, not
  silent `EPERM`s. Complete telemetry as an automatic by-product (the blind-spot
  doctrine, same as `missionchanges`).

## 1. Starting point (what exists today)

Today every bypass path is wide open:

- **Spawn:** `agenthost/externalacp.go` `connectStdio` builds a bare
  `exec.Command(a.Config.Command, …)`, sets `cmd.Dir` to the mission cwd, and
  `cmd.Env = append(os.Environ(), …)` — it **leaks the entire contenox
  environment** and inherits the **real HOME and full filesystem**. No
  `SysProcAttr`, no namespaces, no fs restriction.
- **Control-plane isolation:** `vfs/controlplane.go` denies *native tools* access
  to `~/.contenox` — but a foreign subprocess runs *outside* that guard, so
  nothing stops it reaching `~/.contenox` today.
- **Telemetry seam:** `libtracker.ActivityTracker` — `Start` → `(reportErr,
  reportChange, end)`, chainable, credential-field redaction on by default.
- **Workspace:** `fleetservice` dispatches each mission with a `Cwd` via
  `vfs.Factory` — the per-mission root the wall jails to.

## 2. What the sandbox does — three things, none of them "policy"

### 2.1 Make the tool channel + workspace the only sanctioned surface

The agent keeps exactly two ways to affect anything: its ACP stdio (the tools)
and its own workspace/fork (`cmd.Dir`, where edits are journaled as diffs and
consumed by `missionchanges`). Both are wired at spawn and survive the wall.

### 2.2 Block every other path by construction

Not decisions — *absence*:

- **fs** — only the workspace is visible; the rest of the filesystem is not
  mounted / not reachable (mount-ns or Landlock).
- **net** — no route (net namespace offline).
- **process tree** — the whole tree (agent → Bash → git) is confined; nothing
  escapes the namespaces; die-with-parent.
- **env** — scrubbed to a minimal set; no inherited credentials.

Because these are deny-by-construction, they are complete for the whole class
without enumerating anything — the resource simply is not there.

### 2.3 The necessity carve-outs (the "whitelist" — but it is not permissions)

The only holes are things the agent cannot function without. Two we already know:

- `~/.claude` (or `~/.codex`, `~/.config/goose`) — the agent reads its own auth
  and config to start. Bind **read-only**.
- the package registry host(s) — the toolchain (`npm`/`pip`) needs the registry
  to fetch, or `install` breaks.

The list is authored as plain JSON — but its meaning is *"the agent breaks
without this,"* not *"policy permits this."* Discipline:

- Every entry is justified by a concrete "breaks without it." If it does not
  break, remove it. The default answer is *no hole*.
- **Read-only** wherever possible (cred/config dirs are read, not written).
- The **loot paths stay out** — `~/.ssh`, `~/.aws`, `~/.npmrc` — unless a real
  breakage forces one, because each is exactly what a `postinstall` hunts.
- `~/.contenox` is **never** a carve-out.
- The carve-out list itself lives in the **control plane** (`~/.contenox`,
  unreachable by the agent) — the agent can never punch its own hole.
- One shared list across agents is fine: `~/.claude` is inert to a non-Claude
  agent; per-agent scoping is an optional refinement, not a requirement.

```json
{
  "filesystem": [
    { "path": "~/.claude", "mode": "ro", "needs": "agent auth/config to start" }
  ],
  "network": [
    { "host": "registry.npmjs.org", "needs": "npm install fetch" }
  ]
}
```

The JSON is the *necessity list*; the mechanism (Landlock ruleset / bwrap binds /
egress proxy) is generated from it. The operator edits necessities, never
mechanism config.

The scoped-HOME lever reconciles "needs `~/.claude`" with "deny the rest": set
`HOME` to a per-mission dir; the carve-out binds `~/.claude` read-only into it;
`~/.ssh`, `~/.aws`, `~/.npmrc`, `~/.contenox` are simply not under it.

## 3. Telemetry — watch the wall

Every wall-hit is wrapped in the `ActivityTracker` lifecycle: `Start` at the
attempt (resource, requested access, mission/agent id), `reportErr` on the
blocked bypass, `end` on completion. A `ChainedTracker` fans it to structured
logs, a durable per-mission audit stream, and (later) an anomaly consumer.
Redaction keeps credential *values* out while the *shape* ("tried to read a key
here") stays greppable.

The point: **a well-behaved agent uses its tools and never touches the wall.** An
agent that tries a side effect by another path — direct `~/.ssh` read, a raw
socket, a write outside the workspace — is blocked *and recorded*. The
necessity carve-outs are the known-benign holes; **anything else hitting the wall
is the anomaly signal.** That is "wrap everything, then see if they truly cheat
and how," made concrete: cheating is not inferred, it is the audit trail of
bypass attempts.

Honest ceiling: deny-by-construction blocks well but is *telemetry-poor* — a
Landlock/namespace denial is an `EPERM` the agent sees, not an event you record.
To *see* every attempt you need a notifying interposition (seccomp user-notify
for an enumerated set, or a userspace kernel for all of it). That is the real
cost of "complete telemetry about everything," and §5 states it plainly rather
than pretending the cheap floor delivers it.

## 4. Isolation identity — one wall per mission/fork

One sandbox binds one mission's workspace (the `fleetservice` cwd / COW fork).
Parallel agents on parallel forks cannot reach each other's tree or the base;
each has its own scoped HOME and its own per-mission audit stream. The sandbox
and the parallel-fork isolation are the same plumbing from two angles.

## 5. Enforcement mechanisms — the completeness ceiling

| Mechanism | Builds the wall for | Sees the wall-hit | Cost |
| --- | --- | --- | --- |
| **env-scrub** | credential inheritance | n/a (removal) | ~nil; Phase 0 |
| **Landlock + net-ns + mount-ns** | fs, net, process — by construction | **no** (silent EPERM) | unprivileged, self-contained; the floor |
| **seccomp user-notify** | enumerated syscalls | **yes** — supervisor sees every attempt | TOCTOU/latency/enumeration |
| **egress proxy** | the carve-out net hosts | **yes** — every connection logged | one host process |
| **gVisor (runsc) / microVM** | *all* syscalls | **complete** | userspace kernel / VM; the ceiling |

Two facts decide the stack: the cheap floor (Landlock/netns) is the *soundest*
way to make paths absent but is *telemetry-poor*; the notifying tiers
(seccomp-notify, gVisor) are what turn a blocked attempt into a recorded one.
And using seccomp-notify **as a telemetry tap first** sidesteps its worst hazard
— TOCTOU only bites when you inspect args and CONTINUE on a *security decision*;
merely recording an attempt does not — so it is a good fit for "watch the wall"
before it is ever used to enforce.

Recommended stack:

1. **Deny-by-construction floor** — Landlock (fs = workspace + carve-outs only) +
   net-ns offline + mount-ns (scoped HOME) + env-scrub. Complete for those
   classes, no enumeration.
2. **Egress proxy** on the net carve-outs — enforces the necessity hosts and logs
   every connection.
3. **seccomp user-notify** as a *telemetry tap* on the enumerated set
   (`execve`/`connect`/writes) — records bypass attempts the floor would swallow
   silently. Promote to a blocker only if the audit shows it is worth the cost.
4. **gVisor/microVM** as the strong tier for a genuinely hostile binary — and the
   only path to *complete-syscall* telemetry, i.e. the true ceiling of
   "telemetry about everything."

## 6. Explicitly NOT

- **Not** a HITL/permission policy system, and **not aware of one.** HITL gates
  *tools*, at the intent altitude; the sandbox makes *non-tool paths absent*.
  They are different layers with no shared policy, no shared convention, and no
  overlap. The sandbox never consults, mirrors, or knows about `hitl-policy-*`.
- **Not** a per-operation decision engine. There are no allow/deny *rules* — there
  is a wall and a short list of necessity holes. The wall does not deliberate.
- **Not** the enforcement point for the agent's *sanctioned* actions — those go
  through tools and are gated there. The sandbox only ensures there is no other
  door.
- **Not** macOS parity in v1 (Linux-first; `sandbox-exec` is a later, weaker
  backend).
- **Not** a userspace kernel in v1 unless the threat model is a hostile binary.

### Accepted limitations

- **No PID namespace (accepted).** The sandbox does not add `CLONE_NEWPID`, so the
  agent shares the **host PID namespace**. Mapped to the operator's real uid, it can
  therefore signal the operator's other same-uid processes — including the sandbox
  **supervisor**. A `SIGSTOP` to the supervisor would *freeze* the telemetry tap and
  the egress bridge (both run in the parent) while the agent keeps acting;
  `Pdeathsig` fires when the supervisor **dies**, not when it is **stopped**, so a
  stopped supervisor is an observation gap. This is a **deliberate, accepted** risk:
  a PID namespace would require the shim to become a reaping **pid-1 init**
  (reparent + `waitpid` the agent's whole subtree), a structural change out of the
  current scope. The exposure is bounded — cross-userns **`ptrace` is blocked** by
  the kernel (the agent runs in its own user namespace) and **`/proc` is denied** by
  Landlock (the agent cannot even enumerate host pids) — but the signal surface to
  same-uid processes remains and is knowingly accepted.

## 7. Phased plan

- **Phase 0 — stop the leak.** Env-scrub at `externalacp.go` (`:93`) to a minimal
  allowlist (`PATH`, scoped `HOME`, `TERM`, `LANG` + explicit config env); deny
  `~/.contenox` to the subprocess. Closes the incident class.
- **Phase 1 — the fs wall.** Landlock/mount-ns so only the workspace + fs
  necessity carve-outs are visible; scoped HOME. Proof: a `postinstall` reaching
  `~/.ssh` is blocked.
- **Phase 2 — watch the wall.** seccomp-notify as a telemetry tap on
  `connect`/`execve`/out-of-workspace writes → the audit stream. Learn what
  agents actually try before building more.
- **Phase 3 — net carve-outs.** net-ns offline (already, by construction) +
  egress proxy enforcing the necessity host list, with per-connection telemetry.
- **Phase 4 — promote taps to blockers** only where the audit justifies it.
- **Phase 5 — strong tier (optional).** gVisor/microVM for hostile-binary hosting
  and complete-syscall telemetry.

## 8. Open questions

- The completeness bar for telemetry: instrument a selected op-set (seccomp), or
  a userspace kernel (gVisor) for genuinely total visibility?
- Do any cred dirs need a writable path (token refresh), or is read-only always
  enough?
- Where does the wall-hit audit surface — a cockpit view beside `missionchanges`,
  or folded into it?
- Does the seccomp-notify supervisor live in-process, or as a re-exec shim, to
  keep the single-static-binary property?

## 9. Recommendation

Build the wall, not a policy engine. Make every non-tool path absent by
construction (Landlock + net-ns + scoped HOME + env-scrub); punch holes only for
proven functional necessities, minimized and read-only, in a control-plane list
the agent cannot edit; and put the tracker on the wall so every bypass attempt is
recorded. Keep the sandbox utterly ignorant of HITL — tools are gated at the tool
layer, the wall only guarantees there is no other door. Start Phase 0/1 (they
close the actively-exploited class), watch the wall in Phase 2, and let the audit
— not theory — decide how far up the mechanism ceiling to climb.

The load-bearing idea: you do not secure a hosted agent by deciding, per action,
whether to allow it — you secure it by making the tools the only way to act at
all, and recording every attempt to do otherwise.
