package system

import (
	"context"
	"errors"
	"fmt"
)

// ServiceAction is a systemctl verb that changes a unit's state.
type ServiceAction string

const (
	ServiceStart   ServiceAction = "start"
	ServiceStop    ServiceAction = "stop"
	ServiceRestart ServiceAction = "restart"
	ServiceReload  ServiceAction = "reload"
	ServiceEnable  ServiceAction = "enable"
	ServiceDisable ServiceAction = "disable"
)

// Systemctl applies action to unit.
func Systemctl(ctx context.Context, r Runner, action ServiceAction, unit string) error {
	if _, err := r.Exec(ctx, "systemctl", string(action), unit); err != nil {
		return fmt.Errorf("systemctl %s %s: %w", action, unit, err)
	}
	return nil
}

// DaemonReload makes systemd re-read unit files.
func DaemonReload(ctx context.Context, r Runner) error {
	if _, err := r.Exec(ctx, "systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	return nil
}

// ServiceActive reports whether unit is active.
func ServiceActive(ctx context.Context, r Runner, unit string) (bool, error) {
	return querySystemctl(ctx, r, "is-active", unit)
}

// ServiceEnabled reports whether unit is enabled.
func ServiceEnabled(ctx context.Context, r Runner, unit string) (bool, error) {
	return querySystemctl(ctx, r, "is-enabled", unit)
}

// querySystemctl runs a systemctl is-* query. A non-zero exit code means
// "no"; only a failure to run systemctl at all is an error.
func querySystemctl(ctx context.Context, r Runner, verb, unit string) (bool, error) {
	res, err := r.Query(ctx, "systemctl", verb, "--quiet", unit)
	if err == nil {
		return true, nil
	}
	var ce *CommandError
	if errors.As(err, &ce) && res.ExitCode > 0 {
		return false, nil
	}
	return false, fmt.Errorf("systemctl %s %s: %w", verb, unit, err)
}
