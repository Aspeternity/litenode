package sysopt

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

var showArgs = []string{"show", "-p", "Timezone", "-p", "NTP", "-p", "NTPSynchronized", "-p", "CanNTP"}

func clock(f *systemtest.Fake, out string) {
	f.Respond(systemtest.Response{Result: system.Result{Stdout: out}}, "timedatectl", showArgs...)
}

func TestTimezone(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	clock(f, "Timezone=Etc/UTC\nNTP=no\nNTPSynchronized=no\nCanNTP=no\n")
	if c, err := s.ClockStatus(ctx); err != nil || c != (Clock{Timezone: "Etc/UTC"}) {
		t.Errorf("ClockStatus = %+v, %v", c, err)
	}
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Asia/Shanghai\nUTC\n"}}, "timedatectl", "list-timezones")
	if err := s.SetTimezone(ctx, "Asia/Tokyo"); !errors.Is(err, ErrTimezone) {
		t.Errorf("unknown zone: %v", err)
	}
	if err := s.SetTimezone(ctx, "Asia/Shanghai"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnableNTP(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"timedatectl set-timezone Asia/Shanghai", "env DEBIAN_FRONTEND=noninteractive apt-get install -y systemd-timesyncd", "systemctl try-restart systemd-timedated.service", "timedatectl set-ntp true"}
	if !slices.Equal(f.Execs(), want) {
		t.Errorf("execs = %q", f.Execs())
	}
	if s.WaitSynced(ctx, 0, time.Millisecond) {
		t.Error("synced")
	}
	clock(f, "Timezone=UTC\nNTP=yes\nNTPSynchronized=yes\nCanNTP=yes\n")
	if !s.WaitSynced(ctx, 0, time.Millisecond) {
		t.Error("not synced")
	}
}
