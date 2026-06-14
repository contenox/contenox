# Inline Autocomplete

Contenox requests inline editor suggestions from a **separate, FIM-capable
autocomplete model** — not your chat model. Set one before expecting suggestions:

```sh
contenox config set default-autocomplete-provider mistral
contenox config set default-autocomplete-model codestral-latest
```

With no autocomplete model configured, no ghost text appears. Run
`Contenox: Test Autocomplete At Cursor` to check what the model returned.

Useful commands:

- `Contenox: Test Autocomplete At Cursor`
- `Contenox: Trigger Autocomplete`
- `Contenox: Enable Autocomplete`
- `Contenox: Disable Autocomplete`

Autocomplete uses workspace settings for prefix length, suffix length, debounce,
and maximum output tokens.
