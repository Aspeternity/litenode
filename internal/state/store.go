package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/system"
)

// CorruptError means the state file exists but cannot be parsed. LiteNode
// never overwrites it automatically; the user restores it from a backup.
type CorruptError struct {
	Path string
	Err  error
}

func (e *CorruptError) Error() string {
	return fmt.Sprintf("state file %s is corrupt: %v", e.Path, e.Err)
}

func (e *CorruptError) Unwrap() error { return e.Err }

// TooNewError means the state file was written by a newer LiteNode.
type TooNewError struct {
	Path        string
	FileVersion int
}

func (e *TooNewError) Error() string {
	return fmt.Sprintf("state file %s has schema version %d, newer than supported version %d",
		e.Path, e.FileVersion, SchemaVersion)
}

// Store loads and saves the state file through a Runner, so dry-run mode
// never writes it.
type Store struct {
	Runner    system.Runner
	Path      string
	BackupDir string
	// Now returns the current time, used in backup file names.
	Now func() time.Time
}

// NewStore returns a Store using the default locations.
func NewStore(r system.Runner) *Store {
	return &Store{Runner: r, Path: DefaultPath, BackupDir: DefaultBackupDir, Now: time.Now}
}

// Load reads the state file. A missing file yields New(). An older schema
// is backed up, migrated and saved.
func (s *Store) Load() (*State, error) {
	data, err := s.Runner.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}

	st, version, err := decode(data)
	if err != nil {
		var ce *CorruptError
		if errors.As(err, &ce) {
			ce.Path = s.Path
		}
		var tn *TooNewError
		if errors.As(err, &tn) {
			tn.Path = s.Path
		}
		return nil, err
	}
	if version < SchemaVersion {
		if err := s.backup(data, version); err != nil {
			return nil, err
		}
		if err := s.Save(st); err != nil {
			return nil, fmt.Errorf("save migrated state: %w", err)
		}
	}
	return st, nil
}

// Decode parses the content of a state file, e.g. one kept in a change
// backup, migrating an older schema in memory only.
func Decode(data []byte) (*State, error) {
	st, _, err := decode(data)
	return st, err
}

// decode parses data and migrates it to SchemaVersion. It returns the
// version found in the file.
func decode(data []byte) (*State, int, error) {
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, 0, &CorruptError{Err: err}
	}
	version, err := schemaVersion(doc)
	if err != nil {
		return nil, 0, &CorruptError{Err: err}
	}
	if version > SchemaVersion {
		return nil, 0, &TooNewError{FileVersion: version}
	}
	if version < SchemaVersion {
		if err := migrate(doc, version, SchemaVersion, migrations); err != nil {
			return nil, 0, err
		}
		if data, err = json.Marshal(doc); err != nil {
			return nil, 0, fmt.Errorf("encode migrated state: %w", err)
		}
	}
	st := New()
	if err := json.Unmarshal(data, st); err != nil {
		return nil, 0, &CorruptError{Err: err}
	}
	st.normalize()
	return st, version, nil
}

// Save atomically writes st with mode 0600. The state directory is set to
// ConfigDirMode.
func (s *Store) Save(st *State) error {
	st.SchemaVersion = SchemaVersion
	if st.Nodes == nil {
		st.Nodes = []node.Node{} // "nodes": [] rather than null
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	data = append(data, '\n')
	if err := system.EnsureDir(s.Runner, filepath.Dir(s.Path), ConfigDirMode); err != nil {
		return err
	}
	if err := s.Runner.WriteFile(s.Path, data, PrivateFileMode); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	return nil
}

// backup saves the original file before migration.
func (s *Store) backup(data []byte, version int) error {
	name := fmt.Sprintf("state-v%d-%s.json", version, s.Now().Format("20060102-150405"))
	if err := system.EnsureDir(s.Runner, s.BackupDir, PrivateDirMode); err != nil {
		return err
	}
	if err := s.Runner.WriteFile(filepath.Join(s.BackupDir, name), data, PrivateFileMode); err != nil {
		return fmt.Errorf("back up state before migration: %w", err)
	}
	return nil
}

// schemaVersion reads schema_version from a decoded document. A missing
// field means version 0.
func schemaVersion(doc map[string]any) (int, error) {
	raw, ok := doc["schema_version"]
	if !ok {
		return 0, nil
	}
	n, ok := raw.(json.Number)
	if !ok {
		return 0, fmt.Errorf("schema_version is not a number")
	}
	v, err := n.Int64()
	if err != nil || v < 0 {
		return 0, fmt.Errorf("invalid schema_version %s", n)
	}
	return int(v), nil
}
