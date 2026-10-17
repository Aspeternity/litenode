package system

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/Aspeternity/litenode/internal/text"
)

// DryRun performs read-only operations through Inner and prints every
// mutating operation to Out instead of executing it. It never writes the
// operation log.
type DryRun struct {
	Inner Runner
	Out   io.Writer
}

// NewDryRun returns a DryRun that reads through inner and prints to out.
func NewDryRun(inner Runner, out io.Writer) *DryRun {
	return &DryRun{Inner: inner, Out: out}
}

func (d *DryRun) Query(ctx context.Context, name string, args ...string) (Result, error) {
	return d.Inner.Query(ctx, name, args...)
}

func (d *DryRun) Exec(ctx context.Context, name string, args ...string) (Result, error) {
	fmt.Fprintln(d.Out, text.DryRunExec(CommandLine(name, args...)))
	return Result{}, nil
}

func (d *DryRun) ReadFile(path string) ([]byte, error) {
	return d.Inner.ReadFile(path)
}

func (d *DryRun) ReadDir(path string) ([]string, error) {
	return d.Inner.ReadDir(path)
}

func (d *DryRun) RemoveAll(path string) error {
	fmt.Fprintln(d.Out, text.DryRunRemove(path))
	return nil
}

func (d *DryRun) Rename(oldPath, newPath string) error {
	fmt.Fprintln(d.Out, text.DryRunRename(oldPath, newPath))
	return nil
}

func (d *DryRun) WriteFile(path string, data []byte, perm fs.FileMode) error {
	fmt.Fprintln(d.Out, text.DryRunWriteFile(path, len(data), fmt.Sprintf("%04o", perm)))
	return nil
}

func (d *DryRun) Remove(path string) error {
	fmt.Fprintln(d.Out, text.DryRunRemove(path))
	return nil
}

func (d *DryRun) Chmod(path string, perm fs.FileMode) error {
	fmt.Fprintln(d.Out, text.DryRunChmod(path, fmt.Sprintf("%04o", perm)))
	return nil
}

func (d *DryRun) MkdirAll(path string, perm fs.FileMode) error {
	fmt.Fprintln(d.Out, text.DryRunMkdir(path, fmt.Sprintf("%04o", perm)))
	return nil
}
