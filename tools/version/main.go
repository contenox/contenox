package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const versionFile = "internal/version/version.txt"

func main() {
	if len(os.Args) != 2 || os.Args[1] != "set" {
		fmt.Fprintf(os.Stderr, "usage: version set\n\nWrites `git describe --tags --always --dirty` to %s so a local build\nreports where it came from. Releases are stamped upstream, before the source drop.\n", versionFile)
		os.Exit(2)
	}
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "git describe: %v\n", err)
		os.Exit(1)
	}
	v := strings.TrimSpace(string(out))
	if err := os.WriteFile(versionFile, []byte(v), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", versionFile, err)
		os.Exit(1)
	}
	fmt.Printf("Version set to: %s\n", v)
}
