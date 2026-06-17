import re

with open('modeld/llama/session.go', 'r') as f:
    text = f.read()

replacement = """import "github.com/contenox/runtime/runtime/transport"

type Config = transport.Config
type PrefixInput = transport.PrefixInput
type SuffixInput = transport.SuffixInput
type PrefixStatus = transport.PrefixStatus
type SuffixStatus = transport.SuffixStatus
type DecodeConfig = transport.DecodeConfig
type StreamChunk = transport.StreamChunk
type ContextReport = transport.ContextReport
type Session = transport.Session"""

# We need to replace from 'type Config struct {' down to 'type Session interface {' and its closing brace.
# Also replace the 'import (' block to include transport.

text = re.sub(r'import \(\n\t"context"\n\t"errors"\n\t"fmt"\n\)',
r'''import (
	"context"
	"errors"
	"fmt"

	"github.com/contenox/runtime/runtime/transport"
)''', text)

text = re.sub(r'(?s)// Config is.*?type Session interface \{.*?ExplainContext.*?Close\(\) error\n\}', replacement, text, count=1)

with open('modeld/llama/session.go', 'w') as f:
    f.write(text)
