package acpsvc

import (
	"strings"

	"github.com/contenox/runtime/libacp"
)

func flattenPromptBlocks(blocks []libacp.ContentBlock) (string, []string) {
	var b strings.Builder
	seen := map[string]bool{}
	var dropped []string

	drop := func(kind string) {
		if !seen[kind] {
			seen[kind] = true
			dropped = append(dropped, kind)
		}
	}
	appendText := func(s string) {
		if s == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s)
	}

	for _, block := range blocks {
		switch block.Type {
		case string(libacp.ContentKindText):
			appendText(block.Text)
		case string(libacp.ContentKindResource):
			if block.Resource != nil && block.Resource.Text != "" {
				appendText(block.Resource.Text)
			} else {
				drop(block.Type)
			}
		case string(libacp.ContentKindResourceLink):
			name := strings.TrimSpace(block.Name)
			uri := strings.TrimSpace(block.URI)
			switch {
			case name != "" && uri != "":
				appendText(name + ": " + uri)
			case uri != "":
				appendText(uri)
			case name != "":
				appendText(name)
			default:
				drop(block.Type)
			}
		default:
			drop(block.Type)
		}
	}
	return b.String(), dropped
}
