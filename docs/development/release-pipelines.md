# Release pipelines

**You don't need the release machinery on this page to build or contribute to
this repo.** It documents how the `contenox` CLI, native inference worker and
website get released, what triggers them, and — the one part a contributor does
meet — [what has to be green first](#what-must-be-green-before-a-release).
See [build-requirements.md](build-requirements.md) for what you need to build
any of this locally.

Releases are not cut from this repository. It is a release mirror
([../internal/dev-flow.md](../internal/dev-flow.md)): the tree is developed
upstream and every release arrives here as one commit on `main` plus a signed
tag. Everything below starts from that tag.

| Pipeline | Trigger | External deps |
|---|---|---|
| Portable `contenox` CLI + GitHub Release | `release.yml`, on the tag a release pushes | none (`GITHUB_TOKEN` only) |
| Native modeld packages | built and tested on each native platform, then pushed explicitly | the modeld S3 artifact store |
| Website (contenox.com) | the upstream repository's CI, on a push to `main` touching site sources or deployment configuration | none in this repository |

## 1. `contenox` CLI + GitHub Release

`.github/workflows/release.yml` runs on every `vX.Y.Z` tag:

1. **verify** — the tag must equal `internal/version/version.txt` exactly, or
   the run fails. The file is stamped upstream before the drop, so a release
   commit always names its own version.
2. **build** — cross-compiles the portable CLI (`CGO_ENABLED=0`) for
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
`website/Dockerfile` — and deployed by the upstream repository's CI when a push
to `main` touches site sources or deployment configuration. This deployment is
independent of a release tag. No workflow in this repository builds or
deploys it; `task website:build` produces the same static output locally, and
`website/README.md` describes the content model.

**A separate, unrelated S3 bucket** hosts heavy website media (demo gifs,
screenshots) referenced by the docs — a public bucket
(`contenox-website-assets-*`), read over plain HTTPS with no credentials
needed to build or view the site. `website/src/lib/remark-md-links.mjs`
rewrites root-relative markdown image paths (`/hero.gif`) to that bucket at
build time via its `S3_MEDIA` filename allowlist. General uploads remain a
manual, out-of-band step. The Beam hero is repeatable:
`website/scripts/record-beam-hero.sh --upload` records it with VHS and replaces
its WebM, MP4 fallback and poster in that bucket. Markdown images still need
their filename added to `S3_MEDIA` after uploading.

## What must be green before a release

Neither publication pipeline above runs tests. `.github/workflows/ci.yml`
runs the fast lane on pushes to `main` and pull requests. Read that workflow
for its current commands: it covers formatting, vet, vulnerability scanning,
compile smokes, CLI help, unit tests, backend smoke tests and the CLI end-to-end
suite.

Run `task test-all` before the source drop for the full release gate. There is
no nightly release-gate workflow in this tree. The release workflow does not
wait for the separately triggered CI workflow, so a published binary is not
evidence that CI passed.

`task test-unit` is in `ci.yml` for the fast failure, not as the gate. `-short`
means *unit only* — it switches off every case that needs a container, a kernel
feature, a peer binary or a spawned `contenox`. A release cannot rest on it.

Both system suites print a census of every case that skipped, so read the log
rather than the tick: a case that skipped for want of a dependency the runner
did not have is not a case that passed. Configure the Rust reference peers with
`ACP_TESTY_BIN`, `ACP_MCP_ECHO_BIN`, `ACP_VALIDATOR_BIN` and `ACP_YOPO_BIN`
before running the full gate. Kernel confinement tests also need the host
support described in [the sandbox guide](../guide/confinement/sandbox.md).

Cases with no runner-side dependency at all still skip and say so: the Bedrock
catalog case needs AWS credentials, and the vLLM suite needs
`CONTENOX_RUN_VLLM_TESTS=1`.

## See also

- [build-requirements.md](build-requirements.md) — per-platform toolchain requirements to build any of this
- [../internal/dev-flow.md](../internal/dev-flow.md) — the mirror model: what `main` and a tag mean here
- [../../CONTRIBUTING.md](../../CONTRIBUTING.md) — the test tiers and which task runs each

## Native inference worker

The GitHub binary is the first installation stage. `contenox auto` then selects,
downloads and verifies the compatible native worker from the modeld release
index before it downloads a model. The modeld worker uses the native
dependency-bundle and S3 release-index pipe because its llama.cpp, accelerator
and OpenVINO libraries must be built and tested on their actual platforms. See
[Building modeld](modeld-build.md#native-release-pipe) for assembly, smoke checks,
and publication. A CLI source drop does not publish a native worker. Release
candidates use a separate index prefix so refreshing one cannot change the
stable installer path.
