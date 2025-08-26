package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
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
	fmt.Println("  version set        - Set version from git describe")
	fmt.Println("  version bump TYPE  - Bump version (major, minor, patch)")
}

func getVersionFile() string {
	return "internal/apiframework/version.txt"
}

func getCurrentDescribeVersion() (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git version: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func setVersion() {
	version, err := getCurrentDescribeVersion()
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

	// 5. Update compose file BEFORE committing anything
	if err := updateComposeFile(newVersion); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		// Revert the version file change if it was already done
		os.WriteFile(getVersionFile(), []byte(currentVersion), 0644)
		os.Exit(1)
	}

	// 6. Update version file
	if err := updateVersionFile(newVersion); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}

	// 7. Commit the version file
	if err := commitVersionFile(newVersion); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		// Revert the version file change
		os.WriteFile(getVersionFile(), []byte(currentVersion), 0644)
		updateComposeFile(currentVersion) // Revert compose file
		os.Exit(1)
	}

	// 8. Create tag
	if err := createTag(newVersion); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		// Revert the commit
		exec.Command("git", "reset", "HEAD~1").Run()
		// Revert the version file
		os.WriteFile(getVersionFile(), []byte(currentVersion), 0644)
		os.Exit(1)
	}

	// 9. Regenerate docs and amend the release commit
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
		// If the output indicates there's nothing to commit, it means the docs were already up-to-date.
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

// hasUncommittedChanges checks for any changes in the git repository, ignoring the version file.
func hasUncommittedChanges() bool {
	versionFilePath := getVersionFile()

	cmd := exec.Command("git", "status", "--porcelain")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Git error checking for uncommitted changes: %s\n", string(output))
		return true // Fail safe
	}

	lines := strings.Split(string(output), "\n")

	// Correctly iterate over the lines of output.
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		// If a line indicates a change and it's NOT the version file,
		// then we have uncommitted changes that need to be addressed.
		// We check the file path at the end of the status line.
		if !strings.HasSuffix(trimmedLine, versionFilePath) {
			return true
		}
	}

	return false
}

// getCurrentTagVersion fetches the latest semantic version tag from the repository.
func getCurrentTagVersion() (string, error) {
	// Fetch all tags from the remote repository to ensure we have the latest ones.
	cmd := exec.Command("git", "fetch", "--tags")
	cmd.Run() // We can ignore errors here, as it might fail in offline scenarios.

	// Get the latest tag by sorting them using version semantics (-v:refname).
	cmd = exec.Command("git", "tag", "--sort=-v:refname")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// This might fail if there are no tags yet.
		if strings.Contains(string(output), "No names found") || len(output) == 0 {
			fmt.Println("No existing tags found. Starting with v0.1.0.")
			return "v0.1.0", nil
		}
		return "", fmt.Errorf("failed to get latest git tag: %w\nOutput: %s", err, string(output))
	}

	tags := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(tags) == 0 || tags[0] == "" {
		// No tags exist, so we start from the initial version.
		fmt.Println("No existing tags found. Starting with v0.1.0.")
		return "v0.1.0", nil
	}

	// The first line will be the latest version tag.
	latestTag := tags[0]
	return latestTag, nil
}

// calculateNewVersion increments a semantic version string based on the bump type.
func calculateNewVersion(currentVersion, bumpType string) (string, error) {
	// Remove 'v' prefix for parsing
	if !strings.HasPrefix(currentVersion, "v") {
		return "", fmt.Errorf("invalid version format: missing 'v' prefix in '%s'", currentVersion)
	}
	tag := strings.TrimPrefix(currentVersion, "v")

	parts := strings.Split(tag, ".")
	if len(parts) != 3 {
		// Handle potential non-semver tags by starting fresh
		fmt.Printf("Warning: Could not parse current tag '%s'. Defaulting to v0.1.0 for new version.\n", currentVersion)
		return "v0.1.0", nil
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", fmt.Errorf("invalid major version in '%s': %w", tag, err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid minor version in '%s': %w", tag, err)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", fmt.Errorf("invalid patch version in '%s': %w", tag, err)
	}

	// Bump version based on type
	switch bumpType {
	case "major":
		major++
		minor = 0
		patch = 0
	case "minor":
		minor++
		patch = 0
	case "patch":
		patch++
	default:
		return "", fmt.Errorf("unknown bump type '%s'. Use 'major', 'minor', or 'patch'", bumpType)
	}

	return fmt.Sprintf("v%d.%d.%d", major, minor, patch), nil
}

func updateVersionFile(newVersion string) error {
	fmt.Printf("📝 Updating version file to %s...\n", newVersion)
	return os.WriteFile(getVersionFile(), []byte(newVersion), 0644)
}

func commitVersionFile(newVersion string) error {
	fmt.Println("📦 Committing version and compose files...")

	// Add BOTH files to the commit
	cmd := exec.Command("git", "add", getVersionFile(), "compose.yaml")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add version and compose files: %w\nOutput: %s", err, string(output))
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

	// Create an annotated tag pointing to the release commit
	cmd := exec.Command("git", "tag", "-a", newVersion, "-m", fmt.Sprintf("Release %s", newVersion))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create tag: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func updateComposeFile(newVersion string) error {
	composePath := "compose.yaml"

	// Check if compose file exists
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		fmt.Println("   ⚠️  compose.yaml not found, skipping compose update")
		return nil
	}

	fmt.Printf("   🔄 Updating %s to use version %s...\n", composePath, newVersion)

	// Read the compose file
	content, err := os.ReadFile(composePath)
	if err != nil {
		return fmt.Errorf("failed to read compose file: %w", err)
	}

	// Replace the runtime image tag
	updatedContent := []byte(
		regexp.MustCompile(`image: ghcr\.io/contenox/runtime:[^\s]+`).ReplaceAllString(
			string(content),
			"image: ghcr.io/contenox/runtime:"+newVersion,
		),
	)

	// Write the updated content
	if err := os.WriteFile(composePath, updatedContent, 0644); err != nil {
		return fmt.Errorf("failed to write compose file: %w", err)
	}

	return nil
}
