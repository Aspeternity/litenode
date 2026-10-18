package system

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests only run harmless commands and touch files under t.TempDir().

func TestRealExecCapturesOutput(t *testing.T) {
	r := NewReal(nil)
	res, err := r.Exec(context.Background(), "sh", "-c", "echo out; echo err >&2")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "out\n" || res.Stderr != "err\n" || res.ExitCode != 0 {
		t.Errorf("got %+v", res)
	}
}

func TestRealExecNonZeroExit(t *testing.T) {
	r := NewReal(nil)
	res, err := r.Query(context.Background(), "sh", "-c", "echo boom >&2; exit 3")
	var ce *CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want *CommandError", err)
	}
	if res.ExitCode != 3 || ce.Result.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q should include stderr", err)
	}
}

func TestRealExecMissingCommand(t *testing.T) {
	_, err := NewReal(nil).Query(context.Background(), "litenode-no-such-command")
	var ce *CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want *CommandError", err)
	}
}

func TestRealWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	r := NewReal(nil)
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.WriteFile(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	fi, _ := os.Stat(path)
	if string(data) != "new" || fi.Mode().Perm() != 0o600 {
		t.Errorf("got %q mode %v", data, fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temporary files left behind: %v", entries)
	}
}

func TestRealWriteFileFailureKeepsOriginal(t *testing.T) {
	r := NewReal(nil)
	err := r.WriteFile(filepath.Join(t.TempDir(), "missing-dir", "conf"), []byte("x"), 0o600)
	if err == nil {
		t.Fatal("want error when directory does not exist")
	}

	// Renaming over a directory fails after the temporary file is written;
	// the target must be untouched and no temporary file left.
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := r.WriteFile(target, []byte("x"), 0o600); err == nil {
		t.Fatal("want error when target is a directory")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || !entries[0].IsDir() {
		t.Errorf("unexpected entries after failure: %v", entries)
	}
}

func TestRealRemoveMissingIsNotError(t *testing.T) {
	if err := NewReal(nil).Remove(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Errorf("Remove missing file: %v", err)
	}
}

func TestRealLogsMutationsOnly(t *testing.T) {
	dir := t.TempDir()
	log := NewOpLog(filepath.Join(dir, "litenode.log"))
	log.Now = func() time.Time { return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC) }
	r := NewReal(log)
	ctx := context.Background()

	r.Query(ctx, "true")
	r.ReadFile("/etc/hostname")
	r.Exec(ctx, "true")
	r.Exec(ctx, "false")
	r.WriteFile(filepath.Join(dir, "f"), []byte("abc"), 0o600)
	r.MkdirAll(filepath.Join(dir, "d"), 0o700)
	r.Remove(filepath.Join(dir, "f"))

	data, err := os.ReadFile(log.Path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	want := []string{
		"2026-10-18T12:00:00Z exec true: ok",
		`2026-10-18T12:00:00Z exec false: error: command "false" failed: exit status 1`,
		"2026-10-18T12:00:00Z write " + filepath.Join(dir, "f") + " (3 bytes, mode 0600): ok",
		"2026-10-18T12:00:00Z mkdir " + filepath.Join(dir, "d") + " (mode 0700): ok",
		"2026-10-18T12:00:00Z remove " + filepath.Join(dir, "f") + ": ok",
	}
	if len(lines) != len(want) {
		t.Fatalf("log has %d lines, want %d:\n%s", len(lines), len(want), data)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
	fi, _ := os.Stat(log.Path)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("log mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestOpLogRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "litenode.log")
	log := NewOpLog(path)
	log.MaxSize = 100
	for range 10 {
		log.Record("exec something-long-enough-to-fill-the-log", nil)
	}
	cur, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.Stat(path + ".1")
	if err != nil {
		t.Fatalf("rotated log missing: %v", err)
	}
	if cur.Size() > 2*log.MaxSize || old.Size() > 2*log.MaxSize {
		t.Errorf("sizes %d and %d exceed bound", cur.Size(), old.Size())
	}
}

func TestOpLogMultilineError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "litenode.log")
	NewOpLog(path).Record("exec x", errors.New("line1\nline2"))
	data, _ := os.ReadFile(path)
	if strings.Count(string(data), "\n") != 1 {
		t.Errorf("entry should be a single line: %q", data)
	}
}

func TestOpLogUnwritableIsIgnored(t *testing.T) {
	var nilLog *OpLog
	nilLog.Record("exec x", nil) // must not panic
	dir := t.TempDir()
	NewOpLog(filepath.Join(dir, "missing", "log")).Record("exec x", nil)
	if _, err := os.Stat(filepath.Join(dir, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("log should not create directories")
	}
}

func TestRealExecInput(t *testing.T) {
	r := NewReal(nil)
	res, err := r.ExecInput(context.Background(), []byte("secret\n"), "cat")
	if err != nil || res.Stdout != "secret\n" {
		t.Errorf("got %q, %v", res.Stdout, err)
	}
}

func TestRealStream(t *testing.T) {
	var out strings.Builder
	if err := NewReal(nil).Stream(context.Background(), &out, "echo", "hello"); err != nil || out.String() != "hello\n" {
		t.Errorf("Stream = %q, %v", out.String(), err)
	}
	// Canceling ends the command without an error.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := NewReal(nil).Stream(ctx, &out, "sleep", "5"); err != nil || time.Since(start) > 3*time.Second {
		t.Errorf("canceled Stream = %v after %v", err, time.Since(start))
	}
	if err := NewReal(nil).Stream(context.Background(), &out, "false"); err == nil {
		t.Error("failing command: no error")
	}
}
