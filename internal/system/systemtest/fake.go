// Package systemtest provides a fake system.Runner for unit tests. It never
// touches the real system: commands are answered from canned responses and
// files live in memory.
package systemtest

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/Aspeternity/litenode/internal/system"
)

// Op identifies the kind of a recorded call.
type Op string

const (
	OpQuery  Op = "query"
	OpExec   Op = "exec"
	OpRead   Op = "read"
	OpWrite  Op = "write"
	OpRemove Op = "remove"
	OpRename Op = "rename"
	OpRmAll  Op = "remove-all"
	OpMkdir  Op = "mkdir"
	OpChmod  Op = "chmod"
)

// Call is one recorded Runner call. Command is set for query and exec, Path
// for file operations (NewPath for renames), Data and Perm for writes.
type Call struct {
	Op      Op
	Command string
	Path    string
	NewPath string
	Data    []byte
	Perm    fs.FileMode
}

// Response is the canned answer to a command. A non-zero ExitCode without
// Err produces a *system.CommandError, like the real Runner.
type Response struct {
	Result system.Result
	Err    error
}

// Fake is an in-memory system.Runner that records every call.
type Fake struct {
	// Responses maps a command line (as formatted by system.CommandLine) to
	// its answer. Commands without an entry succeed with empty output.
	Responses map[string]Response
	hooks     map[string]func()
	// Files holds file contents by path. Writes and removals update it.
	Files map[string][]byte
	// Dirs holds directories created by MkdirAll. Directories that contain
	// files exist implicitly.
	Dirs map[string]bool
	// Modes holds the last mode set on a path by WriteFile, MkdirAll (for
	// newly created directories) or Chmod.
	Modes map[string]fs.FileMode
	// WriteErr makes WriteFile fail for the given paths.
	WriteErr map[string]error

	mu    sync.Mutex
	calls []Call
}

// New returns an empty Fake.
func New() *Fake {
	return &Fake{
		Responses: map[string]Response{},
		Files:     map[string][]byte{},
		Dirs:      map[string]bool{},
		Modes:     map[string]fs.FileMode{},
		WriteErr:  map[string]error{},
	}
}

var _ system.Runner = (*Fake)(nil)

// Respond sets the answer for a command.
func (f *Fake) Respond(resp Response, name string, args ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Responses[system.CommandLine(name, args...)] = resp
}

// Calls returns a copy of all recorded calls in order.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// Execs returns the command lines of all Exec calls in order.
func (f *Fake) Execs() []string {
	var out []string
	for _, c := range f.Calls() {
		if c.Op == OpExec {
			out = append(out, c.Command)
		}
	}
	return out
}

// Mutations returns all calls that would change the system, in order.
func (f *Fake) Mutations() []Call {
	var out []Call
	for _, c := range f.Calls() {
		switch c.Op {
		case OpExec, OpWrite, OpRemove, OpRmAll, OpRename, OpMkdir, OpChmod:
			out = append(out, c)
		}
	}
	return out
}

// Snapshot returns a copy of the in-memory files.
func (f *Fake) Snapshot() map[string][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return maps.Clone(f.Files)
}

func (f *Fake) Query(_ context.Context, name string, args ...string) (system.Result, error) {
	return f.command(OpQuery, name, args)
}

func (f *Fake) Exec(_ context.Context, name string, args ...string) (system.Result, error) {
	return f.command(OpExec, name, args)
}

// ExecInput records the input in the call's Data.
func (f *Fake) ExecInput(_ context.Context, input []byte, name string, args ...string) (system.Result, error) {
	res, err := f.command(OpExec, name, args)
	f.mu.Lock()
	f.calls[len(f.calls)-1].Data = append([]byte(nil), input...)
	f.mu.Unlock()
	return res, err
}

// OnCommand runs fn after each run of the command line, e.g. to change a
// response once a service has been reloaded.
func (f *Fake) OnCommand(line string, fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hooks == nil {
		f.hooks = map[string]func(){}
	}
	f.hooks[line] = fn
}

func (f *Fake) command(op Op, name string, args []string) (system.Result, error) {
	line := system.CommandLine(name, args...)
	f.mu.Lock()
	hook := f.hooks[line]
	defer func() {
		if hook != nil {
			hook()
		}
	}()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Op: op, Command: line})
	resp := f.Responses[line]
	if resp.Err == nil && resp.Result.ExitCode != 0 {
		resp.Err = fmt.Errorf("exit status %d", resp.Result.ExitCode)
	}
	if resp.Err != nil {
		var ce *system.CommandError
		if !errors.As(resp.Err, &ce) {
			resp.Err = &system.CommandError{Name: name, Args: args, Result: resp.Result, Err: resp.Err}
		}
	}
	return resp.Result, resp.Err
}

func (f *Fake) ReadFile(path string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Op: OpRead, Path: path})
	data, ok := f.Files[path]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	return append([]byte(nil), data...), nil
}

func (f *Fake) WriteFile(path string, data []byte, perm fs.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	data = append([]byte(nil), data...)
	f.calls = append(f.calls, Call{Op: OpWrite, Path: path, Data: data, Perm: perm})
	if err := f.WriteErr[path]; err != nil {
		return err
	}
	f.Files[path] = data
	f.Modes[path] = perm
	return nil
}

func (f *Fake) Remove(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Op: OpRemove, Path: path})
	delete(f.Files, path)
	return nil
}

func (f *Fake) MkdirAll(path string, perm fs.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Op: OpMkdir, Path: path, Perm: perm})
	for p := filepath.Clean(path); p != "/" && p != "."; p = filepath.Dir(p) {
		if !f.Dirs[p] {
			f.Dirs[p] = true
			f.Modes[p] = perm
		}
	}
	return nil
}

func (f *Fake) Chmod(path string, perm fs.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Op: OpChmod, Path: path, Perm: perm})
	f.Modes[filepath.Clean(path)] = perm
	return nil
}

func (f *Fake) ReadDir(path string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Op: OpRead, Path: path})
	dir := filepath.Clean(path)
	names := map[string]bool{}
	add := func(p string) {
		if rel, ok := strings.CutPrefix(p, dir+"/"); ok {
			name, _, _ := strings.Cut(rel, "/")
			names[name] = true
		}
	}
	for p := range f.Files {
		add(p)
	}
	for p := range f.Dirs {
		add(p)
	}
	if len(names) == 0 && !f.Dirs[dir] {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	return slices.Sorted(maps.Keys(names)), nil
}

func (f *Fake) RemoveAll(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Op: OpRmAll, Path: path})
	p := filepath.Clean(path)
	for k := range f.Files {
		if k == p || strings.HasPrefix(k, p+"/") {
			delete(f.Files, k)
		}
	}
	for k := range f.Dirs {
		if k == p || strings.HasPrefix(k, p+"/") {
			delete(f.Dirs, k)
		}
	}
	return nil
}

func (f *Fake) Rename(oldPath, newPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Op: OpRename, Path: oldPath, NewPath: newPath})
	data, ok := f.Files[oldPath]
	if !ok {
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: fs.ErrNotExist}
	}
	delete(f.Files, oldPath)
	f.Files[newPath] = data
	return nil
}
