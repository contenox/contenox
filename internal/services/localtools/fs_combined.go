package localtools

import (
	"context"
	"fmt"
	"time"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/getkin/kin-openapi/openapi3"
)

// localFSTools serves the local_fs namespace from its two halves: the content
// tools, which read and write through the FileIO seam, and the browse tools,
// which walk the host tree directly.
//
// One namespace, because the split is an implementation boundary and never was
// a decision a model should have to make: content and browsing were once two
// toolset names, and a model that had learned "files are local_fs" asked
// local_fs.list_dir until it failed. Here one allowlist entry grants both, one
// tools_policies section governs both, and the tool a caller names decides
// which half runs — never the order they were registered in.
type localFSTools struct {
	content taskengine.ToolsRepo
	browse  taskengine.ToolsRepo
}

// NewLocalFSToolsFromHalves binds a content toolset and the browse toolset
// under the single local_fs name. Both halves keep their own reach and their
// own policy keys; only the namespace is shared, and the tool a call names
// decides which half runs.
func NewLocalFSToolsFromHalves(content, browse taskengine.ToolsRepo) taskengine.ToolsRepo {
	return &localFSTools{content: content, browse: browse}
}

var (
	_ taskengine.ToolsRepo  = (*localFSTools)(nil)
	_ taskengine.Prechecker = (*localFSTools)(nil)
)

// fsBrowseToolNames is the set of leaves the browse half serves, which is what
// routes a call. It is derived from the toolset's own schema specs so a tool
// added there is routed without a second list to keep in step.
var fsBrowseToolNames = func() map[string]bool {
	names := map[string]bool{}
	for _, spec := range fsBrowseSchemaSpecs() {
		names[spec.tool] = true
	}
	return names
}()

// IsLocalFSBrowseTool reports whether a leaf is served by the browse half,
// which runs in-process; the ACP surface uses it to tell which local_fs tools
// an attached client is needed for.
func IsLocalFSBrowseTool(tool string) bool { return fsBrowseToolNames[tool] }

func (h *localFSTools) Exec(ctx context.Context, startingTime time.Time, input any, debug bool, args *taskengine.ToolsCall) (any, taskengine.DataType, error) {
	if h.isBrowse(args) {
		return h.browse.Exec(ctx, startingTime, input, debug, args)
	}
	return h.content.Exec(ctx, startingTime, input, debug, args)
}

// Precheck forwards to the half that would run the call, so a refusal from
// static configuration alone arrives before any gate asks a human.
func (h *localFSTools) Precheck(ctx context.Context, input any, args *taskengine.ToolsCall) error {
	half := h.content
	if h.isBrowse(args) {
		half = h.browse
	}
	pre, ok := half.(taskengine.Prechecker)
	if !ok {
		return nil
	}
	return pre.Precheck(ctx, input, args)
}

func (h *localFSTools) Supports(ctx context.Context) ([]string, error) {
	names := []string{LocalFSToolsName}
	seen := map[string]bool{LocalFSToolsName: true}
	for _, half := range []taskengine.ToolsRepo{h.content, h.browse} {
		supported, err := half.Supports(ctx)
		if err != nil {
			return nil, err
		}
		for _, name := range supported {
			if seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, nil
}

func (h *localFSTools) GetToolsForToolsByName(ctx context.Context, name string) ([]taskengine.Tool, error) {
	if name != LocalFSToolsName {
		return nil, fmt.Errorf("%w: %q", taskengine.ErrToolsNotFound, name)
	}
	tools, err := h.content.GetToolsForToolsByName(ctx, name)
	if err != nil {
		return nil, err
	}
	browseTools, err := h.browse.GetToolsForToolsByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return append(tools, browseTools...), nil
}

// GetSchemasForSupportedTools publishes one contract for the namespace: the
// content tools and the browse tools in a single document, so a consumer of the
// schema sees the same toolset a caller does.
func (h *localFSTools) GetSchemasForSupportedTools(ctx context.Context) (map[string]*openapi3.T, error) {
	declared, err := h.GetToolsForToolsByName(ctx, LocalFSToolsName)
	if err != nil {
		return nil, err
	}
	specs := append(fsSchemaSpecs(), fsBrowseSchemaSpecs()...)
	doc, err := buildToolsetDoc(LocalFSToolsName, "Local Filesystem Tools",
		"Read, write, list, search and inspect files inside the workspace directory. Content tools read and modify files: every path is contained to that directory, binaries are refused rather than dumped into the transcript, every result is capped and says what it withheld, and modifying an existing file requires having read its current version first. Browse tools list, search and describe the tree read-only: gitignored and high-noise paths are omitted.",
		declared, specs)
	if err != nil {
		return nil, err
	}
	return map[string]*openapi3.T{LocalFSToolsName: doc}, nil
}

func (h *localFSTools) isBrowse(args *taskengine.ToolsCall) bool {
	if args == nil {
		return false
	}
	leaf := args.ToolName
	if leaf == "" {
		leaf = args.Name
	}
	return IsLocalFSBrowseTool(leaf)
}
