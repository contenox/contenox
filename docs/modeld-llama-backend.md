# Testing the llama modeld backend

This runbook verifies the local path:

```text
contenox CLI -> llama provider -> modeld daemon -> direct llama.cpp runtime
```

Use this when developing from source and you want to prove that Contenox is
talking to the local llama backend.

## Prerequisites

- Go 1.25+
- `make`
- `curl`, `git`, `gcc`, `g++`, and `cmake`
- For CUDA: CUDA toolkit with `nvcc` on `PATH`

From the repo root:

```bash
cd /home/naro/src/github.com/contenox/enterprise/runtime
make deps-llama-headers deps-llamacpp-ref
make build-contenox
```

## Start modeld

Run modeld in one terminal. CPU:

```bash
make run-modeld-llama
```

CUDA:

```bash
make run-modeld-llama-gpu
```

For CUDA, the startup log should show a CUDA device and the llama session config
should include nonzero GPU layers, usually:

```text
num_gpu_layers=999
```

Leave this terminal running while testing the CLI.

## Configure Contenox

In a second terminal:

```bash
cd /home/naro/src/github.com/contenox/enterprise/runtime

./bin/contenox init
./bin/contenox backend add llama --type llama --url "$HOME/.contenox/models/llama" || true
./bin/contenox config set default-provider llama
```

Pull a small or target model. To see curated options:

```bash
./bin/contenox model registry-list
```

Example:

```bash
./bin/contenox model pull qwen3-8b
./bin/contenox config set default-model qwen3-8b
```

For faster smoke tests on weak hardware, use a smaller curated model:

```bash
./bin/contenox model pull tiny
./bin/contenox config set default-model tiny
```

## Verify

Check that Contenox can see the local backend and model:

```bash
./bin/contenox model list
./bin/contenox doctor
```

Run a direct prompt through the configured defaults:

```bash
./bin/contenox run "say pong in one word" --trace
```

Or force the provider/model for this call:

```bash
./bin/contenox run "say pong in one word" --provider llama --model qwen3-8b --trace
```

## Useful modeld checks

While modeld is running:

```bash
./bin/modeld-llama status
```

If you started the CUDA target and want to watch placement:

```bash
nvidia-smi
```

## Notes

- Use provider `llama` for this path.
- `modeld` serves one compiled backend at a time. Use `make run-modeld-openvino`
  for OpenVINO testing.
- `make run-modeld-llama-gpu` builds and links the direct CUDA llama.cpp runtime
  from the pinned source tree.
