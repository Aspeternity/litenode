package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

func newTestStore(f *systemtest.Fake) *Store {
	return &Store{
		Runner:    f,
		Path:      "/etc/litenode/state.json",
		BackupDir: "/etc/litenode/backups",
		Now:       func() time.Time { return time.Date(2026, 10, 18, 12, 30, 0, 0, time.UTC) },
	}
}

func TestLoadMissingReturnsNew(t *testing.T) {
	f := systemtest.New()
	st, err := newTestStore(f).Load()
	if err != nil {
		t.Fatal(err)
	}
	if st.SchemaVersion != SchemaVersion || st.NextID != 1 {
		t.Errorf("got %+v", st)
	}
	if m := f.Mutations(); len(m) != 0 {
		t.Errorf("loading a missing file must not write: %v", m)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	f := systemtest.New()
	s := newTestStore(f)
	st := New()
	st.Server.ShareAddress = "203.0.113.10"
	st.Xray.Version = "v25.9.11"
	if id := st.AllocID(); id != 1 {
		t.Errorf("first ID = %d", id)
	}
	if err := s.Save(st); err != nil {
		t.Fatal(err)
	}

	calls := f.Mutations()
	last := calls[len(calls)-1]
	if last.Op != systemtest.OpWrite || last.Path != s.Path || last.Perm != 0o600 {
		t.Errorf("state must be written with mode 0600, got %+v", last)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if *got != *st {
		t.Errorf("round trip: got %+v, want %+v", got, st)
	}
	if id := got.AllocID(); id != 2 {
		t.Errorf("ID after reload = %d, want 2", id)
	}
}

func TestAllocIDNeverReuses(t *testing.T) {
	st := &State{} // NextID missing from an old or hand-edited file
	seen := map[int64]bool{}
	for range 5 {
		id := st.AllocID()
		if id < 1 || seen[id] {
			t.Fatalf("ID %d reused or invalid", id)
		}
		seen[id] = true
	}
}

func TestSaveFailureKeepsOriginal(t *testing.T) {
	f := systemtest.New()
	s := newTestStore(f)
	orig := New()
	orig.Server.ShareAddress = "original"
	if err := s.Save(orig); err != nil {
		t.Fatal(err)
	}
	before := string(f.Files[s.Path])

	f.WriteErr[s.Path] = errors.New("disk full")
	changed := New()
	changed.Server.ShareAddress = "changed"
	if err := s.Save(changed); err == nil {
		t.Fatal("want error")
	}
	if string(f.Files[s.Path]) != before {
		t.Error("failed save modified the state file")
	}
}

func TestSaveFailureKeepsOriginalOnDisk(t *testing.T) {
	// Same as above with the real atomic writer: the rename target is a
	// directory, so the write fails after the temporary file is created.
	dir := t.TempDir()
	s := &Store{Runner: system.NewReal(nil), Path: filepath.Join(dir, "state.json"), Now: time.Now}
	if err := s.Save(New()); err != nil {
		t.Fatal(err)
	}
	st, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	s.Path = dir // a directory: rename onto it fails
	if err := s.Save(st); err == nil {
		t.Fatal("want error")
	}
	s.Path = filepath.Join(dir, "state.json")
	if _, err := s.Load(); err != nil {
		t.Errorf("original state unreadable after failed save: %v", err)
	}
}

func TestLoadCorrupt(t *testing.T) {
	for name, data := range map[string]string{
		"not json":        "{oops",
		"bad version":     `{"schema_version": "one"}`,
		"negative":        `{"schema_version": -1}`,
		"wrong type":      `{"schema_version": 1, "next_id": "x"}`,
		"truncated write": `{"schema_version": 1, "server": {`,
	} {
		t.Run(name, func(t *testing.T) {
			f := systemtest.New()
			s := newTestStore(f)
			f.Files[s.Path] = []byte(data)
			_, err := s.Load()
			var ce *CorruptError
			if !errors.As(err, &ce) {
				t.Fatalf("err = %v, want CorruptError", err)
			}
			if m := f.Mutations(); len(m) != 0 {
				t.Errorf("corrupt file must not be touched: %v", m)
			}
		})
	}
}

func TestLoadTooNew(t *testing.T) {
	f := systemtest.New()
	s := newTestStore(f)
	f.Files[s.Path] = []byte(fmt.Sprintf(`{"schema_version": %d}`, SchemaVersion+1))
	var tn *TooNewError
	if _, err := s.Load(); !errors.As(err, &tn) {
		t.Fatalf("err = %v, want TooNewError", err)
	}
}

func TestLoadMigratesFakeV0(t *testing.T) {
	// A made-up v0 format: the share address lived at the top level as
	// "address". The v0 -> v1 migration moves it into server.share_address.
	saved := migrations
	t.Cleanup(func() { migrations = saved })
	migrations = map[int]Migration{
		0: func(doc map[string]any) error {
			addr, _ := doc["address"].(string)
			delete(doc, "address")
			doc["server"] = map[string]any{"share_address": addr}
			doc["next_id"] = 1
			return nil
		},
	}

	f := systemtest.New()
	s := newTestStore(f)
	v0 := `{"address": "203.0.113.10"}`
	f.Files[s.Path] = []byte(v0)

	st, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if st.SchemaVersion != 1 || st.Server.ShareAddress != "203.0.113.10" || st.NextID != 1 {
		t.Errorf("migrated state = %+v", st)
	}

	backup := "/etc/litenode/backups/state-v0-20261018-123000.json"
	if string(f.Files[backup]) != v0 {
		t.Errorf("backup %s = %q, want original v0 content", backup, f.Files[backup])
	}
	var onDisk map[string]any
	if err := json.Unmarshal(f.Files[s.Path], &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk["schema_version"] != float64(1) || onDisk["address"] != nil {
		t.Errorf("migrated file not saved as v1: %s", f.Files[s.Path])
	}

	// The backup must be written before the migrated file.
	var order []string
	for _, c := range f.Mutations() {
		if c.Op == systemtest.OpWrite {
			order = append(order, c.Path)
		}
	}
	if len(order) != 2 || order[0] != backup || order[1] != s.Path {
		t.Errorf("write order = %v", order)
	}
}

func TestLoadMissingMigration(t *testing.T) {
	saved := migrations
	t.Cleanup(func() { migrations = saved })
	migrations = map[int]Migration{}

	f := systemtest.New()
	s := newTestStore(f)
	f.Files[s.Path] = []byte(`{}`)
	_, err := s.Load()
	if err == nil || !strings.Contains(err.Error(), "no migration") {
		t.Fatalf("err = %v", err)
	}
	if string(f.Files[s.Path]) != `{}` {
		t.Error("state file changed after failed migration")
	}
}

func TestLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "litenode.lock")
	first, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLock(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("second lock: err = %v, want ErrLocked", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	again, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	again.Release()
}

func TestStatePermissions(t *testing.T) {
	old := syscall.Umask(0)
	defer syscall.Umask(old)
	saved := migrations
	t.Cleanup(func() { migrations = saved })
	migrations = map[int]Migration{0: func(map[string]any) error { return nil }}

	dir := t.TempDir()
	backups := filepath.Join(dir, "backups")
	if err := os.Mkdir(backups, 0o755); err != nil { // pre-existing and too open
		t.Fatal(err)
	}
	s := &Store{Runner: system.NewReal(nil), Path: filepath.Join(dir, "state.json"), BackupDir: backups, Now: time.Now}
	if err := os.WriteFile(s.Path, []byte(`{}`), 0o644); err != nil { // v0, too open
		t.Fatal(err)
	}
	if _, err := s.Load(); err != nil { // migrates: backup + save
		t.Fatal(err)
	}
	bad, err := AuditPermissions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) > 0 {
		t.Errorf("readable by others:\n%s", strings.Join(bad, "\n"))
	}
}
