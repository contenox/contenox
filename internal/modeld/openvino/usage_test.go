package openvino

import (
	"testing"

	"github.com/contenox/contenox/internal/modeld/openvino/ovsession"
)

func TestUnit_NativeUsagePreservesMeasuredTokens(t *testing.T) {
	chunk := nativeStreamChunk(ovsession.StreamChunk{Text: "answer", Metrics: &ovsession.PipelineMetrics{UsageKnown: true, PromptTokens: 71, CompletionTokens: 13, FinishReason: "length"}})
	if chunk.Usage == nil || chunk.Usage.PromptTokens != 71 || chunk.Usage.CompletionTokens != 13 || chunk.FinishReason != "length" || chunk.Text != "answer" {
		t.Fatalf("chunk = %+v", chunk)
	}
	unknown := nativeStreamChunk(ovsession.StreamChunk{Metrics: &ovsession.PipelineMetrics{}})
	if unknown.Usage != nil {
		t.Fatalf("unreported usage = %+v", unknown.Usage)
	}
}
