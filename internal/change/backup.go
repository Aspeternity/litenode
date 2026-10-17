package change

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

const (
	manifestName = "manifest.json"
	stateName    = "state.json"
	// timeLayout sorts lexicographically in time order.
	timeLayout = "20060102-150405.000"
)

// Backup is one backup directory: <BackupDir>/<kind>-<time>/. It holds the
// original files under their base names, state.json, and manifest.json.
// The manifest is written last, so a directory without one is incomplete
// and ignored. Users can restore a single file by copying it back.
type Backup struct {
	ID           string        `json:"id"`
	Kind         string        `json:"kind"`
	Files        []BackupEntry `json:"files"`
	StateExisted bool          `json:"state_existed"`
	Dir          string        `json:"-"`
}

// BackupEntry records one managed file as it was before the change.
type BackupEntry struct {
	Path    string      `json:"path"`
	Name    string      `json:"name,omitempty"`
	Mode    fs.FileMode `json:"mode"`
	Existed bool        `json:"existed"`
}

func (m *Manager) backup(c Change) (*Backup, error) {
	id := c.Kind.Name + "-" + m.Now().Format(timeLayout)
	b := &Backup{ID: id, Kind: c.Kind.Name, Dir: filepath.Join(m.BackupDir, id)}
	if err := m.Runner.MkdirAll(b.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("create backup directory: %w", err)
	}

	used := map[string]bool{manifestName: true, stateName: true}
	for _, f := range c.Files {
		e := BackupEntry{Path: f.Path, Mode: f.Mode}
		data, err := m.Runner.ReadFile(f.Path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return nil, fmt.Errorf("read %s: %w", f.Path, err)
		default:
			e.Existed = true
			e.Name = uniqueName(filepath.Base(f.Path), used)
			if err := m.Runner.WriteFile(filepath.Join(b.Dir, e.Name), data, 0o600); err != nil {
				return nil, fmt.Errorf("back up %s: %w", f.Path, err)
			}
		}
		b.Files = append(b.Files, e)
	}

	data, err := m.Runner.ReadFile(m.Store.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("read state file: %w", err)
	default:
		b.StateExisted = true
		if err := m.Runner.WriteFile(filepath.Join(b.Dir, stateName), data, 0o600); err != nil {
			return nil, fmt.Errorf("back up state file: %w", err)
		}
	}

	manifest, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode backup manifest: %w", err)
	}
	if err := m.Runner.WriteFile(filepath.Join(b.Dir, manifestName), append(manifest, '\n'), 0o600); err != nil {
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

// Backups returns the IDs of the backups of a kind, oldest first.
func (m *Manager) Backups(kind string) ([]string, error) {
	names, err := m.Runner.ReadDir(m.BackupDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	var ids []string
	for _, n := range names {
		if strings.HasPrefix(n, kind+"-") {
			ids = append(ids, n)
		}
	}
	slices.Sort(ids)
	return ids, nil
}

// prune removes the oldest backups of a kind beyond Keep. Failures are
// ignored: an extra backup is harmless.
func (m *Manager) prune(kind string) {
	ids, err := m.Backups(kind)
	if err != nil || len(ids) <= m.Keep {
		return
	}
	for _, id := range ids[:len(ids)-m.Keep] {
		m.Runner.RemoveAll(filepath.Join(m.BackupDir, id))
	}
}
