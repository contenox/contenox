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
	// ── Google Gemma 4 ───────────────────────────────────────────────────────
	"gemma4-e2b": {
		Name:         "gemma4-e2b",
		SourceURL:    "https://huggingface.co/ggml-org/gemma-4-E2B-it-GGUF/resolve/main/gemma-4-E2B-it-Q8_0.gguf",
		SizeBytes:    4_967_494_592,
		Curated:      true,
		ToolProtocol: toolProtocolLlamaCommonChat,
	},
	"gemma4-e4b": {
		Name:         "gemma4-e4b",
		SourceURL:    "https://huggingface.co/ggml-org/gemma-4-E4B-it-GGUF/resolve/main/gemma-4-E4B-it-Q4_K_M.gguf",
		SizeBytes:    5_335_289_824,
		Curated:      true,
		ToolProtocol: toolProtocolLlamaCommonChat,
	},
	"gemma4-12b": {
		Name:         "gemma4-12b",
		SourceURL:    "https://huggingface.co/ggml-org/gemma-4-12B-it-GGUF/resolve/main/gemma-4-12B-it-Q4_K_M.gguf",
		SizeBytes:    7_381_382_048,
		Curated:      true,
		ToolProtocol: toolProtocolLlamaCommonChat,
	},
	"gemma4-26b-a4b": {
		Name:         "gemma4-26b-a4b",
		SourceURL:    "https://huggingface.co/ggml-org/gemma-4-26B-A4B-it-GGUF/resolve/main/gemma-4-26B-A4B-it-Q4_K_M.gguf",
		SizeBytes:    16_796_015_136,
		Curated:      true,
		ToolProtocol: toolProtocolLlamaCommonChat,
	},
	// ── Microsoft Phi 4 ──────────────────────────────────────────────────────
	"phi-4-mini": {
		Name:         "phi-4-mini",
		SourceURL:    "https://huggingface.co/bartowski/microsoft_Phi-4-mini-instruct-GGUF/resolve/main/microsoft_Phi-4-mini-instruct-Q4_K_M.gguf",
		SizeBytes:    2_491_874_688,
		Curated:      true,
		ToolProtocol: toolProtocolLlamaCommonChat,
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
	// ── OpenAI open-weight ───────────────────────────────────────────────────
	"gpt-oss-20b": {
		Name:         "gpt-oss-20b",
		SourceURL:    "https://huggingface.co/ggml-org/gpt-oss-20b-GGUF/resolve/main/gpt-oss-20b-mxfp4.gguf",
		SizeBytes:    12_109_566_560,
		Curated:      true,
		ToolProtocol: toolProtocolLlamaCommonChat,
	},
	// ── IBM Granite 3.2 ──────────────────────────────────────────────────────
	"granite-3.2-2b": {
		Name:         "granite-3.2-2b",
		SourceURL:    "https://huggingface.co/bartowski/ibm-granite_granite-3.2-2b-instruct-GGUF/resolve/main/ibm-granite_granite-3.2-2b-instruct-Q4_K_M.gguf",
		SizeBytes:    1_545_296_512,
		Curated:      true,
		ToolProtocol: toolProtocolLlamaCommonChat,
	},
	"granite-3.2-8b": {
		Name:         "granite-3.2-8b",
		SourceURL:    "https://huggingface.co/bartowski/ibm-granite_granite-3.2-8b-instruct-GGUF/resolve/main/ibm-granite_granite-3.2-8b-instruct-Q4_K_M.gguf",
		SizeBytes:    4_942_859_808,
		Curated:      true,
		ToolProtocol: toolProtocolLlamaCommonChat,
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
	"tinyllama-1.1b-chat-v1.0-int4-ov": {
		Name:      "tinyllama-1.1b-chat-v1.0-int4-ov",
		Backend:   "openvino",
		Repo:      "OpenVINO/TinyLlama-1.1B-Chat-v1.0-int4-ov",
		SourceURL: "https://huggingface.co/OpenVINO/TinyLlama-1.1B-Chat-v1.0-int4-ov",
		SizeBytes: 636_000_000,
		Curated:   true,
	},
	"phi-4-mini-ov": {
		Name:         "phi-4-mini-ov",
		Backend:      "openvino",
		Repo:         "OpenVINO/Phi-4-mini-instruct-int4-ov",
		SourceURL:    "https://huggingface.co/OpenVINO/Phi-4-mini-instruct-int4-ov",
		SizeBytes:    2_388_940_940,
		Curated:      true,
		ToolProtocol: toolProtocolOpenVINOJSONSchemaToolCalls,
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
		Name:         "gpt-oss-20b-ov",
		Backend:      "openvino",
		Repo:         "OpenVINO/gpt-oss-20b-int4-ov",
		SourceURL:    "https://huggingface.co/OpenVINO/gpt-oss-20b-int4-ov",
		SizeBytes:    12_623_951_367,
		Curated:      true,
		ToolProtocol: toolProtocolOpenVINOJSONSchemaToolCalls,
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
	{CanonicalName: "tinyllama-1.1b-chat-v1.0-int4-ov", Substrings: []string{"openvino/tinyllama-1.1b-chat-v1.0-int4-ov", "tinyllama-1.1b-chat-v1.0-int4-ov"}},
	{CanonicalName: "qwen3-coder-30b-a3b-ov", Substrings: []string{"openvino/qwen3-coder-30", "qwen3-coder-30b-a3b-instruct-int4-ov", "qwen3-coder-30b-a3b-ov"}},
	{CanonicalName: "qwen3-30b-ov", Substrings: []string{"openvino/qwen3-30", "qwen3-30b-a3b-int4-ov", "qwen3-30b-ov"}},
	{CanonicalName: "qwen3-14b-ov", Substrings: []string{"openvino/qwen3-14", "qwen3-14b-int4-ov", "qwen3-14b-ov"}},
	{CanonicalName: "qwen3-8b-ov", Substrings: []string{"openvino/qwen3-8", "qwen3-8b-int4-ov", "qwen3-8b-ov"}},
	{CanonicalName: "qwen3-4b-ov", Substrings: []string{"openvino/qwen3-4", "qwen3-4b-int4-ov", "qwen3-4b-ov"}},
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
	// Gemma 4.
	{CanonicalName: "gemma4-26b-a4b", Substrings: []string{"gemma-4-26", "gemma4-26", "gemma4:26"}},
	{CanonicalName: "gemma4-12b", Substrings: []string{"gemma-4-12", "gemma4-12", "gemma4:12"}},
	{CanonicalName: "gemma4-e2b", Substrings: []string{"gemma-4-e2", "gemma4-e2", "gemma4:e2"}},
	{CanonicalName: "gemma4-e4b", Substrings: []string{"gemma-4-e4", "gemma4-e4", "gemma4:e4", "gemma-4", "gemma4", "gemma"}},
	// Phi 4.
	{CanonicalName: "phi-4-mini", Substrings: []string{"phi-4-mini", "phi4-mini", "phi4mini", "phi-4", "phi4"}},
	// DeepSeek.
	{CanonicalName: "deepseek-r1-0528-qwen3-8b", Substrings: []string{"deepseek-r1-0528", "deepseek-r1-qwen3", "deepseek-r1", "deepseek"}},
	// OpenAI open-weight.
	{CanonicalName: "gpt-oss-20b", Substrings: []string{"openai/gpt-oss-20b", "openai_gpt-oss-20b", "gpt-oss-20b", "gpt-oss", "openai-oss", "openai oss"}},
	// Granite.
	{CanonicalName: "granite-3.2-8b", Substrings: []string{"granite-3.2-8", "granite-3.1", "granite3.", "granite"}},
}

const defaultFallback = "qwen3-4b"
