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
	// SlidingWindowPattern marks per-layer attention type when present. true
	// means the layer uses sliding-window attention; false means global.
	SlidingWindowPattern       []bool
	SlidingWindowPatternStride int // legacy scalar: every nth layer is global
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
		meta, ok := ggufReadValueForKey(r, vtype, key)
		if !ok {
			return p
		}
		switch {
		case strings.HasSuffix(key, ".context_length"):
			if meta.IsInt {
				p.ContextLength = int(meta.Int)
			}
		case strings.HasSuffix(key, ".block_count"):
			if meta.IsInt {
				p.BlockCount = int(meta.Int)
			}
		case strings.HasSuffix(key, ".attention.head_count_kv"):
			if meta.IsInt {
				p.HeadCountKV = int(meta.Int)
			}
		case strings.HasSuffix(key, ".attention.head_count"):
			if meta.IsInt {
				p.HeadCount = int(meta.Int)
			}
		case strings.HasSuffix(key, ".attention.key_length"):
			if meta.IsInt {
				p.KeyLength = int(meta.Int)
			}
		case strings.HasSuffix(key, ".attention.sliding_window"):
			if meta.IsInt {
				p.SlidingWindow = int(meta.Int)
			}
		case strings.HasSuffix(key, ".attention.sliding_window_pattern"):
			switch {
			case meta.IsInt:
				p.SlidingWindowPatternStride = int(meta.Int)
			case len(meta.Bools) > 0:
				p.SlidingWindowPattern = meta.Bools
			case len(meta.Ints) > 0:
				p.SlidingWindowPattern = make([]bool, len(meta.Ints))
				for i, v := range meta.Ints {
					p.SlidingWindowPattern[i] = v != 0
				}
			}
		case strings.HasSuffix(key, ".embedding_length"):
			if meta.IsInt {
				p.EmbeddingLength = int(meta.Int)
			}
		}
	}
	return p
}

func (p ggufParams) layerSplit() (global, windowed int) {
	if p.BlockCount <= 0 {
		return 0, 0
	}
	if p.SlidingWindow <= 0 {
		return p.BlockCount, 0
	}
	if len(p.SlidingWindowPattern) > 0 {
		n := min(len(p.SlidingWindowPattern), p.BlockCount)
		for i := 0; i < n; i++ {
			if p.SlidingWindowPattern[i] {
				windowed++
			} else {
				global++
			}
		}
		global += p.BlockCount - n
		return global, windowed
	}
	if p.SlidingWindowPatternStride > 0 {
		global = (p.BlockCount + p.SlidingWindowPatternStride - 1) / p.SlidingWindowPatternStride
		if global > p.BlockCount {
			global = p.BlockCount
		}
		return global, p.BlockCount - global
	}
	return 0, p.BlockCount
}

// ggufMaxStringLen caps a single GGUF metadata string. modeld only needs small
// architecture facts; real metadata strings (chat templates, tokenizer model
// names) are at most a few MiB. The cap stops a corrupt or hostile header from
// requesting a multi-gigabyte allocation — or tripping makeslice on a bogus
// 64-bit length — and crashing the daemon: parsing just stops and modeld falls
// back to zero/partial params (and thus model defaults).
const ggufMaxStringLen = 64 << 20

func ggufReadString(r io.Reader) (string, bool) {
	var n uint64
	if binary.Read(r, binary.LittleEndian, &n) != nil {
		return "", false
	}
	if n > ggufMaxStringLen {
		return "", false
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", false
	}
	return string(buf), true
}

type ggufMetadataValue struct {
	Int   int64
	IsInt bool
	Ints  []int64
	Bools []bool
}

func ggufReadValueForKey(r io.Reader, vtype uint32, key string) (ggufMetadataValue, bool) {
	collectArray := strings.HasSuffix(key, ".attention.sliding_window_pattern")
	return ggufReadValue(r, vtype, collectArray)
}

// ggufReadValue consumes one metadata value of vtype, advancing r. It returns
// scalar integers for architecture facts and, only when collectArray is true,
// integer/bool array contents for sliding_window_pattern. Other values are
// skipped so parsing can continue to the next key.
func ggufReadValue(r io.Reader, vtype uint32, collectArray bool) (ggufMetadataValue, bool) {
	switch vtype {
	case ggufUint8, ggufInt8, ggufBool:
		var v uint8
		if binary.Read(r, binary.LittleEndian, &v) != nil {
			return ggufMetadataValue{}, false
		}
		return ggufMetadataValue{Int: int64(v), IsInt: true}, true
	case ggufUint16, ggufInt16:
		var v uint16
		if binary.Read(r, binary.LittleEndian, &v) != nil {
			return ggufMetadataValue{}, false
		}
		return ggufMetadataValue{Int: int64(v), IsInt: true}, true
	case ggufUint32, ggufInt32:
		var v uint32
		if binary.Read(r, binary.LittleEndian, &v) != nil {
			return ggufMetadataValue{}, false
		}
		return ggufMetadataValue{Int: int64(v), IsInt: true}, true
	case ggufUint64, ggufInt64:
		var v uint64
		if binary.Read(r, binary.LittleEndian, &v) != nil {
			return ggufMetadataValue{}, false
		}
		return ggufMetadataValue{Int: int64(v), IsInt: true}, true
	case ggufFloat32:
		if _, err := io.CopyN(io.Discard, r, 4); err != nil {
			return ggufMetadataValue{}, false
		}
		return ggufMetadataValue{}, true
	case ggufFloat64:
		if _, err := io.CopyN(io.Discard, r, 8); err != nil {
			return ggufMetadataValue{}, false
		}
		return ggufMetadataValue{}, true
	case ggufString:
		if _, ok := ggufReadString(r); !ok {
			return ggufMetadataValue{}, false
		}
		return ggufMetadataValue{}, true
	case ggufArray:
		var elemType uint32
		if binary.Read(r, binary.LittleEndian, &elemType) != nil {
			return ggufMetadataValue{}, false
		}
		var count uint64
		if binary.Read(r, binary.LittleEndian, &count) != nil {
			return ggufMetadataValue{}, false
		}
		out := ggufMetadataValue{}
		if collectArray {
			switch elemType {
			case ggufBool:
				out.Bools = make([]bool, 0, count)
			case ggufUint8, ggufInt8, ggufUint16, ggufInt16, ggufUint32, ggufInt32, ggufUint64, ggufInt64:
				out.Ints = make([]int64, 0, count)
			}
		}
		for j := uint64(0); j < count; j++ {
			elem, ok := ggufReadValue(r, elemType, false)
			if !ok {
				return ggufMetadataValue{}, false
			}
			if collectArray && elem.IsInt {
				switch elemType {
				case ggufBool:
					out.Bools = append(out.Bools, elem.Int != 0)
				case ggufUint8, ggufInt8, ggufUint16, ggufInt16, ggufUint32, ggufInt32, ggufUint64, ggufInt64:
					out.Ints = append(out.Ints, elem.Int)
				}
			}
		}
		return out, true
	default:
		return ggufMetadataValue{}, false
	}
}
