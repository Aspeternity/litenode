package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

// Real executes operations on the actual system and records every mutating
// operation in the operation log.
type Real struct {
	// Log records mutating operations. It may be nil.
	Log *OpLog
}

// NewReal returns a Real runner that logs to log (which may be nil).
func NewReal(log *OpLog) *Real {
	return &Real{Log: log}
}

func (r *Real) Query(ctx context.Context, name string, args ...string) (Result, error) {
	return run(ctx, name, args)
}

func (r *Real) Stream(ctx context.Context, out io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = out, out
	ownProcessGroup(cmd)
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", CommandLine(name, args...), err)
	}
	return nil
}

func (r *Real) Exec(ctx context.Context, name string, args ...string) (Result, error) {
	res, err := run(ctx, name, args)
	r.Log.Record("exec "+CommandLine(name, args...), err)
	return res, err
}

func (r *Real) ExecInput(ctx context.Context, input []byte, name string, args ...string) (Result, error) {
	res, err := runInput(ctx, input, name, args)
	r.Log.Record("exec "+CommandLine(name, args...)+" (with input)", err)
	return res, err
}

func (r *Real) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (r *Real) ReadDir(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, err
}

func (r *Real) RemoveAll(path string) error {
	err := os.RemoveAll(path)
	r.Log.Record("remove-all "+path, err)
	return err
}

func (r *Real) Rename(oldPath, newPath string) error {
	err := os.Rename(oldPath, newPath)
	r.Log.Record("rename "+oldPath+" -> "+newPath, err)
	return err
}

func (r *Real) WriteFile(path string, data []byte, perm fs.FileMode) error {
	err := writeFileAtomic(path, data, perm)
	r.Log.Record(fmt.Sprintf("write %s (%d bytes, mode %04o)", path, len(data), perm), err)
	return err
}

func (r *Real) Remove(path string) error {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	r.Log.Record("remove "+path, err)
	return err
}

func (r *Real) Chmod(path string, perm fs.FileMode) error {
	err := os.Chmod(path, perm)
	r.Log.Record(fmt.Sprintf("chmod %s %04o", path, perm), err)
	return err
}

func (r *Real) MkdirAll(path string, perm fs.FileMode) error {
	err := os.MkdirAll(path, perm)
	r.Log.Record(fmt.Sprintf("mkdir %s (mode %04o)", path, perm), err)
	return err
}

func run(ctx context.Context, name string, args []string) (Result, error) {
	return runInput(ctx, nil, name, args)
}

func runInput(ctx context.Context, input []byte, name string, args []string) (Result, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	// Force a stable locale so command output can be parsed reliably.
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return res, &CommandError{Name: name, Args: args, Result: res, Err: err}
	}
	return res, nil
}

// writeFileAtomic writes data to a temporary file in the same directory and
// renames it over path, so readers never see a partially written file.
func writeFileAtomic(path string, data []byte, perm fs.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("write temporary file for %s: %w", path, err)
	}
	if err = tmp.Chmod(perm); err != nil {
		return fmt.Errorf("set mode of temporary file for %s: %w", path, err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary file for %s: %w", path, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temporary file for %s: %w", path, err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename temporary file to %s: %w", path, err)
	}
	// Persist the rename. Failure here does not undo the write, so it is ignored.
	if d, derr := os.Open(dir); derr == nil {
		d.Sync()
		d.Close()
	}
	return nil
}
