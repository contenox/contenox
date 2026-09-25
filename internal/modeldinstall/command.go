package modeldinstall

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// CommandContext constructs a worker command with its packaged native libraries.
// Windows batch launchers are resolved to their adjacent executable so arguments
// are passed directly, without interpreting paths or flags through a shell.
func CommandContext(ctx context.Context, launcher string, args ...string) *exec.Cmd {
	if runtime.GOOS != "windows" || !strings.EqualFold(filepath.Ext(launcher), ".cmd") {
		return exec.CommandContext(ctx, launcher, args...)
	}
	root := filepath.Dir(launcher)
	cmd := exec.CommandContext(ctx, filepath.Join(root, "modeld.exe"), args...)
	libs := filepath.Join(root, "lib", "llamacpp") + string(os.PathListSeparator) + filepath.Join(root, "modeld-libs")
	env := os.Environ()
	found := false
	for i, value := range env {
		name, path, ok := strings.Cut(value, "=")
		if ok && strings.EqualFold(name, "PATH") {
			env[i] = name + "=" + libs + string(os.PathListSeparator) + path
			found = true
			break
		}
	}
	if !found {
		env = append(env, "PATH="+libs)
	}
	cmd.Env = env
	return cmd
}
