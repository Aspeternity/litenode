package warp

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/Aspeternity/litenode/internal/system"
)

// Units of the endpoint check (DESIGN section 19.4). WARP endpoints that
// worked can stop answering from a given network, e.g. after a reboot;
// the check moves the tunnel to one that answers.
const (
	CheckService = "litenode-warp-check.service"
	CheckTimer   = "litenode-warp-check.timer"
	unitDir      = "/etc/systemd/system"
	// staleAfter: with PersistentKeepalive a working tunnel renews its
	// handshake every two minutes.
	staleAfter = 3 * time.Minute
)

// CheckUnits returns the service and timer that run "litenode internal
// warp-check" every 2 minutes. The service skips its runs while the
// program is missing (after "uninstall program"); the tunnel keeps
// running.
func CheckUnits(executable string) (service, timer string) {
	service = `[Unit]
Description=LiteNode: keep the WARP tunnel on an endpoint that answers
Documentation=https://github.com/Aspeternity/litenode
After=` + Unit + `
ConditionPathExists=` + executable + `
ConditionPathExists=` + ConfPath + `

[Service]
Type=oneshot
ExecStart=` + executable + ` internal warp-check
`
	timer = `[Unit]
Description=LiteNode: check the WARP tunnel every 2 minutes
Documentation=https://github.com/Aspeternity/litenode

[Timer]
OnCalendar=*:0/2
AccuracySec=10s

[Install]
WantedBy=timers.target
`
	return service, timer
}

func (s *Service) executable() string {
	if s.Executable != "" {
		return s.Executable
	}
	return "/usr/local/bin/litenode"
}

// installCheck installs and starts the check timer.
func (s *Service) installCheck(ctx context.Context, r system.Runner) error {
	svc, timer := CheckUnits(s.executable())
	if err := r.WriteFile(unitDir+"/"+CheckService, []byte(svc), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", CheckService, err)
	}
	if err := r.WriteFile(unitDir+"/"+CheckTimer, []byte(timer), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", CheckTimer, err)
	}
	if _, err := r.Exec(ctx, "systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	if _, err := r.Exec(ctx, "systemctl", "enable", "--now", CheckTimer); err != nil {
		return fmt.Errorf("enable %s: %w", CheckTimer, err)
	}
	return nil
}

// removeCheck stops and removes the check timer; it is not an error if
// it does not exist.
func removeCheck(ctx context.Context, r system.Runner) {
	if _, err := r.ReadFile(unitDir + "/" + CheckTimer); err != nil {
		return
	}
	_, _ = r.Exec(ctx, "systemctl", "disable", "--now", CheckTimer)
	_ = r.Remove(unitDir + "/" + CheckTimer)
	_ = r.Remove(unitDir + "/" + CheckService)
	_, _ = r.Exec(ctx, "systemctl", "daemon-reload")
}

// ErrAllStale means no endpoint answered; the tunnel stays on its
// endpoint and the next check tries again.
var ErrAllStale = errors.New("no warp endpoint answered")

// Watch moves the tunnel to another endpoint when its handshake is stale
// and saves the endpoint that answered. It returns the new endpoint, or
// an invalid one when nothing had to change.
func (s *Service) Watch(ctx context.Context) (netip.AddrPort, error) {
	c, err := s.Conf()
	if err != nil {
		return netip.AddrPort{}, err
	}
	if _, err := s.Runner.Query(ctx, "ip", "link", "show", "dev", Interface); err != nil {
		// wg-quick failed at boot: start it again.
		if err := system.Systemctl(ctx, s.Runner, system.ServiceRestart, Unit); err != nil {
			return netip.AddrPort{}, err
		}
	}
	if hs := s.handshake(ctx); !hs.IsZero() && s.now().Sub(hs) < staleAfter {
		return netip.AddrPort{}, nil
	}
	wait := s.HandshakeWait
	if wait <= 0 || wait > 6*time.Second {
		wait = 6 * time.Second
	}
	for _, ep := range Candidates(nil, nil, !c.IPv6, c.Endpoint) {
		start := s.now()
		if _, err := s.Runner.Exec(ctx, "wg", "set", Interface, "peer", c.PeerKey, "endpoint", ep.String()); err != nil {
			return netip.AddrPort{}, fmt.Errorf("set endpoint %s: %w", ep, err)
		}
		for s.now().Sub(start) < wait {
			s.sleep(time.Second)
			if hs := s.handshake(ctx); !hs.Before(start.Add(-time.Second)) && !hs.IsZero() {
				if ep == c.Endpoint {
					return netip.AddrPort{}, nil
				}
				c.Endpoint = ep
				if err := s.Runner.WriteFile(ConfPath, c.Render(), 0o600); err != nil {
					return ep, fmt.Errorf("write %s: %w", ConfPath, err)
				}
				return ep, nil
			}
		}
	}
	// Back to the saved endpoint; it may come back.
	_, _ = s.Runner.Exec(ctx, "wg", "set", Interface, "peer", c.PeerKey, "endpoint", c.Endpoint.String())
	return netip.AddrPort{}, ErrAllStale
}

func (s *Service) handshake(ctx context.Context) time.Time {
	res, err := s.Runner.Query(ctx, "wg", "show", Interface, "latest-handshakes")
	if err != nil {
		return time.Time{}
	}
	return ParseHandshake(res.Stdout)
}
