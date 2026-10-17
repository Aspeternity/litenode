package system

import (
	"fmt"
	"io/fs"
)

// EnsureDir creates a directory if needed and then sets its mode
// explicitly, so neither the umask nor an existing directory's mode is
// relied on.
func EnsureDir(r Runner, path string, perm fs.FileMode) error {
	if err := r.MkdirAll(path, perm); err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := r.Chmod(path, perm); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}
