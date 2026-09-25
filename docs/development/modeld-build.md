# Building the native inference worker

`task build` builds the portable Contenox control and harness binary. The inference
worker is a separate native executable, built by the existing Make pipe. Its transport
contract connects it to Contenox without linking inference libraries into the CLI.

Run `make help` for native build and packaging targets. A local native build needs
Go, GNU Make, Bash, GNU coreutils, CMake, a C++ compiler, the pinned llama.cpp sources, and the OpenVINO SDK
on platforms that include that backend:

```sh
make deps-modeld
task modeld:build
```

On macOS, use `make deps-llamacpp-ref` in place of `make deps-modeld`; the worker
uses llama.cpp and Metal there. The pinned versions and native flags live in
`mk/` and the backend Makefiles. Windows native builds use the existing
MinGW or Clang/MSVC dependency-bundle path; they are not Linux cross-builds.

OpenVINO runtime, GenAI and tokenizer wheels must match the pinned C++ headers.
`make deps-openvino` installs those versions; `make -f Makefile.openvino check-sdk`
checks an existing SDK. A newer wheel can link successfully against older headers
and crash during inference. Dependency bundles carry the verified SDK version;
rebuild older bundles that lack it rather than copying the marker into them.

On Linux hosts with a RAM-backed `/tmp`, use a disk-backed temporary directory
for native compilation, for example `TMPDIR=/var/tmp make deps-modeld`. Keep
build directories on disk too. CUDA compilation can exhaust a small tmpfs even
when the destination filesystem has ample free space. The CUDA toolkit must
provide `nvcc` on `PATH`; use a host C++ compiler supported by that toolkit.
After building, verify inference on the GPU: a successful build alone does not
establish driver compatibility or GPU offload.

`task dev-install` is the complete checkout installation. It builds a relocatable
modeld bundle with every backend supported on the current platform, installs it
below the managed modeld data root, updates the same current pointer used by a
downloaded release, and exposes the newly built CLI at the path the shell actually
resolves. On Linux it includes CUDA automatically when `nvcc` is on `PATH` or at
`/usr/local/cuda/bin/nvcc`. `NVCC_CCBIN` selects a compatible host compiler when
the system compiler is newer than the CUDA toolkit supports.

`task dev-link` remains the fast CLI-only loop. When using an unpackaged worker
instead, set `CONTENOX_MODELD_BIN` to the absolute path of `bin/modeld`
(`bin/modeld.exe` on Windows). `contenox modeld status` probes its lease and
endpoint. Models live below the worker data root, which defaults to `~/.contenox`;
`CONTENOX_DATA_ROOT` selects another root.

## Native release pipe

The native release path remains separate from the pure-Go CLI release. It reuses
the dependency bundles, checksums, package manifests, and release index from
modeld. It does not upload anything during a build.

On each native build host:

```sh
make build-modeld
make bundle-modeld-deps
make push-modeld-deps MODELD_DEPS_S3_URI=s3://YOUR-BUCKET/modeld-deps
```

To consume a prepared dependency bundle on its matching platform:

```sh
task modeld:package -- MODELD_DEPS_ROOT=/absolute/path/to/dependency-bundle
```

The packager checks required libraries, links the worker, runs the packaged
`version --json` smoke probe, and creates the archive, checksum, and index metadata
under `dist/`. It refuses to silently omit OpenVINO when required by the selected
profile. `MODELD_RELEASE_OPENVINO=0` explicitly selects a llama-only profile.

To publish through the existing store mechanism:

```sh
make push-modeld-release MODELD_RELEASE_S3_URI=s3://YOUR-BUCKET/modeld
```

Store arguments also accept local directories for offline assembly and testing.
The release prefix must be exposed over HTTP for installation. Then users run:

```sh
contenox modeld install --base-url https://YOUR-RELEASE-HOST/modeld
```

The installer reads `index.json`, chooses a compatible stable worker, verifies its
checksum and transport/backend report, and updates the managed installation
pointer. Compatibility is based on transport protocol and backend capability;
the worker need not have the same version string as the CLI. The command's
default URL retains the existing modeld release prefix; copying source does not
publish new artifacts there.

The platform matrix in `.github/workflows/release.yml` remains the supported CLI
matrix. `task modeld:check` compiles worker control paths for that same matrix
without native engines. This is a compile check, not evidence that native
libraries load or inference works on those machines. Native package smoke and
backend tests must run on the corresponding platform with its actual SDKs and
models before publishing that worker.

To verify the actual package installation using a local release store:

```sh
make push-modeld-release MODELD_RELEASE_S3_URI=/tmp/modeld-release-store
MODELD_TEST_RELEASE_DIR=/tmp/modeld-release-store go test ./internal/modeldinstall -run '^TestSystem_EnsureInstalled_NativeRelease$' -count=1
```

This starts a local HTTP server, downloads and checks the archive, probes the
worker after relocation into a temporary managed installation, and checks reuse.
It does not load a model or establish inference correctness.
