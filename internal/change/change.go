// Package change implements the unified configuration change flow: backup,
// generate, validate, apply and rollback on failure.
//
// One user operation often touches several kinds of configuration (adding
// a node: xray + nftables + ufw). A Change groups them so they succeed or
// fail together (D-040): one backup, validate everything, then write, save
// the state and apply each kind in order; on any failure every file and
// the state are restored and the old configuration is reapplied.
package change

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// DefaultKeep is how many backups are kept for each kind (P-006).
const DefaultKeep = 10

// stagedMarker is inserted before a path's extension to stage a candidate
// file for validation before it replaces the real one. The extension is
// kept because some tools pick the format from it (xray: "Failed to get
// format").
const stagedMarker = ".litenode-new"

// StagedPath returns where the candidate for path is validated:
// /etc/litenode/xray.json -> /etc/litenode/xray.litenode-new.json.
func StagedPath(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + stagedMarker + ext
}

// Kind describes one category of managed configuration, such as "sshd",
// "nft", "xray" or "ufw". Backups are pruned per kind.
type Kind struct {
	// Name is lowercase letters only, so backup names "<a>+<b>-<time>"
	// can be split back into kinds.
	Name string
	// Validate checks candidate files. paths maps each managed path to the
	// file to check: a staged copy, or the real path if ValidateInPlace.
	// It is not called when the part writes no files. Nil means no
	// validation.
	Validate func(ctx context.Context, r system.Runner, paths map[string]string) error
	// ValidateInPlace writes files to their real paths before validating,
	// for tools that cannot check a file elsewhere (sshd -t). Services are
	// not reloaded until validation passes.
	ValidateInPlace bool
	// Apply makes services pick up what is now on disk and in the state
	// file and checks that they work. It is also called after a restore,
	// so it must act on the actual situation: after a restore the
	// configuration may be gone, and then the service is stopped or the
	// rules removed. A kind without files (ufw reconciliation) has only
	// Apply. Nil means nothing to do.
	Apply func(ctx context.Context, r system.Runner) error
}

var kindName = regexp.MustCompile(`^[a-z]+$`)

// Registry maps kind names to kinds. The rollback subcommand uses it to
// reapply a restored configuration in a separate process.
type Registry map[string]*Kind

// Register adds k to the registry.
func (reg Registry) Register(k *Kind) error {
	if !kindName.MatchString(k.Name) {
		return fmt.Errorf("invalid kind name %q", k.Name)
	}
	reg[k.Name] = k
	return nil
}

// File is a managed file in its new form. Remove deletes it instead.
//
// Files under the config directory must follow state.CheckFileMode: 0600,
// or 0640 with Group set (Apply rejects anything else).
type File struct {
	Path string
	Data []byte
	Mode fs.FileMode
	// Group, if set, is given to the file (LiteNode runs as root, so the
	// owner is root), for a service that runs as its own user.
	Group  string
	Remove bool
}

// Part is the share of one kind in a change.
type Part struct {
	Kind  *Kind
	Files []File
}

// Change is one configuration change. Parts are validated and applied in
// order; each kind appears at most once.
type Change struct {
	Parts []Part
	// State is the new state to save. Nil leaves the state file unchanged.
	State *state.State
	// BeforeApply runs after validation and before anything takes effect,
	// with the ID of the backup that undoes the change. Confirmer uses it
	// to schedule the rollback. Nil means nothing to do.
	BeforeApply func(ctx context.Context, backupID string) error
}

// Kinds returns the kinds of the change in order.
func (c Change) Kinds() []*Kind {
	ks := make([]*Kind, len(c.Parts))
	for i, p := range c.Parts {
		ks[i] = p.Kind
	}
	return ks
}

// Stage identifies the step of the flow that failed.
type Stage string

const (
	StageBackup   Stage = "backup"
	StageValidate Stage = "validate"
	StageConfirm  Stage = "schedule rollback"
	StageApply    Stage = "apply"
)

