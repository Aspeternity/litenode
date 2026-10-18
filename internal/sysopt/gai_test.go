package sysopt

import (
	"context"
	"testing"
)

func TestGai(t *testing.T) {
	const user = "# Configuration for getaddrinfo(3).\n#precedence ::ffff:0:0/96  100\nlabel ::1/128 0\n"
	if p := ParseGai([]byte(user)); p.IPv4 {
		t.Errorf("commented line: %+v", p)
	}
	with := GaiWithIPv4([]byte(user), true)
	want := user + "\n" + gaiBegin + "\n" + gaiLine + "\n" + gaiEnd + "\n"
	if string(with) != want {
		t.Errorf("with:\n%s", with)
	}
	if p := ParseGai(with); !p.IPv4 || !p.Managed || p.Foreign {
		t.Errorf("with: %+v", p)
	}
	if again := GaiWithIPv4(with, true); string(again) != want {
		t.Errorf("twice:\n%s", again)
	}
	if without := GaiWithIPv4(with, false); string(without) != user {
		t.Errorf("without:\n%s", without)
	}
	if GaiWithIPv4(GaiWithIPv4(nil, true), false) != nil {
		t.Error("empty file kept")
	}
	if p := ParseGai([]byte("precedence ::ffff:0:0/96 100\n")); !p.IPv4 || p.Managed || !p.Foreign {
		t.Errorf("foreign: %+v", p)
	}
	if p := ParseGai([]byte("precedence ::ffff:0:0/96 10\n")); p.IPv4 {
		t.Errorf("low precedence: %+v", p)
	}
}

func TestSetPreference(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	if err := s.SetPreference(ctx, true); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.PreferenceStatus(); !p.Managed {
		t.Errorf("after IPv4: %+v, file:\n%s", p, f.Files[GaiPath])
	}
	if err := s.SetPreference(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Files[GaiPath]; ok {
		t.Error("created file not removed")
	}
	if err := s.SetPreference(ctx, false); err != nil {
		t.Error(err)
	}
}
