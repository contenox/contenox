package llama

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// writeGGUFString appends a gguf string (uint64 length + bytes).
func writeGGUFString(b *bytes.Buffer, s string) {
	_ = binary.Write(b, binary.LittleEndian, uint64(len(s)))
	b.WriteString(s)
}

// buildGGUF builds a minimal valid GGUF header with the given metadata kv pairs.
// Each kv is {key, vtype, encoded-value-writer}.
func buildGGUF(t *testing.T, kvs []func(*bytes.Buffer)) []byte {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("GGUF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(3)) // version
	_ = binary.Write(&b, binary.LittleEndian, uint64(0)) // tensor count
	_ = binary.Write(&b, binary.LittleEndian, uint64(len(kvs)))
	for _, kv := range kvs {
		kv(&b)
	}
	return b.Bytes()
}

func TestUnit_GGUFContextLength_ParsesArchContextLength(t *testing.T) {
	data := buildGGUF(t, []func(*bytes.Buffer){
		// a string kv before the target, to exercise skipping
		func(b *bytes.Buffer) {
			writeGGUFString(b, "general.architecture")
			_ = binary.Write(b, binary.LittleEndian, ggufString)
			writeGGUFString(b, "qwen2")
		},
		// an array kv (of uint32), to exercise array skipping
		func(b *bytes.Buffer) {
			writeGGUFString(b, "qwen2.attention.head_count_kv")
			_ = binary.Write(b, binary.LittleEndian, ggufArray)
			_ = binary.Write(b, binary.LittleEndian, ggufUint32)
			_ = binary.Write(b, binary.LittleEndian, uint64(2))
			_ = binary.Write(b, binary.LittleEndian, uint32(4))
			_ = binary.Write(b, binary.LittleEndian, uint32(4))
		},
		// the target
		func(b *bytes.Buffer) {
			writeGGUFString(b, "qwen2.context_length")
			_ = binary.Write(b, binary.LittleEndian, ggufUint32)
			_ = binary.Write(b, binary.LittleEndian, uint32(32768))
		},
	})

	dir := t.TempDir()
	path := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ggufContextLength(path); got != 32768 {
		t.Fatalf("ggufContextLength = %d, want 32768", got)
	}
}

func TestUnit_GGUFContextLength_MissingFileIsZero(t *testing.T) {
	if got := ggufContextLength(filepath.Join(t.TempDir(), "nope.gguf")); got != 0 {
		t.Fatalf("missing file = %d, want 0", got)
	}
}

func TestUnit_GGUFModelParams_ParsesSlidingWindowAttention(t *testing.T) {
	data := buildGGUF(t, []func(*bytes.Buffer){
		func(b *bytes.Buffer) {
			writeGGUFString(b, "gemma2.context_length")
			_ = binary.Write(b, binary.LittleEndian, ggufUint32)
			_ = binary.Write(b, binary.LittleEndian, uint32(8192))
		},
		func(b *bytes.Buffer) {
			writeGGUFString(b, "gemma2.attention.sliding_window")
			_ = binary.Write(b, binary.LittleEndian, ggufUint32)
			_ = binary.Write(b, binary.LittleEndian, uint32(4096))
		},
	})
	path := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got := ggufModelParams(path)
	if got.ContextLength != 8192 || got.SlidingWindow != 4096 {
		t.Fatalf("ggufModelParams = %+v, want context=8192 sliding_window=4096", got)
	}
}

// TestUnit_GGUFContextLength_RealModel reads an actual GGUF if one is provided,
// so the parser can be checked against a real model (e.g. a pulled qwen).
func TestUnit_GGUFContextLength_RealModel(t *testing.T) {
	path := os.Getenv("CONTENOX_LLAMA_TEST_GGUF")
	if path == "" {
		t.Skip("set CONTENOX_LLAMA_TEST_GGUF to a real GGUF to check the parser against it")
	}
	got := ggufContextLength(path)
	if got <= 0 {
		t.Fatalf("ggufContextLength(%s) = %d, want > 0", path, got)
	}
	t.Logf("context_length(%s) = %d", path, got)
}
