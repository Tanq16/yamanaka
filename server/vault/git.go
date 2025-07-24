package vault

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tanq16/yamanaka/server/state"
)

// initializes a git repository in the given path
func InitRepo(vaultPath string) error {
	gitPath := filepath.Join(vaultPath, ".git")
	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		cmd := exec.Command("git", "init")
		cmd.Dir = vaultPath
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run 'git init': %w", err)
		}
		configUserNameCmd := exec.Command("git", "config", "user.name", "yamanaka")
		configUserNameCmd.Dir = vaultPath
		if err := configUserNameCmd.Run(); err != nil {
			return fmt.Errorf("failed to set git config user.name: %w", err)
		}
		configUserEmailCmd := exec.Command("git", "config", "user.email", "yamanaka@obsidian.sync")
		configUserEmailCmd.Dir = vaultPath
		if err := configUserEmailCmd.Run(); err != nil {
			return fmt.Errorf("failed to set git config user.email: %w", err)
		}
	}
	return nil
}

// returns the latest commit hash (HEAD)
func GetCurrentHash(vaultPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = vaultPath
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", fmt.Errorf("failed to get current hash: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// stages all changes and creates a new commit
func CommitChanges(vaultPath, message string) (string, error) {
	state.FileSystemMutex.Lock()
	defer state.FileSystemMutex.Unlock()
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = vaultPath
	if output, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to run 'git add -A': %w\nOutput: %s", err, string(output))
	}
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = vaultPath
	statusOutput, err := statusCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run 'git status --porcelain': %w", err)
	}
	if len(strings.TrimSpace(string(statusOutput))) == 0 {
		return GetCurrentHash(vaultPath) // Return the existing hash
	}
	commitCmd := exec.Command("git", "commit", "-m", message)
	commitCmd.Dir = vaultPath
	if output, err := commitCmd.CombinedOutput(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if strings.Contains(string(exitErr.Stderr), "nothing to commit") || strings.Contains(string(output), "nothing to commit") {
				return GetCurrentHash(vaultPath) // Return the existing hash
			}
		}
		return "", fmt.Errorf("failed to run 'git commit -m \"%s\"': %w\nOutput: %s", message, err, string(output))
	}
	return GetCurrentHash(vaultPath)
}
