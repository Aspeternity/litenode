package sysopt

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// CommonTimezones are the time zone choices, UTC first.
var CommonTimezones = []string{
	"UTC", "Asia/Shanghai", "Asia/Hong_Kong", "Asia/Taipei", "Asia/Tokyo", "Asia/Singapore",
	"America/Los_Angeles", "America/New_York", "Europe/London", "Europe/Berlin",
}

// ErrTimezone means the time zone is not known to the system.
var ErrTimezone = errors.New("unknown time zone")

// Clock is the time state shown in the menu.
type Clock struct {
	Timezone string
	// NTP is true when time synchronization is turned on.
	NTP bool
	// Synced is true when the clock is synchronized.
	Synced bool
	// CanNTP is false when no time synchronization service is installed.
	CanNTP bool
}

// ParseTimedatectl parses "timedatectl show" output.
func ParseTimedatectl(out string) Clock {
	var c Clock
	for _, line := range strings.Split(out, "\n") {
		k, v, _ := strings.Cut(strings.TrimSpace(line), "=")
		switch k {
		case "Timezone":
			c.Timezone = v
		case "NTP":
			c.NTP = v == "yes"
		case "NTPSynchronized":
			c.Synced = v == "yes"
		case "CanNTP":
			c.CanNTP = v == "yes"
		}
	}
	return c
}

// ClockStatus runs timedatectl. It fails where systemd-timedated is not
// available (some containers), which hides the menu item.
func (s *Service) ClockStatus(ctx context.Context) (Clock, error) {
	res, err := s.Runner.Query(ctx, "timedatectl", "show", "-p", "Timezone", "-p", "NTP", "-p", "NTPSynchronized", "-p", "CanNTP")
	if err != nil {
		return Clock{}, fmt.Errorf("timedatectl show: %w", err)
	}
	return ParseTimedatectl(res.Stdout), nil
}

// SetTimezone checks tz against the system's list and sets it.
func (s *Service) SetTimezone(ctx context.Context, tz string) error {
	res, err := s.Runner.Query(ctx, "timedatectl", "list-timezones")
	if err != nil {
		return fmt.Errorf("timedatectl list-timezones: %w", err)
	}
	if !slices.Contains(strings.Fields(res.Stdout), tz) {
		return ErrTimezone
	}
	if _, err := s.Runner.Exec(ctx, "timedatectl", "set-timezone", tz); err != nil {
		return fmt.Errorf("set time zone %s: %w", tz, err)
	}
	return nil
}

// EnableNTP turns on time synchronization, installing
// systemd-timesyncd first when no synchronization service is installed
// (minimal Debian images). An installed chrony or ntp is used as it is.
func (s *Service) EnableNTP(ctx context.Context) error {
	c, err := s.ClockStatus(ctx)
	if err != nil {
		return err
	}
	if !c.CanNTP {
		if _, err := s.Runner.Exec(ctx, "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "systemd-timesyncd"); err != nil {
			return fmt.Errorf("install systemd-timesyncd: %w", err)
		}
		// A running systemd-timedated keeps the list of NTP services it
		// found at start and would refuse set-ntp.
		if _, err := s.Runner.Exec(ctx, "systemctl", "try-restart", "systemd-timedated.service"); err != nil {
			return fmt.Errorf("restart systemd-timedated: %w", err)
		}
	}
	if _, err := s.Runner.Exec(ctx, "timedatectl", "set-ntp", "true"); err != nil {
		return fmt.Errorf("timedatectl set-ntp: %w", err)
	}
	return nil
}

// WaitSynced polls until the clock is synchronized or timeout passes.
func (s *Service) WaitSynced(ctx context.Context, timeout, interval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if c, err := s.ClockStatus(ctx); err == nil && c.Synced {
			return true
		}
		if time.Now().Add(interval).After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(interval):
		}
	}
}
