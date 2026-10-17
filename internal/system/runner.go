// Package system defines the Runner interface for executing system commands
// (with dry-run support), system detection and capability probing.
//
// Every system command, file write and service change goes through a Runner.
// Read-only operations (Query, ReadFile) always execute, including in dry-run
// mode, so menus can show the real system state. Mutating operations (Exec,
// WriteFile, Remove, RemoveAll, Rename, MkdirAll, Chmod) are only printed
// in dry-run mode.
package system

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
)

// Result is the outcome of a command.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner executes system operations.
type Runner interface {
	// Query runs a read-only command. It always executes, even in dry-run mode.
	Query(ctx context.Context, name string, args ...string) (Result, error)
	// Exec runs a command that changes the system.
	Exec(ctx context.Context, name string, args ...string) (Result, error)
	// ReadFile reads a file. It always executes, even in dry-run mode.
	ReadFile(path string) ([]byte, error)
	// ReadDir returns the sorted names of the entries in a directory. It
	// always executes, even in dry-run mode.
	ReadDir(path string) ([]string, error)
	// WriteFile atomically replaces path with data (temporary file + rename).
	WriteFile(path string, data []byte, perm fs.FileMode) error
	// Remove deletes a file. Removing a file that does not exist is not an error.
	Remove(path string) error
	// RemoveAll deletes a path and everything under it.
	RemoveAll(path string) error
	// Rename atomically renames a file. Unlike Remove, a missing source is
	// an error (fs.ErrNotExist), which makes it usable as an atomic claim.
	Rename(oldPath, newPath string) error
	// MkdirAll creates a directory and any missing parents. It does not
	// change the mode of existing directories; use EnsureDir for that.
	MkdirAll(path string, perm fs.FileMode) error
	// Chmod sets the permission bits of a file or directory.
	Chmod(path string, perm fs.FileMode) error
}

// CommandError is returned when a command cannot be started or exits non-zero.
type CommandError struct {
	Name   string
	Args   []string
	Result Result
	Err    error
}

func (e *CommandError) Error() string {
	msg := fmt.Sprintf("command %q failed: %v", CommandLine(e.Name, e.Args...), e.Err)
	if s := strings.TrimSpace(e.Result.Stderr); s != "" {
		msg += ": " + s
	}
	return msg
}

func (e *CommandError) Unwrap() error { return e.Err }

// CommandLine formats a command for display and logging, quoting arguments
// that contain spaces or shell special characters.
func CommandLine(name string, args ...string) string {
	parts := make([]string, 0, len(args)+1)
	for _, s := range append([]string{name}, args...) {
		parts = append(parts, quote(s))
	}
	return strings.Join(parts, " ")
}

func quote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`&|;<>()*?[]{}#~!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
