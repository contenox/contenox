package openvino

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// modelIdentity derives a model's stable cache identity from the files in its
// OpenVINO IR directory — model-agnostic, with no template assumed or rendered
// here. modelDigest content-addresses the architecture + tokenizer + generation
// config; templateDigest hashes the model's OWN chat template (the Jinja in
// tokenizer_config.json that modeld applies via the IR tokenizer). The runtime
// only needs these for cache keying; OpenVINO owns applying the template and
// stopping at the model's EOS. Missing files degrade to empty, never a guess.
func modelIdentity(modelDir string) (modelDigest, templateDigest string) {
	h := sha256.New()
	for _, name := range []string{"config.json", "tokenizer_config.json", "generation_config.json"} {
		if b, err := os.ReadFile(filepath.Join(modelDir, name)); err == nil {
			h.Write(b)
		}
	}
	modelDigest = hex.EncodeToString(h.Sum(nil))

	if b, err := os.ReadFile(filepath.Join(modelDir, "tokenizer_config.json")); err == nil {
		// chat_template may be a string or a list of named templates; hash the raw
		// JSON either way so the digest tracks the model's actual template.
		var cfg struct {
			ChatTemplate json.RawMessage `json:"chat_template"`
		}
		if json.Unmarshal(b, &cfg) == nil && len(cfg.ChatTemplate) > 0 {
			templateDigest = hashString(string(cfg.ChatTemplate))
		}
	}
	return modelDigest, templateDigest
}
