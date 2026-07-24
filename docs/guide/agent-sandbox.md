# Confining agents: the sandbox wall

When contenox runs an agent (one of its task chains), it spawns it as a
subprocess and confines it behind "the wall": the agent can act on the world only
through the tools it is given and the workspace it is handed. The wall governs the
surface no tool gate can see — the code the agent runs *inside its own process*:
its own Bash, its own file access, whatever its toolchain drags in (an `npm
install` postinstall script). A well-behaved agent uses its tools and never
touches the wall; anything that does is confined.

There are two layers. One is always on and needs nothing. One is opt-in.

## The default: a zero-privilege fence (nothing to turn on)

Every confined agent gets this by default, with no configuration and **no kernel
privilege**:

- **Filesystem** — the agent may *write* only inside its workspace (its cwd) and
  *read* only its workspace plus a short list of auth/config directories
  (`~/.claude`, `~/.codex`, `~/.config/goose`, read-only). Everything else —
  `~/.ssh`, `~/.aws`, `~/.npmrc`, `~/.contenox`, the rest of the disk — is simply
  unreachable, so there is nothing to guard.
- **Exec** — it can only run programs it is allowed to reach.
- **Environment** — a scrubbed, minimal environment with a scoped `$HOME`, so no
  inherited credential rides along into the agent.

This runs on any Linux host, needs no setup, and requires no elevated privilege.
**The network is left open in this mode** — the agent can reach the network (it
needs to, to reach its model), but it cannot touch your filesystem or run outside
its box. That is the right default when the agent is trusted to use the network
and what you care about is that it can't reach your files or your credentials.

## Confining the network too (the opt-in wall)

If you also want to restrict *which hosts* the agent may reach — so it can talk to
its model but can't fetch arbitrary URLs or exfiltrate through its own HTTP client
— turn on the network wall.

There is no global on/off flag. The sandbox is necessity-driven: you enable the
network wall by **naming the hosts the agent legitimately needs**. Naming even one
host turns the wall on and refuses every host you did not name.

### Where to whitelist allowed domains

Edit `~/.contenox/sandbox-carveouts.json` — the necessity list. It is deny-default:
absent or empty means the FS/exec fence only, network open. Unknown fields are
rejected, and every hole must say why it exists.

```json
{
  "filesystem": [
    { "path": "~/.claude", "mode": "ro", "needs": "agent auth/config to start" }
  ],
  "network": [
    { "host": "api.anthropic.com", "needs": "the agent's model endpoint" },
    { "host": "registry.npmjs.org", "ports": [443], "needs": "npm install fetch" }
  ]
}
```

- Each `network` entry needs a `host` and a `needs` (in plain words, why the agent
  breaks without it). `ports` is optional and narrows the hole to specific
  destination ports; omit it to allow the host on any port.
- Every host **not** listed is refused (and logged). Listing any network host is
  what enables the wall.
- `filesystem` works the same way: extra read (`"mode":"ro"`) or write
  (`"mode":"rw"`) holes beyond the defaults, each justified. Keep them read-only
  unless a read-only hole demonstrably breaks the agent.

For a fully-offline agent (no network at all), set the environment toggle
`CONTENOX_SANDBOX_NETWORK_WALL=1` on the serve process and leave `network` empty —
the wall builds with no route.

## Making the network wall work on Ubuntu

The network wall puts the agent in a private user + network namespace so the
network is absent by construction. Creating that namespace without root needs
**unprivileged user namespaces** to be permitted — and this is the one thing a
default Ubuntu box does not allow.

Ubuntu 23.10+/24.04 restrict unprivileged user namespaces by default, via an
AppArmor hardening (`kernel.apparmor_restrict_unprivileged_userns = 1`). A
freshly-installed `contenox` binary has no AppArmor profile granting the
exception, so on a stock Ubuntu host **the network wall cannot be built and the
agent fails to start** — fail-closed by design, so it never runs with the network
open.

To allow it, pick one:

- **Relax the restriction host-wide** (simplest):
  ```
  sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0
  ```
  Persist it by dropping `kernel.apparmor_restrict_unprivileged_userns = 0` into a
  file under `/etc/sysctl.d/`.
- **Ship an AppArmor profile** that grants only the contenox binary `userns
  create`, keeping the restriction on for everything else on the host.

**Why this matters — and when it doesn't:** this is required *only* for the
network wall. The default filesystem/exec/environment fence uses no namespaces and
no kernel-namespace privilege, so it runs on stock Ubuntu unchanged. If you do not
need per-host network confinement, you never touch any of this.

### Troubleshooting

- Agent fails to start with an "unprivileged userns disabled" error, or a downstream
  "connection closed", right after you added `network` carve-outs → that is the
  Ubuntu userns restriction. Apply one of the fixes above, or remove the `network`
  carve-outs to fall back to the FS/exec fence (network open, no privilege needed).
- Non-Linux hosts have no wall — the confinement mechanisms are Linux-only.
