# Plan: modeld Interface Boundary — State vs. Compute

> **Status:** decision blueprint, drafted 2026-06-17.
> **Context:** This supersedes the initial `modeld` abstraction which attempted to push the standard `modelprovider` interface across the local daemon IPC boundary.

## Problem Statement

We are introducing `modeld` as an in-process local runtime owner to prevent multiple frontends (VS Code, Zed, CLI) from fighting over local hardware resources (GPU VRAM, System RAM). 

In the initial prototype, we moved *all* model providers (OpenAI, Gemini, Anthropic, OpenVINO, Llama.cpp) behind the `modeld` IPC boundary, and exposed the standard, stateless `modelprovider` interface (e.g., `Chat(ctx, Request{Messages}) -> Response`) over gRPC.

This approach failed for two structural reasons:

1. **Cloud providers don't need a local daemon.** Routing stateless HTTP API calls to Anthropic or OpenAI through a local gRPC IPC hop is architectural theater. There is no local VRAM to protect and no local KV cache to keep warm.
2. **Stateless APIs break local hardware economics.** The `modelprovider` interface is inherently stateless—the client sends the *entire* conversation history on every turn. If `modeld` sits behind this interface, it must parse strings, apply chat templates, tokenize the text, and attempt to dynamically match the resulting token array against hardware KV cache slots to figure out what needs to be evaluated. This hides the actual mechanics of local inference and forces the daemon to understand high-level agent logic.

## What We Assumed

We assumed that because the execution engine (the ACP agent, workflow runner) wants a unified way to talk to *any* AI (local or remote), that unified boundary (`modelprovider`) should also be the IPC boundary for the daemon.

We assumed that the daemon's job was to "be an AI API." 

## Why It Is Wrong

The daemon's job is not to be an AI API. The daemon's job is to be a **hardware allocator and compute scheduler**.

By putting `modelprovider` at the daemon boundary:
*   **We obscure context state:** A stateless API hides the KV cache. The daemon is forced into expensive heuristic diffing to realize that the first 4,000 tokens of a request are identical to a previous request.
*   **We obscure memory lifecycle:** A stateless `Chat()` call does not express "reserve 8GB of VRAM for the next 5 minutes" or "this model is currently loading into memory."
*   **We force string/token translation into the wrong layer:** The daemon has to hold tokenizers and chat templates (ChatML, Zephyr, etc.). The daemon should not care what a "User Message" or a "Tool Call" is; it should only care about tensors and tokens.

## What We Need

We need a strictly defined separation of concerns:

1.  **The Engine (Runtime):** Cares about Messages, Tool Calls, JSON schemas, and stateless completion requests.
2.  **The Wrapper (`runtime/modelrepo/local`):** Implements the `modelprovider` interface for the engine. It holds the tokenizer, applies the chat template, converts strings to integer tokens, and manages long-lived "Sessions" with the daemon.
3.  **The Daemon (`modeld`):** A dumb, stateful hardware manager. It allocates VRAM (Models), manages KV cache slots (Sessions), ingests integers (Tokens), and samples integers (Tokens).

## Options for the `modeld` Boundary

### Option A: The "Local OpenAI" (Status Quo)
The daemon implements `Chat(Messages)`. The frontend sends full string histories. The daemon tokenizes, diffs the history against its KV cache, and runs inference.
*   *Pros:* Simplest for the frontend to call.
*   *Cons:* Inefficient KV cache utilization, forces tokenization and templating into the daemon, obscures VRAM loading states. 

### Option B: The "Token Streamer"
The daemon accepts arrays of Tokens, but remains stateless. The frontend tokenizes the full history and sends `Evaluate([token1, token2, ..., tokenN])`. The daemon still has to diff the token array to find a matching KV cache prefix.
*   *Pros:* Moves tokenization and templating out of the daemon.
*   *Cons:* Still stateless; the daemon has to guess what the frontend is trying to do with the KV cache. No explicit session management.

### Option C: The "Compute \u0026 Context Allocator" (Recommended)
The daemon exposes a highly stateful, low-level API. The frontend explicitly allocates a Session (KV cache slot). On subsequent turns, the frontend only sends the *delta* tokens to the daemon for that specific Session. 
*   *Pros:* Perfect mapping to how local inference hardware actually works (llama.cpp `llama_context`, OpenVINO state). Allows advanced features like Session cloning/forking (evaluating three different tool calls without re-evaluating the prompt). 
*   *Cons:* The `runtime` wrapper becomes more complex, as it has to manage Session IDs and track token offsets across conversation turns.

## Recommended Path

1.  **Revert Cloud Providers:** Keep all remote, stateless API providers (OpenAI, Gemini, Anthropic) in `runtime/modelrepo`. They bypass `modeld` entirely.
2.  **Redesign `modeld` Interface:** Design `modeld/transport` around **Option C**. The interface should deal in `LoadModel`, `CreateSession`, `Evaluate(Tokens)`, and `Sample(Tokens)`. 
3.  **Build the Translation Layer:** Create a specific `modelprovider` implementation in the runtime that bridges the stateless `Chat()` request to the highly stateful `modeld` token API.