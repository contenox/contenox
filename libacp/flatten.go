package libacp

import "strings"

// FlattenContent projects a content block list down to a single string, and is
// the missing inverse of the constructor half in content.go (NewTextContent,
// NewResourceLink, ...): those build the structured wire form, this collapses it
// back for a consumer that can only accept flat text — a plain-string prompt
// field, a log line, a title.
//
// It is a LOSSY TEXT PROJECTION. Image, audio, binary-blob resources and any
// unknown block type carry no text and are simply gone from the result; a
// resource block contributes only its inline Resource.Text, never its Blob.
//
// The exact rendering is *a* policy, not *the* canonical one: blocks are joined
// with a single newline (empty pieces contribute nothing, so no blank runs) and
// a resource link renders as "name: uri", degrading to whichever of the two is
// present. Nothing in the protocol blesses that shape — a caller that needs
// markdown links or space-joined text should write its own walk rather than
// bend this one.
//
// The dropped return is deliberate and must not be optimized away: it is the
// deduplicated, first-seen-ordered list of block types that could not be
// represented, so a caller can tell the user "3 images were not sent to the
// model" instead of silently losing them. Ignoring it turns a visible
// degradation into a silent one.
func FlattenContent(blocks []ContentBlock) (text string, dropped []string) {
	var b strings.Builder
	seen := map[string]bool{}

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
		case string(ContentKindText):
			appendText(block.Text)
		case string(ContentKindResource):
			if block.Resource != nil && block.Resource.Text != "" {
				appendText(block.Resource.Text)
			} else {
				drop(block.Type)
			}
		case string(ContentKindResourceLink):
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
