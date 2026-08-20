# Release pipelines

**You don't need the release machinery on this page to build or contribute to
this repo.** It documents how the two things this repo ships — the `contenox`
CLI and the website — actually get released, what triggers that, which
external services are involved (none beyond GitHub), and — the one part a
contributor does meet — [what has to be green first](#what-must-be-green-before-a-release).
See [build-requirements.md](build-requirements.md) for what you need to build
any of this locally.

Releases are not cut from this repository. It is a release mirror
([../internal/dev-flow.md](../internal/dev-flow.md)): the tree is developed
upstream and every release arrives here as one commit on `main` plus a signed
tag. Everything below starts from that tag.

| Pipeline | Trigger | External deps |
|---|---|---|
| `contenox` CLI + GitHub Release | `release.yml`, on the tag a release pushes | none (`GITHUB_TOKEN` only) |
| Website (contenox.com) | the upstream repository's CI, on a release that touches `docs/`, `schema/` or `website/` | none in this repository |

## 1. `contenox` CLI + GitHub Release

`.github/workflows/release.yml` runs on every `vX.Y.Z` tag:

1. **verify** — the tag must equal `internal/version/version.txt` exactly, or
   the run fails. The file is stamped upstream before the drop, so a release
   commit always names its own version.
2. **build** — cross-compiles the pure-Go CLI (`CGO_ENABLED=0`) for
   `linux/amd64`, `linux/arm64`, `darwin/arm64`, `windows/amd64` from one
   Ubuntu runner, and packages both the raw binary (for `install.sh`) and an
   ACP-registry archive (`.tar.gz`/`.zip`) per target.
3. **release** — downloads every artifact, writes `SHA256SUMS`, attests build
   provenance through Sigstore, and runs `gh release create` with the tag's
   annotation as the release notes, authenticated with the ambient
   `GITHUB_TOKEN`. No other secret is used.

Upstream, cutting a release is one task: it runs the fast gates, stamps
`internal/version/version.txt` and the `TAG=` marker in the README, exports
the tracked files, commits them here as `Release vX.Y.Z`, signs the tag, and
pushes. Note `darwin/amd64` is not one of the four raw CLI release targets —
Intel Mac users build from source.

`task version:set` stamps a local build with `git describe` instead; without
it, `task build` reports the last release's version.

## 2. Website (contenox.com)

The site is built from this tree — `docs/`, `schema/` and `website/`, via
`website/Dockerfile` — and deployed by the upstream repository's CI whenever a
release touches one of those paths. No workflow in this repository builds or
deploys it; `task website:build` produces the same static output locally, and
`website/README.md` describes the content model.

**A separate, unrelated S3 bucket** hosts heavy website media (demo gifs,
screenshots) referenced by the docs — a public bucket
(`contenox-website-assets-*`), read over plain HTTPS with no credentials
needed to build or view the site. `website/src/lib/remark-md-links.mjs`
rewrites root-relative markdown image paths (`/hero.gif`) to that bucket at
build time via its `S3_MEDIA` filename allowlist. Uploading a new asset there
is a manual, out-of-band step (no in-repo script does it) — add the filename
to `S3_MEDIA` after uploading.

## What must be green before a release

Neither pipeline above runs tests. Two workflows do, and they select tests by
the name-prefix convention CONTRIBUTING publishes:

| Workflow | Trigger | Runs |
|---|---|---|
| `.github/workflows/ci.yml` | every push to `main` (every release) and every pull request | gofmt, `go vet`, govulncheck, both compile smokes, CLI help drift, schema drift, `task test-unit`, a container-backed smoke of the opt-in Postgres/NATS/Valkey backends, and the black-box `e2e-cli` suite |
| `.github/workflows/release-gate.yml` | nightly at 03:00 UTC, on pull requests, and on demand | `task test-all` — everything above plus the ACP wire-conformance harnesses against the Rust reference peers, the Ollama and vLLM model suites, `task test-system`, `task test-integration` and `task test-rest` |

The fast lane also runs upstream before every drop, so a release commit has
passed it twice by the time its binaries exist. The full claim runs here
nightly over what shipped.

`task test-unit` is in `ci.yml` for the fast failure, not as the gate. `-short`
means *unit only* — it switches off every case that needs a container, a kernel
feature, a peer binary or a spawned `contenox`. A release cannot rest on it.

Both system suites print a census of every case that skipped, so read the log
rather than the tick: a case that skipped for want of a dependency the runner
did not have is not a case that passed. Two dependencies are supplied by the
workflows on purpose —

- `ci.yml` sets `kernel.apparmor_restrict_unprivileged_userns=0` before the
  system suites, because Ubuntu blocks unprivileged user namespaces by default
  and `internal/libsandbox` then skips its whole confinement story. This is the
  same sysctl [the sandbox guide](../guide/confinement/sandbox.md) documents for
  operators.
- `release-gate.yml` builds the Rust ACP reference peers and exports
  `ACP_TESTY_BIN` / `ACP_MCP_ECHO_BIN` / `ACP_VALIDATOR_BIN` / `ACP_YOPO_BIN`,
  without which the ACP host and client end-to-end cases skip.

Cases with no runner-side dependency at all still skip and say so: the Bedrock
catalog case needs AWS credentials, and the vLLM suite needs
`CONTENOX_RUN_VLLM_TESTS=1`.

## See also

- [build-requirements.md](build-requirements.md) — per-platform toolchain requirements to build any of this
- [../internal/dev-flow.md](../internal/dev-flow.md) — the mirror model: what `main` and a tag mean here
- [../../CONTRIBUTING.md](../../CONTRIBUTING.md) — the test tiers and which task runs each
