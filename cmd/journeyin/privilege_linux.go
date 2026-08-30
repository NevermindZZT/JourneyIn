//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	dockerDataRoot = "/data"
	dockerDataUID  = 65532
	dockerDataGID  = 65532
)

// prepareDockerRuntime makes a bind-mounted /data usable without requiring a
// host-side chown, then permanently drops the process to the non-root runtime
// identity before SQLite or the HTTP server is initialized.
func prepareDockerRuntime(dataPath string) error {
	runtimeMode := strings.TrimSpace(os.Getenv("JOURNEYIN_DOCKER_RUNTIME"))
	if runtimeMode == "" {
		return nil
	}
	if runtimeMode != "1" {
		return fmt.Errorf("JOURNEYIN_DOCKER_RUNTIME must be 1 when set")
	}

	// A caller that overrides the image user is responsible for its own file
	// permissions. The normal image starts as root only for this small bootstrap.
	if os.Geteuid() != 0 {
		return nil
	}

	if strings.TrimSpace(os.Getenv("JOURNEYIN_DOCKER_AUTO_FIX_PERMISSIONS")) == "1" {
		if dataPath != ":memory:" {
			cleanPath := filepath.Clean(dataPath)
			if cleanPath == dockerDataRoot || !dockerPathWithin(cleanPath) {
				return fmt.Errorf("JOURNEYIN_DATA_DIR must point to a file under %s when Docker permission repair is enabled", dockerDataRoot)
			}
			if err := ensureDockerDirectory(filepath.Dir(cleanPath)); err != nil {
				return err
			}
			for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
				if err := ensureDockerFile(cleanPath + suffix); err != nil {
					return err
				}
			}
		} else if err := ensureDockerDirectory(dockerDataRoot); err != nil {
			return err
		}
	}

	// Set supplementary groups before changing the primary group and uid. The
	// resulting process is the same PID, so it continues to receive signals as
	// PID 1 in the container.
	if err := syscall.Setgroups([]int{dockerDataGID}); err != nil {
		return fmt.Errorf("drop supplementary groups to %d: %w", dockerDataGID, err)
	}
	if err := syscall.Setgid(dockerDataGID); err != nil {
		return fmt.Errorf("drop group to %d: %w", dockerDataGID, err)
	}
	if err := syscall.Setuid(dockerDataUID); err != nil {
		return fmt.Errorf("drop user to %d: %w", dockerDataUID, err)
	}
	return nil
}

func dockerPathWithin(path string) bool {
	cleanPath := filepath.Clean(path)
	relative, err := filepath.Rel(dockerDataRoot, cleanPath)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)))
}

func ensureDockerDirectory(path string) error {
	cleanPath := filepath.Clean(path)
	if !dockerPathWithin(cleanPath) {
		return fmt.Errorf("refusing to prepare data directory outside %s: %s", dockerDataRoot, cleanPath)
	}

	if err := ensureDockerDirectoryEntry(dockerDataRoot); err != nil {
		return err
	}
	relative, err := filepath.Rel(dockerDataRoot, cleanPath)
	if err != nil {
		return fmt.Errorf("resolve data directory %s: %w", cleanPath, err)
	}
	if relative == "." {
		return nil
	}

	current := dockerDataRoot
	for _, part := range strings.Split(relative, string(os.PathSeparator)) {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid data directory component in %s", cleanPath)
		}
		current = filepath.Join(current, part)
		if err := ensureDockerDirectoryEntry(current); err != nil {
			return err
		}
	}
	return nil
}

func ensureDockerDirectoryEntry(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.Mkdir(path, 0o750); err != nil {
			return fmt.Errorf("create data directory %s: %w", path, err)
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("inspect data directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink in data directory: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("data path is not a directory: %s", path)
	}
	return ensureDockerOwnership(path)
}

func ensureDockerFile(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect SQLite sidecar %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink for SQLite file: %s", path)
	}
	if info.IsDir() {
		return fmt.Errorf("SQLite path is a directory: %s", path)
	}
	return ensureDockerOwnership(path)
}

func ensureDockerOwnership(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect data path %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink for data path: %s", path)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid == dockerDataUID && stat.Gid == dockerDataGID {
		return nil
	}
	if err := os.Lchown(path, dockerDataUID, dockerDataGID); err != nil {
		return fmt.Errorf("set data ownership on %s: %w", path, err)
	}
	return nil
}
