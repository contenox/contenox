package llama

import (
	"errors"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/transport"
)

// Golden test for the image wire encoding: every attached image contributes one
// media marker in the volatile text (in reading order) and its bytes ride on
// SuffixInput.Images in the same order, so modeld's mtmd path can pair marker i
// with image i. The stable prefix stays token-only.
func TestUnit_LlamaPromptPlan_EncodesImagesOnVolatileSuffix(t *testing.T) {
	img1 := modelrepo.ImagePart{Data: []byte("png-1"), MimeType: "image/png"}
	img2 := modelrepo.ImagePart{Data: []byte("jpg-2"), MimeType: "image/jpeg"}
	img3 := modelrepo.ImagePart{Data: []byte("png-3"), MimeType: "image/png"}
	messages := []modelrepo.Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "What color is this?", Images: []modelrepo.ImagePart{img1}},
		{Role: "assistant", Content: "Red."},
		{Role: "user", Content: "And these two?", Images: []modelrepo.ImagePart{img2, img3}},
	}

	plan, err := buildPromptPlan(messages, Config{}, promptIdentity{ProfileID: "p", ModelDigest: "d", BackendVersion: "v"}, "")
	if err != nil {
		t.Fatalf("buildPromptPlan: %v", err)
	}

	if plan.Stable.Text != "You are helpful." {
		t.Fatalf("stable text = %q", plan.Stable.Text)
	}
	if len(plan.Stable.Images) != 0 {
		t.Fatalf("stable prefix must stay token-only, got %d images", len(plan.Stable.Images))
	}
	m := transport.MediaMarker
	wantVolatile := m + "\nWhat color is this?" + "Red." + m + "\n" + m + "\nAnd these two?"
	if plan.Volatile.Text != wantVolatile {
		t.Fatalf("volatile text = %q\nwant %q", plan.Volatile.Text, wantVolatile)
	}
	if got := len(plan.Volatile.Images); got != 3 {
		t.Fatalf("volatile images = %d, want 3", got)
	}
	for i, want := range []modelrepo.ImagePart{img1, img2, img3} {
		got := plan.Volatile.Images[i]
		if string(got.Data) != string(want.Data) || got.MimeType != want.MimeType {
			t.Fatalf("image %d = %+v, want %+v (order pairs each image with its marker)", i, got, want)
		}
	}
	if n := strings.Count(plan.Volatile.Text, m); n != len(plan.Volatile.Images) {
		t.Fatalf("marker count %d != image count %d", n, len(plan.Volatile.Images))
	}
}

func TestUnit_LlamaPromptPlan_RejectsSystemImages(t *testing.T) {
	messages := []modelrepo.Message{
		{Role: "system", Content: "look", Images: []modelrepo.ImagePart{{Data: []byte("x"), MimeType: "image/png"}}},
		{Role: "user", Content: "hi"},
	}
	_, err := buildPromptPlan(messages, Config{}, promptIdentity{}, "")
	if !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("expected typed unsupported-feature refusal for system images, got: %v", err)
	}
}

// Coalescing consecutive same-role text turns must not drop attachments: the
// merged turn keeps every image in list order, and an image-only turn (no text)
// survives normalization.
func TestUnit_LlamaPromptPlan_CoalescingKeepsImages(t *testing.T) {
	img1 := modelrepo.ImagePart{Data: []byte("a"), MimeType: "image/png"}
	img2 := modelrepo.ImagePart{Data: []byte("b"), MimeType: "image/png"}
	messages := []modelrepo.Message{
		{Role: "user", Content: "first", Images: []modelrepo.ImagePart{img1}},
		{Role: "user", Content: "", Images: []modelrepo.ImagePart{img2}},
	}

	plan, err := buildPromptPlan(messages, Config{}, promptIdentity{}, "")
	if err != nil {
		t.Fatalf("buildPromptPlan: %v", err)
	}
	if got := len(plan.Volatile.Images); got != 2 {
		t.Fatalf("volatile images = %d, want both images kept through coalescing", got)
	}
	if string(plan.Volatile.Images[0].Data) != "a" || string(plan.Volatile.Images[1].Data) != "b" {
		t.Fatalf("image order lost through coalescing: %+v", plan.Volatile.Images)
	}
	if n := strings.Count(plan.Volatile.Text, transport.MediaMarker); n != 2 {
		t.Fatalf("marker count = %d, want 2", n)
	}
}
