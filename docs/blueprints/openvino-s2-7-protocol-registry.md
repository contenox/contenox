# OpenVINO S2.7 Parser Protocol Registry Log

Date: 2026-06-15

This log records the tool-call and reasoning parser work after S2 prefix reuse.
The decision from this session is strict:

- no model-output regex fallback;
- no chat-template scraping as a parser contract;
- no Contenox-invented tool-call schema;
- tool-call parsing requires a model/profile-declared OpenVINO parser protocol;
- reasoning parsing uses the same registry and bridge as tool-call parsing.

## Problem

OpenVINO GenAI already ships parser and structured-output primitives, including
Llama 3 tool parsers, reasoning parsers, Python `VLLMParserWrapper`, and XGrammar
structured-output support. The runtime must select those primitives by declared
protocol name instead of hardcoding how a specific model happens to emit tool
calls.

The local Qwen chat template says it emits:

```text
<tool_call>
{"name": <function-name>, "arguments": <args-json-object>}
</tool_call>
```

That is useful model text, but it is not a stable machine-readable parser API.
The runtime no longer parses that with a local regex.

## Step Log

### 1. Audited OpenVINO GenAI Parser Surface

Verified against local OpenVINO GenAI `2026.2.0.0` headers:

- `ov::genai::Parser`
- `ReasoningParser`
- `DeepSeekR1ReasoningParser`
- `Phi4ReasoningParser`
- `Llama3PythonicToolParser`
- `Llama3JsonToolParser`
- `IncrementalParser`
- `ReasoningIncrementalParser`
- `DeepSeekR1ReasoningIncrementalParser`
- `Phi4ReasoningIncrementalParser`

Also confirmed `VLLMParserWrapper` exists in the Python binding, not the public
C++ parser header used by this Go/C++ session bridge.

### VLLMParserWrapper Gotcha

`VLLMParserWrapper` does not mean OpenVINO embeds vLLM as an inference backend,
and it does not mean there is a native C++ parser class we can instantiate from
Go.

In OpenVINO GenAI `src/python/py_parsers.cpp`, `VLLMParserWrapper` is a Python
binding adapter:

- it accepts an already constructed Python parser object from
  `vllm.entrypoints.openai.tool_parsers.*` or vLLM reasoning parser modules;
- if that Python object has `extract_tool_calls`, OpenVINO calls it;
- if that Python object has `extract_reasoning`, OpenVINO calls it;
- OpenVINO converts the Python result back into `JsonContainer`;
- the wrapper is exported to Python as `openvino_genai.VLLMParserWrapper`.

Operational consequences for Contenox:

- a native Go/C++ OpenVINO session cannot create `VLLMParserWrapper` from only a
  protocol string;
- using it requires a Python runtime, vLLM installed, a concrete vLLM parser
  object, tokenizer setup when that parser needs one, and GIL-managed calls;
- `openvino:vllm_parser_wrapper` is therefore registered as an explicit profile
  name, but the current native bridge returns a clear error instead of pretending
  it can run;
- supporting it for real means either adding a Python parser-object bridge,
  upstreaming/exposing a native C++ equivalent, or choosing a different native
  parser protocol for that model.

This is compatibility reuse of vLLM's parser ecosystem, not a portable
model-native parser contract by itself.

### 2. Added Profile Declarations

File:

- `runtime/modelrepo/openvino/profile.go`

New strict profile fields:

```json
{
  "tool_calls": {"protocol": "openvino:llama3_json_tool_parser"},
  "reasoning": {"protocol": "openvino:deepseek_r1_reasoning_parser"}
}
```

Validation now rejects unknown protocol names while loading
`contenox-openvino.json`. `tool_calls.protocol` accepts parser protocols only;
`openvino:regex` and other structured-output primitives are not accepted as
tool-call parsers.

### 3. Added Protocol Registry

File:

- `runtime/modelrepo/openvino/protocols.go`

Registered tool parser protocols:

- `openvino:llama3_pythonic_tool_parser`
- `openvino:llama3_json_tool_parser`
- `openvino:vllm_parser_wrapper`

Registered reasoning parser protocols:

- `openvino:reasoning_parser`
- `openvino:deepseek_r1_reasoning_parser`
- `openvino:phi4_reasoning_parser`
- `openvino:reasoning_incremental_parser`
- `openvino:deepseek_r1_reasoning_incremental_parser`
- `openvino:phi4_reasoning_incremental_parser`
- `openvino:vllm_parser_wrapper`

Registered structured-output protocols at the `ovsession` generation layer:

- `openvino:json_schema`
- `openvino:regex`
- `openvino:ebnf`
- `openvino:qwen_xml_parameters`
- `openvino:structural_tag`
- `openvino:triggered_tags`
- `openvino:tags_with_separator`
- `openvino:concat`
- `openvino:union`
- `openvino:const_string`
- `openvino:any_text`

Structured output constrains generation; it does not by itself produce neutral
`modelrepo.ToolCall` values. Tool-call extraction still needs a parser protocol.

### 4. Removed Qwen Regex Parsing

File:

- `runtime/modelrepo/openvino/tools.go`

Removed the local `parseQwenToolCalls` path. The only remaining tool handling in
this file is serialization of tool definitions into the model chat template's
`tools` argument.

### 5. Wired Provider Chat Through Parser Protocols

