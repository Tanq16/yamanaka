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

type File struct {
	Path    string `json:"path"`
	Content string `json:"content"` // base64 encoded
}

// walks vault and returns slice of all files (skip .git)
func GetAllFiles(vaultPath string) ([]File, error) {
	state.FileSystemMutex.RLock()
	defer state.FileSystemMutex.RUnlock()
	var files []File
	err := filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || strings.Contains(path, ".git") {
			return nil
		}
		relPath, err := filepath.Rel(vaultPath, path)
		if err != nil {
			return err
		}
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

// removes all files and dirs from vault except .git
func CleanDir(vaultPath string) error {
	state.FileSystemMutex.Lock()
	defer state.FileSystemMutex.Unlock()
	entries, err := os.ReadDir(vaultPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(vaultPath, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// decompresses gzipped tar archive into destination
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
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
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

// writes content to a specific file path
func WriteFile(vaultPath, relPath string, content []byte) error {
	state.FileSystemMutex.Lock()
	defer state.FileSystemMutex.Unlock()
	fullPath := filepath.Join(vaultPath, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(fullPath, content, 0644)
}

// removes a file from vault
func DeleteFile(vaultPath, relPath string) error {
	state.FileSystemMutex.Lock()
	defer state.FileSystemMutex.Unlock()
	fullPath := filepath.Join(vaultPath, relPath)
	return os.Remove(fullPath)
}
