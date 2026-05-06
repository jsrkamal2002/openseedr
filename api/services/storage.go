package services

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func UserStoragePath(userID string) string {
	base := os.Getenv("STORAGE_PATH")
	if base == "" {
		base = "/data"
	}
	return filepath.Join(base, userID)
}

func EnsureUserDir(userID string) error {
	path := UserStoragePath(userID)
	// G301: 0750 — owner rwx, group rx, no world access
	return os.MkdirAll(path, 0750)
}

type FileInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime int64  `json:"mod_time"`
	Path    string `json:"path"`
}

// openUserRoot opens an *os.Root scoped to the user's storage directory.
// os.Root (Go ≥1.24) prevents directory-traversal attacks at the OS level.
func openUserRoot(userID string) (*os.Root, error) {
	basePath := UserStoragePath(userID)
	if err := os.MkdirAll(basePath, 0750); err != nil {
		return nil, err
	}
	return os.OpenRoot(basePath)
}

func ListFiles(userID, subPath string) ([]FileInfo, error) {
	root, err := openUserRoot(userID)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	clean := filepath.Clean(subPath)
	if clean == "/" {
		clean = "."
	}

	// Open the target directory, then ReadDir on the *os.File
	dir, err := root.Open(clean)
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		relPath := filepath.Join(subPath, e.Name())
		files = append(files, FileInfo{
			Name:    e.Name(),
			Size:    info.Size(),
			IsDir:   e.IsDir(),
			ModTime: info.ModTime().Unix(),
			Path:    relPath,
		})
	}
	return files, nil
}

func DeleteFile(userID, subPath string) error {
	root, err := openUserRoot(userID)
	if err != nil {
		return err
	}
	defer root.Close()

	clean := filepath.Clean(subPath)
	if clean == "." || clean == "/" || clean == "" {
		return errors.New("cannot delete root directory")
	}

	// root.RemoveAll is traversal-safe and handles both files and directories
	return root.RemoveAll(clean)
}

// OpenFile opens a file for reading within the user's storage.
// Uses os.Root for traversal-safe access. Caller must close the returned file.
func OpenFile(userID, subPath string) (*os.File, os.FileInfo, error) {
	root, err := openUserRoot(userID)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()

	clean := filepath.Clean(subPath)

	// G304 resolved: os.Root.Open cannot escape the root directory
	f, err := root.Open(clean)
	if err != nil {
		return nil, nil, err
	}

	info, err := f.Stat()
	if err != nil {
		// G104 resolved: propagate close error alongside stat error
		if cerr := f.Close(); cerr != nil {
			return nil, nil, fmt.Errorf("stat: %w; close: %v", err, cerr)
		}
		return nil, nil, err
	}

	return f, info, nil
}

func DirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func CopyStream(dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, src)
}
