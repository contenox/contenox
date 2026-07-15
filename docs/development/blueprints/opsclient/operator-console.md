# Blueprint: The Operator Console — contenox as the operated-system agent

## The product split

Two binaries, two niches, already separated at the build boundary
(`modeld/owner-coordination.md`: "everything CGO lives in `modeld`;
everything else lives in `runtime`"). This document states the *product*
consequence of that split, not just the build one:

| Binary | What it is | What you drop it on |
| --- | --- | --- |
| `modeld` | the inference node | the box with the GPU — a server on your network, a gaming rig, a workstation |
| `contenox` | the operated-system agent | the system you want an agent to **operate**: configure the OS, install dependencies, fix config bugs, set up firewalls, install SSH keys, set up WSL on Windows, produce reports from logs |

`modeld` is the thing that answers "can this box do inference." `contenox` is
the thing that answers "can an agent safely act on this box." A single host
can be both (`contenox serve` colocated with a local `modeld`), but the ops
niche is defined by the second row independent of the first: an operator
drops `contenox` on a machine to have it administered, whether or not that
machine ever runs a model.

VS Code and Zed are coding-shaped clients built around a text buffer and a
repo. They are not the surface for ops work, and no new client app is built
for it either — the ops surface is beam, settled below.

## Session = host, not repo

Every existing framing of a contenox session assumes a workspace: "one
developer, one machine, one active repo/workspace"
(`../local-coding-node-goals.md`). Ops sessions invert the unit: the thing a
session is *about* is a **host**, not a repository. The "workspace" being
edited is systemd units, firewall rules, installed packages, and log files —
not source files. ACP's own session model already generalizes cleanly for
this (a session's `cwd` need not be a repo root — a scoped ops directory or
`/` works the same way at the protocol level), but the product framing must
lead with "which host is this session administering," not "which repo is this
session editing."

## Topologies

| Topology | Transport | Status |
| --- | --- | --- |
| Same box | loopback `contenox serve` + `contenox acp`, no network hop | works today — `ValidateLocalServeSecurity` (`runtime/serverapi/local_security.go`) permits loopback without a bearer token |
| LAN, one central `modeld` | multiple ops-driven `contenox` hosts share one inference node over the network | **tension with existing doctrine, unresolved — see below** |
| Remote, ssh-spawned `contenox acp` | operator's engine dials out over SSH, spawns `contenox acp` on the remote host, drives it stdio-over-ssh via the client core (`../acp/acp-client-engine.md`) | **blueprint-stage only** — `runtime/localtools/ssh.go` exists but is an agent-invocable *tool* (`ssh.execute_remote_command`) for a chain to call, not CLI/process orchestration that spawns and drives a remote `contenox acp`; no code implements this topology yet |
| Remote, `/acp` WebSocket | operator's engine dials the remote `contenox serve`'s `/acp` endpoint directly, no SSH hop | **not yet built** — `acp-chat-workspace.md`'s §C.0 lists the inbound `/acp` WebSocket as a prerequisite repair not yet landed; the existing WS terminal path (`runtime/internal/terminalapi/routes.go`, `GET /terminal/sessions/{id}/ws`) is a working precedent for wrapping a WebSocket as `io.ReadWriteCloser`, proving the pattern rather than the feature |

The LAN topology names a real tension rather than a settled design:
`modeld/owner-coordination.md` states, as a Non-Goal, "Not a multi-user or LAN
inference server" — one `modeld` today is a per-user, per-host owner, not a
shared network resource. An ops fleet of multiple `contenox` hosts sharing one
central `modeld` either needs that non-goal revisited, or each ops-driven host
needs its own local `modeld` — which reintroduces the GPU-box cost this niche
split exists to avoid for hosts that only need to be *administered*, not do
inference themselves. This is an open question, not resolved here.

## Beam is the screen

Beam, served by the box itself via `contenox serve`, is the phone-reachable
operator console — not a separate desktop app, not a hosted multi-tenant
service.

- **Local-first, precisely bounded.** What local-first bars is
  hosting-the-platform-for-others and cloud-IDE-style coding on server-side
  workspaces. It does not bar browsing in to hardware you own on your own
  network: the full stack — inference on your GPU box, the runtime on the
  operated box, the browser as viewport — stays on hardware the user owns.
  Remote administration of your own boxes is consistent with local-first;
  hosting contenox as a shared service for other people's workspaces is not.
- **The approval gate must be phone-usable.** This is a layout rule, not just
  an ops-specific one — see the corresponding rule added to
  `../beam/acp-chat-workspace.md` §B.3. On narrow viewports the workspace
  collapses to transcript + gate with thumb-reachable approve/deny; an
  operator who gets a permission-request push while away from a desk must be
  able to review and answer it from a phone.
- **No new client app.** The three-zone workspace `acp-chat-workspace.md`
  specifies is the same surface for ops sessions as for coding sessions; only
  the session's subject (host vs. repo) and the registered agent differ.

## Standing work is chains, not new surfaces

Scheduled log reports, periodic audits, and other recurring ops work are
expressed as chains, not as a bespoke scheduler feature bolted onto the
console. This matches the design center `acp-chat-workspace.md` §C.0 already
states for the whole protocol surface: contenox is built for **managed
background agents running with no human watching**, where each chain step is
a guardrail evaluating the *complete* output of the step before it — which is
also, by that same document's account, why token streaming is deliberately
not part of the requirements for this kind of surface. A standing log-report
or audit chain is exactly this shape: it runs unattended, each step reviews
the previous step's complete output, and the only human-facing moment is the
gate (or the final report), never a token-by-token feed.

## Ops-grade policy is a gap, not a given

The HITL policy machinery that gates every tool call today
(`runtime/hitlservice/policy.go`: `Policy{DefaultAction, Rules}`, first-match
`Rule{Tools, Tool, When []Condition, Action, TimeoutS, OnTimeout}`) is
dev-tool-shaped. Its condition operators (`eq`, `glob`, `host`,
`command_blacklist`, `command_ask_always`, `no_command_substitution`) and its
shipped policy files (`hitl-policy-default.json`, `-dev.json`, `-strict.json`,
`-acp.json`, `-acpx.json`) reason about `local_fs` path globs, `local_shell`
command basenames (`rm`, `sudo`, `dd`, `chmod`, `chown`, `mv`, `cp`,
redirection), and `webtools` HTTP verbs/hosts. None of these primitives
express `systemctl` unit/action pairs, firewall rule shapes, package-manager
install/remove distinctions, or SSH-key lifecycle actions (generate, install,
revoke) — the actions an operated-system agent spends most of its time
performing. Firewall/systemctl/ssh-key/package-manager actions need an
ops-grade policy file with new tool categories and condition primitives before
this surface can be trusted the way the dev-tool surface already is; treating
today's dev-tool-shaped policies as sufficient for ops actions is the gap this
blueprint names, not a feature this blueprint delivers.

## Open decisions

**The trust profile for a device owner driving remotely.** Two profiles exist
today, and neither models this operator: the default/`acp` profile
(unhardened, dev-tool-shaped — built for a trusted developer at their own
keyboard, present and reacting in real time) and the `acpx` profile
(`hitl-policy-acpx.json`: `default_action: deny`, denies `local_shell`
entirely, denies all web reads and writes, allows only read-only `local_fs` —
built for an untrusted headless driver). An owner who is remote but trusted —
it is their own hardware, but they cannot physically reach the machine or pull
a plug if something goes wrong — has a risk shape distinct from both: too
permissive under the default profile (a mistake or hallucinated command has no
one standing over the keyboard to interrupt it), too restrictive under `acpx`
(a device owner administering their own box legitimately needs `local_shell`
and package-manager actions that `acpx` blanket-denies). What a third,
ops-grade trust profile looks like — and whether it is a new named profile or
a parameterization of the existing policy schema — is open.

**Windows as a target.** The ops task list explicitly includes "set up WSL on
Windows" — but driving Windows ops assumes a way to reach the box *before*
WSL (and the SSH/Unix-shaped tooling that assumes) exists, a chicken-and-egg
`windows/windows-product-surface.md` does not address: that document frames
Windows as a GUI-first local desktop agent experience (Store/PowerShell
install, Beam as a local UI, VS Code as the primary surface), not a remote-ops
driver reaching an unconfigured Windows box. Whether the first Windows ops
session assumes PowerShell remoting, an already-present SSH server, or a
different bootstrap path entirely is unresolved.

## Invariants (anti-patterns to reject in review)

- **An ops session framed around a repo or workspace instead of a host.**
  Breaks the mental model this blueprint exists to establish.
- **A new client app built for ops** instead of extending beam. Beam is the
  settled screen; a second app duplicates the workspace this document and
  `acp-chat-workspace.md` already specify.
- **Live token streaming demanded for an ops surface.** Contradicts the
  guardrail design center this surface is built on; standing ops work is
  chains reviewing complete step output, not a chat feed.
- **Dev-tool-shaped HITL policy applied unchanged to system-administration
  actions.** `systemctl`, firewall, package-manager, and SSH-key actions need
  an ops-grade policy file, not a `local_shell` command-blacklist entry
  standing in for one.
- **Repair sideways into a beam-only ops endpoint instead of downward** into
  `acpsvc` or the ACP transport — the same doctrine `acp-chat-workspace.md`
  and `beam-on-acp.md` already state for chat applies unchanged here.
- **Auto-approve as the trust profile for a remote-but-owned host** because
  hardening it like `acpx` felt inconvenient. The open trust-profile question
  above is a design gap to close, not a license to default to the permissive
  profile for convenience.
