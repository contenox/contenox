package contenoxcli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/internal/modeldinstall"
	"github.com/contenox/runtime/runtime/transport"
)

func TestUnit_LocalModeldSourceBuildStepsKeepModelChoices(t *testing.T) {
	oldVersion := Version
	Version = "v9.9.9"
	t.Cleanup(func() { Version = oldVersion })

	var llama bytes.Buffer
	printLocalModeldSourceBuildSteps(&llama, "llama")
	got := llama.String()
	for _, want := range []string{
		"git clone --branch v9.9.9",
		"CONTENOX_MODELD_BACKEND=llama make run-modeld",
		"FITS",
		"VRAM",
		"USE",
		"EST. RESIDENT",
		"qwen2.5-coder-7b",
		"devstral-small-2507",
		"qwen3-4b",
		"qwen3-8b",
		"qwen3-coder-30b-a3b",
		"gemma4-26b-a4b",
		"Optional VS Code autocomplete model",
		"default-autocomplete-provider llama",
		"default-autocomplete-model qwen3-coder-30b-a3b",
		"contenox model registry-list",
		"docs/development/modeld-source-build.md",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("llama modeld setup text missing %q:\n%s", want, got)
		}
	}

	var openvino bytes.Buffer
	printLocalModeldSourceBuildSteps(&openvino, "openvino")
	got = openvino.String()
	for _, want := range []string{
		"make deps-modeld",
		"CONTENOX_MODELD_BACKEND=openvino make run-modeld",
		"qwen2.5-coder-0.5b-ov",
		"qwen2.5-coder-7b-ov",
		"qwen2.5-coder-14b-ov",
		"qwen3-8b-ov",
		"qwen3-coder-30b-a3b-ov",
		"default-autocomplete-provider openvino",
		"default-autocomplete-model qwen2.5-coder-1.5b-ov",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("openvino modeld setup text missing %q:\n%s", want, got)
		}
	}
}

// On a successful prebuilt check, setup installs the package and shows the live
// next-step commands instead of the source-build instructions.
func TestUnit_RunLocalModeldSetup_InstallsPrebuilt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake launcher is a POSIX shell script")
	}
	const version = "v9.9.9"
	platform := runtime.GOOS + "-" + runtime.GOARCH
	archive, sum := buildFakeModeldArchive(t, version, platform)
	index := buildFakeModeldIndex(version, platform)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/index.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(index))
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			fmt.Fprintf(w, "%s  modeld-%s-%s.tar.gz\n", sum, version, platform)
		case strings.HasSuffix(r.URL.Path, ".tar.gz"):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	runLocalModeldSetup(&out, "openvino", modeldinstall.Options{
		BaseURL:       srv.URL,
		ClientVersion: "v99.99.99",
		DataRoot:      t.TempDir(),
		Progress:      &out,
	})
	got := out.String()
	for _, want := range []string{
		"Resolving a compatible modeld build",
		"Selected modeld v9.9.9",
		"Validated modeld v9.9.9 (protocol 1) with compiled backends: llama, openvino",
		"Start modeld:",
		"serve",
		"qwen2.5-coder-0.5b-ov", // openvino model choices still shown
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("install output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "git clone") {
		t.Fatalf("install path should not print source-build steps:\n%s", got)
	}
}

// When the release index is unavailable, setup keeps the source-build
// guidance and the model choices remain visible.
func TestUnit_RunLocalModeldSetup_FallsBackOnMissingIndex(t *testing.T) {
	oldVersion := Version
	Version = "v9.9.9"
	t.Cleanup(func() { Version = oldVersion })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	var out bytes.Buffer
	runLocalModeldSetup(&out, "llama", modeldinstall.Options{
		BaseURL:  srv.URL,
		DataRoot: t.TempDir(),
		Progress: &out,
	})
	got := out.String()
	for _, want := range []string{
		"Could not reach the modeld release index",
		"Use the source-build path",
		"git clone --branch v9.9.9",
		"qwen3-8b", // llama model choices still shown
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("fallback output missing %q:\n%s", want, got)
		}
	}
}

// A checksum mismatch is a hard failure: it is reported and does NOT fall through
// to the source-build path.
func TestUnit_RunLocalModeldSetup_ChecksumMismatchIsHardFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake launcher is a POSIX shell script")
	}
	const version = "v9.9.9"
	platform := runtime.GOOS + "-" + runtime.GOARCH
	archive, _ := buildFakeModeldArchive(t, version, platform)
	index := buildFakeModeldIndex(version, platform)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/index.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(index))
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			fmt.Fprintf(w, "%s  modeld-%s-%s.tar.gz\n", strings.Repeat("0", 64), version, platform)
		case strings.HasSuffix(r.URL.Path, ".tar.gz"):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	runLocalModeldSetup(&out, "llama", modeldinstall.Options{
		BaseURL:  srv.URL,
		DataRoot: t.TempDir(),
		Progress: &out,
	})
	got := out.String()
	if !strings.Contains(got, "failed checksum verification") {
		t.Fatalf("checksum failure not reported:\n%s", got)
	}
	if strings.Contains(got, "git clone") {
		t.Fatalf("checksum mismatch must not fall back to source-build:\n%s", got)
	}
}

func buildFakeModeldArchive(t *testing.T, version, platform string) ([]byte, string) {
	t.Helper()
	top := fmt.Sprintf("modeld-%s-%s", version, platform)
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' '{\"version\":\"%s\",\"protocol\":%d,\"backends\":[\"llama\",\"openvino\"]}'\n", version, transport.ProtocolVersion)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: top + "/", Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
		t.Fatal(err)
	}
	body := []byte(script)
	if err := tw.WriteHeader(&tar.Header{Name: top + "/modeld", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

func buildFakeModeldIndex(version, platform string) string {
	name := fmt.Sprintf("modeld-%s-%s.tar.gz", version, platform)
	return fmt.Sprintf(`{"schema":1,"builds":[{"version":%q,"platform":%q,"protocol":%d,"backends":["llama","openvino"],"channel":"stable","archive":%q,"sha256":%q,"size":123}]}`,
		version, platform, transport.ProtocolVersion, version+"/"+name, version+"/"+name+".sha256")
}
