package contenoxcli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/contenox/contenox/runtime/taskengine"
)

// AGENTS.md is a community standard for project-level agent instructions.
// See https://agents.md — adopted by Codex, Aider, Cursor, Gemini CLI, etc.
//
// We treat it as reference material rather than rules: it is appended to the
// chat history once per session as a single system message, prefix-cached by
// providers on subsequent turns, and survives restarts via messagestore.
const (
	agentsMDFilename  = "AGENTS.md"
	maxAgentsMDBytes  = 64 * 1024
	agentsMDTruncated = "\n\n[AGENTS.md truncated to 64 KiB; remove this limit by editing maxAgentsMDBytes]"
)

// LoadAgentsMD walks up from startDir looking for an AGENTS.md file. Returns
// (content, absPath, true) on the first hit or ("", "", false) if no file is
// found before reaching the filesystem root. Content over maxAgentsMDBytes is
// truncated with a marker so a pathological 10 MiB file can't bloat every
// session.
func LoadAgentsMD(startDir string) (string, string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", "", false
	}
	for {
		candidate := filepath.Join(dir, agentsMDFilename)
		if data, err := os.ReadFile(candidate); err == nil {
			content := string(data)
			if len(content) > maxAgentsMDBytes {
				content = content[:maxAgentsMDBytes] + agentsMDTruncated
			}
			return content, candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}

// AgentsMDMessage builds the system message that gets prepended to the chain
// input. Includes the absolute path so the model can refer to it explicitly
// and so users can audit which file was loaded via the persisted history.
func AgentsMDMessage(content, path string) taskengine.Message {
	return taskengine.Message{
		Role:      "system",
		Content:   fmt.Sprintf("Project context loaded from %s (AGENTS.md, community standard from agents.md). Treat this as project-specific reference material and conventions, not unconditional rules. Loaded once at session start; if it changes, start a new session to pick up the update.\n\n%s", path, content),
		Timestamp: time.Now().UTC(),
	}
}
