package services

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	// 0777: both the api user and qBittorrent (different UID) must be able to
	// read and write this directory. Security isolation between app users is
	// enforced at the application layer (JWT), not the filesystem layer.
	// os.Chmod is used after MkdirAll because MkdirAll applies the process
	// umask, which would silently strip write bits (e.g. 0777 → 0755).
	if err := os.MkdirAll(path, 0777); err != nil {
		return err
	}
	return os.Chmod(path, 0777)
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
	// 0777: must be writable by qBittorrent (different UID) for downloads.
	// os.Chmod overrides the process umask which would strip write bits.
	if err := os.MkdirAll(basePath, 0777); err != nil {
		return nil, err
	}
	if err := os.Chmod(basePath, 0777); err != nil {
		return nil, err
	}
	return os.OpenRoot(basePath)
}

// rootRelPath converts a user-supplied path into a relative path suitable for
// use with os.Root methods. os.Root (Go ≥1.24) requires relative paths
// (fs.ValidPath rejects any path beginning with "/").
func rootRelPath(subPath string) string {
	// filepath.Clean first to normalise ".." sequences etc., then strip the
	// leading "/" so the result is always relative.
	clean := strings.TrimLeft(filepath.Clean(subPath), "/")
	if clean == "" {
		return "."
	}
	return clean
}

func ListFiles(userID, subPath string) ([]FileInfo, error) {
	root, err := openUserRoot(userID)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	clean := rootRelPath(subPath)

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

	clean := rootRelPath(subPath)
	if clean == "." || clean == "" {
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

	clean := rootRelPath(subPath)

	// G304 resolved: os.Root.Open cannot escape the root directory.
	// root.Open returns an independent *os.File with its own fd, so we close
	// the root immediately after opening to avoid holding the directory fd open
	// for the duration of the (potentially long) file transfer.
	f, err := root.Open(clean)
	root.Close() // close root dir fd; the file fd is independent
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
			// Directory doesn't exist yet (e.g. new user with no downloads) — treat as 0 bytes.
			if os.IsNotExist(err) {
				return filepath.SkipAll
			}
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	if err == filepath.SkipAll {
		return 0, nil
	}
	return size, err
}

func CopyStream(dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, src)
}
