---
title: "How contenox is developed and released"
description: The public repository is a release mirror — one commit per release, a signed tag per release, binaries built from the tag — and a contribution travels through it by being carried upstream.
---

# How contenox is developed and released

Decided 2026-08-20, replacing the trunk-and-release-PR model written on this
page the day before, which was never set up.

## Two trees, one direction

contenox is developed in the maintainer's monorepo, beside the hosted relay
and app. The open-source server — this whole repository — is published from
there by **source drop**: a release takes the tracked files of the OSS tree as
they stand, commits them here as one `Release vX.Y.Z` commit carrying the
complete tree, tags it, and pushes. Nothing else ever pushes here.

So:

- **`main` is the release ledger.** Each commit is a release. The diff between
  two commits is the diff between two releases, and the commit message carries
  the release notes.
- **Every tag is a signed release.** `.github/workflows/release.yml` runs on
  the tag: it checks that `internal/version/version.txt` equals the tag,
  cross-compiles the four targets, writes `SHA256SUMS`, attests build
  provenance, and publishes the GitHub release with the tag's annotation as
  the notes. `install.sh` fetches from that release and refuses a checksum
  mismatch.
- **There is no development branch.** Work between releases happens upstream;
  what you can see is every released state, and only those.

## What runs where

| Gate | Where | When |
|---|---|---|
| hygiene, vulnerability scan, compile smokes, CLI help drift, schema drift, unit tests | upstream, before every drop | every release |
| `ci.yml` — the same fast lane plus the substrate smoke and the black-box `e2e-cli` suite | here | every push to `main`, which is every release, and every pull request |
| `release-gate.yml` — `task test-all`, the full release claim | here | nightly, on pull requests, and on demand |

`task test-all` is the one-command gate CONTRIBUTING documents; anyone can run
it on a checkout.

## How a contribution travels

A pull request here is a proposed change, not a merge. A maintainer carries it
upstream, credits the author as `Co-authored-by` on the release commit and in
the release notes, and the change arrives here with the next drop.
[CONTRIBUTING.md](https://github.com/contenox/contenox/blob/main/CONTRIBUTING.md)
has the details, including what to open an issue for first.

## Why this shape

One maintainer, two trees, and a hard rule that nothing private reaches the
public one. A mirror that only ever receives complete released trees is the
smallest mechanism that keeps that rule checkable: the export is a list of
tracked files, a release is a commit you can diff, and a leak is a search over
exactly what shipped, run before every push.
