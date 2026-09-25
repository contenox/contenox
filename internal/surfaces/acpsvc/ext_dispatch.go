package acpsvc

import (
	"context"
	"encoding/json"

	libacp "github.com/contenox/contenox/libacp"
)

func (t *Transport) handleExtRequest(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	switch method {
	case extMethodTerminalRun:
		return t.handleTerminalRun(ctx, params)
	case extMethodAutocomplete:
		return t.handleAutocomplete(ctx, params)
	case extMethodFSStat:
		return t.handleFSStat(ctx, params)
	case extMethodFSReadDir:
		return t.handleFSReadDir(ctx, params)
	case extMethodFSReadFile:
		return t.handleFSReadFile(ctx, params)
	case extMethodFSWriteFile:
		return t.handleFSWriteFile(ctx, params)
	case extMethodFSCreateDir:
		return t.handleFSCreateDir(ctx, params)
	case extMethodFSRename:
		return t.handleFSRename(ctx, params)
	case extMethodFSDelete:
		return t.handleFSDelete(ctx, params)
	case extMethodFSWatch:
		return t.handleFSWatch(ctx, params)
	case extMethodFSUnwatch:
		return t.handleFSUnwatch(ctx, params)
	case extMethodMissionsList:
		return t.handleMissionsList(ctx, params)
	case extMethodMissionsGet:
		return t.handleMissionsGet(ctx, params)
	case extMethodMissionsReports:
		return t.handleMissionsReports(ctx, params)
	case extMethodInboxList:
		return t.handleInboxList(ctx, params)
	case extMethodAssetActions:
		return t.handleAssetActions(ctx, params)
	case extMethodAssetRun:
		return t.handleAssetRun(ctx, params)
	default:
		return nil, libacp.MethodNotFound(method)
	}
}
