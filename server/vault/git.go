package vault

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tanq16/yamanaka/server/state"
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
	state.FileSystemMutex.Lock()
	defer state.FileSystemMutex.Unlock()

	// Stage all changes (new, modified, deleted files)
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = vaultPath
	if output, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to run 'git add -A': %w\nOutput: %s", err, string(output))
	}

	// Check git status to see if there's anything to commit
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = vaultPath
	statusOutput, err := statusCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run 'git status --porcelain': %w", err)
	}

	if len(strings.TrimSpace(string(statusOutput))) == 0 {
		// No changes to commit
		return GetCurrentHash(vaultPath) // Return the existing hash
	}

	// Commit the staged changes
	commitCmd := exec.Command("git", "commit", "-m", message)
	commitCmd.Dir = vaultPath
	if output, err := commitCmd.CombinedOutput(); err != nil {
		// It's possible there were no changes to commit (though we checked with status).
		// This might also catch other commit errors.
		if exitErr, ok := err.(*exec.ExitError); ok {
			if strings.Contains(string(exitErr.Stderr), "nothing to commit") || strings.Contains(string(output), "nothing to commit") {
				return GetCurrentHash(vaultPath) // Return the existing hash
			}
		}
		return "", fmt.Errorf("failed to run 'git commit -m \"%s\"': %w\nOutput: %s", message, err, string(output))
	}

	// Return the new hash
	return GetCurrentHash(vaultPath)
}
