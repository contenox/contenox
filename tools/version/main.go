package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		showHelp()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "set":
		setVersion()
	case "bump":
		if len(os.Args) < 3 {
			fmt.Println("Error: Must specify bump type (major, minor, patch)")
			os.Exit(1)
		}
		bumpVersion(os.Args[2])
	default:
		showHelp()
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Println("Version management tool")
	fmt.Println("Usage:")
	fmt.Println("  version set           - Set version from git")
	fmt.Println("  version bump TYPE     - Bump version (major, minor, patch)")
}

func getVersionFile() string {
	return "apiframework/version.txt"
}

func getCurrentVersion() (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git version: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func setVersion() {
	version, err := getCurrentVersion()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	filePath := getVersionFile()
	err = os.WriteFile(filePath, []byte(version), 0644)
	if err != nil {
		fmt.Printf("Error writing version file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Version set to: %s\n", version)
}

func bumpVersion(bumpType string) {
	// Get current latest tag
	cmd := exec.Command("git", "fetch", "--tags")
	cmd.Run() // Ignore error, might be first run

	// Get the latest tag
	cmd = exec.Command("git", "describe", "--tags", "--abbrev=0", "--exclude=*-*")
	output, err := cmd.CombinedOutput()
	currentTag := strings.TrimSpace(string(output))

	// If no tags exist, start with v0.1.0
	if err != nil || currentTag == "" || strings.Contains(currentTag, "fatal") {
		currentTag = "v0.1.0"
	}

	// Parse version
	major, minor, patch := parseVersion(currentTag)

	// Bump version based on type
	var newVersion string
	switch bumpType {
	case "major":
		newVersion = fmt.Sprintf("v%d.0.0", major+1)
	case "minor":
		newVersion = fmt.Sprintf("v%d.%d.0", major, minor+1)
	case "patch":
		newVersion = fmt.Sprintf("v%d.%d.%d", major, minor, patch+1)
	default:
		fmt.Printf("Error: Unknown bump type '%s'\n", bumpType)
		os.Exit(1)
	}

	// Update version file
	err = os.WriteFile(getVersionFile(), []byte(newVersion), 0644)
	if err != nil {
		fmt.Printf("Error writing version file: %v\n", err)
		os.Exit(1)
	}

	// Commit the version file change
	cmd = exec.Command("git", "add", getVersionFile())
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error adding version file: %v\n", err)
		os.Exit(1)
	}

	cmd = exec.Command("git", "commit", "-m", fmt.Sprintf("chore: release %s", newVersion))
	if err := cmd.Run(); err != nil {
		// If no changes to commit (already committed), continue
		if !strings.Contains(err.Error(), "nothing to commit") {
			fmt.Printf("Error committing version file: %v\n", err)
			os.Exit(1)
		}
	}

	// Create tag pointing to THIS commit
	cmd = exec.Command("git", "tag", "-a", newVersion, "-m", fmt.Sprintf("Release %s", newVersion))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error creating tag: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ Release %s created successfully!\n", newVersion)
	fmt.Printf("   Push with: git push origin %s\n", newVersion)
}

func parseVersion(tag string) (int, int, int) {
	// Remove 'v' prefix if present
	tag = strings.TrimPrefix(tag, "v")

	// Split version parts
	parts := strings.Split(tag, ".")

	// Parse version numbers
	major := 0
	minor := 0
	patch := 0

	if len(parts) > 0 {
		fmt.Sscanf(parts[0], "%d", &major)
	}
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &minor)
	}
	if len(parts) > 2 {
		fmt.Sscanf(parts[2], "%d", &patch)
	}

	return major, minor, patch
}
