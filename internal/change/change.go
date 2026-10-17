// Package change implements the unified configuration change flow: backup,
// generate, validate, apply and rollback on failure.
//
// Callers generate the new files (logic layer) and hand them to
// Manager.Apply together with the new state. Apply backs up the current
// files and state, validates and applies the new ones, and restores the
// backup if anything fails.
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
// "nft" or "xray". Backups are grouped and pruned per kind.
type Kind struct {
	// Name is lowercase letters only, so backup names "<name>-<time>" of
	// different kinds never share a prefix.
	Name string
	// Validate checks candidate files. paths maps each managed path to the
	// file to check: a staged copy, or the real path if ValidateInPlace.
	// Nil means no validation.
	Validate func(ctx context.Context, r system.Runner, paths map[string]string) error
	// ValidateInPlace writes files to their real paths before validating,
	// for tools that cannot check a file elsewhere (sshd -t). Services are
	// not reloaded until validation passes.
	ValidateInPlace bool
	// Apply makes services pick up the files now in place and checks that
	// they are running. It is also called after a restore to reapply the
	// old configuration. Nil means nothing to do.
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

// Change is one configuration change.
type Change struct {
	Kind  *Kind
	Files []File
	// State is the new state to save. Nil leaves the state file unchanged.
	State *state.State
	// BeforeApply runs after validation and before anything takes effect,
	// with the ID of the backup that undoes the change. Confirmer uses it
	// to schedule the rollback. Nil means nothing to do.
	BeforeApply func(ctx context.Context, backupID string) error
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
	Stage      Stage
	Err        error
	RestoreErr error
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("%s failed: %v", e.Stage, e.Err)
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

// Apply runs the change flow and returns the ID of the backup it made.
// On failure the backup is restored and an *Error is returned.
func (m *Manager) Apply(ctx context.Context, c Change) (string, error) {
	for _, f := range c.Files {
		if f.Remove {
			continue
		}
		if err := state.CheckFileMode(m.configDir(), f.Path, f.Mode, f.Group); err != nil {
			return "", &Error{Stage: StageBackup, Err: err}
		}
	}
	b, err := m.backup(c)
	if err != nil {
		return "", &Error{Stage: StageBackup, Err: err}
	}
	fail := func(stage Stage, err error) (string, error) {
		return b.ID, &Error{Stage: stage, Err: err, RestoreErr: m.restore(ctx, c.Kind, b)}
	}

	if err := m.validate(ctx, c); err != nil {
		return fail(StageValidate, err)
	}
	if c.BeforeApply != nil {
		if err := c.BeforeApply(ctx, b.ID); err != nil {
			return fail(StageConfirm, err)
		}
	}
	if !c.Kind.ValidateInPlace {
		if err := m.writeFiles(c.Files); err != nil {
			return fail(StageApply, err)
		}
	}
	if c.State != nil {
		if err := m.Store.Save(c.State); err != nil {
			return fail(StageApply, err)
		}
	}
	if c.Kind.Apply != nil {
		if err := c.Kind.Apply(ctx, m.Runner); err != nil {
			return fail(StageApply, err)
		}
	}
	m.prune(c.Kind.Name)
	return b.ID, nil
}

func (m *Manager) validate(ctx context.Context, c Change) error {
	paths := map[string]string{}
	if c.Kind.ValidateInPlace {
		if err := m.writeFiles(c.Files); err != nil {
			return err
		}
		for _, f := range c.Files {
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
		for _, f := range c.Files {
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
	if c.Kind.Validate == nil || len(paths) == 0 {
		return nil
	}
	return c.Kind.Validate(ctx, m.Runner, paths)
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

// restore puts back the files and state from b and reapplies them.
func (m *Manager) restore(ctx context.Context, k *Kind, b *Backup) error {
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
	if k != nil && k.Apply != nil {
		if err := k.Apply(ctx, m.Runner); err != nil {
			errs = append(errs, fmt.Errorf("reapply previous configuration: %w", err))
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
