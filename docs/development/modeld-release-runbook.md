# Modeld Release Runbook

This is the maintainer runbook for producing official `modeld` artifacts. It
separates the two roles in the process: dependency producer devices publish
native dependency bundles, and the later release assembly step consumes those
bundles to build final packages. It is the operational companion to the design in
[modeld release artifacts](blueprints/modeld/release-artifacts.md).

`modeld` is released through a device-driven S3 flow, not through
`.github/workflows/release.yml`. The normal GitHub release workflow publishes the
pure-Go `contenox` CLI and VS Code packages only.

## Release Shape

The flow has two artifact layers:

1. Native dependency bundles: build inputs containing llama.cpp, OpenVINO, headers,
   libraries, manifests, and licenses. These are stored in S3 by platform and
   fingerprint. These are what the multi-device/manual contribution process
   produces.
2. Final `modeld` packages: user-facing archives built later by linking Go/CGO
   against a pulled dependency bundle, smoke-tested with `modeld version --json`,
   then uploaded to S3 by version.

No single device is expected to build every platform's native dependencies. Each
capable device contributes only the native dependency bundle for the
platform/accelerator variant it can build. Final package creation is a separate
release assembly step on a host that can link and smoke-test that target.

## One-Time S3 Store Setup

Create one private bucket for both dependency bundles and final packages. Bucket
names are globally unique, so replace the example name.

For `us-east-1`:

```bash
BUCKET=contenox-modeld-artifacts-<account-or-org>
REGION=us-east-1

aws sts get-caller-identity

aws s3api create-bucket \
  --bucket "$BUCKET" \
  --region "$REGION" \
  --object-ownership BucketOwnerEnforced

aws s3api put-public-access-block \
  --bucket "$BUCKET" \
  --public-access-block-configuration \
  BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true

aws s3api put-bucket-encryption \
  --bucket "$BUCKET" \
  --server-side-encryption-configuration \
  '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"},"BucketKeyEnabled":true}]}'

aws s3api put-bucket-tagging \
  --bucket "$BUCKET" \
  --tagging 'TagSet=[{Key=Project,Value=contenox},{Key=Purpose,Value=modeld-artifacts},{Key=ManagedBy,Value=aws-cli}]'
```

Optional prefix markers make empty prefixes visible in `aws s3 ls`:

```bash
tmp="$(mktemp)"
aws s3api put-object --bucket "$BUCKET" --key modeld-deps/.keep --body "$tmp"
aws s3api put-object --bucket "$BUCKET" --key modeld/.keep --body "$tmp"
rm -f "$tmp"
```

For regions other than `us-east-1`, create the bucket with a location constraint:

```bash
aws s3api create-bucket \
  --bucket "$BUCKET" \
  --region "$REGION" \
  --create-bucket-configuration LocationConstraint="$REGION" \
  --object-ownership BucketOwnerEnforced
```

## Repo-Local Environment

The root `Makefile` automatically loads a repo-root `.env` file when it exists and
exports the variables in it to subprocesses. The file is ignored by git, so every
release device needs its own copy.

Dependency producer devices only need `MODELD_DEPS_S3_URI` plus AWS region
settings. Keep `MODELD_RELEASE_S3_URI` in the same file when the checkout may also
run the release assembly step.

Create `.env` in the repository root:

```bash
cat > .env <<EOF
AWS_REGION=us-east-1
AWS_DEFAULT_REGION=us-east-1
MODELD_ARTIFACT_BUCKET=$BUCKET
MODELD_DEPS_S3_URI=s3://$BUCKET/modeld-deps
MODELD_RELEASE_S3_URI=s3://$BUCKET/modeld
MODELD_RELEASE_BASE_URL=https://$BUCKET.s3.amazonaws.com/modeld

# Match the prebuilt dependency variant this checkout should consume.
# These values identify the uploaded linux-amd64 CUDA/HIP/OpenVINO bundle.
MODELD_EXPECT_CUDA=ON
MODELD_EXPECT_HIP=ON
MODELD_EXPECT_OPENVINO=1
EOF
```

Verify the Makefile sees the values without manually sourcing `.env`:

```bash
make -pn help | rg '^(AWS_REGION|AWS_DEFAULT_REGION|MODELD_DEPS_S3_URI|MODELD_RELEASE_S3_URI) ='
```

## Platform Matrix

