package modeldinstall

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUnit_ResolveArtifact(t *testing.T) {
	const base = "https://example.test/modeld"
	cases := []struct {
		goos, goarch string
		wantName     string
	}{
		{"linux", "amd64", "modeld-v0.32.5-linux-amd64.tar.gz"},
		{"darwin", "arm64", "modeld-v0.32.5-darwin-arm64.tar.gz"},
		{"windows", "amd64", "modeld-v0.32.5-windows-amd64.zip"},
	}
	for _, c := range cases {
		t.Run(c.goos+"-"+c.goarch, func(t *testing.T) {
			a, err := resolveArtifact(base, "v0.32.5", c.goos, c.goarch)
			if err != nil {
				t.Fatalf("resolveArtifact: %v", err)
			}
			if a.name != c.wantName {
				t.Fatalf("name = %q, want %q", a.name, c.wantName)
			}
			wantURL := base + "/v0.32.5/" + c.wantName
			if a.archiveURL != wantURL {
				t.Fatalf("archiveURL = %q, want %q", a.archiveURL, wantURL)
			}
			if a.sumURL != wantURL+".sha256" {
				t.Fatalf("sumURL = %q, want %q", a.sumURL, wantURL+".sha256")
			}
			if a.topLevelDir() != "modeld-v0.32.5-"+c.goos+"-"+c.goarch {
				t.Fatalf("topLevelDir = %q", a.topLevelDir())
			}
		})
	}
}

func TestUnit_ResolveArtifact_Rejections(t *testing.T) {
	if _, err := resolveArtifact(DefaultBaseURL, "dev", "linux", "amd64"); !errors.Is(err, ErrNoOfficialVersion) {
		t.Fatalf("dev version err = %v, want ErrNoOfficialVersion", err)
	}
	if _, err := resolveArtifact(DefaultBaseURL, "", "linux", "amd64"); !errors.Is(err, ErrNoOfficialVersion) {
		t.Fatalf("empty version err = %v, want ErrNoOfficialVersion", err)
	}
	if _, err := resolveArtifact(DefaultBaseURL, "v1.0.0", "plan9", "amd64"); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("plan9 err = %v, want ErrUnsupportedPlatform", err)
	}
}

