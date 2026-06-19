package modelregistry

const (
	toolProtocolLlamaCommonChat             = "llama:common_chat_tool_parser"
	toolProtocolOpenVINOJSONSchemaToolCalls = "openvino:json_schema_tool_calls"
	reasoningProtocolLlamaCommonChat        = "llama:common_chat_reasoning_parser"
	reasoningFormatDeepSeek                 = "deepseek"
)

var curatedModels = map[string]ModelDescriptor{
	// ── Qwen 3 ───────────────────────────────────────────────────────────────
	"qwen3-4b": {
		Name:              "qwen3-4b",
		SourceURL:         "https://huggingface.co/Qwen/Qwen3-4B-GGUF/resolve/main/Qwen3-4B-Q4_K_M.gguf",
		SizeBytes:         2_497_280_256,
		Curated:           true,
		ToolProtocol:      toolProtocolLlamaCommonChat,
		ReasoningProtocol: reasoningProtocolLlamaCommonChat,
		ReasoningFormat:   reasoningFormatDeepSeek,
	},
	"qwen3-8b": {
		Name:              "qwen3-8b",
		SourceURL:         "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q4_K_M.gguf",
		SizeBytes:         5_027_783_488,
		Curated:           true,
		ToolProtocol:      toolProtocolLlamaCommonChat,
		ReasoningProtocol: reasoningProtocolLlamaCommonChat,
		ReasoningFormat:   reasoningFormatDeepSeek,
	},
	"qwen3-14b": {
		Name:              "qwen3-14b",
		SourceURL:         "https://huggingface.co/Qwen/Qwen3-14B-GGUF/resolve/main/Qwen3-14B-Q4_K_M.gguf",
		SizeBytes:         9_001_752_960,
		Curated:           true,
		ToolProtocol:      toolProtocolLlamaCommonChat,
		ReasoningProtocol: reasoningProtocolLlamaCommonChat,
		ReasoningFormat:   reasoningFormatDeepSeek,
	},
	"qwen3-30b": {
		Name:              "qwen3-30b",
		SourceURL:         "https://huggingface.co/Qwen/Qwen3-30B-A3B-GGUF/resolve/main/Qwen3-30B-A3B-Q4_K_M.gguf",
		SizeBytes:         18_556_685_824,
		Curated:           true,
		ToolProtocol:      toolProtocolLlamaCommonChat,
		ReasoningProtocol: reasoningProtocolLlamaCommonChat,
		ReasoningFormat:   reasoningFormatDeepSeek,
	},
	"qwen3-coder-30b-a3b": {
		Name:         "qwen3-coder-30b-a3b",
		SourceURL:    "https://huggingface.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF/resolve/main/Qwen3-Coder-30B-A3B-Instruct-Q4_K_M.gguf",
		SizeBytes:    18_556_689_568,
		Curated:      true,
		ToolProtocol: toolProtocolLlamaCommonChat,
	},
	// ── Llama ────────────────────────────────────────────────────────────────
	"llama3.2-1b": {
		Name:      "llama3.2-1b",
		SourceURL: "https://huggingface.co/bartowski/Llama-3.2-1B-Instruct-GGUF/resolve/main/Llama-3.2-1B-Instruct-Q4_K_M.gguf",
		SizeBytes: 800_000_000,
		Curated:   true,
	},
	"llama4-scout": {
		Name:      "llama4-scout",
		SourceURL: "https://huggingface.co/bartowski/meta-llama_Llama-4-Scout-17B-16E-Instruct-GGUF/resolve/main/Llama-4-Scout-17B-16E-Instruct-Q4_K_M.gguf",
		SizeBytes: 67_550_000_000,
		Curated:   true,
	},
	// ── Google Gemma 3 ───────────────────────────────────────────────────────
	"gemma3-1b": {
		Name:      "gemma3-1b",
		SourceURL: "https://huggingface.co/google/gemma-3-1b-it-qat-q4_0-gguf/resolve/main/gemma-3-1b-it-q4_0.gguf",
		SizeBytes: 1_003_541_152,
		Curated:   true,
	},
	"gemma3-4b": {
		Name:      "gemma3-4b",
		SourceURL: "https://huggingface.co/google/gemma-3-4b-it-qat-q4_0-gguf/resolve/main/gemma-3-4b-it-q4_0.gguf",
		SizeBytes: 3_155_051_328,
		Curated:   true,
	},
	"gemma3-12b": {
		Name:      "gemma3-12b",
		SourceURL: "https://huggingface.co/google/gemma-3-12b-it-qat-q4_0-gguf/resolve/main/gemma-3-12b-it-q4_0.gguf",
		SizeBytes: 8_074_473_920,
		Curated:   true,
	},
	"gemma3-27b": {
		Name:      "gemma3-27b",
		SourceURL: "https://huggingface.co/google/gemma-3-27b-it-qat-q4_0-gguf/resolve/main/gemma-3-27b-it-q4_0.gguf",
		SizeBytes: 17_229_630_496,
		Curated:   true,
	},
	// ── Microsoft Phi 4 ──────────────────────────────────────────────────────
	"phi-4-mini": {
		Name:      "phi-4-mini",
		SourceURL: "https://huggingface.co/bartowski/microsoft_Phi-4-mini-instruct-GGUF/resolve/main/microsoft_Phi-4-mini-instruct-Q4_K_M.gguf",
		SizeBytes: 2_491_874_688,
		Curated:   true,
	},
	// ── DeepSeek ──────────────────────────────────────────────────────────────
	"deepseek-r1-0528-qwen3-8b": {
		Name:              "deepseek-r1-0528-qwen3-8b",
		SourceURL:         "https://huggingface.co/bartowski/deepseek-ai_DeepSeek-R1-0528-Qwen3-8B-GGUF/resolve/main/deepseek-ai_DeepSeek-R1-0528-Qwen3-8B-Q4_K_M.gguf",
		SizeBytes:         5_027_783_040,
		Curated:           true,
		ToolProtocol:      toolProtocolLlamaCommonChat,
		ReasoningProtocol: reasoningProtocolLlamaCommonChat,
		ReasoningFormat:   reasoningFormatDeepSeek,
	},
	"deepseek-r1-distill-qwen-7b": {
		Name:              "deepseek-r1-distill-qwen-7b",
		SourceURL:         "https://huggingface.co/bartowski/DeepSeek-R1-Distill-Qwen-7B-GGUF/resolve/main/DeepSeek-R1-Distill-Qwen-7B-Q4_K_M.gguf",
		SizeBytes:         4_683_073_504,
		Curated:           true,
		ToolProtocol:      toolProtocolLlamaCommonChat,
		ReasoningProtocol: reasoningProtocolLlamaCommonChat,
		ReasoningFormat:   reasoningFormatDeepSeek,
	},
	"deepseek-coder-v2-lite": {
		Name:      "deepseek-coder-v2-lite",
		SourceURL: "https://huggingface.co/bartowski/DeepSeek-Coder-V2-Lite-Instruct-GGUF/resolve/main/DeepSeek-Coder-V2-Lite-Instruct-Q4_K_M.gguf",
		SizeBytes: 10_364_416_768,
		Curated:   true,
	},
	// ── OpenAI open-weight ───────────────────────────────────────────────────
	"gpt-oss-20b": {
		Name:      "gpt-oss-20b",
		SourceURL: "https://huggingface.co/ggml-org/gpt-oss-20b-GGUF/resolve/main/gpt-oss-20b-mxfp4.gguf",
		SizeBytes: 12_109_566_560,
		Curated:   true,
	},
	// ── IBM Granite 3.2 ──────────────────────────────────────────────────────
	"granite-3.2-2b": {
		Name:      "granite-3.2-2b",
		SourceURL: "https://huggingface.co/bartowski/ibm-granite_granite-3.2-2b-instruct-GGUF/resolve/main/granite-3.2-2b-instruct-Q4_K_M.gguf",
		SizeBytes: 1_665_000_000,
		Curated:   true,
	},
	"granite-3.2-8b": {
		Name:      "granite-3.2-8b",
		SourceURL: "https://huggingface.co/bartowski/ibm-granite_granite-3.2-8b-instruct-GGUF/resolve/main/granite-3.2-8b-instruct-Q4_K_M.gguf",
		SizeBytes: 5_303_304_806,
		Curated:   true,
	},
	// ── Moonshot Kimi ─────────────────────────────────────────────────────────
	"kimi-linear": {
		Name:      "kimi-linear",
		SourceURL: "https://huggingface.co/bartowski/moonshotai_Kimi-Linear-48B-A3B-Instruct-GGUF/resolve/main/Kimi-Linear-48B-A3B-Instruct-Q4_K_M.gguf",
		SizeBytes: 30_060_000_000,
		Curated:   true,
	},
	// ── Tiny (testing) ───────────────────────────────────────────────────────
	"tiny": {
		Name:              "tiny",
		SourceURL:         "https://huggingface.co/Hjgugugjhuhjggg/FastThink-0.5B-Tiny-Q2_K-GGUF/resolve/main/fastthink-0.5b-tiny-q2_k.gguf",
		SizeBytes:         200_000_000,
		Curated:           true,
		ReasoningProtocol: reasoningProtocolLlamaCommonChat,
		ReasoningFormat:   reasoningFormatDeepSeek,
	},

	// ── OpenVINO IR (served by modeld in openvino mode) ───────────────────────
	// Multi-file IR repos pulled over the HF Hub HTTP API into models/openvino/.
	"qwen2.5-coder-0.5b-ov": {
		Name:         "qwen2.5-coder-0.5b-ov",
		Backend:      "openvino",
		Repo:         "OpenVINO/Qwen2.5-Coder-0.5B-Instruct-int4-ov",
		SourceURL:    "https://huggingface.co/OpenVINO/Qwen2.5-Coder-0.5B-Instruct-int4-ov",
		SizeBytes:    348_761_603,
		Curated:      true,
		ToolProtocol: toolProtocolOpenVINOJSONSchemaToolCalls,
	},
	"qwen2.5-coder-1.5b-ov": {
		Name:         "qwen2.5-coder-1.5b-ov",
		Backend:      "openvino",
		Repo:         "OpenVINO/Qwen2.5-Coder-1.5B-Instruct-int4-ov",
		SourceURL:    "https://huggingface.co/OpenVINO/Qwen2.5-Coder-1.5B-Instruct-int4-ov",
		SizeBytes:    941_408_897,
		Curated:      true,
		ToolProtocol: toolProtocolOpenVINOJSONSchemaToolCalls,
	},
	"gemma3-4b-ov": {
		Name:      "gemma3-4b-ov",
		Backend:   "openvino",
		Repo:      "OpenVINO/gemma-3-4b-it-int4-ov",
		SourceURL: "https://huggingface.co/OpenVINO/gemma-3-4b-it-int4-ov",
		SizeBytes: 3_504_681_112,
		Curated:   true,
	},
	"gemma3-12b-ov": {
		Name:      "gemma3-12b-ov",
		Backend:   "openvino",
		Repo:      "OpenVINO/gemma-3-12b-it-int4-ov",
		SourceURL: "https://huggingface.co/OpenVINO/gemma-3-12b-it-int4-ov",
		SizeBytes: 8_103_001_155,
		Curated:   true,
	},
	"phi-4-mini-ov": {
		Name:      "phi-4-mini-ov",
		Backend:   "openvino",
		Repo:      "OpenVINO/Phi-4-mini-instruct-int4-ov",
		SourceURL: "https://huggingface.co/OpenVINO/Phi-4-mini-instruct-int4-ov",
		SizeBytes: 2_388_940_940,
		Curated:   true,
	},
	"qwen3-4b-ov": {
		Name:         "qwen3-4b-ov",
		Backend:      "openvino",
		Repo:         "OpenVINO/Qwen3-4B-int4-ov",
		SourceURL:    "https://huggingface.co/OpenVINO/Qwen3-4B-int4-ov",
		SizeBytes:    2_290_768_181,
		Curated:      true,
		ToolProtocol: toolProtocolOpenVINOJSONSchemaToolCalls,
	},
	"qwen3-8b-ov": {
		Name:         "qwen3-8b-ov",
		Backend:      "openvino",
		Repo:         "OpenVINO/Qwen3-8B-int4-ov",
		SourceURL:    "https://huggingface.co/OpenVINO/Qwen3-8B-int4-ov",
		SizeBytes:    4_882_865_352,
		Curated:      true,
		ToolProtocol: toolProtocolOpenVINOJSONSchemaToolCalls,
	},
	"qwen3-14b-ov": {
		Name:         "qwen3-14b-ov",
		Backend:      "openvino",
		Repo:         "OpenVINO/Qwen3-14B-int4-ov",
		SourceURL:    "https://huggingface.co/OpenVINO/Qwen3-14B-int4-ov",
		SizeBytes:    9_731_519_596,
		Curated:      true,
		ToolProtocol: toolProtocolOpenVINOJSONSchemaToolCalls,
	},
	"qwen3-30b-ov": {
		Name:         "qwen3-30b-ov",
		Backend:      "openvino",
		Repo:         "OpenVINO/Qwen3-30B-A3B-int4-ov",
		SourceURL:    "https://huggingface.co/OpenVINO/Qwen3-30B-A3B-int4-ov",
		SizeBytes:    16_344_043_009,
		Curated:      true,
		ToolProtocol: toolProtocolOpenVINOJSONSchemaToolCalls,
	},
	"qwen3-coder-30b-a3b-ov": {
		Name:         "qwen3-coder-30b-a3b-ov",
		Backend:      "openvino",
		Repo:         "OpenVINO/Qwen3-Coder-30B-A3B-Instruct-int4-ov",
		SourceURL:    "https://huggingface.co/OpenVINO/Qwen3-Coder-30B-A3B-Instruct-int4-ov",
		SizeBytes:    16_344_057_522,
		Curated:      true,
		ToolProtocol: toolProtocolOpenVINOJSONSchemaToolCalls,
	},
	"deepseek-r1-distill-qwen-7b-ov": {
		Name:         "deepseek-r1-distill-qwen-7b-ov",
		Backend:      "openvino",
		Repo:         "OpenVINO/DeepSeek-R1-Distill-Qwen-7B-int4-ov",
		SourceURL:    "https://huggingface.co/OpenVINO/DeepSeek-R1-Distill-Qwen-7B-int4-ov",
		SizeBytes:    4_503_931_427,
		Curated:      true,
		ToolProtocol: toolProtocolOpenVINOJSONSchemaToolCalls,
	},
	"gpt-oss-20b-ov": {
		Name:      "gpt-oss-20b-ov",
		Backend:   "openvino",
		Repo:      "OpenVINO/gpt-oss-20b-int4-ov",
		SourceURL: "https://huggingface.co/OpenVINO/gpt-oss-20b-int4-ov",
		SizeBytes: 12_623_951_367,
		Curated:   true,
	},
}

