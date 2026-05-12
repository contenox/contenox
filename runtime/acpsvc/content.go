package acpsvc

import (
	"log/slog"
	"strings"

	"github.com/contenox/contenox/libacp"
)

func flattenPromptBlocks(blocks []libacp.ContentBlock) string {
	var b strings.Builder
	dropped := 0
	for _, block := range blocks {
		switch block.Type {
		case string(libacp.ContentKindText):
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(block.Text)
		case string(libacp.ContentKindResource):
			if block.Resource != nil && block.Resource.Text != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(block.Resource.Text)
			} else {
				dropped++
			}
		default:
			dropped++
		}
	}
	if dropped > 0 {
		slog.Debug("acpsvc: dropped non-text content blocks", "count", dropped)
	}
	return b.String()
}
