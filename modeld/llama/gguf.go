package llama

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"strings"
)

// GGUF metadata value type tags (llama.cpp gguf spec, little-endian).
const (
	ggufUint8 uint32 = iota
	ggufInt8
	ggufUint16
	ggufInt16
	ggufUint32
	ggufInt32
	ggufFloat32
	ggufBool
	ggufString
	ggufArray
	ggufUint64
	ggufInt64
	ggufFloat64
)

// ggufParams are the architecture facts modeld needs from a GGUF header to size
// the KV cache and the context ceiling. Zero means absent.
type ggufParams struct {
	ContextLength   int
	BlockCount      int // transformer layers
	HeadCountKV     int // KV heads (GQA); falls back to HeadCount
	HeadCount       int
	KeyLength       int // per-head dim, when declared
	EmbeddingLength int
	SlidingWindow   int // model-native sliding-window attention size (SWA)
}

// headDim is the per-head KV dimension: the declared key_length, else
// embedding_length / head_count.
func (p ggufParams) headDim() int {
	if p.KeyLength > 0 {
		return p.KeyLength
	}
	if p.HeadCount > 0 && p.EmbeddingLength > 0 {
		return p.EmbeddingLength / p.HeadCount
	}
	return 0
}

// kvHeads is the number of KV heads (head_count_kv for GQA, else head_count).
func (p ggufParams) kvHeads() int {
	if p.HeadCountKV > 0 {
		return p.HeadCountKV
	}
	return p.HeadCount
}

// ggufContextLength reads only the trained context window (back-compat helper).
func ggufContextLength(path string) int { return ggufModelParams(path).ContextLength }

// ggufModelParams reads the model's architecture facts from a GGUF file's
// metadata block — no tensor data, no model load. Returns the zero value when
// the file is unreadable. This is the daemon (backend adapter) reading the model
// format, which is its job; the pure-Go runtime never parses model files.
func ggufModelParams(path string) ggufParams {
	var p ggufParams
	f, err := os.Open(path)
	if err != nil {
		return p
	}
	defer f.Close()
	r := bufio.NewReader(f)

	magic := make([]byte, 4)
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != "GGUF" {
		return p
	}
	var version uint32
	if binary.Read(r, binary.LittleEndian, &version) != nil {
		return p
	}
	var nTensors, nKV uint64
	if binary.Read(r, binary.LittleEndian, &nTensors) != nil || binary.Read(r, binary.LittleEndian, &nKV) != nil {
		return p
	}

	for i := uint64(0); i < nKV; i++ {
		key, ok := ggufReadString(r)
		if !ok {
			return p
		}
		var vtype uint32
		if binary.Read(r, binary.LittleEndian, &vtype) != nil {
			return p
		}
		val, isInt, ok := ggufReadValue(r, vtype)
		if !ok {
			return p
		}
		if !isInt {
			continue
		}
		switch {
		case strings.HasSuffix(key, ".context_length"):
			p.ContextLength = int(val)
		case strings.HasSuffix(key, ".block_count"):
			p.BlockCount = int(val)
		case strings.HasSuffix(key, ".attention.head_count_kv"):
			p.HeadCountKV = int(val)
		case strings.HasSuffix(key, ".attention.head_count"):
			p.HeadCount = int(val)
		case strings.HasSuffix(key, ".attention.key_length"):
			p.KeyLength = int(val)
		case strings.HasSuffix(key, ".attention.sliding_window"):
			p.SlidingWindow = int(val)
		case strings.HasSuffix(key, ".embedding_length"):
			p.EmbeddingLength = int(val)
		}
	}
	return p
}

func ggufReadString(r io.Reader) (string, bool) {
	var n uint64
	if binary.Read(r, binary.LittleEndian, &n) != nil {
		return "", false
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", false
	}
	return string(buf), true
}

// ggufReadValue consumes one metadata value of vtype, advancing r. It returns the
// integer value when the type is a scalar integer (the only case context_length
// needs); other types are skipped so parsing can continue to the next key.
func ggufReadValue(r io.Reader, vtype uint32) (val int64, isInt, ok bool) {
	switch vtype {
	case ggufUint8, ggufInt8, ggufBool:
		var v uint8
		if binary.Read(r, binary.LittleEndian, &v) != nil {
			return 0, false, false
		}
		return int64(v), true, true
	case ggufUint16, ggufInt16:
		var v uint16
		if binary.Read(r, binary.LittleEndian, &v) != nil {
			return 0, false, false
		}
		return int64(v), true, true
	case ggufUint32, ggufInt32:
		var v uint32
		if binary.Read(r, binary.LittleEndian, &v) != nil {
			return 0, false, false
		}
		return int64(v), true, true
	case ggufUint64, ggufInt64:
		var v uint64
		if binary.Read(r, binary.LittleEndian, &v) != nil {
			return 0, false, false
		}
		return int64(v), true, true
	case ggufFloat32:
		if _, err := io.CopyN(io.Discard, r, 4); err != nil {
			return 0, false, false
		}
		return 0, false, true
	case ggufFloat64:
		if _, err := io.CopyN(io.Discard, r, 8); err != nil {
			return 0, false, false
		}
		return 0, false, true
	case ggufString:
		if _, ok := ggufReadString(r); !ok {
			return 0, false, false
		}
		return 0, false, true
	case ggufArray:
		var elemType uint32
		if binary.Read(r, binary.LittleEndian, &elemType) != nil {
			return 0, false, false
		}
		var count uint64
		if binary.Read(r, binary.LittleEndian, &count) != nil {
			return 0, false, false
		}
		for j := uint64(0); j < count; j++ {
			if _, _, ok := ggufReadValue(r, elemType); !ok {
				return 0, false, false
			}
		}
		return 0, false, true
	default:
		return 0, false, false
	}
}
