package forwardmgr

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// Units of the resolve timer (DESIGN section 11). They exist only while a
// forward has a domain target.
const (
	ResolveService = "litenode-resolve.service"
	ResolveTimer   = "litenode-resolve.timer"
	DefaultUnitDir = "/etc/systemd/system"
)

// lookupTimeout bounds the lookups of one domain.
const lookupTimeout = 15 * time.Second

// TimerError means the forwarding change was applied but the resolve
// timer could not be set up or removed.
type TimerError struct {
	Err error
}

func (e *TimerError) Error() string { return "resolve timer: " + e.Err.Error() }
func (e *TimerError) Unwrap() error { return e.Err }

func (s *Service) unitDir() string {
	if s.UnitDir != "" {
		return s.UnitDir
	}
	return DefaultUnitDir
}

// ResolveUnits returns the service and timer unit files that run
// "litenode internal resolve" every 5 minutes. OnCalendar keeps the
// interval fixed however long a run takes.
func ResolveUnits(executable string) (service, timer string) {
	service = `[Unit]
Description=LiteNode: re-resolve port forwarding targets
Documentation=https://github.com/Aspeternity/litenode
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=` + executable + ` internal resolve
`
	timer = `[Unit]
Description=LiteNode: re-resolve port forwarding targets every 5 minutes
Documentation=https://github.com/Aspeternity/litenode

[Timer]
OnCalendar=*:0/5
AccuracySec=10s

[Install]
WantedBy=timers.target
`
	return service, timer
}

func hasDomain(st *state.State) bool {
	return slices.ContainsFunc(st.Forwards, forward.Forward.IsDomain)
}

// syncTimer installs and starts the resolve timer while st has a domain
// target, and removes it otherwise. When nothing needs to change this is
// two file reads and a status query.
func (s *Service) syncTimer(ctx context.Context, st *state.State) error {
	r := s.Runner
	svcPath, timerPath := filepath.Join(s.unitDir(), ResolveService), filepath.Join(s.unitDir(), ResolveTimer)
	if !hasDomain(st) {
		_, err1 := r.ReadFile(svcPath)
		_, err2 := r.ReadFile(timerPath)
		if err1 != nil && err2 != nil {
			return nil
		}
		// Fails harmlessly when the units were never loaded.
		_, _ = r.Exec(ctx, "systemctl", "disable", "--now", ResolveTimer)
		for _, p := range []string{timerPath, svcPath} {
			if err := r.Remove(p); err != nil {
				return fmt.Errorf("remove %s: %w", p, err)
			}
		}
		return system.DaemonReload(ctx, r)
	}
	svc, timer := ResolveUnits(s.Executable)
	changed := false
	for _, u := range []struct{ path, data string }{{svcPath, svc}, {timerPath, timer}} {
		if got, err := r.ReadFile(u.path); err == nil && string(got) == u.data {
			continue
		}
		if err := r.WriteFile(u.path, []byte(u.data), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", u.path, err)
		}
		changed = true
	}
	if changed {
		if err := system.DaemonReload(ctx, r); err != nil {
			return err
		}
	} else if active, _ := system.ServiceActive(ctx, r, ResolveTimer); active {
		return nil
	}
	if _, err := r.Exec(ctx, "systemctl", "enable", "--now", ResolveTimer); err != nil {
		return fmt.Errorf("systemctl enable --now %s: %w", ResolveTimer, err)
	}
	return nil
}

// ResolveResult is what one run of the resolve timer did.
type ResolveResult struct {
	// Changed are the rules whose addresses changed, with the new ones.
	Changed []forward.Forward
	// Failed are the lookups that failed; their rules kept the addresses.
	Failed []error
}

// Resolve looks up every domain target again (litenode internal resolve).
// When an address changed it applies the new rules, which also updates
// the ufw route rules (D-038); otherwise it changes nothing. Disabled
// rules are kept up to date too, so enabling one uses fresh addresses.
func (s *Service) Resolve(ctx context.Context) (ResolveResult, error) {
	var res ResolveResult
	st, err := s.Store.Load()
	if err != nil {
		return res, err
	}
	for i, f := range st.Forwards {
		if !f.IsDomain() {
			continue
		}
		lctx, cancel := context.WithTimeout(ctx, lookupTimeout)
		v4, v6, err := forward.Reresolve(lctx, s.resolver(), f.TargetHost, f.ResolvedIPv4, f.ResolvedIPv6)
		cancel()
		if err != nil {
			res.Failed = append(res.Failed, err)
		}
		if v4 != f.ResolvedIPv4 || v6 != f.ResolvedIPv6 {
			st.Forwards[i].ResolvedIPv4, st.Forwards[i].ResolvedIPv6 = v4, v6
			res.Changed = append(res.Changed, st.Forwards[i])
		}
	}
	if len(res.Changed) == 0 {
		// Also repairs a timer that is missing or no longer needed.
		if err := s.syncTimer(ctx, st); err != nil {
			return res, &TimerError{Err: err}
		}
		return res, nil
	}
	return res, s.apply(ctx, st)
}
