package ssh

import (
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/Aspeternity/litenode/internal/state"
)

func TestParseEffectiveReal(t *testing.T) {
	data, err := os.ReadFile("testdata/sshd-T-ubuntu2404.txt")
	if err != nil {
		t.Fatal(err)
	}
	e, err := ParseEffective(string(data))
	if err != nil {
		t.Fatal(err)
	}
	want := Effective{Ports: []int{22}, PubkeyAuth: true, PasswordAuth: true, KbdInteractive: false, PermitRootLogin: RootYes}
	if !slices.Equal(e.Ports, want.Ports) || e.PubkeyAuth != want.PubkeyAuth || e.PasswordAuth != want.PasswordAuth ||
		e.KbdInteractive != want.KbdInteractive || e.PermitRootLogin != want.PermitRootLogin {
		t.Errorf("got %+v, want %+v", e, want)
	}
}

func TestParseEffective(t *testing.T) {
	out := "port 22\nport 2222\nport 22\npubkeyauthentication yes\npasswordauthentication no\nkbdinteractiveauthentication no\npermitrootlogin without-password\n"
	e, err := ParseEffective(out)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(e.Ports, []int{22, 2222}) || e.PasswordAuth || e.PermitRootLogin != RootKeysOnly {
		t.Errorf("got %+v", e)
	}
	if _, err := ParseEffective("port 22\n"); err == nil {
		t.Error("incomplete output accepted")
	}
	if _, err := ParseEffective("port x\npubkeyauthentication yes\npasswordauthentication no\npermitrootlogin no\n"); err == nil {
		t.Error("bad port accepted")
	}
}

func TestCheck(t *testing.T) {
	e := Effective{Ports: []int{22222}, PubkeyAuth: true, PasswordAuth: false, PermitRootLogin: RootKeysOnly}
	s := state.SSH{Port: 22222, PubkeyAuth: ptr(true), PasswordAuth: ptr(false), PermitRootLogin: RootKeysOnly}
	if err := Check(s, e); err != nil {
		t.Errorf("matching settings: %v", err)
	}
	if err := Check(state.SSH{}, Effective{Ports: []int{22, 61234}}); err != nil {
		t.Errorf("nothing managed: %v", err)
	}
	for name, eff := range map[string]Effective{
		"extra port":     {Ports: []int{22222, 61234}, PubkeyAuth: true, PermitRootLogin: RootKeysOnly},
		"password on":    {Ports: []int{22222}, PubkeyAuth: true, PasswordAuth: true, PermitRootLogin: RootKeysOnly},
		"kbd still on":   {Ports: []int{22222}, PubkeyAuth: true, KbdInteractive: true, PermitRootLogin: RootKeysOnly},
		"pubkey off":     {Ports: []int{22222}, PermitRootLogin: RootKeysOnly},
		"root different": {Ports: []int{22222}, PubkeyAuth: true, PermitRootLogin: RootYes},
	} {
		if err := Check(s, eff); !errors.Is(err, ErrNotEffective) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
