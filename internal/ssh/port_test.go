package ssh

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/ufw"
)

const effective22 = "port 22\nlistenaddress 0.0.0.0:22\nlistenaddress [::]:22\npubkeyauthentication yes\npasswordauthentication yes\nkbdinteractiveauthentication no\npermitrootlogin yes\n"

// procTCP22 lists sshd on 22 (0x16) and a program on 8080 (0x1F90).
const procTCP22 = "  sl  local_address rem_address   st\n   0: 00000000:0016 00000000:0000 0A\n   1: 00000000:1F90 00000000:0000 0A\n"

func TestSetPortChecks(t *testing.T) {
	ctx := context.Background()
	f, svc := keyService(t, effective22)
	f.Files["/proc/net/tcp"] = []byte(procTCP22)
	st, _ := svc.Store.Load()
	st.Nodes = []node.Node{{ID: 7, Port: 443, Enabled: true}}
	svc.Store.Save(st)

	for port, want := range map[int]error{0: ErrPortRange, 65536: ErrPortRange, 22: ErrPortSame} {
		if _, err := svc.SetPort(ctx, port); !errors.Is(err, want) {
			t.Errorf("port %d: %v, want %v", port, err, want)
		}
	}
	var pt *PortTakenError
	for _, port := range []int{8080, 443} { // a program; a node
		if _, err := svc.SetPort(ctx, port); !errors.As(err, &pt) {
			t.Errorf("port %d: %v", port, err)
		}
	}
	if pt.Conflicts[0].Owner != "node:7" {
		t.Errorf("conflict = %+v", pt.Conflicts)
	}
}

func TestSetPortOpensUFWFirstAndRestores(t *testing.T) {
	ctx := context.Background()
	// sshd keeps listening on 22: the change is not effective and is
	// restored, ufw included.
	f, svc := keyService(t, effective22)
	f.Files["/proc/net/tcp"] = []byte(procTCP22)
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Status: active\n"}}, "ufw", "status")
	old, err := svc.SetPort(ctx, 22222)
	if !errors.Is(err, ErrNotEffective) {
		t.Fatalf("old=%d err=%v", old, err)
	}
	execs := f.Execs()
	add := slices.Index(execs, "ufw allow proto tcp to any port 22222 comment litenode:ssh")
	test := -1
	for i, c := range f.Calls() {
		if c.Command == SshdPath+" -t" {
			test = i
			break
		}
	}
	if add < 0 || test < 0 {
		t.Fatalf("execs = %v", execs)
	}
	// The ufw rule is added before sshd is reloaded.
	if reload := slices.Index(execs, "systemctl reload "+Unit); reload < add {
		t.Errorf("sshd reloaded before the port was opened: %v", execs)
	}
	if st, _ := svc.Store.Load(); st.SSH.Port != 0 {
		t.Errorf("state not restored: %+v", st.SSH)
	}
	if _, ok := f.Files[change.DefaultPendingDir+"/pending"]; ok {
		t.Error("rollback still armed after a failed change")
	}
}

func TestOldPortRulesOnlyWhenActive(t *testing.T) {
	f, svc := keyService(t, effective22)
	f.Files[ufw.UserRules] = []byte("### tuple ### allow tcp 22 0.0.0.0/0 any 0.0.0.0/0 in\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Status: inactive\n"}}, "ufw", "status")
	if rules, _, _ := svc.OldPortRules(context.Background(), 22); len(rules) != 0 {
		t.Errorf("inactive ufw: %v", rules)
	}
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Status: active\n"}}, "ufw", "status")
	if rules, _, _ := svc.OldPortRules(context.Background(), 22); len(rules) != 1 || rules[0].Display != "22/tcp" {
		t.Errorf("active ufw: %v", rules)
	}
}

func TestCreateSudoUser(t *testing.T) {
	ctx := context.Background()
	f, svc := keyService(t, effective22)
	for name, tc := range map[string]struct {
		user, pass string
		want       error
	}{
		"bad name":    {"Bad", "longenough", ErrUserName},
		"exists":      {"alice", "longenough", ErrUserExists},
		"short":       {"bob", "short", ErrPasswordShort},
		"colon":       {"bob", "pass:word123", ErrPasswordChars},
		"newline":     {"bob", "pass\nword123", ErrPasswordChars},
		"leading dig": {"1bob", "longenough", ErrUserName},
	} {
		if err := svc.CreateSudoUser(ctx, tc.user, tc.pass, nil); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(f.Execs()) != 0 {
		t.Fatalf("rejected input ran commands: %v", f.Execs())
	}
	if err := svc.CreateSudoUser(ctx, "bob", "s3cret-pass", nil); err != nil {
		t.Fatal(err)
	}
	var chpasswd systemtest.Call
	for _, c := range f.Calls() {
		if c.Command == "chpasswd" {
			chpasswd = c
		}
	}
	if !slices.Contains(f.Execs(), "useradd --create-home --shell /bin/bash --groups sudo -- bob") || string(chpasswd.Data) != "bob:s3cret-pass\n" {
		t.Errorf("execs = %v, chpasswd input %q", f.Execs(), chpasswd.Data)
	}
	for _, e := range f.Execs() {
		if strings.Contains(e, "s3cret") {
			t.Errorf("password on a command line: %s", e)
		}
	}
	// A failing step removes the half-created user.
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}}, "chpasswd")
	if err := svc.CreateSudoUser(ctx, "carol", "s3cret-pass", nil); err == nil {
		t.Fatal("chpasswd failure ignored")
	}
	if !slices.Contains(f.Execs(), "userdel --remove -- carol") {
		t.Errorf("user not removed: %v", f.Execs())
	}
}

func TestGeneratePassword(t *testing.T) {
	a, err := GeneratePassword(rand.Reader)
	if err != nil || len(a) != 16 || CheckPassword(a) != nil {
		t.Fatalf("%q %v", a, err)
	}
	for _, c := range a {
		if !strings.ContainsRune(passwordAlphabet, c) {
			t.Errorf("char %q", c)
		}
	}
	if b, _ := GeneratePassword(rand.Reader); a == b {
		t.Error("same password twice")
	}
	// Uniform: bytes at or above the rejection limit are skipped.
	in := bytes.NewReader([]byte{255, 254, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15})
	got, err := GeneratePassword(in)
	if err != nil || got != passwordAlphabet[:16] {
		t.Errorf("got %q, %v", got, err)
	}
}
