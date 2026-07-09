# Local Models (GGUF)

Run inference entirely on your own hardware — no Ollama, no API key. Contenox downloads GGUF (and OpenVINO IR) artifacts and serves them via the `modeld` daemon.

See the dedicated [modeld page](/docs/integrations/providers/modeld/) for architecture, capacity planning, remote nodes, and the daemon lifecycle.

For a deeper technical look at the lease system, capacity planner, slot model, and residency, read the [modeld Architecture page](/docs/integrations/providers/modeld-architecture/).

Contenox can download GGUF model files directly from HuggingFace and serve them via the built-in llama.cpp backend (powered by modeld).

## Curated models

Run `contenox model registry-list` to see all available models with sizes. The table below lists the curated set; approximate VRAM figures assume Q4\_K\_M quantization.

| Name             | Description                   | ~VRAM  |
| ---------------- | ----------------------------- | ------ |
| `tiny`           | FastThink 0.5B (testing only) | ~1 GB  |
| `llama3.2-1b`    | Llama 3.2 1B                  | ~1 GB  |
| `qwen2.5-1.5b`   | Qwen 2.5 1.5B                 | ~1 GB  |
| `granite-3.2-2b` | IBM Granite 3.2 2B            | ~1 GB  |
| `qwen3-4b`       | Qwen 3 4B                     | ~3 GB  |
| `gemma4-e2b`     | Gemma 4 E2B                   | ~3 GB  |
| `phi-4-mini`     | Microsoft Phi-4 Mini          | ~3 GB  |
| `gemma4-e4b`     | Gemma 4 E4B                   | ~5 GB  |
| `granite-3.2-8b` | IBM Granite 3.2 8B            | ~5 GB  |
| `qwen2.5-7b`     | Qwen 2.5 7B                   | ~5 GB  |
| `qwen3-14b`      | Qwen 3 14B                    | ~9 GB  |
| `qwen3-30b`      | Qwen 3 30B (MoE, fast)        | ~19 GB |
| `kimi-linear`    | Kimi Linear 48B (MoE)         | ~30 GB |
| `llama4-scout`   | Llama 4 Scout 17Bx16E         | ~68 GB |

> [!NOTE]
> Multi-GPU models (`llama4-scout`) require several GPUs or unified memory. MoE models (`qwen3-30b`, `kimi-linear`) use far less active VRAM than their parameter count suggests.

---

## 1. Download a model

Initialize the workspace first if you have not already:

```bash
contenox init
```

Then pick a model from the table and pull it. The file is stored at `~/.contenox/models/<name>/model.gguf`.

```bash
contenox model pull qwen3-4b
```

Progress is printed in-line. The download is resumable — if interrupted, re-run the same command.

---

## 2. What gets configured

`contenox init` creates the built-in `local` backend automatically. `contenox model pull` adds the model to the local registry and, on a fresh install, sets the first pulled model as `default-model`.

Contenox scans `~/.contenox/models/` and exposes every `*/model.gguf` it finds as a model name on the `local` provider.

---

## 3. Verify and run

```bash
contenox doctor
contenox "hello, what can you do?"
```

If you are switching back to local models after using a cloud provider, set the defaults explicitly:

```bash
contenox config set default-provider local
contenox config set default-model qwen3-4b
```

---

## Bring your own model

Any GGUF file hosted on HuggingFace (or any public URL) can be pulled by name:

```bash
contenox model pull my-model --url https://huggingface.co/org/repo/resolve/main/model.gguf
```

Use `/resolve/main/` (not `/blob/main/`) in the URL so HuggingFace serves the raw file.

After the download completes, the model is automatically registered in the local registry and available from the `local` backend.

---

## Registry management

The model registry is the authoritative name → URL index. Manage it from the CLI.

### CLI

```bash
contenox model registry-list          # list all curated + user-added entries
contenox model add my-model --url https://huggingface.co/org/repo/resolve/main/model.gguf
contenox model show my-model          # print registry details as JSON
contenox model remove my-model        # remove a user-added entry
```

Curated entries (`tiny`, `qwen3-4b`, etc.) cannot be removed — they are embedded in the binary.

---

## Next steps

- [CLI reference](/docs/reference/contenox-cli/) — full `contenox model` subcommand reference
- [Quickstart](/docs/guide/quickstart/) — wire the backend into your first agent
- [Core Concepts](/docs/guide/concepts/) — chains, tasks, tools
