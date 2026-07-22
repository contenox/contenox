# Local Models (GGUF)

Run inference entirely on your own hardware — no Ollama, no API key. Contenox downloads GGUF (and OpenVINO IR) artifacts and serves them via the `modeld` daemon.

See the dedicated [modeld page](/docs/integrations/providers/modeld/) for architecture, capacity planning, remote nodes, and the daemon lifecycle.

For a deeper technical look at the lease system, capacity planner, slot model, and residency, read the [modeld Architecture page](/docs/integrations/providers/modeld-architecture/).

Contenox can download GGUF model files directly from HuggingFace and serve them via the `llama` backend (GGUF) or the `openvino` backend (OpenVINO IR) — both run in the `modeld` daemon, not in the `contenox` binary itself.

## Curated models

Run `contenox model registry-list` to see all available models with sizes, use cases, and best-effort local fit. The table below lists the curated **GGUF** set (served by the `llama` backend); the "advisory VRAM" column is the recommended minimum GPU tier at the curated quantization.

| Name | Model | Use case | Advisory VRAM |
| ---- | ----- | -------- | ------------- |
| `granite-3.2-2b` | IBM Granite 3.2 2B | chat | ~6 GB |
| `phi-4-mini` | Phi-4 Mini | chat | ~6 GB |
| `qwen3-4b` | Qwen 3 4B | chat | ~6 GB |
| `qwen2.5-coder-0.5b` | Qwen 2.5 Coder 0.5B | coding (smoke) | ~6 GB |
| `qwen2.5-coder-1.5b` | Qwen 2.5 Coder 1.5B | coding | ~6 GB |
| `qwen2.5-coder-3b` | Qwen 2.5 Coder 3B | coding | ~6 GB |
| `gemma4-e4b` | Gemma 4 E4B | chat + vision | ~6 GB |
| `gemma4-e2b` | Gemma 4 E2B | chat + vision | ~8 GB |
| `granite-3.2-8b` | IBM Granite 3.2 8B | chat | ~8 GB |
| `qwen3-8b` | Qwen 3 8B | chat | ~8 GB |
| `qwen2.5-coder-7b` | Qwen 2.5 Coder 7B | coding (default) | ~8 GB |
| `starcoder2-7b-instruct` | StarCoder2 7B Instruct | coding (FIM) | ~8 GB |
| `deepseek-r1-0528-qwen3-8b` | DeepSeek R1 0528 (Qwen3 8B distill) | reasoning | ~8 GB |
| `deepseek-r1-distill-qwen-7b` | DeepSeek R1 Distill Qwen 7B | reasoning | ~8 GB |
| `gemma4-12b` | Gemma 4 12B | chat + vision | ~12 GB |
| `qwen3-14b` | Qwen 3 14B | chat | ~16 GB |
| `qwen2.5-coder-14b` | Qwen 2.5 Coder 14B | coding | ~16 GB |
| `gpt-oss-20b` | GPT-OSS 20B | chat | ~24 GB |
| `deepseek-coder-v2-lite` | DeepSeek Coder V2 Lite (MoE) | coding | ~24 GB |
| `codestral-22b` | Codestral 22B | coding (FIM) | ~24 GB |
| `devstral-small-2507` | Devstral Small 2507 | agentic coding | ~24 GB |
| `qwen3-30b` | Qwen 3 30B-A3B (MoE) | reasoning | ~32 GB |
| `qwen3-coder-30b-a3b` | Qwen 3 Coder 30B-A3B (MoE) | coding | ~32 GB |
| `qwen2.5-coder-32b` | Qwen 2.5 Coder 32B | coding | ~32 GB |
| `gemma4-26b-a4b` | Gemma 4 26B-A4B (MoE) | chat | ~32 GB |

> [!NOTE]
> Most curated GGUF models also ship an **OpenVINO IR** counterpart for the `openvino` backend — same name with an `-ov` suffix (e.g. `qwen3-4b-ov`, `qwen2.5-coder-7b-ov`, `gemma4-e4b-ov`). MoE models (`qwen3-30b`, `qwen3-coder-30b-a3b`, `gemma4-26b-a4b`) use far less active VRAM than their total parameter count suggests. Run `contenox model registry-list` for the authoritative, always-current list including the OpenVINO entries.

### Vision models

Curated entries marked **chat + vision** accept image input in addition to text. One `model pull` installs everything vision needs:

- **`llama` backend** — the pull fetches the model GGUF *and* its multimodal projector, stored beside it as `mmproj.gguf`. If the projector download fails, the pull fails loudly (never a silently text-only model); re-run the same pull to fetch it. A model pulled before it was curated for vision upgrades in place: re-running `model pull` adds the missing projector.
- **`openvino` backend** — the multi-file snapshot already includes the vision encoder (`openvino_vision_embeddings_model.*`); nothing extra to fetch.

```bash
# 6 GB GPU tier — flagship vision default:
contenox model pull gemma4-e4b

# 12-16 GB GPU tier:
contenox model pull gemma4-12b

# CPU / iGPU via OpenVINO (needs ~7GB+ free memory):
contenox model pull gemma4-e4b-ov

# Smallest vision model (~0.8 GB, OpenVINO) for trying image input on small machines:
contenox model pull internvl2-1b-ov
```

`contenox model list` shows a VISION column with the capability the running daemon actually reports for each model — the truth comes from `modeld` resolving the projector (or vision encoder), not from the model's name.

> [!NOTE]
> OpenVINO vision sessions run through the GenAI VLM pipeline, which in its first version has no prefix-cache reuse, no context offload, no snapshot/restore, and no tool calls or LoRA — every turn re-processes the full multimodal prompt. Text-only OpenVINO models keep all of those features; the llama backend serves vision with its usual session features.

---

## 1. Download a model

Initialize the workspace first if you have not already:

```bash
contenox init
```

Then pick a model from the table and pull it. GGUF files are stored at `~/.contenox/models/llama/<name>/model.gguf`; OpenVINO IR models (curated names ending in `-ov`) are fetched into `~/.contenox/models/openvino/<name>/`.

```bash
contenox model pull qwen3-4b
```

Progress is printed in-line. The download is resumable — if interrupted, re-run the same command.

---

## 2. What gets configured

`contenox init` registers the `llama` and `openvino` backends automatically. `contenox model pull` adds the model to the local registry and, on a fresh install, sets the first pulled model as `default-model`.

Contenox scans `~/.contenox/models/llama/` and exposes every `*/model.gguf` it finds as a model name on the `llama` provider (and `~/.contenox/models/openvino/` for the `openvino` provider).

---

## 3. Verify and run

```bash
contenox doctor
contenox "hello, what can you do?"
```

If you are switching back to local models after using a cloud provider, set the defaults explicitly:

```bash
contenox config set default-provider llama
contenox config set default-model qwen3-4b
```

---

## Bring your own model

Any GGUF file hosted on HuggingFace (or any public URL) can be pulled by name:

```bash
contenox model pull my-model --url https://huggingface.co/org/repo/resolve/main/model.gguf
```

Use `/resolve/main/` (not `/blob/main/`) in the URL so HuggingFace serves the raw file.

After the download completes, the model is automatically registered in the local registry and available from the `llama` backend.

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

Curated entries (`qwen3-4b`, `granite-3.2-2b`, etc.) cannot be removed — they are embedded in the binary.

---

## Next steps

- [CLI reference](/docs/reference/contenox-cli/) — full `contenox model` subcommand reference
- [Quickstart](/docs/guide/quickstart/) — wire the backend into your first agent
- [Core Concepts](/docs/guide/concepts/) — chains, tasks, tools
