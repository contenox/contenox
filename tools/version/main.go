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
	// 1. Verify we're in a git repository
	if !isGitRepository() {
		fmt.Println("ERROR: Not in a git repository")
		os.Exit(1)
	}

	// 2. Check for uncommitted changes (this now correctly ignores the version file)
	if hasUncommittedChanges() {
		fmt.Println("ERROR: Cannot create release with uncommitted changes.")
		fmt.Println("Please commit or stash your changes first.")
		os.Exit(1)
	}

	// 3. Get current version
	currentVersion, err := getCurrentTagVersion()
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Current version: %s\n", currentVersion)

	// 4. Calculate new version
	newVersion, err := calculateNewVersion(currentVersion, bumpType)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("New version will be: %s\n", newVersion)

	// 5. Update version file
	if err := updateVersionFile(newVersion); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}

	// 6. Commit the version file
	if err := commitVersionFile(newVersion); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		// Revert the version file change
		os.WriteFile(getVersionFile(), []byte(currentVersion), 0644)
		os.Exit(1)
	}

	// 7. Create tag
	if err := createTag(newVersion); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		// Revert the commit
		exec.Command("git", "reset", "HEAD~1").Run()
		// Revert the version file
		os.WriteFile(getVersionFile(), []byte(currentVersion), 0644)
		os.Exit(1)
	}

	// 8. Regenerate docs and amend the release commit
	fmt.Println("\n🔄 Regenerating documentation with new version...")
	if err := updateDocsAndAmendCommit(); err != nil {
		fmt.Printf("⚠️  WARNING: Failed to update documentation: %v\n", err)
		fmt.Println("   The tag was created, but the docs need to be updated and committed manually.")
	}

	fmt.Printf("\n✅ Release %s created successfully!\n", newVersion)
	fmt.Printf("   Push with: git push && git push origin %s\n", newVersion)
}

func updateDocsAndAmendCommit() error {
	// Regenerate OpenAPI spec and Markdown.
	// We run 'make docs-markdown' as it handles both steps.
	cmd := exec.Command("make", "docs-markdown")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to run 'make docs-markdown': %w\nOutput: %s", err, string(output))
	}

	// Add the updated docs to the index
	cmd = exec.Command("git", "add", "docs/")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to git add docs/: %w\nOutput: %s", err, string(output))
	}

	cmd = exec.Command("git", "commit", "--amend", "--no-edit")
	if output, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "nothing to commit") {
			fmt.Println("   Documentation was already up-to-date.")
			return nil
		}
		return fmt.Errorf("failed to amend commit: %w\nOutput: %s", err, string(output))
	}

	fmt.Println("   Documentation updated and included in the release commit.")
	return nil
}

func isGitRepository() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	err := cmd.Run()
	return err == nil
}

func hasUncommittedChanges() bool {
	versionFilePath := getVersionFile()

	cmd := exec.Command("git", "status", "--porcelain")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Git error: %s\n", string(output))
		return true
	}

	lines := strings.SplitSeq(string(output), "\n")

	for line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		if !strings.Contains(trimmedLine, versionFilePath) {
			return true
		}
	}

	return false
}

func getCurrentTagVersion() (string, error) {
	// Try to get the latest tag
	cmd := exec.Command("git", "fetch", "--tags")
	cmd.Run() // Ignore error

	cmd = exec.Command("git", "describe", "--tags", "--abbrev=0", "--exclude=*-*")
	output, err := cmd.CombinedOutput()
	currentTag := strings.TrimSpace(string(output))

	// If no tags exist, start with v0.1.0
	if err != nil || currentTag == "" || strings.Contains(currentTag, "fatal") {
		return "v0.1.0", nil
	}

	return currentTag, nil
}

func calculateNewVersion(currentVersion, bumpType string) (string, error) {
	// Remove 'v' prefix if present
	tag := strings.TrimPrefix(currentVersion, "v")

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

	// Bump version based on type
	switch bumpType {
	case "major":
		return fmt.Sprintf("v%d.0.0", major+1), nil
	case "minor":
		return fmt.Sprintf("v%d.%d.0", major, minor+1), nil
	case "patch":
		return fmt.Sprintf("v%d.%d.%d", major, minor, patch+1), nil
	default:
		return "", fmt.Errorf("unknown bump type '%s'", bumpType)
	}
}

func updateVersionFile(newVersion string) error {
	fmt.Printf("📝 Updating version file to %s...\n", newVersion)
	return os.WriteFile(getVersionFile(), []byte(newVersion), 0644)
}

func commitVersionFile(newVersion string) error {
	fmt.Println("📦 Committing version file...")

	// Add the file
	cmd := exec.Command("git", "add", getVersionFile())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add version file: %w\nOutput: %s", err, string(output))
	}

	// Commit the change
	cmd = exec.Command("git", "commit", "-m", fmt.Sprintf("chore: release %s", newVersion))
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to commit version file: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func createTag(newVersion string) error {
	fmt.Printf("🔖 Creating tag %s...\n", newVersion)

	// Create tag pointing to THIS commit
	cmd := exec.Command("git", "tag", "-a", newVersion, "-m", fmt.Sprintf("Release %s", newVersion))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create tag: %w\nOutput: %s", err, string(output))
	}

	return nil
}