// Error reports a failed change. If RestoreErr is nil, the previous
// configuration and state are back in place.
type Error struct {
	Stage Stage
	// Kind is the kind whose step failed, "" if not specific to one.
	Kind       string
	Err        error
	RestoreErr error
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("%s failed: %v", e.Stage, e.Err)
	if e.Kind != "" {
		msg = fmt.Sprintf("%s %s failed: %v", e.Kind, e.Stage, e.Err)
	}
	if e.RestoreErr != nil {
		msg += fmt.Sprintf("; restore also failed: %v", e.RestoreErr)
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// Manager runs changes.
type Manager struct {
	Runner    system.Runner
	Store     *state.Store
	BackupDir string
	Keep      int
	Now       func() time.Time
}

// NewManager returns a Manager using the store's backup directory.
func NewManager(r system.Runner, store *state.Store) *Manager {
	return &Manager{Runner: r, Store: store, BackupDir: store.BackupDir, Keep: DefaultKeep, Now: time.Now}
}

// check rejects malformed changes before anything is touched.
func (m *Manager) check(c Change) error {
	if len(c.Parts) == 0 {
		return errors.New("change has no parts")
	}
	seen := map[string]bool{}
	for _, p := range c.Parts {
		if p.Kind == nil || !kindName.MatchString(p.Kind.Name) {
			return errors.New("change part without a valid kind")
		}
		if seen[p.Kind.Name] {
			return fmt.Errorf("kind %s appears twice in one change", p.Kind.Name)
		}
		seen[p.Kind.Name] = true
		for _, f := range p.Files {
			if f.Remove {
				continue
			}
			if err := state.CheckFileMode(m.configDir(), f.Path, f.Mode, f.Group); err != nil {
				return err
			}
		}
	}
	return nil
}

// Apply runs the change flow and returns the ID of the backup it made.
// On failure the backup is restored and an *Error is returned.
func (m *Manager) Apply(ctx context.Context, c Change) (string, error) {
	if err := m.check(c); err != nil {
		return "", &Error{Stage: StageBackup, Err: err}
	}
	b, err := m.backup(c)
	if err != nil {
		return "", &Error{Stage: StageBackup, Err: err}
	}
	fail := func(stage Stage, kind string, err error) (string, error) {
		return b.ID, &Error{Stage: stage, Kind: kind, Err: err, RestoreErr: m.restore(ctx, c.Kinds(), b)}
	}

	for _, p := range c.Parts {
		if err := m.validate(ctx, p); err != nil {
			return fail(StageValidate, p.Kind.Name, err)
		}
	}
	if c.BeforeApply != nil {
		if err := c.BeforeApply(ctx, b.ID); err != nil {
			return fail(StageConfirm, "", err)
		}
	}
	for _, p := range c.Parts {
		if !p.Kind.ValidateInPlace {
			if err := m.writeFiles(p.Files); err != nil {
				return fail(StageApply, p.Kind.Name, err)
			}
		}
	}
	if c.State != nil {
		if err := m.Store.Save(c.State); err != nil {
			return fail(StageApply, "", err)
		}
	}
	for _, p := range c.Parts {
		if p.Kind.Apply != nil {
			if err := p.Kind.Apply(ctx, m.Runner); err != nil {
				return fail(StageApply, p.Kind.Name, err)
			}
		}
	}
	m.prune()
	return b.ID, nil
}

func (m *Manager) validate(ctx context.Context, p Part) error {
	paths := map[string]string{}
	if p.Kind.ValidateInPlace {
		if err := m.writeFiles(p.Files); err != nil {
			return err
		}
		for _, f := range p.Files {
			if !f.Remove {
				paths[f.Path] = f.Path
			}
		}
	} else {
		defer func() {
			for _, staged := range paths {
				m.Runner.Remove(staged)
			}
		}()
		for _, f := range p.Files {
			if f.Remove {
				continue
			}
			// Validators run as root, so the staged copy is always private.
			staged := StagedPath(f.Path)
			paths[f.Path] = staged
			if err := m.Runner.WriteFile(staged, f.Data, state.PrivateFileMode); err != nil {
				return fmt.Errorf("stage %s: %w", f.Path, err)
			}
		}
	}
	if p.Kind.Validate == nil || len(paths) == 0 {
		return nil
	}
	return p.Kind.Validate(ctx, m.Runner, paths)
}

func (m *Manager) writeFiles(files []File) error {
	for _, f := range files {
		if f.Remove {
			if err := m.Runner.Remove(f.Path); err != nil {
				return fmt.Errorf("remove %s: %w", f.Path, err)
			}
			continue
		}
		if err := m.writeFile(f.Path, f.Data, f.Mode, f.Group); err != nil {
			return err
		}
	}
	return nil
}

// writeFile writes atomically and then sets the file's group. Until the
// chgrp the group is root's, which is only more restrictive.
func (m *Manager) writeFile(path string, data []byte, mode fs.FileMode, group string) error {
	if err := m.Runner.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if group != "" {
		if _, err := m.Runner.Exec(context.Background(), "chgrp", group, path); err != nil {
			return fmt.Errorf("chgrp %s: %w", path, err)
		}
	}
	return nil
}

// configDir is the directory whose permissions LiteNode controls.
func (m *Manager) configDir() string {
	return filepath.Dir(m.Store.Path)
}

// restore puts back every file and the state from b, then reapplies the
// kinds in order. Unknown kinds (nil) are skipped after the files are
// restored, and reported.
func (m *Manager) restore(ctx context.Context, kinds []*Kind, b *Backup) error {
	var errs []error
	for _, e := range b.Files {
		if !e.Existed {
			if err := m.Runner.Remove(e.Path); err != nil {
				errs = append(errs, fmt.Errorf("remove %s: %w", e.Path, err))
			}
			continue
		}
		data, err := m.Runner.ReadFile(filepath.Join(b.Dir, e.Name))
		if err == nil {
			err = m.writeFile(e.Path, data, e.Mode, e.Group)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("restore %s: %w", e.Path, err))
		}
	}
	if err := m.restoreState(b); err != nil {
		errs = append(errs, err)
	}
	for i, k := range kinds {
		if k == nil {
			errs = append(errs, fmt.Errorf("cannot reapply unknown kind %q", b.Kinds[i]))
			continue
		}
		if k.Apply != nil {
			if err := k.Apply(ctx, m.Runner); err != nil {
				errs = append(errs, fmt.Errorf("reapply previous %s configuration: %w", k.Name, err))
			}
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) restoreState(b *Backup) error {
	if !b.StateExisted {
		if err := m.Runner.Remove(m.Store.Path); err != nil {
			return fmt.Errorf("remove state file: %w", err)
		}
		return nil
	}
	data, err := m.Runner.ReadFile(filepath.Join(b.Dir, stateName))
	if err == nil {
		err = m.Runner.WriteFile(m.Store.Path, data, state.PrivateFileMode)
	}
	if err != nil {
		return fmt.Errorf("restore state file: %w", err)
	}
	return nil
}
