# Autonomous Planning Recipes

Contenox `plan` mode turns a high-level goal into a sequence of verified steps. It's an agentic loop: it plans, you approve (or use `--auto`), and it executes using local tools.

---

## Scaffolding a new project

Instead of manually creating directories and files, describe your project structure and let Contenox build it.

**Create a plan for a new Go service:**

```bash
contenox plan new "scaffold a new Go project named 'weather-api' with a cmd/ main.go, an internal/ weather package, and a Dockerfile"
```

**Review and execute:**

```bash
# See what it planned
contenox plan show

# Execute the first step
contenox plan next --shell

# Or run the whole plan autonomously
contenox plan next --auto --shell
```

---

## Batch Documentation Generation

Generate `README.md` or `DEVELOPMENT.md` files for subdirectories by having the agent analyze each folder's contents.

```bash
contenox plan new "crawl the internal/ directory and create a README.md in each subdirectory explaining its purpose and main symbols"
```

---

## How it works

Unlike `run` (stateless) or `chat` (conversational), `plan` is **stateful and persistent**:

1.  **Generation:** The model breaks your goal into discrete, actionable steps stored in a local SQLite database.
2.  **State Management:** Each step tracks its own status (`pending`, `running`, `completed`, `failed`).
3.  **Tool-Augmented Execution:** The executor uses the `chain-step-executor.json` which is optimized for calling `local_shell` and `local_fs` tools.
4.  **Verification:** The model is instructed to verify the result of a tool call before marking a step as done.

---

## Tips for Reliable Planning

-   **Enable Shell Access:** Most plans require `--shell` to actually perform work on the filesystem.
-   **Granular Goals:** If a plan feels too large, the model might miss details. Break complex projects into smaller plans.
-   **Interactive Correction:** If a step fails, use `contenox plan retry <N>` or `contenox plan replan` to let the model adjust the remaining steps based on the new state.
-   **Security:** Always review the plan steps with `contenox plan show` before running with `--auto --shell`.