File:

- `runtime/modelrepo/openvino/client.go`

`Chat` now:

- serializes tool definitions for the model chat template;
- fails fast if tools are requested and the model profile does not declare
  `tool_calls.protocol`;
- passes declared tool and reasoning parser protocols into `ovsession.Generate`;
- decodes parser JSON into `Message.Content`, `Message.Thinking`, and
  `Message.ToolCalls`.

No parser protocol means no tool call extraction. There is no hidden fallback.

### 6. Extended The Native GenAI ABI

Files:

- `runtime/modelrepo/openvino/ovsession/genai.h`
- `runtime/modelrepo/openvino/ovsession/genai.go`
- `runtime/modelrepo/openvino/ovsession/genai_stub.go`
- `runtime/modelrepo/openvino/ovsession/genai.cpp`

`GenerateOptions` now carries:

- `ParserProtocols []string`
- `StructuredOutput {Protocol, Payload}`

`GenAIResult` now carries:

- raw `Text`
- parsed `ParsedJSON`
- `Metrics`

The C ABI now passes parser protocol names and an output buffer for parsed JSON.

### 7. Implemented C++ Parser Bridge

File:

- `runtime/modelrepo/openvino/ovsession/genai.cpp`

The bridge instantiates complete-output OpenVINO parser classes before
generation:

- `openvino:llama3_pythonic_tool_parser` ->
  `ov::genai::Llama3PythonicToolParser`
- `openvino:llama3_json_tool_parser` ->
  `ov::genai::Llama3JsonToolParser`
- `openvino:reasoning_parser` -> `ov::genai::ReasoningParser`
- `openvino:deepseek_r1_reasoning_parser` ->
  `ov::genai::DeepSeekR1ReasoningParser`
- `openvino:phi4_reasoning_parser` -> `ov::genai::Phi4ReasoningParser`

Explicit non-support cases:

- `openvino:vllm_parser_wrapper` returns a clear native-bridge error because
  OpenVINO exposes it in Python, not in the C++ header used here.
- Incremental reasoning parser protocols return a clear non-stream-chat error
  because they require the streaming parser bridge.

The complete-output parsers run on a `JsonContainer` seeded with `content` and
return the resulting JSON separately from raw generated text.

### 8. Added Structured-Output Bridge

File:

- `runtime/modelrepo/openvino/ovsession/genai.cpp`

The lower session API can now apply OpenVINO structured-output primitives:

- JSON Schema
- Regex
- EBNF
- Qwen XML parameters format
- structural tag JSON
- triggered tags
- tags with separator
- concat
- union
- const string
- any text

This is generation control, not a tool-call parser. The runtime keeps that
boundary explicit.

### 9. Normalized Parser Output

File:

- `runtime/modelrepo/openvino/protocols.go`

`decodeParsedGeneration` normalizes parser JSON into runtime messages:

- `content` -> assistant visible text
- `reasoning_content` or `reasoning` -> `Message.Thinking`
- direct tool calls: `{"name": "...", "arguments": {...}}`
- wrapped tool calls:
  `{"type":"function","function":{"name":"...","arguments":{...}}}`

This normalization operates on OpenVINO parser output, not raw model text.

### 10. Added Tests

Files:

- `runtime/modelrepo/openvino/tools_test.go`
- `runtime/modelrepo/openvino/profile_test.go`

Coverage added:

- strict profile loading with `tool_calls` and `reasoning` protocol fields;
- rejection of `openvino:regex` as `tool_calls.protocol`;
- parser-output normalization for direct tool calls;
- parser-output normalization for wrapped tool calls;
- reasoning output normalization.

## Verification

Default build:

```sh
go test ./runtime/modelrepo/openvino/...
```

Result:

```text
ok   github.com/contenox/runtime/runtime/modelrepo/openvino
?    github.com/contenox/runtime/runtime/modelrepo/openvino/ovsession [no test files]
```

Full OpenVINO GenAI build:

```sh
make -f Makefile.openvino test-s1-5
```

Result:

```text
ok   github.com/contenox/runtime/runtime/modelrepo/openvino          30.609s
ok   github.com/contenox/runtime/runtime/modelrepo/openvino/ovsession 157.973s
```

Post-cleanup native compile check:

```sh
make -f Makefile.openvino test-s1
```

Result:

```text
--- PASS: TestSystem_OpenVINOGenAI_SchedulerControlsReachable
PASS
```

## Current State

Done:

- profile-declared tool parser protocol slot;
- profile-declared reasoning parser protocol slot;
- shared registry pattern for tool and reasoning parser names;
- native C++ bridge for complete-output OpenVINO parser classes;
- native stream bridge for incremental reasoning parser classes, carrying
  `reasoning_content` as `StreamParcel.Thinking`;
- native structured-output bridge at the generation layer;
- removal of local Qwen regex tool parsing;
- parser-output normalization into `modelrepo.Message`.

Still open:

- C++ native support for `VLLMParserWrapper`, unless OpenVINO exposes it outside
  Python or this runtime adds a Python parser-object bridge;
- a model profile for each actual model family that declares the correct parser
  protocol;
- a real Qwen tool-call parser primitive from OpenVINO or an explicit
  model-profile parser adapter. Until then, the Qwen template text is rendered
  for the model, but the runtime will not guess a parser for it.