func TestUnit_IsOfficialVersion(t *testing.T) {
	for v, want := range map[string]bool{
		"v0.32.5":  true,
		" v1.2.3 ": true,
		"":         false,
		"dev":      false,
		"main":     false,
		"unknown":  false,
		"1.2.3":    false,
	} {
		if got := IsOfficialVersion(v); got != want {
			t.Errorf("IsOfficialVersion(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestUnit_ParseSHA256(t *testing.T) {
	const sum = "e4fcaf5a6ffba8c2e28d7182914c48aeb767b7c46a3ffa0d60b19267487e0066"
	cases := map[string]string{
		"real sha256sum format": sum + "  modeld-v0.32.5-linux-amd64.tar.gz\n",
		"leading blank line":    "\n" + sum + "  file.tar.gz\n",
		"backslash prefix":      `\` + sum + "  weird name.tar.gz\n",
		"uppercase":             strings.ToUpper(sum) + "  file\n",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := parseSHA256(text)
			if err != nil {
				t.Fatalf("parseSHA256: %v", err)
			}
			if got != sum {
				t.Fatalf("got %q, want %q", got, sum)
			}
		})
	}

	for name, text := range map[string]string{
		"empty":     "",
		"too short": "abc123  file\n",
		"non-hex":   strings.Repeat("z", 64) + "  file\n",
	} {
		t.Run("reject "+name, func(t *testing.T) {
			if _, err := parseSHA256(text); err == nil {
				t.Fatalf("parseSHA256(%q) = nil err, want error", text)
			}
		})
	}
}

func TestUnit_VerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "blob")
	if err := os.WriteFile(p, []byte("hello modeld"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("hello modeld"))
	want := hex.EncodeToString(sum[:])

	if err := verifyChecksum(p, want); err != nil {
		t.Fatalf("verifyChecksum (match): %v", err)
	}
	if err := verifyChecksum(p, strings.Repeat("0", 64)); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("verifyChecksum (mismatch) = %v, want ErrChecksumMismatch", err)
	}
}

func TestUnit_GetSmallText_StatusMapping(t *testing.T) {
	cases := []struct {
		status  int
		wantErr error // nil => expect success
	}{
		{http.StatusOK, nil},
		{http.StatusNotFound, ErrNoPrebuiltArtifact},
		{http.StatusForbidden, ErrPublicAccess},
	}
	for _, c := range cases {
		t.Run(http.StatusText(c.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte("body"))
			}))
			defer srv.Close()
			c0 := &client{version: "v1", http: srv.Client()}
			body, err := c0.getSmallText(context.Background(), srv.URL)
			if c.wantErr == nil {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				if body != "body" {
					t.Fatalf("body = %q", body)
				}
				return
			}
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
		})
	}

	t.Run("500 is not a sentinel", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		c0 := &client{version: "v1", http: srv.Client()}
		_, err := c0.getSmallText(context.Background(), srv.URL)
		if err == nil || errors.Is(err, ErrNoPrebuiltArtifact) || errors.Is(err, ErrPublicAccess) {
			t.Fatalf("500 err = %v, want a non-sentinel error", err)
		}
	})
}

func TestUnit_SafeExtraction_RejectsUnsafeTar(t *testing.T) {
	for _, bad := range []string{"/etc/evil", "../escape", "sub/../../escape"} {
		t.Run(bad, func(t *testing.T) {
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gz)
			body := []byte("x")
			_ = tw.WriteHeader(&tar.Header{Name: bad, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
			_, _ = tw.Write(body)
			_ = tw.Close()
			_ = gz.Close()

			arc := filepath.Join(t.TempDir(), "a.tar.gz")
			if err := os.WriteFile(arc, buf.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			dest := t.TempDir()
			if err := extractTarGz(arc, dest); err == nil {
				t.Fatalf("extractTarGz(%q) = nil err, want rejection", bad)
			}
		})
	}
}

func TestUnit_SafeExtraction_RejectsUnsafeZip(t *testing.T) {
	for _, bad := range []string{"/etc/evil", "../escape"} {
		t.Run(bad, func(t *testing.T) {
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			w, err := zw.Create(bad)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte("x"))
			_ = zw.Close()

			arc := filepath.Join(t.TempDir(), "a.zip")
			if err := os.WriteFile(arc, buf.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			dest := t.TempDir()
			if err := extractZip(arc, dest); err == nil {
				t.Fatalf("extractZip(%q) = nil err, want rejection", bad)
			}
		})
	}
}

func TestUnit_CheckCapability(t *testing.T) {
	both := versionInfo{Version: "v1", Backends: []string{"llama", "openvino"}}
	if err := checkCapability(both, "v1", "llama"); err != nil {
		t.Fatalf("llama from both: %v", err)
	}
	if err := checkCapability(both, "v1", "openvino"); err != nil {
		t.Fatalf("openvino from both: %v", err)
	}

	llamaOnly := versionInfo{Version: "v1", Backends: []string{"llama"}}
	if err := checkCapability(llamaOnly, "v1", "openvino"); !errors.Is(err, ErrBackendMissing) {
		t.Fatalf("openvino from llama-only = %v, want ErrBackendMissing", err)
	}

	if err := checkCapability(both, "v2", "llama"); err == nil || errors.Is(err, ErrBackendMissing) {
		t.Fatalf("version mismatch = %v, want a version error", err)
	}
}

// --- end-to-end install against a fake release server ---

func TestUnit_EnsureInstalled_EndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake launcher is a POSIX shell script")
	}
	const version = "v9.9.9"
	platform := Platform(runtime.GOOS, runtime.GOARCH)
	archive, sum := buildFakeTarGz(t, version, platform)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			fmt.Fprintf(w, "%s  modeld-%s-%s.tar.gz\n", sum, version, platform)
		case strings.HasSuffix(r.URL.Path, ".tar.gz"):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dataRoot := t.TempDir()
	opts := Options{BaseURL: srv.URL, Version: version, DataRoot: dataRoot}

	res, err := EnsureInstalled(context.Background(), "openvino", opts)
	if err != nil {
		t.Fatalf("EnsureInstalled: %v", err)
	}
	wantLauncher := ManagedLauncherPath(dataRoot, version, runtime.GOOS, runtime.GOARCH)
	if res.LauncherPath != wantLauncher {
		t.Fatalf("launcher = %q, want %q", res.LauncherPath, wantLauncher)
	}
	if !fileExists(res.LauncherPath) {
		t.Fatalf("launcher not installed at %q", res.LauncherPath)
	}
	if res.Version != version {
		t.Fatalf("version = %q, want %q", res.Version, version)
	}
	if res.AlreadyInstalled {
		t.Fatalf("first install reported AlreadyInstalled")
	}
	// staging must be cleaned up.
	if entries, _ := os.ReadDir(filepath.Join(dataRoot, "modeld", ".staging")); len(entries) != 0 {
		t.Fatalf("staging left %d entries", len(entries))
	}

	// Second call reuses the existing install without re-downloading.
	res2, err := EnsureInstalled(context.Background(), "llama", opts)
	if err != nil {
		t.Fatalf("second EnsureInstalled: %v", err)
	}
	if !res2.AlreadyInstalled {
		t.Fatalf("second install did not report AlreadyInstalled")
	}
}

func TestUnit_EnsureInstalled_ChecksumMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake launcher is a POSIX shell script")
	}
	const version = "v9.9.9"
	platform := Platform(runtime.GOOS, runtime.GOARCH)
	archive, _ := buildFakeTarGz(t, version, platform)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			fmt.Fprintf(w, "%s  modeld-%s-%s.tar.gz\n", strings.Repeat("0", 64), version, platform)
		case strings.HasSuffix(r.URL.Path, ".tar.gz"):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dataRoot := t.TempDir()
	_, err := EnsureInstalled(context.Background(), "llama", Options{BaseURL: srv.URL, Version: version, DataRoot: dataRoot})
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("err = %v, want ErrChecksumMismatch", err)
	}
	if dirExists(ManagedInstallDir(dataRoot, version, runtime.GOOS, runtime.GOARCH)) {
		t.Fatalf("install dir created despite checksum mismatch")
	}
}

func TestUnit_EnsureInstalled_NoArtifact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := EnsureInstalled(context.Background(), "llama", Options{
		BaseURL: srv.URL, Version: "v9.9.9", DataRoot: t.TempDir(),
	})
	if !errors.Is(err, ErrNoPrebuiltArtifact) {
		t.Fatalf("err = %v, want ErrNoPrebuiltArtifact", err)
	}
}

// buildFakeTarGz produces a release-shaped archive: a single
// modeld-<version>-<platform>/ directory containing a runnable `modeld` launcher
// that prints the expected `version --json`. Returns the gzip bytes and their
// lowercase-hex SHA-256.
func buildFakeTarGz(t *testing.T, version, platform string) ([]byte, string) {
	t.Helper()
	top := fmt.Sprintf("modeld-%s-%s", version, platform)
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' '{\"version\":\"%s\",\"backends\":[\"llama\",\"openvino\"]}'\n", version)

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
