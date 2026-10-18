package change

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

const (
	manifestName = "manifest.json"
	stateName    = "state.json"
	// timeLayout sorts lexicographically in time order.
	timeLayout = "20060102-150405.000"
)

// Backup is one backup directory: <BackupDir>/<kind>[+<kind>...]-<time>/.
// It holds the original files under their base names, state.json, and
// manifest.json. The manifest is written last, so a directory without one
// is incomplete and ignored. Users can restore a single file by copying it
// back.
type Backup struct {
	ID string `json:"id"`
	// Kinds are the kinds of the change, in apply order.
	Kinds        []string      `json:"kinds"`
	Files        []BackupEntry `json:"files"`
	StateExisted bool          `json:"state_existed"`
	Dir          string        `json:"-"`
}

// BackupEntry records one managed file as it was before the change.
type BackupEntry struct {
	Kind    string      `json:"kind"`
	Path    string      `json:"path"`
	Name    string      `json:"name,omitempty"`
	Mode    fs.FileMode `json:"mode"`
	Group   string      `json:"group,omitempty"`
	Existed bool        `json:"existed"`
}

// backupID names a backup by its kinds and time, e.g.
// "xray+ufw-20261018-120000.000".
func backupID(kinds []string, t string) string {
	return strings.Join(kinds, "+") + "-" + t
}

// parseBackupID splits a backup ID into its kinds and time. ok is false
// for names that are not change backups (e.g. state migration backups).
func parseBackupID(id string) (kinds []string, t string, ok bool) {
	head, t, found := strings.Cut(id, "-")
	if !found || t == "" || strings.HasSuffix(id, ".json") {
		return nil, "", false
	}
	kinds = strings.Split(head, "+")
	for _, k := range kinds {
		if !kindName.MatchString(k) {
			return nil, "", false
		}
	}
	return kinds, t, true
}

func (m *Manager) backup(c Change) (*Backup, error) {
	var kinds []string
	for _, p := range c.Parts {
		kinds = append(kinds, p.Kind.Name)
	}
	id := backupID(kinds, m.Now().Format(timeLayout))
	b := &Backup{ID: id, Kinds: kinds, Dir: filepath.Join(m.BackupDir, id)}
	// Backups hold copies of private keys: directories 0700, files 0600,
	// whatever the original file's mode was.
	if err := system.EnsureDir(m.Runner, m.BackupDir, state.PrivateDirMode); err != nil {
		return nil, err
	}
	if err := system.EnsureDir(m.Runner, b.Dir, state.PrivateDirMode); err != nil {
		return nil, err
	}

	used := map[string]bool{manifestName: true, stateName: true}
	for _, p := range c.Parts {
		for _, f := range p.Files {
			e := BackupEntry{Kind: p.Kind.Name, Path: f.Path, Mode: f.Mode, Group: f.Group}
			data, err := m.Runner.ReadFile(f.Path)
			switch {
			case errors.Is(err, fs.ErrNotExist):
			case err != nil:
				return nil, fmt.Errorf("read %s: %w", f.Path, err)
			default:
				e.Existed = true
				e.Name = uniqueName(filepath.Base(f.Path), used)
				if err := m.Runner.WriteFile(filepath.Join(b.Dir, e.Name), data, state.PrivateFileMode); err != nil {
					return nil, fmt.Errorf("back up %s: %w", f.Path, err)
				}
			}
			b.Files = append(b.Files, e)
		}
	}

	data, err := m.Runner.ReadFile(m.Store.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("read state file: %w", err)
	default:
		b.StateExisted = true
		if err := m.Runner.WriteFile(filepath.Join(b.Dir, stateName), data, state.PrivateFileMode); err != nil {
			return nil, fmt.Errorf("back up state file: %w", err)
		}
	}

	manifest, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode backup manifest: %w", err)
	}
	if err := m.Runner.WriteFile(filepath.Join(b.Dir, manifestName), append(manifest, '\n'), state.PrivateFileMode); err != nil {
		return nil, fmt.Errorf("write backup manifest: %w", err)
	}
	return b, nil
}

func uniqueName(name string, used map[string]bool) string {
	candidate := name
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%s.%d", name, i)
	}
	used[candidate] = true
	return candidate
}

// LoadBackup reads a complete backup by ID.
func (m *Manager) LoadBackup(id string) (*Backup, error) {
	dir := filepath.Join(m.BackupDir, id)
	data, err := m.Runner.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return nil, fmt.Errorf("read backup %s: %w", id, err)
	}
	var b Backup
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse backup %s: %w", id, err)
	}
	b.Dir = dir
	return &b, nil
}

// BackupState is the state saved in b, as it was before that change.
func (m *Manager) BackupState(b *Backup) (*state.State, error) {
	if !b.StateExisted {
		return state.New(), nil
	}
	data, err := m.Runner.ReadFile(filepath.Join(b.Dir, stateName))
	if err != nil {
		return nil, fmt.Errorf("read state of backup %s: %w", b.ID, err)
	}
	return state.Decode(data)
}

// Backups returns the IDs of the backups that include kind, oldest first.
func (m *Manager) Backups(kind string) ([]string, error) {
	all, err := m.allBackups()
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, b := range all {
		if slices.Contains(b.kinds, kind) {
			ids = append(ids, b.id)
		}
	}
	return ids, nil
}

type backupName struct {
	id    string
	kinds []string
	time  string
}

// allBackups lists change backups, oldest first.
func (m *Manager) allBackups() ([]backupName, error) {
	names, err := m.Runner.ReadDir(m.BackupDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	var out []backupName
	for _, n := range names {
		if kinds, t, ok := parseBackupID(n); ok {
			out = append(out, backupName{id: n, kinds: kinds, time: t})
		}
	}
	slices.SortStableFunc(out, func(a, b backupName) int { return strings.Compare(a.time, b.time) })
	return out, nil
}

// prune keeps, for every kind, its Keep newest backups. A backup that
// covers several kinds counts once for each, and is removed only when it
// is outside the newest Keep of every kind it covers (D-040). Failures are
// ignored: an extra backup is harmless.
func (m *Manager) prune() {
	all, err := m.allBackups()
	if err != nil {
		return
	}
	keep := map[string]bool{}
	count := map[string]int{}
	for i := len(all) - 1; i >= 0; i-- {
		for _, k := range all[i].kinds {
			if count[k] < m.Keep {
				count[k]++
				keep[all[i].id] = true
			}
		}
	}
	for _, b := range all {
		if !keep[b.id] {
			m.Runner.RemoveAll(filepath.Join(m.BackupDir, b.id))
		}
	}
}
