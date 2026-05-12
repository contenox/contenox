package contenoxcli

import (
	"os"
	"path/filepath"

	"github.com/contenox/contenox/runtime/agentservice"
	"github.com/contenox/contenox/runtime/taskengine"
)

const (
	agentsMDFilename  = "AGENTS.md"
	maxAgentsMDBytes  = 64 * 1024
	agentsMDTruncated = "\n\n[AGENTS.md truncated to 64 KiB; remove this limit by editing maxAgentsMDBytes]"
)

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

func AgentsMDMessage(content, path string) taskengine.Message {
	return agentservice.AgentsMDMessage(content, path)
}

func loadAgentsMDFromCwd() (string, string) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", ""
	}
	content, path, ok := LoadAgentsMD(cwd)
	if !ok {
		return "", ""
	}
	return content, path
}