| Platform | Official backend set | Producer notes |
| --- | --- | --- |
| `linux-amd64` | llama.cpp + OpenVINO | CUDA/HIP plugins are included when the host toolchain is present. Run `make deps-modeld` first for OpenVINO. |
| `linux-arm64` | llama.cpp + OpenVINO | Same release contract as Linux, subject to available native toolchain and OpenVINO support. |
| `darwin-arm64` | llama.cpp + Metal | OpenVINO is off by default; package target sets `MODELD_RELEASE_OPENVINO=0`. |
| `darwin-amd64` | llama.cpp | Requires a matching macOS Intel build host/toolchain if released. |
| `windows-amd64` | llama.cpp + OpenVINO | Requires a Windows MinGW-w64 or MSVC+Clang native CGO build host. |

## Build and Upload a Dependency Bundle

Run this on the device that can build the target platform's native dependencies.
This is the normal multi-device contribution step. It creates no final `modeld`
archive and does not call `push-modeld-release`.

For Linux OpenVINO releases:

```bash
make deps-modeld
make bundle-modeld-deps
make push-modeld-deps
```

For macOS:

```bash
make deps-llamacpp-ref
# Build/install the platform llama.cpp runtime expected by mk/llama-flags.mk.
make bundle-modeld-deps
make push-modeld-deps
```

For Windows, use the Windows build shell/toolchain expected by
`scripts/modeld-deps-bundle-windows.sh`, then:

```bash
make deps-llamacpp-ref
# Build/install the platform llama.cpp runtime and OpenVINO inputs.
make bundle-modeld-deps
make push-modeld-deps
```

See `docs/development/windows-development.md` for the Windows-specific
development guide and the focused SSH-only steps to run just the bundler
script on a remote Windows worker (no `make` required on the worker).

`push-modeld-deps` uploads to:

```text
$MODELD_DEPS_S3_URI/<platform>/<fingerprint>/
```

If that exact fingerprint already exists, the upload is skipped.

Verify the contributed bundle:

```bash
for envf in bin/modeld-deps/*/bundle.env; do
  (
    . "$envf"
    aws s3 ls "$MODELD_DEPS_S3_URI/$MODELD_BUNDLE_PLATFORM/$MODELD_BUNDLE_FINGERPRINT/manifest.json"
  )
done
```

## Consume a Prebuilt Dependency Bundle

This is the dev/release consumer path when you want to avoid rebuilding the heavy
native dependencies on the current machine.

First print the exact dependency profile and hash the checkout expects:

```bash
make modeld-deps-profile
```

That prints the platform, llama.cpp commit, OpenVINO GenAI version, accelerator
profile, fingerprint, local pull directory, and manifest URI. The fingerprint is
computed from pinned inputs only, so this check runs before any native dependency
build.

Preflight the store:

```bash
make check-modeld-deps-store
```

If the manifest exists, pull and validate the prebuilt bundle:

```bash
make deps-modeld-prebuilt
```

For a local package built from prebuilt native dependencies, without uploading:

```bash
make package-modeld-prebuilt
```

Override the expected profile when you intentionally want a different variant:

```bash
MODELD_EXPECT_CUDA=OFF MODELD_EXPECT_HIP=OFF make check-modeld-deps-store
MODELD_PLATFORM=darwin-arm64 MODELD_EXPECT_OPENVINO=0 make modeld-deps-profile
```

## Downstream Release Assembly

Run this only when intentionally assembling the final `modeld` release artifact.
It is not part of the per-device dependency contribution step.

Run this on a device that can link and smoke-test the target platform package.

```bash
make pull-modeld-deps

DEPS_ROOT="$(make -s modeld-deps-pull-dir)"

make check-modeld-deps-bundle MODELD_DEPS_ROOT="$DEPS_ROOT"
make package-modeld-release MODELD_DEPS_ROOT="$DEPS_ROOT"
make push-modeld-release
```

If you are assembling on the current platform and want the Makefile to compute the
pull directory for you, `make package-modeld-prebuilt` performs the preflight,
pull, validation, and local package build. Run `make push-modeld-release` only
after deciding that local package is the release artifact to publish.

`push-modeld-release` generates/refreshes the matching `.build.json` sidecar from
each package archive before upload, then refuses to upload unless both `.sha256`
and `.build.json` exist. The sidecar is what feeds `modeld/index.json`. To
generate sidecars without uploading:

```bash
make modeld-release-metadata
```

