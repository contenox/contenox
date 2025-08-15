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
			fmt.Println("Error: Must specify bump type (major, minor, patch, rc, beta)")
			os.Exit(1)
		}
		bumpType := os.Args[2]
		var rcName string
		if len(os.Args) >= 4 {
			rcName = os.Args[3]
		}
		bumpVersion(bumpType, rcName)
	case "finalize":
		finalizeRelease()
	default:
		showHelp()
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Println("Version management tool")
	fmt.Println("Usage:")
	fmt.Println("  version set           - Set version from git")
	fmt.Println("  version bump TYPE     - Bump version (major, minor, patch, rc, beta)")
	fmt.Println("  version finalize      - Finalize a release candidate")
}

func getVersionFile() string {
	return "apiframework/version.txt"
}

func getCurrentGitVersion() (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git version: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func setVersion() {
	version, err := getCurrentGitVersion()
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

func bumpVersion(bumpType, rcName string) {
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
	major, minor, patch, preReleaseType, preReleaseNum := parseVersion(currentTag)

	// Bump version based on type
	var newVersion string
	switch bumpType {
	case "major":
		newVersion = fmt.Sprintf("v%d.0.0", major+1)
	case "minor":
		newVersion = fmt.Sprintf("v%d.%d.0", major, minor+1)
	case "patch":
		newVersion = fmt.Sprintf("v%d.%d.%d", major, minor, patch+1)
	case "rc":
		if rcName == "" {
			rcName = "1"
		}
		if preReleaseType == "rc" {
			newVersion = fmt.Sprintf("v%d.%d.%d-rc%d", major, minor, patch, preReleaseNum+1)
		} else {
			newVersion = fmt.Sprintf("v%d.%d.%d-rc%s", major, minor, patch, rcName)
		}
	case "beta":
		if rcName == "" {
			rcName = "1"
		}
		if preReleaseType == "beta" {
			newVersion = fmt.Sprintf("v%d.%d.%d-beta%d", major, minor, patch, preReleaseNum+1)
		} else {
			newVersion = fmt.Sprintf("v%d.%d.%d-beta%s", major, minor, patch, rcName)
		}
	default:
		fmt.Printf("Error: Unknown bump type '%s'\n", bumpType)
		os.Exit(1)
	}

	// Create and push tag
	cmd = exec.Command("git", "tag", "-a", newVersion, "-m", fmt.Sprintf("Release %s", newVersion))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error creating tag: %v\n", err)
		os.Exit(1)
	}

	cmd = exec.Command("git", "push", "origin", newVersion)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error pushing tag: %v\n", err)
		// Don't exit here - maybe we're on a branch without push access
		fmt.Println("Warning: Failed to push tag to remote. You may need to push manually.")
	}

	// Update version file
	err = os.WriteFile(getVersionFile(), []byte(newVersion), 0644)
	if err != nil {
		fmt.Printf("Error writing version file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created new version: %s\n", newVersion)
}

func finalizeRelease() {
	// Get current version from file
	versionFile := getVersionFile()
	currentVersion, err := os.ReadFile(versionFile)
	if err != nil {
		fmt.Printf("Error reading version file: %v\n", err)
		os.Exit(1)
	}

	version := strings.TrimSpace(string(currentVersion))

	// Check if it's already a production version
	if !strings.Contains(version, "-") {
		fmt.Println("Current version is already a production release")
		return
	}

	// Remove pre-release suffix
	prodVersion := regexp.MustCompile(`-[a-zA-Z0-9.]+$`).ReplaceAllString(version, "")

	// Create production tag
	cmd := exec.Command("git", "tag", "-a", prodVersion, "-m", fmt.Sprintf("Production release %s", prodVersion))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error creating production tag: %v\n", err)
		os.Exit(1)
	}

	cmd = exec.Command("git", "push", "origin", prodVersion)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error pushing production tag: %v\n", err)
		// Don't exit here - maybe we're on a branch without push access
		fmt.Println("Warning: Failed to push tag to remote. You may need to push manually.")
	}

	// Update version file
	err = os.WriteFile(versionFile, []byte(prodVersion), 0644)
	if err != nil {
		fmt.Printf("Error writing version file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Finalized production release: %s\n", prodVersion)
}

func parseVersion(tag string) (int, int, int, string, int) {
	// Remove 'v' prefix if present
	tag = strings.TrimPrefix(tag, "v")

	// Extract pre-release info if present
	preReleaseType := ""
	preReleaseNum := 0
	if idx := strings.IndexAny(tag, "-"); idx != -1 {
		preRelease := tag[idx+1:]
		tag = tag[:idx]

		// Parse pre-release info
		if strings.HasPrefix(preRelease, "rc") {
			preReleaseType = "rc"
			numStr := strings.TrimPrefix(preRelease, "rc")
			if num, err := strconv.Atoi(numStr); err == nil {
				preReleaseNum = num
			} else {
				preReleaseNum = 1
			}
		} else if strings.HasPrefix(preRelease, "beta") {
			preReleaseType = "beta"
			numStr := strings.TrimPrefix(preRelease, "beta")
			if num, err := strconv.Atoi(numStr); err == nil {
				preReleaseNum = num
			} else {
				preReleaseNum = 1
			}
		}
	}

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

	return major, minor, patch, preReleaseType, preReleaseNum
}
