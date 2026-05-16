package hitlservice_test

import (
	"context"
	"errors"
	"testing"

	"github.com/contenox/contenox/libtracker"
	"github.com/contenox/contenox/runtime/hitlservice"
	"github.com/contenox/contenox/runtime/vfsservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nopKVReader struct{}

func (nopKVReader) GetKV(_ context.Context, _ string, _ any) error {
	return errors.New("not found")
}

type fixedKVReader struct{ name string }

func (f fixedKVReader) GetKV(_ context.Context, _ string, out any) error {
	if p, ok := out.(*string); ok {
		*p = f.name
	}
	return nil
}

func TestUnit_Evaluate_FallsBackToDefaultWhenFileMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	vfs := vfsservice.NewLocalFS(dir)
	svc := hitlservice.New(vfs, nopKVReader{}, libtracker.NoopTracker{})
	result, err := svc.Evaluate(context.Background(), "local_fs", "write_file", nil)
	require.NoError(t, err)
	assert.Equal(t, hitlservice.ActionApprove, result.Action)
}

func TestUnit_Evaluate_LoadsFromVFS(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	vfs := vfsservice.NewLocalFS(dir)
	ctx := context.Background()
	data := []byte(`{"default_action":"deny","rules":[{"tools":"webtools","tool":"call","action":"allow"}]}`)
	_, err := vfs.CreateFile(ctx, &vfsservice.File{Name: "hitl-policy.json", Data: data})
	require.NoError(t, err)

	svc := hitlservice.New(vfs, fixedKVReader{"hitl-policy.json"}, libtracker.NoopTracker{})
	result, err := svc.Evaluate(ctx, "webtools", "call", nil)
	require.NoError(t, err)
	assert.Equal(t, hitlservice.ActionAllow, result.Action)

	result, err = svc.Evaluate(ctx, "local_fs", "write_file", nil)
	require.NoError(t, err)
	assert.Equal(t, hitlservice.ActionDeny, result.Action)
}

func TestUnit_Evaluate_WhenConditionFromVFS(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	vfs := vfsservice.NewLocalFS(dir)
	ctx := context.Background()
	data := []byte(`{
		"default_action": "approve",
		"rules": [
			{"tools":"local_fs","tool":"write_file","when":[{"key":"path","op":"glob","value":"./src/**"}],"action":"allow"},
			{"tools":"local_fs","tool":"write_file","action":"approve","timeout_s":30,"on_timeout":"deny"}
		]
	}`)
	_, err := vfs.CreateFile(ctx, &vfsservice.File{Name: "hitl-policy.json", Data: data})
	require.NoError(t, err)

	svc := hitlservice.New(vfs, fixedKVReader{"hitl-policy.json"}, libtracker.NoopTracker{})

	result, err := svc.Evaluate(ctx, "local_fs", "write_file", map[string]any{"path": "./src/main.go"})
	require.NoError(t, err)
	assert.Equal(t, hitlservice.ActionAllow, result.Action)
	assert.Equal(t, 0, result.TimeoutS)

	result, err = svc.Evaluate(ctx, "local_fs", "write_file", map[string]any{"path": "./etc/passwd"})
	require.NoError(t, err)
	assert.Equal(t, hitlservice.ActionApprove, result.Action)
	assert.Equal(t, 30, result.TimeoutS)
	assert.Equal(t, hitlservice.ActionDeny, result.OnTimeout)
}

func TestUnit_Evaluate_ResolvesFromKV(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	vfs := vfsservice.NewLocalFS(dir)
	ctx := context.Background()

	data := []byte(`{"default_action":"deny","rules":[]}`)
	_, err := vfs.CreateFile(ctx, &vfsservice.File{Name: "hitl-policy-strict.json", Data: data})
	require.NoError(t, err)

	svc := hitlservice.New(vfs, fixedKVReader{"hitl-policy-strict.json"}, libtracker.NoopTracker{})
	result, err := svc.Evaluate(ctx, "local_fs", "write_file", nil)
	require.NoError(t, err)
	assert.Equal(t, hitlservice.ActionDeny, result.Action, "strict policy (deny-by-default) should deny write_file")
}

func TestUnit_Evaluate_FallsBackToBuiltinWhenKVEmptyAndFileMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	vfs := vfsservice.NewLocalFS(dir)
	svc := hitlservice.New(vfs, nopKVReader{}, libtracker.NoopTracker{})
	result, err := svc.Evaluate(context.Background(), "local_fs", "write_file", nil)
	require.NoError(t, err)
	assert.Equal(t, hitlservice.ActionApprove, result.Action, "built-in default requires approval for write_file")
}

func TestUnit_Evaluate_BuiltinDefaultIsFailClosedForUnaccountedTool(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	vfs := vfsservice.NewLocalFS(dir)
	svc := hitlservice.New(vfs, nopKVReader{}, libtracker.NoopTracker{})

	r, err := svc.Evaluate(context.Background(), "some_mcp_server", "arbitrary_tool", nil)
	require.NoError(t, err)
	assert.Equal(t, hitlservice.ActionApprove, r.Action, "a tool no policy rule accounts for must fail closed to approve, not silently allow")

	r, err = svc.Evaluate(context.Background(), "local_shell", "local_shell", nil)
	require.NoError(t, err)
	assert.Equal(t, hitlservice.ActionApprove, r.Action, "the editor shell must require approval; it is the model's preferred and most powerful tool")

	r, err = svc.Evaluate(context.Background(), "local_fs", "read_file", nil)
	require.NoError(t, err)
	assert.Equal(t, hitlservice.ActionAllow, r.Action, "read-only tools stay allowed so fail-closed HITL remains usable")
}

func TestUnit_Evaluate_DefaultPolicyOverrideSelectsACPPolicyWhenKVUnset(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	vfs := vfsservice.NewLocalFS(dir)
	ctx := context.Background()

	acp := []byte(`{"default_action":"approve","rules":[{"tools":"local_fs","tool":"read_file","action":"allow"},{"tools":"local_shell","tool":"local_shell","action":"approve"}]}`)
	_, err := vfs.CreateFile(ctx, &vfsservice.File{Name: "hitl-policy-acp.json", Data: acp})
	require.NoError(t, err)
	other := []byte(`{"default_action":"allow"}`)
	_, err = vfs.CreateFile(ctx, &vfsservice.File{Name: "hitl-policy.json", Data: other})
	require.NoError(t, err)

	svc := hitlservice.NewWithDefaultPolicy(vfs, nopKVReader{}, libtracker.NoopTracker{}, "hitl-policy-acp.json")
	r, err := svc.Evaluate(ctx, "local_shell", "local_shell", nil)
	require.NoError(t, err)
	assert.Equal(t, hitlservice.ActionApprove, r.Action, "with no KV set, the ACP entrypoint must fall back to hitl-policy-acp.json, not the generic default")
	r, err = svc.Evaluate(ctx, "local_fs", "read_file", nil)
	require.NoError(t, err)
	assert.Equal(t, hitlservice.ActionAllow, r.Action)

	explicit := hitlservice.NewWithDefaultPolicy(vfs, fixedKVReader{"hitl-policy.json"}, libtracker.NoopTracker{}, "hitl-policy-acp.json")
	r, err = explicit.Evaluate(ctx, "local_shell", "local_shell", nil)
	require.NoError(t, err)
	assert.Equal(t, hitlservice.ActionAllow, r.Action, "an explicit hitl-policy-name KV must still override the per-process ACP default")
}