The final package upload path is:

```text
$MODELD_RELEASE_S3_URI/$(cat cmd/modeld/version.txt)/
```

Verify:

```bash
MODELD_VERSION="$(tr -d '\r\n' < cmd/modeld/version.txt)"
aws s3 ls "$MODELD_RELEASE_S3_URI/$MODELD_VERSION/"
aws s3 cp "$MODELD_RELEASE_S3_URI/index.json" -
```

If only sidecars changed and the packages are already in the store, regenerate the
public index without re-uploading packages:

```bash
make push-modeld-index
```

## Public Download Surface

`contenox setup` should download only final `modeld` packages, not native
dependency bundles. Keep `modeld-deps/` private and expose `modeld/*` for
anonymous `GetObject`:

```bash
aws s3api put-public-access-block \
  --bucket "$BUCKET" \
  --public-access-block-configuration \
  '{"BlockPublicAcls":true,"IgnorePublicAcls":true,"BlockPublicPolicy":false,"RestrictPublicBuckets":false}'

aws s3api put-bucket-policy \
  --bucket "$BUCKET" \
  --policy "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Sid\":\"PublicReadModeldReleaseObjects\",\"Effect\":\"Allow\",\"Principal\":\"*\",\"Action\":\"s3:GetObject\",\"Resource\":\"arn:aws:s3:::$BUCKET/modeld/*\"}]}"
```

Verify anonymous access:

```bash
MODELD_VERSION="$(tr -d '\r\n' < cmd/modeld/version.txt)"
curl -fsSL "$MODELD_RELEASE_BASE_URL/index.json"
curl -fsSL "$MODELD_RELEASE_BASE_URL/$MODELD_VERSION/modeld-$MODELD_VERSION-linux-amd64.tar.gz.sha256"
```

Expected names look like:

```text
modeld-vX.Y.Z-linux-amd64.tar.gz
modeld-vX.Y.Z-linux-amd64.tar.gz.sha256
modeld-vX.Y.Z-darwin-arm64.tar.gz
modeld-vX.Y.Z-darwin-arm64.tar.gz.sha256
modeld-vX.Y.Z-windows-amd64.zip
modeld-vX.Y.Z-windows-amd64.zip.sha256
```

## Creating Missing Pieces from Another Device

When `make pull-modeld-deps` fails with a missing variant, that platform's
dependency bundle has not been contributed to S3 yet.

On a device that can build that platform:

```bash
git checkout <same-release-commit-or-tag>
# Create the same repo-root .env used by the other release devices.
aws sts get-caller-identity

make bundle-modeld-deps
make push-modeld-deps
```

Then return to the release assembly step and rerun `make pull-modeld-deps` for
that platform. If the dependency producer device is also the intended release
assembly host, keep the roles separate: finish the dependency upload first, then
run the commands in [Downstream Release Assembly](#downstream-release-assembly).

## Local Dry Run Without AWS

Use a local directory instead of the dependency `s3://` URI to exercise
dependency-bundle dedup/upload without AWS credentials:

```bash
MODELD_DEPS_S3_URI="$PWD/tmp/modeld-store/deps" \
make push-modeld-deps
```

To test release assembly locally as a separate step, also set
`MODELD_RELEASE_S3_URI` to a local directory before `push-modeld-release`.

The same URI scheme rule applies to every store operation: `s3://...` uses the
AWS CLI; anything else is treated as a local filesystem directory.

## Failure Guide

`set MODELD_DEPS_S3_URI=...`

The Makefile did not see a dependency store URI. Create the repo-root `.env` or
pass `MODELD_DEPS_S3_URI=...` on the command line.

`variant not in store`

The requested platform/fingerprint has not been uploaded. Build and push it from
a device that can produce that platform's native dependencies.

`MODELD_RELEASE_OPENVINO=1 but bundle manifest does not declare openvino:true`

The official package expects OpenVINO, but the dependency bundle was built without
it. On Linux/Windows, run the OpenVINO dependency setup before bundling. On darwin,
OpenVINO is intentionally disabled by the package target.

`packaged binary does not report the 'openvino' backend`

The smoke test caught a reduced backend set. Treat this as a failed release; fix
the dependency bundle or build tags before uploading.

`aws: OperationAborted`

S3 sometimes reports a conflicting operation immediately after bucket creation.
Retry the bucket configuration command after a short delay.
