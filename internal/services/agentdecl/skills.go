package agentdecl

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillDirName is where procedures live inside a contenox directory, beside the
// agents that use them.
const SkillDirName = "skills"

// SkillsMacro is what a declaration writes to pull the inventory in. It expands
// when the chain is generated, not per request.
const SkillsMacro = "{{skills}}"

// Skill is one procedure: how to orchestrate several tools for a repeated job.
// The runtime does not load or execute it; the agent reads it with its file tool.
type Skill struct {
	Name        string
	Description string
	Path        string
}

// DiscoverSkills reads every skill under the given contenox roots, nearest root
// first, in either the flat `timesheet.md` or the `timesheet/SKILL.md` layout. A
// nearer skill shadows one of the same name further out, and one outside
// workspaceRoot is left out because the agent could not address it.
func DiscoverSkills(ctx context.Context, contenoxDirs []Root, workspaceRoot string) []Skill {
	seen := map[string]bool{}
	var out []Skill
	for _, root := range contenoxDirs {
		if root.FS == nil {
			continue
		}
		skills, err := root.Child(SkillDirName)
		if err != nil {
			continue
		}
		for _, skill := range skillsIn(ctx, skills) {
			if seen[skill.Name] {
				continue
			}
			rel, ok := readablePath(skill.Path, workspaceRoot)
			if !ok {
				continue
			}
			skill.Path = rel
			seen[skill.Name] = true
			out = append(out, skill)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func readablePath(path, workspaceRoot string) (string, bool) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return "", false
	}
	rel, err := filepath.Rel(workspaceRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func skillsIn(ctx context.Context, root Root) []Skill {
	entries, err := root.FS.ReadDir(ctx, ".")
	if err != nil {
		return nil
	}
	var out []Skill
	for _, entry := range entries {
		rel := entry.Name()
		if entry.IsDir() {
			rel = path.Join(rel, "SKILL.md")
			if info, err := root.FS.Stat(ctx, rel); err != nil || info.IsDir() {
				continue
			}
		} else if !strings.EqualFold(path.Ext(entry.Name()), ".md") {
			continue
		}
		data, err := root.FS.ReadFile(ctx, rel)
		if err != nil {
			continue
		}
		if skill, ok := readSkill(data, root.Path(rel), entry.Name(), entry.IsDir()); ok {
			out = append(out, skill)
		}
	}
	return out
}

func readSkill(data []byte, display, entryName string, isDir bool) (Skill, bool) {
	name := strings.TrimSuffix(entryName, path.Ext(entryName))
	if isDir {
		name = entryName
	}
	skill := Skill{Name: name, Path: display}

	// Frontmatter is optional: a bare Markdown procedure is still a skill.
	if front, _, ok := splitFrontmatter(data); ok {
		fields := map[string]any{}
		if err := yaml.Unmarshal(front, &fields); err == nil {
			if v := strings.TrimSpace(stringField(fields, "name")); v != "" {
				skill.Name = v
			}
			skill.Description = strings.TrimSpace(stringField(fields, "description"))
		}
	}
	if skill.Description == "" {
		skill.Description = firstProseLine(data)
	}
	return skill, true
}

func firstProseLine(data []byte) string {
	_, body, ok := splitFrontmatter(data)
	if !ok {
		body = data
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) > 200 {
			line = line[:197] + "..."
		}
		return line
	}
	return ""
}

// RenderSkillInventory is what {{skills}} becomes: one line per procedure, with
// the path to read. The index, not the bodies.
func RenderSkillInventory(skills []Skill) string {
	if len(skills) == 0 {
		return "No skills are available."
	}
	b := strings.Builder{}
	b.WriteString("Skills are procedures for repeated work. When a request matches one, read its file before starting, then follow it.\n")
	for _, skill := range skills {
		if skill.Description == "" {
			fmt.Fprintf(&b, "\n- %s — read %s", skill.Name, skill.Path)
			continue
		}
		fmt.Fprintf(&b, "\n- %s: %s — read %s", skill.Name, skill.Description, skill.Path)
	}
	return b.String()
}

func expandSkills(prompt string, skills []Skill) string {
	if !strings.Contains(prompt, SkillsMacro) {
		return prompt
	}
	return strings.ReplaceAll(prompt, SkillsMacro, RenderSkillInventory(skills))
}
