package state

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// ConfigDir holds LiteNode's state, generated configuration and backups.
const ConfigDir = "/etc/litenode"

// Permissions under ConfigDir. Everything is set explicitly, never left to
// the umask or to an existing file's mode.
//
// ConfigDir itself is 0711 so that Xray, which runs as its own user (P-002),
// can open its config by name without being able to list the directory.
// Every other directory is 0700 and every file 0600, except files shared
// with a service group (only xray.json): those are 0640 with that group.
// Backups contain copies of xray.json with private keys, so they stay 0600.
const (
	ConfigDirMode   fs.FileMode = 0o711
	PrivateDirMode  fs.FileMode = 0o700
	PrivateFileMode fs.FileMode = 0o600
	GroupFileMode   fs.FileMode = 0o640
)

// CheckFileMode enforces the policy for a file written under dir (the
// config directory): mode 0600 without a group, or 0640 with a group.
// Files outside dir are not restricted.
func CheckFileMode(dir, path string, mode fs.FileMode, group string) error {
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return nil
	}
	switch {
	case mode == PrivateFileMode && group == "":
		return nil
	case mode == GroupFileMode && group != "":
		return nil
	}
	return fmt.Errorf("%s: mode %04o with group %q is not allowed under %s (use %04o, or %04o with a group)",
		path, mode, group, dir, PrivateFileMode, GroupFileMode)
}

// SharedConfigName is the only file under ConfigDir that another user (the
// Xray service group) may read.
const SharedConfigName = "xray.json"

// AuditPermissions walks dir (normally ConfigDir) and returns every entry
// that a user other than root could read: dir itself may only be
// traversable by others (0711), other directories and files must have no
// group or other permissions, and only dir/xray.json may be group-readable.
func AuditPermissions(dir string) ([]string, error) {
	var bad []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		perm := info.Mode().Perm()
		var allowed fs.FileMode
		switch {
		case path == dir:
			allowed = 0o011 // traverse only
		case !d.IsDir() && path == filepath.Join(dir, SharedConfigName):
			allowed = 0o040 // group read only
		}
		if extra := perm & 0o077 &^ allowed; extra != 0 {
			bad = append(bad, fmt.Sprintf("%s: mode %04o", path, perm))
		}
		return nil
	})
	return bad, err
}
