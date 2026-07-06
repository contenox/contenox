# CI Release Hardening Blueprint

## Problem

The current release path is not defensive enough.

Today these workflows are separate:

- `.github/workflows/ci.yml` runs cheap Go compile and unit checks on `push` and `pull_request`.
- `.github/workflows/vscode-extension-ci.yml` runs VS Code package smoke checks on relevant `push` and `pull_request` paths.
- `.github/workflows/release.yml` runs when any `v*` tag is pushed.
- `.github/workflows/vscode-marketplace.yml` also runs when any `v*` tag is pushed, plus manual dispatch.

That means a tag can start binary and Marketplace release jobs without proving that the normal CI checks passed for the tagged commit. The release workflows have some internal verification, but they do not currently act as a single promotion gate for all required checks.

This is exactly the kind of setup where a unit test can fail on a branch or PR while a manually pushed tag still starts release publishing, unless GitHub branch/tag rules outside the repository block it.

## Target Policy

Release should be promotion-based:

1. Nobody pushes directly to `main`.
2. Nobody creates release tags manually.
3. Version bumps land through PRs.
4. A release promotion workflow verifies the exact commit to release.
5. Only after all required checks pass does automation create the tag.
6. Publishing runs from that promoted tag or from the same promotion workflow.

The release tag becomes evidence that the commit passed the release gate. It should not be the mechanism that starts validation from an untrusted state.

## Feasibility

This is feasible on GitHub Actions, but there is one important plumbing detail:

Tags created with the repository default `GITHUB_TOKEN` generally do not trigger another `push` workflow. That is an intentional GitHub Actions recursion guard. If we want "promotion workflow creates tag, tag push triggers release workflow", the tag must be created with a token that is allowed to trigger workflows, usually a GitHub App installation token or a tightly scoped machine-user PAT.

There are two practical designs.

## Option A: Promotion Workflow Creates Tag, Then Calls Release Logic Directly

This is the simplest and most deterministic design.

Create a new workflow, for example `.github/workflows/promote-release.yml`, triggered by `workflow_dispatch` with an input like `version` or `ref`.

The workflow:

1. Checks out the requested commit.
2. Verifies the working tree version files:
   - `runtime/version/version.txt`
   - `packages/vscode/package.json`
   - `packages/vscode/package-lock.json`
   - `packages/vscode/README.md`
3. Runs the required CI checks directly:
   - Go compile smoke.
   - Cheap Go unit tests.
   - CLI help smoke.
   - VS Code typecheck.
   - VS Code integration tests.
   - Stable VSIX package check.
   - Cross-platform release builds.
4. Creates annotated tag `vX.Y.Z` only after those checks pass.
5. Builds and publishes release artifacts in the same workflow, or invokes reusable workflows with `workflow_call`.

Advantages:

- No dependence on tag-push recursion behavior.
- One workflow owns the release gate.
- The release cannot start unless tests and package checks pass.
- Logs show the full promotion chain.

Tradeoff:

- The existing `release.yml` and `vscode-marketplace.yml` need to be refactored into reusable workflow pieces, or their build/publish steps need to move into the promotion workflow.

## Option B: Promotion Workflow Creates Tag With a Workflow-Capable Token

This keeps the current tag-triggered release shape.

The promotion workflow:

1. Runs all required checks for the target commit.
2. Creates tag `vX.Y.Z` with a GitHub App token or tightly scoped PAT.
3. The tag push triggers:
   - `.github/workflows/release.yml`
   - `.github/workflows/vscode-marketplace.yml`

Advantages:

- Smaller changes to current release workflows.
- Existing tag-triggered jobs stay recognizable.

Tradeoffs:

- Requires managing a token with permission to create tags and trigger workflows.
- More moving pieces: if the token is wrong, the tag may be created without release workflows firing.
- The release workflows still need to verify that the tag was created by the promotion path, not manually.

If we use this option, add a hard guard in the tag-triggered release workflows: require the tag actor/token identity to be the release automation identity, or require an attestation file/artifact/status written by the promotion workflow.

## Required Repository Protections

This cannot be solved only in YAML. Repository rules should enforce the policy:

- Protect `main` or use a ruleset for `main`.
- Require PRs before merge.
- Require current checks before merge:
  - `CI / test`
  - `VS Code Extension CI / package-smoke`
  - any additional release-quality checks we decide to make mandatory.
- Block direct pushes to `main`.
- Protect tag pattern `v*`.
- Allow `v*` tag creation only by the release automation identity.
- Require signed commits/tags if the project wants cryptographic provenance.

Without tag protection, a human can still push `vX.Y.Z` and trigger release jobs.

## Workflow Changes

Recommended implementation order:

1. Add `promote-release.yml` in dry-run mode.
   - It runs the full release gate but does not create a tag.
   - This lets us prove the checks and timing before touching publishing.

2. Refactor release build logic into reusable workflows.
   - Convert build/publish pieces to `workflow_call`, or factor common scripts under `scripts/release/`.
   - Keep artifact naming consistent with the Marketplace-safe VS Code ID: `contenox-runtime-<target>-<version>.vsix`.

3. Add automated tag creation.
   - Use annotated tags.
   - Tag the exact SHA that passed the gate.
   - Refuse to overwrite existing tags.
   - Refuse dirty generated metadata.

4. Disable manual tag publishing path.
   - Keep tag-triggered release workflows only if tags are protected and automation-created.
   - Remove broad `workflow_dispatch publish=true` paths, or make them dry-run/pre-release only.

5. Add release status checks.
   - Verify package ID is `contenox.contenox-runtime`.
   - Verify `runtime/version/version.txt` matches the tag.
   - Verify VS Code package metadata is committed.
   - Verify generated VSIX manifests contain the expected publisher/name/version/target.
   - Verify no legacy `contenox.runtime` package identity is present, except explicit cleanup/docs references.

## Immediate Hardening Before Full Refactor

These changes reduce risk before a full promotion workflow exists:

- Add Go unit tests to `.github/workflows/release.yml` before build/publish jobs.
- Add VS Code integration tests to `.github/workflows/release.yml`, or make release call the same package-smoke logic as VS Code Extension CI.
- Remove manual Marketplace `workflow_dispatch publish=true` unless it is explicitly limited to pre-release or dry-run.
- Add `concurrency` to release workflows by tag.
- Add a release verify step that checks the tag commit already has successful required checks via the GitHub API. This is weaker than promotion-based release, but better than trusting a tag push alone.

## Proposed Final Shape

Long term, use one public entry point:

- `promote-release.yml`
  - Manual dispatch by maintainers.
  - Input: version, target ref, release channel.
  - Runs all release gates.
  - Creates protected tag only after success.
  - Publishes GitHub release and VS Code Marketplace artifacts.

Keep tag-triggered workflows only as compatibility wrappers or remove them after the promotion workflow is stable.

The important invariant is:

> A release tag must be created only after the exact commit has passed the release gate.

