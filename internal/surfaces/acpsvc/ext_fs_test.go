package acpsvc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/services/localfileservice"
	"github.com/contenox/contenox/libacp"
	"github.com/stretchr/testify/require"
)

func newLocalFileSvc(t *testing.T) localfileservice.Service {
	t.Helper()
	root := t.TempDir()
	svc, err := localfileservice.New(root)
	require.NoError(t, err)
	return svc
}

func extTransportWithFiles(svc localfileservice.Service) *Transport {
	return &Transport{deps: Deps{Files: svc}}
}

func extCallHandler[T any](t *testing.T, handler func(context.Context, json.RawMessage) (json.RawMessage, *libacp.Error), params T) (json.RawMessage, *libacp.Error) {
	t.Helper()
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	return handler(context.Background(), raw)
}

func TestExtFS_WriteStatReadDirReadFileRoundtrip(t *testing.T) {
	svc := newLocalFileSvc(t)
	tr := extTransportWithFiles(svc)

	content := base64.StdEncoding.EncodeToString([]byte("hello bus"))
	res, rpcErr := extCallHandler(t, tr.handleFSWriteFile, map[string]any{
		"path": "sub/f.txt", "content": content, "create": true,
	})
	require.Nil(t, rpcErr)
	require.JSONEq(t, `{"ok":true}`, string(res))

	res, rpcErr = extCallHandler(t, tr.handleFSStat, map[string]any{"path": "sub/f.txt"})
	require.Nil(t, rpcErr)
	var st struct {
		Name  string `json:"name"`
		IsDir bool   `json:"isDir"`
		Size  int64  `json:"size"`
	}
	require.NoError(t, json.Unmarshal(res, &st))
	require.Equal(t, "f.txt", st.Name)
	require.False(t, st.IsDir)
	require.Equal(t, int64(len("hello bus")), st.Size)

	res, rpcErr = extCallHandler(t, tr.handleFSReadFile, map[string]any{"path": "sub/f.txt"})
	require.Nil(t, rpcErr)
	var read struct {
		Content string `json:"content"`
	}
	require.NoError(t, json.Unmarshal(res, &read))
	decoded, err := base64.StdEncoding.DecodeString(read.Content)
	require.NoError(t, err)
	require.Equal(t, "hello bus", string(decoded))

	res, rpcErr = extCallHandler(t, tr.handleFSReadDir, map[string]any{"path": "/"})
	require.Nil(t, rpcErr)
	var listing struct {
		Entries []fsEntry `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(res, &listing))
	require.Len(t, listing.Entries, 1)
	require.Equal(t, "sub", listing.Entries[0].Name)
	require.True(t, listing.Entries[0].IsDir)
}

func TestExtFS_MissingPathIsResourceNotFound(t *testing.T) {
	svc := newLocalFileSvc(t)
	tr := extTransportWithFiles(svc)

	for _, tc := range []struct {
		handler func(context.Context, json.RawMessage) (json.RawMessage, *libacp.Error)
		params  map[string]any
	}{
		{tr.handleFSStat, map[string]any{"path": "absent.txt"}},
		{tr.handleFSReadFile, map[string]any{"path": "absent.txt"}},
		{tr.handleFSReadDir, map[string]any{"path": "absent-dir"}},
		{tr.handleFSDelete, map[string]any{"path": "absent.txt"}},
	} {
		_, rpcErr := extCallHandler(t, tc.handler, tc.params)
		require.NotNil(t, rpcErr)
		require.Equal(t, libacp.ErrResourceNotFound, rpcErr.Code)
		require.True(t, libacp.IsNotFound(rpcErr))
		require.ErrorIs(t, libacp.AsNotExist(rpcErr), os.ErrNotExist)
	}
}

func TestExtFS_FlagMismatchesAreInvalidParams(t *testing.T) {
	svc := newLocalFileSvc(t)
	tr := extTransportWithFiles(svc)

	create := func(path string) {
		_, rpcErr := extCallHandler(t, tr.handleFSWriteFile, map[string]any{
			"path": path, "content": base64.StdEncoding.EncodeToString([]byte("x")), "create": true,
		})
		require.Nil(t, rpcErr)
	}
	create("exists.txt")

	cases := []struct {
		name    string
		params  map[string]any
		handler func(context.Context, json.RawMessage) (json.RawMessage, *libacp.Error)
	}{
		{"neither flag", map[string]any{"path": "new.txt", "content": ""}, tr.handleFSWriteFile},
		{"create without overwrite on existing", map[string]any{"path": "exists.txt", "content": ""}, tr.handleFSWriteFile},
		{"missing file without create", map[string]any{"path": "nope.txt", "content": "", "overwrite": true}, tr.handleFSWriteFile},
		{"bad base64", map[string]any{"path": "new2.txt", "content": "!!", "create": true}, tr.handleFSWriteFile},
		{"write to the root", map[string]any{"path": "/", "content": "eA==", "create": true}, tr.handleFSWriteFile},
		{"escape via traversal", map[string]any{"path": "../escape.txt", "content": "eA==", "create": true}, tr.handleFSWriteFile},
	}
	for _, tc := range cases {
		_, rpcErr := extCallHandler(t, tc.handler, tc.params)
		require.NotNil(t, rpcErr, tc.name)
		require.Equal(t, libacp.ErrInvalidParams, rpcErr.Code, tc.name)
	}
}

func TestExtFS_PathsOutsideTheRootAreRejected(t *testing.T) {
	root := t.TempDir()
	svc, err := localfileservice.New(root)
	require.NoError(t, err)
	tr := extTransportWithFiles(svc)

	outside := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(outside, []byte("s"), 0o600))

	for _, handler := range []func(context.Context, json.RawMessage) (json.RawMessage, *libacp.Error){
		tr.handleFSStat, tr.handleFSReadFile, tr.handleFSReadDir,
	} {
		_, rpcErr := extCallHandler(t, handler, map[string]any{"path": outside})
		require.NotNil(t, rpcErr)
		require.Equal(t, libacp.ErrInvalidParams, rpcErr.Code)
	}
}

func TestExtFS_DeleteRespectsRecursiveFlag(t *testing.T) {
	svc := newLocalFileSvc(t)
	tr := extTransportWithFiles(svc)

	_, rpcErr := extCallHandler(t, tr.handleFSCreateDir, map[string]any{"path": "dir"})
	require.Nil(t, rpcErr)
	_, rpcErr = extCallHandler(t, tr.handleFSWriteFile, map[string]any{
		"path": "dir/file.txt", "content": base64.StdEncoding.EncodeToString([]byte("x")), "create": true,
	})
	require.Nil(t, rpcErr)

	_, rpcErr = extCallHandler(t, tr.handleFSDelete, map[string]any{"path": "dir", "recursive": false})
	require.NotNil(t, rpcErr)
	require.Equal(t, libacp.ErrInvalidParams, rpcErr.Code)

	_, rpcErr = extCallHandler(t, tr.handleFSDelete, map[string]any{"path": "dir", "recursive": true})
	require.Nil(t, rpcErr)
	_, rpcErr = extCallHandler(t, tr.handleFSStat, map[string]any{"path": "dir"})
	require.NotNil(t, rpcErr)
	require.Equal(t, libacp.ErrResourceNotFound, rpcErr.Code)
}

func TestExtFS_RenameMove(t *testing.T) {
	svc := newLocalFileSvc(t)
	tr := extTransportWithFiles(svc)

	_, rpcErr := extCallHandler(t, tr.handleFSWriteFile, map[string]any{
		"path": "a.txt", "content": base64.StdEncoding.EncodeToString([]byte("x")), "create": true,
	})
	require.Nil(t, rpcErr)

	_, rpcErr = extCallHandler(t, tr.handleFSRename, map[string]any{"oldPath": "a.txt", "newPath": "b.txt"})
	require.Nil(t, rpcErr)

	_, rpcErr = extCallHandler(t, tr.handleFSStat, map[string]any{"path": "a.txt"})
	require.Equal(t, libacp.ErrResourceNotFound, rpcErr.Code)
	_, rpcErr = extCallHandler(t, tr.handleFSStat, map[string]any{"path": "b.txt"})
	require.Nil(t, rpcErr)
}

func TestExtFS_ReadFileOfDirectoryIsInvalidParams(t *testing.T) {
	svc := newLocalFileSvc(t)
	tr := extTransportWithFiles(svc)
	_, rpcErr := extCallHandler(t, tr.handleFSCreateDir, map[string]any{"path": "d"})
	require.Nil(t, rpcErr)

	_, rpcErr = extCallHandler(t, tr.handleFSReadFile, map[string]any{"path": "d"})
	require.NotNil(t, rpcErr)
	require.Equal(t, libacp.ErrInvalidParams, rpcErr.Code)
}
