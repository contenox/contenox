# Reasoning / Think Docs Handoff

This handoff covers documentation still needed for the runtime reasoning setting work.
Do not treat this document as a request to change capability detection code.

## Implemented Runtime Contract To Document

- Reasoning is controlled by a normalized `think` level:
  `auto`, `off`, `minimal`, `low`, `medium`, `high`, `xhigh`.
- Aliases:
  `false`, `none`, `disabled`, `disable`, `no`, `0` map to `off`;
  `true`, `on`, `yes`, `1` map to `high`.
- User-facing CLI and ACP agent chat turns default to `high`.
- Effective precedence:
  per-invocation or per-session override, then `default-think`, then hard default `high`.
- `auto` means do not send explicit provider reasoning controls and let the provider default apply.
- `off` means disable reasoning where the provider API supports a disable form.
- Built-in user-facing `chat_completion` tasks now receive `execute_config.think: "{{var:think}}"`.
- Routing, verification, and compaction control tasks should remain deterministic and should not receive `{{var:think}}` unless a chain author explicitly opts in.
- `MacroEnv` expands `{{var:think}}` in `execute_config.think`, matching `model` and `provider`.

## CLI Docs Needed

Update public CLI documentation to explain `--think <level>` as an activation/control flag, not only a display flag.

Important points:

- `contenox --think high "..."` and `contenox run --think high ...` force the effective level for that invocation.
- `--think off` disables reasoning controls for that invocation.
- `--think auto` omits provider-specific reasoning controls.
- When the effective level is not `off` or `auto`, streamed or returned reasoning/thinking is printed to stderr when available.
- `config set default-think <level>` stores the default used by future CLI invocations and ACP sessions.
- Existing `--trace`, `--steps`, and `--raw` behavior is separate from reasoning activation.

Suggested docs locations:

- `docs/contenox-cli.md`
- README quickstart/config examples if they list persistent config keys
- CLI generated/help docs if this repo publishes command reference output

## ACP Docs Needed

Document the session-local ACP command:

```text
/think
/think high
/think off
/think auto
```

Important points:

- `/think` without args reports the current session level.
- `/think <level>` changes only the current ACP session.
- The ACP `/think` setting is not persisted globally.
- New ACP sessions seed from `default-think`, or `high` when unset.
- ACP emits reasoning through existing `agent_thought_chunk` events when providers return thought chunks.
- Session replay continues to include stored thought chunks from messages with `Thinking`.

Suggested docs locations:

- ACP command/reference docs
- Any editor integration docs that list advertised slash commands

## Chain Author Docs Needed

Document the `think` field in `execute_config`:

```json
"execute_config": {
  "model": "{{var:model}}",
  "provider": "{{var:provider}}",
  "think": "{{var:think}}"
}
```

Important points:

- Use `{{var:think}}` for user-facing assistant turns that should follow CLI/ACP reasoning settings.
- Leave `think` unset for deterministic route, verification, label, and compaction tasks unless the chain intentionally wants reasoning there.
- Missing `{{var:think}}` now errors the same way missing `{{var:model}}` or `{{var:provider}}` does.

## Provider Docs Needed

Document provider request behavior at a high level only:

- OpenAI maps supported levels to `reasoning_effort`.
- Gemini direct maps Gemini 2.5 style models to thinking budget controls and Gemini 3 style models to thinking level controls.
- Vertex Google follows Gemini request-shape parity.
- Anthropic maps older extended-thinking models to manual budgets and newer adaptive-thinking APIs to adaptive thinking/effort controls.
- Ollama sends the native `think` request option.
- vLLM sends OpenAI-compatible `reasoning_effort` and chat template thinking controls.
- Bedrock and local should not be documented as supporting reasoning controls unless a concrete request API is implemented.

Avoid documenting unsupported or guessed model support as a guarantee.

## Capability Detection Handoff

Capability detection is intentionally left for another session.

The important design constraint from the codebase is:

- Do not hardcode model names to advertise `CanThink`.
- Provider `CanThink()` should come from explicit `CapabilityConfig` or concrete backend/catalog metadata.
- If a provider API does not expose reliable thinking capability metadata, leave `CanThink` false unless the user or registry has explicitly configured it.

Separate follow-up work should audit any temporary model-name capability inference and replace it with the repo's established capability flow.

## EE Docs Handoff

Enterprise Edition docs should be updated separately if they maintain their own command reference, ACP integration guide, model registry guide, or policy docs.

Likely EE doc areas:

- Admin model capability configuration
- ACP slash-command reference
- Agent/session defaults
- Provider/backend capability inventory
- Policy examples that distinguish user-facing chat tasks from route/compact/control tasks

Keep EE docs aligned with the OSS runtime contract above, but document EE-only controls in the EE docs session.
