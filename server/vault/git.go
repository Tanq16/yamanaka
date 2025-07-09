package vault

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// InitRepo initializes a new git repository in the given path if one doesn't exist.
func InitRepo(vaultPath string) error {
	gitPath := filepath.Join(vaultPath, ".git")
	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		cmd := exec.Command("git", "init")
		cmd.Dir = vaultPath
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run 'git init': %w", err)
		}
	}
	return nil
}

// GetCurrentHash returns the latest commit hash (HEAD) of the repository.
func GetCurrentHash(vaultPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = vaultPath
	out, err := cmd.Output()
	if err != nil {
		// This can happen in a new repo with no commits yet. Return an empty hash.
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil 
		}
		return "", fmt.Errorf("failed to get current hash: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CommitChanges stages all changes and creates a new commit.
// It returns the new commit hash.
func CommitChanges(vaultPath, message string) (string, error) {
	// Stage all changes (new, modified, deleted files)
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = vaultPath
	if err := addCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run 'git add': %w", err)
	}

	// Commit the staged changes
	commitCmd := exec.Command("git", "commit", "-m", message)
	commitCmd.Dir = vaultPath
	if err := commitCmd.Run(); err != nil {
		// It's possible there were no changes to commit. This is not a fatal error.
		if exitErr, ok := err.(*exec.ExitError); ok {
			if strings.Contains(string(exitErr.Stderr), "nothing to commit") {
				return GetCurrentHash(vaultPath) // Return the existing hash
			}
		}
		return "", fmt.Errorf("failed to run 'git commit': %w", err)
	}

	// Return the new hash
	return GetCurrentHash(vaultPath)
}

