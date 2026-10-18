package nodemgr

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/xray"
)

// ErrNoEnabledNodes means Xray has nothing to run.
var ErrNoEnabledNodes = errors.New("no enabled nodes")

// XrayStatus is what the Xray menu shows (DESIGN section 12).
type XrayStatus struct {
	Installed bool
	Version   string
	Active    bool
	// StoppedByUser is the D-045 flag.
	StoppedByUser bool
	// Uptime and Memory are 0 when unknown or not running.
	Uptime time.Duration
	Memory int64
}

// XrayStatus reads the Xray state.
func (s *Service) XrayStatus(ctx context.Context) XrayStatus {
	var st XrayStatus
	st.Version, st.Installed = s.Xray.Installed(ctx)
	if state, err := s.Store.Load(); err == nil {
		st.StoppedByUser = state.Xray.Stopped
	}
	if !st.Installed {
		return st
	}
	res, err := s.Runner.Query(ctx, "systemctl", "show", xray.UnitName,
		"-p", "ActiveState", "-p", "ActiveEnterTimestampMonotonic", "-p", "MemoryCurrent")
	if err != nil {
		return st
	}
	props := ParseProperties(res.Stdout)
	st.Active = props["ActiveState"] == "active"
	if !st.Active {
		return st
	}
	if mem, err := strconv.ParseInt(props["MemoryCurrent"], 10, 64); err == nil {
		st.Memory = mem
	}
	if since, err := strconv.ParseInt(props["ActiveEnterTimestampMonotonic"], 10, 64); err == nil && since > 0 {
		if data, err := s.Runner.ReadFile("/proc/uptime"); err == nil {
			if up, ok := parseUptime(string(data)); ok && up > since {
				st.Uptime = time.Duration(up-since) * time.Microsecond
			}
		}
	}
	return st
}

// ParseProperties parses "systemctl show" output (Key=Value lines).
func ParseProperties(out string) map[string]string {
	m := map[string]string{}
	for line := range strings.Lines(out) {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			m[k] = v
		}
	}
	return m
}

// parseUptime returns /proc/uptime in microseconds (the clock used by
// ActiveEnterTimestampMonotonic).
func parseUptime(s string) (int64, bool) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0, false
	}
	secs, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0, false
	}
	return int64(secs * 1e6), true
}

// StartXray clears the manual stop and runs Xray as the nodes require
// (D-045). With no enabled node it returns ErrNoEnabledNodes and leaves
// the flag cleared.
func (s *Service) StartXray(ctx context.Context) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	st.Xray.Stopped = false
	if err := s.apply(ctx, st); err != nil {
		return err
	}
	if !xray.Running(st) {
		return ErrNoEnabledNodes
	}
	return nil
}

// StopXray stops Xray and remembers it: later changes and reboots keep it
// stopped until StartXray (D-045).
func (s *Service) StopXray(ctx context.Context) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	st.Xray.Stopped = true
	return s.apply(ctx, st)
}

// RestartXray restarts a running Xray.
func (s *Service) RestartXray(ctx context.Context) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	if !xray.Running(st) {
		if st.Xray.Stopped {
			return errors.New("xray was stopped by the user")
		}
		return ErrNoEnabledNodes
	}
	return s.Xray.Restart(ctx)
}

// XrayLogs returns the last lines of the Xray journal (D-057).
func (s *Service) XrayLogs(ctx context.Context, lines int) (string, error) {
	res, err := s.Runner.Query(ctx, "journalctl", "-u", xray.UnitName, "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso")
	return res.Stdout, err
}

// UpdateXray replaces Xray (DefaultVersion if version is "") and records
// the new version. On failure the old version keeps running.
func (s *Service) UpdateXray(ctx context.Context, version string) (string, error) {
	v, err := s.Xray.Update(ctx, version)
	if err != nil {
		return "", err
	}
	st, err := s.Store.Load()
	if err != nil {
		return v, err
	}
	st.Xray.Version = v
	return v, s.Store.Save(st)
}

// UninstallXray deletes all nodes and clients and removes Xray (D-020).
// The node deletion is one change, so ufw and firewall node rules go and
// Xray stops; then the program, unit, config and user are removed.
func (s *Service) UninstallXray(ctx context.Context) error {
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	st.Nodes = nil
	st.Xray.Stopped = false
	if _, ok := s.Xray.Installed(ctx); ok {
		if err := s.apply(ctx, st); err != nil {
			return err
		}
	} else {
		// Without Xray only the port rules change.
		k := s.Kinds()
		if _, err := s.Changes.Apply(ctx, change.Change{Parts: []change.Part{{Kind: k[1]}, {Kind: k[2]}}, State: st}); err != nil {
			return err
		}
	}
	if err := s.Xray.Uninstall(ctx); err != nil {
		return err
	}
	st, err = s.Store.Load()
	if err != nil {
		return err
	}
	st.Xray.Version = ""
	return s.Store.Save(st)
}

// ErrFamilyUnavailable means an "only" outbound mode was chosen but the
// host has no address or default route of that family.
var ErrFamilyUnavailable = errors.New("address family unavailable")

// SetOutboundIP sets the node outbound IP mode (D-084). The "only" modes
// first check that the host can reach the internet over that family.
func (s *Service) SetOutboundIP(ctx context.Context, mode string) error {
	if ipv6, only := xray.OnlyIPv6(mode); only {
		ok, err := xray.FamilyReady(ctx, s.Runner, ipv6)
		if err != nil {
			return err
		}
		if !ok {
			return ErrFamilyUnavailable
		}
	}
	st, err := s.Store.Load()
	if err != nil {
		return err
	}
	st.Xray.OutboundIP = mode
	return s.apply(ctx, st)
}