type familyMapping struct {
	CanonicalName string
	Substrings    []string
}

var defaultFamilies = []familyMapping{
	// OpenVINO aliases first so backend-specific repo ids keep the -ov target.
	{CanonicalName: "qwen2.5-coder-1.5b-ov", Substrings: []string{"openvino/qwen2.5-coder-1.5", "qwen2.5-coder-1.5b-instruct-int4-ov", "qwen2.5-coder-1.5b-ov"}},
	{CanonicalName: "qwen2.5-coder-0.5b-ov", Substrings: []string{"openvino/qwen2.5-coder-0.5", "qwen2.5-coder-0.5b-instruct-int4-ov", "qwen2.5-coder-0.5b-ov"}},
	{CanonicalName: "qwen3-coder-30b-a3b-ov", Substrings: []string{"openvino/qwen3-coder-30", "qwen3-coder-30b-a3b-instruct-int4-ov", "qwen3-coder-30b-a3b-ov"}},
	{CanonicalName: "qwen3-30b-ov", Substrings: []string{"openvino/qwen3-30", "qwen3-30b-a3b-int4-ov", "qwen3-30b-ov"}},
	{CanonicalName: "qwen3-14b-ov", Substrings: []string{"openvino/qwen3-14", "qwen3-14b-int4-ov", "qwen3-14b-ov"}},
	{CanonicalName: "qwen3-8b-ov", Substrings: []string{"openvino/qwen3-8", "qwen3-8b-int4-ov", "qwen3-8b-ov"}},
	{CanonicalName: "qwen3-4b-ov", Substrings: []string{"openvino/qwen3-4", "qwen3-4b-int4-ov", "qwen3-4b-ov"}},
	{CanonicalName: "gemma3-12b-ov", Substrings: []string{"openvino/gemma-3-12", "gemma-3-12b-it-int4-ov", "gemma3-12b-ov"}},
	{CanonicalName: "gemma3-4b-ov", Substrings: []string{"openvino/gemma-3-4", "gemma-3-4b-it-int4-ov", "gemma3-4b-ov"}},
	{CanonicalName: "phi-4-mini-ov", Substrings: []string{"openvino/phi-4-mini", "phi-4-mini-instruct-int4-ov", "phi-4-mini-ov"}},
	{CanonicalName: "deepseek-r1-distill-qwen-7b-ov", Substrings: []string{"openvino/deepseek-r1-distill-qwen-7", "deepseek-r1-distill-qwen-7b-int4-ov", "deepseek-r1-distill-qwen-7b-ov"}},
	{CanonicalName: "gpt-oss-20b-ov", Substrings: []string{"openvino/gpt-oss-20b", "gpt-oss-20b-int4-ov", "gpt-oss-20b-ov"}},
	// Qwen 3 (checked before 2.5 to avoid substring collision).
	{CanonicalName: "qwen3-coder-30b-a3b", Substrings: []string{"qwen3-coder-30", "qwen3-coder", "qwen-coder"}},
	{CanonicalName: "qwen3-30b", Substrings: []string{"qwen3-30", "qwen3:30"}},
	{CanonicalName: "qwen3-14b", Substrings: []string{"qwen3-14", "qwen3:14"}},
	{CanonicalName: "qwen3-8b", Substrings: []string{"qwen3-8", "qwen3:8"}},
	{CanonicalName: "qwen3-4b", Substrings: []string{"qwen3-4", "qwen3:4"}},
	{CanonicalName: "qwen3-8b", Substrings: []string{"qwen3"}},
	// Gemma 3.
	{CanonicalName: "gemma3-27b", Substrings: []string{"gemma-3-27", "gemma3-27", "gemma3:27"}},
	{CanonicalName: "gemma3-12b", Substrings: []string{"gemma-3-12", "gemma3-12", "gemma3:12"}},
	{CanonicalName: "gemma3-1b", Substrings: []string{"gemma-3-1", "gemma3-1", "gemma3:1"}},
	{CanonicalName: "gemma3-4b", Substrings: []string{"gemma-3-4", "gemma3-4", "gemma3:4", "gemma-3", "gemma3", "gemma"}},
	// Phi 4.
	{CanonicalName: "phi-4-mini", Substrings: []string{"phi-4-mini", "phi4-mini", "phi4mini", "phi-4", "phi4"}},
	// DeepSeek.
	{CanonicalName: "deepseek-coder-v2-lite", Substrings: []string{"deepseek-coder-v2-lite", "deepseek-coder-v2", "deepseek-coder"}},
	{CanonicalName: "deepseek-r1-0528-qwen3-8b", Substrings: []string{"deepseek-r1-0528", "deepseek-r1-qwen3", "deepseek-r1", "deepseek"}},
	// OpenAI open-weight.
	{CanonicalName: "gpt-oss-20b", Substrings: []string{"openai/gpt-oss-20b", "openai_gpt-oss-20b", "gpt-oss-20b", "gpt-oss", "openai-oss", "openai oss"}},
	// Llama
	{CanonicalName: "llama4-scout", Substrings: []string{"llama-4", "llama4"}},
	{CanonicalName: "llama3.2-1b", Substrings: []string{"llama-3.2-1", "llama3.2-1", "llama3.2:1"}},
	// Granite
	{CanonicalName: "granite-3.2-8b", Substrings: []string{"granite-3.2-8", "granite-3.1", "granite3.", "granite"}},
	// Kimi
	{CanonicalName: "kimi-linear", Substrings: []string{"kimi"}},
}

const defaultFallback = "tiny"
