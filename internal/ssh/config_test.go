package ssh

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/state"
)

func ptr(b bool) *bool { return &b }

func TestGenerate(t *testing.T) {
	if got := Generate(state.SSH{}, true); got != nil {
		t.Errorf("nothing managed: %q", got)
	}
	got := string(Generate(state.SSH{Port: 22222, PubkeyAuth: ptr(true), PasswordAuth: ptr(false), PermitRootLogin: RootKeysOnly}, true))
	want := "# Managed by LiteNode; changes here are overwritten.\n" +
		"# sshd uses the first value it reads, and this file is read first.\n" +
		"Port 22222\n" +
		"ListenAddress 0.0.0.0:22222\n" +
		"ListenAddress [::]:22222\n" +
		"PubkeyAuthentication yes\n" +
		"PasswordAuthentication no\n" +
		"KbdInteractiveAuthentication no\n" +
		"PermitRootLogin prohibit-password\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	// Password login on leaves keyboard-interactive to the system.
	// Without IPv6 there is no IPv6 wildcard.
	if got := string(Generate(state.SSH{Port: 2222}, false)); strings.Contains(got, "[::]") || !strings.Contains(got, "ListenAddress 0.0.0.0:2222\n") {
		t.Errorf("no IPv6: %q", got)
	}
	got = string(Generate(state.SSH{PasswordAuth: ptr(true)}, true))
	if want := "PasswordAuthentication yes\n"; got[len(got)-len(want):] != want {
		t.Errorf("password on: %q", got)
	}
}

func TestCheckIncludeRealConfigs(t *testing.T) {
	for _, name := range []string{"sshd_config-ubuntu2404", "sshd_config-debian12"} {
		data, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if err := CheckInclude(data); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCheckInclude(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		want         error
	}{
		{"absolute", "Include /etc/ssh/sshd_config.d/*.conf\nPort 22\n", nil},
		{"relative", "# comment\n\ninclude sshd_config.d/*.conf\n", nil},
		{"quoted with others", `Include /etc/foo.conf "/etc/ssh/sshd_config.d/*.conf"` + "\n", nil},
		{"harmless option first", "HostKey /etc/ssh/ssh_host_ed25519_key\nInclude /etc/ssh/sshd_config.d/*.conf\n", nil},
		{"missing", "Port 22\nPasswordAuthentication yes\n", ErrNoInclude},
		{"other directory", "Include /etc/ssh/other.d/*.conf\n", ErrNoInclude},
		{"managed option first", "PasswordAuthentication yes\nInclude /etc/ssh/sshd_config.d/*.conf\n", ErrIncludeLate},
		{"managed option with =", "PermitRootLogin=yes\nInclude /etc/ssh/sshd_config.d/*.conf\n", ErrIncludeLate},
		{"listen address first", "ListenAddress 0.0.0.0\nInclude /etc/ssh/sshd_config.d/*.conf\n", ErrIncludeLate},
		{"inside match", "Match User git\nInclude /etc/ssh/sshd_config.d/*.conf\n", ErrNoInclude},
	} {
		err := CheckInclude([]byte(tc.config))
		if tc.want == nil && err != nil || tc.want != nil && !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}
