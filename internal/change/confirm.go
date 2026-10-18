package change

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// DefaultConfirmSeconds is how long the user has to confirm (P-006).
const DefaultConfirmSeconds = 60

// DefaultPendingDir holds the pending-rollback marker. It is on disk, not
// tmpfs, so a change left unconfirmed by a reboot is still rolled back at
// boot (D-062).
const DefaultPendingDir = state.ConfigDir + "/rollback"

// DefaultBootUnitDir is where the boot-time rollback unit is installed.
const DefaultBootUnitDir = "/etc/systemd/system"

// BootUnit rolls back at boot a change whose confirmation a reboot cut
// short: the transient timer is gone then, the marker is not (D-062).
const BootUnit = "litenode-rollback-boot.service"

// TimerUnit is the transient systemd unit that rolls back an unconfirmed
// change. Only one change can await confirmation at a time (the menu
// holds the instance lock), so there is a single unit and marker (D-040).
const TimerUnit = "litenode-rollback"

// ErrRolledBack means the confirmation came too late: the rollback timer
// already fired and restored the previous configuration.
var ErrRolledBack = errors.New("confirmation timed out; previous configuration restored")

// Confirmer implements confirm-or-rollback: before a risky change takes
// effect, Arm schedules a rollback with systemd-run, which survives the
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
	// BootUnitDir, if set, gets BootUnit before the first rollback is
	// armed.
	BootUnitDir string
	Seconds     int
}

// NewConfirmer returns a Confirmer with the default timeout and paths.
func NewConfirmer(m *Manager, executable string) *Confirmer {
	return &Confirmer{Manager: m, Executable: executable, PendingDir: DefaultPendingDir,
		BootUnitDir: DefaultBootUnitDir, Seconds: DefaultConfirmSeconds}
}

// bootUnit is the unit file of BootUnit. It does nothing unless a marker
// is pending.
func (c *Confirmer) bootUnit() []byte {
	return []byte(`[Unit]
Description=LiteNode: roll back a change left unconfirmed by a reboot
ConditionPathExists=` + c.pendingPath() + `

[Service]
Type=oneshot
ExecStart=` + c.Executable + ` internal rollback

[Install]
WantedBy=multi-user.target
`)
}

// ensureBootUnit installs and enables BootUnit if it is missing or out of
// date. After the first time this is a single file read.
func (c *Confirmer) ensureBootUnit(ctx context.Context) error {
	if c.BootUnitDir == "" {
		return nil
	}
	r := c.Manager.Runner
	path := filepath.Join(c.BootUnitDir, BootUnit)
	want := c.bootUnit()
	if got, err := r.ReadFile(path); err == nil && string(got) == string(want) {
		return nil
	}
	if err := r.WriteFile(path, want, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := system.DaemonReload(ctx, r); err != nil {
		return err
	}
	return system.Systemctl(ctx, r, system.ServiceEnable, BootUnit)
}

func (c *Confirmer) pendingPath() string {
	return filepath.Join(c.PendingDir, "pending")
}

// Arm records backupID as pending and schedules its rollback.
func (c *Confirmer) Arm(ctx context.Context, backupID string) error {
	r := c.Manager.Runner
	// Clear leftovers of an earlier run so systemd-run can reuse the name.
	r.Exec(ctx, "systemctl", "stop", TimerUnit+".timer")
	r.Exec(ctx, "systemctl", "reset-failed", TimerUnit+".service")

	if err := c.ensureBootUnit(ctx); err != nil {
		return fmt.Errorf("install boot-time rollback: %w", err)
	}
	if err := system.EnsureDir(r, c.PendingDir, state.PrivateDirMode); err != nil {
		return err
	}
	if err := r.WriteFile(c.pendingPath(), []byte(backupID+"\n"), state.PrivateFileMode); err != nil {
		return fmt.Errorf("write pending marker: %w", err)
	}
	_, err := r.Exec(ctx, "systemd-run",
		"--unit="+TimerUnit,
		"--on-active="+strconv.Itoa(c.Seconds),
		"--timer-property=AccuracySec=1s",
		c.Executable, "internal", "rollback")
	if err != nil {
		r.Remove(c.pendingPath())
		return fmt.Errorf("schedule rollback: %w", err)
	}
	return nil
}

// Disarm keeps the change. It returns ErrRolledBack if the timer already
// claimed the marker and restored the previous configuration.
func (c *Confirmer) Disarm(ctx context.Context) error {
	r := c.Manager.Runner
	pending := c.pendingPath()
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
	r.Exec(ctx, "systemctl", "stop", TimerUnit+".timer")
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
		if err := c.Arm(ctx, backupID); err != nil {
			return err
		}
		armed = true
		return nil
	}
	id, err := c.Manager.Apply(ctx, ch)
	if err != nil && armed {
		c.Disarm(ctx)
	}
	return id, err
}

// Revert undoes the pending change now, when the user says it does not
// work, instead of waiting for the timer. It returns ErrRolledBack if the
// timer already did it.
func (c *Confirmer) Revert(ctx context.Context, reg Registry) error {
	done, err := c.Rollback(ctx, reg)
	c.Manager.Runner.Exec(ctx, "systemctl", "stop", TimerUnit+".timer")
	if err != nil {
		return err
	}
	if !done {
		return ErrRolledBack
	}
	return nil
}

// Rollback is run by the timer (litenode internal rollback). It restores
// the pending change, all of its kinds, unless the user confirmed first.
// It reports whether a rollback happened.
func (c *Confirmer) Rollback(ctx context.Context, reg Registry) (bool, error) {
	r := c.Manager.Runner
	pending := c.pendingPath()
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
	kinds := make([]*Kind, len(b.Kinds))
	for i, name := range b.Kinds {
		kinds[i] = reg[name] // nil if unknown: files are restored, reapply is reported
	}
	if err := c.Manager.restore(ctx, kinds, b); err != nil {
		return true, fmt.Errorf("rollback %s: %w", b.ID, err)
	}
	return true, nil
}
