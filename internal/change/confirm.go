package change

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultConfirmSeconds is how long the user has to confirm (P-006).
const DefaultConfirmSeconds = 60

// DefaultPendingDir holds pending-rollback markers. It is on tmpfs.
const DefaultPendingDir = "/run/litenode"

// ErrRolledBack means the confirmation came too late: the rollback timer
// already fired and restored the previous configuration.
var ErrRolledBack = errors.New("confirmation timed out; previous configuration restored")

// Confirmer implements confirm-or-rollback: before a risky change is
// applied, Arm schedules a rollback with systemd-run, which survives the
// LiteNode process being killed when SSH drops. Disarm cancels it once the
// user confirms.
//
// The timer and the user race for a marker file holding the backup ID.
// Whoever renames it first wins, so exactly one of "keep" or "roll back"
// happens.
type Confirmer struct {
	Manager *Manager
	// Executable is the absolute path of the litenode binary.
	Executable string
	PendingDir string
	Seconds    int
}

// NewConfirmer returns a Confirmer with the default timeout and paths.
func NewConfirmer(m *Manager, executable string) *Confirmer {
	return &Confirmer{Manager: m, Executable: executable, PendingDir: DefaultPendingDir, Seconds: DefaultConfirmSeconds}
}

// TimerUnit is the transient unit name for a kind's rollback.
func TimerUnit(kind string) string {
	return "litenode-rollback-" + kind
}

func (c *Confirmer) pendingPath(kind string) string {
	return filepath.Join(c.PendingDir, kind+".pending")
}

// Arm records backupID as pending and schedules its rollback.
func (c *Confirmer) Arm(ctx context.Context, kind, backupID string) error {
	r := c.Manager.Runner
	unit := TimerUnit(kind)
	// Clear leftovers of an earlier run so systemd-run can reuse the name.
	r.Exec(ctx, "systemctl", "stop", unit+".timer")
	r.Exec(ctx, "systemctl", "reset-failed", unit+".service")

	if err := r.MkdirAll(c.PendingDir, 0o700); err != nil {
		return fmt.Errorf("create pending directory: %w", err)
	}
	if err := r.WriteFile(c.pendingPath(kind), []byte(backupID+"\n"), 0o600); err != nil {
		return fmt.Errorf("write pending marker: %w", err)
	}
	_, err := r.Exec(ctx, "systemd-run",
		"--unit="+unit,
		"--on-active="+strconv.Itoa(c.Seconds),
		"--timer-property=AccuracySec=1s",
		c.Executable, "internal", "rollback", kind)
	if err != nil {
		r.Remove(c.pendingPath(kind))
		return fmt.Errorf("schedule rollback: %w", err)
	}
	return nil
}

// Disarm keeps the change. It returns ErrRolledBack if the timer already
// claimed the marker and restored the previous configuration.
func (c *Confirmer) Disarm(ctx context.Context, kind string) error {
	r := c.Manager.Runner
	pending := c.pendingPath(kind)
	err := r.Rename(pending, pending+".confirmed")
	if errors.Is(err, fs.ErrNotExist) {
		return ErrRolledBack
	}
	if err != nil {
		return fmt.Errorf("claim pending marker: %w", err)
	}
	r.Remove(pending + ".confirmed")
	// The marker is gone, so even if stopping the timer fails, a late
	// firing finds nothing to do.
	r.Exec(ctx, "systemctl", "stop", TimerUnit(kind)+".timer")
	return nil
}

// Apply runs a change under confirm-or-rollback: the rollback is scheduled
// after validation and before the change takes effect. If the change fails
// it is restored at once and the scheduled rollback is cancelled. On
// success the caller asks the user, then calls Disarm, or lets the timer
// roll back.
func (c *Confirmer) Apply(ctx context.Context, ch Change) (string, error) {
	armed := false
	ch.BeforeApply = func(ctx context.Context, backupID string) error {
		if err := c.Arm(ctx, ch.Kind.Name, backupID); err != nil {
			return err
		}
		armed = true
		return nil
	}
	id, err := c.Manager.Apply(ctx, ch)
	if err != nil && armed {
		c.Disarm(ctx, ch.Kind.Name)
	}
	return id, err
}

// Rollback is run by the timer (litenode internal rollback <kind>). It
// restores the pending backup unless the user confirmed first. It reports
// whether a rollback happened.
func (c *Confirmer) Rollback(ctx context.Context, reg Registry, kind string) (bool, error) {
	k, ok := reg[kind]
	if !ok {
		return false, fmt.Errorf("unknown kind %q", kind)
	}
	r := c.Manager.Runner
	pending := c.pendingPath(kind)
	claimed := pending + ".rollback"
	err := r.Rename(pending, claimed)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim pending marker: %w", err)
	}
	defer r.Remove(claimed)

	data, err := r.ReadFile(claimed)
	if err != nil {
		return false, fmt.Errorf("read pending marker: %w", err)
	}
	b, err := c.Manager.LoadBackup(strings.TrimSpace(string(data)))
	if err != nil {
		return false, err
	}
	if err := c.Manager.restore(ctx, k, b); err != nil {
		return true, fmt.Errorf("rollback %s: %w", b.ID, err)
	}
	return true, nil
}
