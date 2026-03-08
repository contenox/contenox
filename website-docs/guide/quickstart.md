# Quickstart

Get a working AI agent on your machine in 5 minutes.

## Prerequisites

- [Ollama](https://ollama.com) running locally with at least one model pulled:
  ```bash
  ollama pull qwen2.5:7b
  ```

## Install Contenox

**Linux/Ubuntu:**
```bash
TAG=$(curl -sL https://api.github.com/repos/contenox/contenox/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -sL "https://github.com/contenox/contenox/releases/download/${TAG}/contenox-${TAG}-linux-${ARCH}" -o contenox
chmod +x contenox && sudo mv contenox /usr/local/bin/contenox
contenox --version
```

**macOS:**
```bash
TAG=$(curl -sL https://api.github.com/repos/contenox/contenox/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/arm64/arm64/')
curl -sL "https://github.com/contenox/contenox/releases/download/${TAG}/contenox-${TAG}-darwin-${ARCH}" -o contenox
chmod +x contenox && sudo mv contenox /usr/local/bin/contenox
contenox --version
```

## Initialize a workspace

```bash
mkdir my-agent && cd my-agent
contenox init
```

This creates `.contenox/` with default chain files:
```
.contenox/
├── default-chain.json       ← default chat chain
└── default-run-chain.json   ← default run chain
```

Next, register a backend and set your default model:

```bash
# Local Ollama:
contenox backend add local --type ollama
contenox config set default-model qwen2.5:7b

# Or OpenAI:
# contenox backend add openai --type openai --api-key-env OPENAI_API_KEY
# contenox config set default-model gpt-4o
```

## Start chatting

```bash
contenox "what is the capital of France?"
# → Paris.

contenox "list files in the current directory" --shell
# → the model calls local_shell with `ls` and returns the result
```

`contenox` without a subcommand is interactive — type your message and press Enter. `Ctrl+D` exits.

## Run a chain explicitly

```bash
contenox run --chain .contenox/default-chain.json --input-type chat "explain recursion briefly"
```

## Add a remote API as a tool

```bash
# US National Weather Service — free, no API key
contenox hook add nws --url https://api.weather.gov --timeout 15000
contenox hook show nws       # lists 60 discovered tools

contenox run --chain .contenox/chain-nws.json --input-type chat \
  "how many active weather alerts are there right now?"
```

## Next steps

- [Core Concepts](/guide/concepts) — understand chains, tasks, and hooks
- [Chains reference](/chains/) — write your own chains
- [CLI reference](/reference/contenox-cli) — all flags and subcommands
