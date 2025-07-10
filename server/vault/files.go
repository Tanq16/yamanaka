package vault

import (
	"archive/tar"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tanq16/yamanaka/server/state"
)

// File represents a single file in the vault for API transfer.
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"` // base64 encoded
}

// GetAllFiles walks the vault directory and returns a slice of all files.
// It skips the .git directory.
// This function uses a read lock to ensure consistency while reading files.
func GetAllFiles(vaultPath string) ([]File, error) {
	state.FileSystemMutex.RLock()
	defer state.FileSystemMutex.RUnlock()

	var files []File
	err := filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip directories and the .git folder
		if info.IsDir() || strings.Contains(path, ".git") {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(vaultPath, path)
		if err != nil {
			return err
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		files = append(files, File{
			Path:    filepath.ToSlash(relPath), // Ensure forward slashes for consistency
			Content: base64.StdEncoding.EncodeToString(content),
		})
		return nil
	})
	return files, err
}

// CleanDir removes all files and directories from the vault path, except for .git.
func CleanDir(vaultPath string) error {
	state.FileSystemMutex.Lock()
	defer state.FileSystemMutex.Unlock()

	entries, err := os.ReadDir(vaultPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue // Skip the git directory
		}
		if err := os.RemoveAll(filepath.Join(vaultPath, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// ExtractTarGz decompresses a gzipped tar archive into the destination path.
func ExtractTarGz(gzipStream io.Reader, dst string) error {
	state.FileSystemMutex.Lock()
	defer state.FileSystemMutex.Unlock()

	uncompressedStream, err := gzip.NewReader(gzipStream)
	if err != nil {
		return err
	}
	defer uncompressedStream.Close()

	tarReader := tar.NewReader(uncompressedStream)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dst, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			// Create the file
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			// Copy content
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		default:
			return fmt.Errorf("unsupported file type in tar: %c for %s", header.Typeflag, header.Name)
		}
	}
	return nil
}

// WriteFile writes content to a specific file path within the vault, creating parent directories if needed.
func WriteFile(vaultPath, relPath string, content []byte) error {
	state.FileSystemMutex.Lock()
	defer state.FileSystemMutex.Unlock()

	fullPath := filepath.Join(vaultPath, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(fullPath, content, 0644)
}

// DeleteFile removes a file from the vault.
func DeleteFile(vaultPath, relPath string) error {
	state.FileSystemMutex.Lock()
	defer state.FileSystemMutex.Unlock()

	fullPath := filepath.Join(vaultPath, relPath)
	return os.Remove(fullPath)
}
