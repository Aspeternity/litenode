//go:build vps

// Runs ufw rule changes against a real, disabled ufw on the test VPS
// (M1-07a). Disabled ufw accepts rule changes and cannot affect SSH.
//
//	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o ufw-vps.test ./internal/ufw
//	./ufw-vps.test -test.v -test.run TestVPS      (as root, ufw installed but inactive)
package ufw

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
)

func TestVPS(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("run as root on the test VPS")
	}
	ctx := context.Background()
	r := system.NewReal(nil)
	if Active(ctx, r) {
		t.Skip("ufw is active; this test only runs against a disabled ufw")
	}
	run := func(args ...string) {
		t.Helper()
		if _, err := r.Exec(ctx, "ufw", args...); err != nil {
			t.Fatal(err)
		}
	}
	read := func() []Rule {
		t.Helper()
		rules, err := Current(r)
		if err != nil {
			t.Fatal(err)
		}
		return rules
	}
	run("--force", "reset")
	t.Cleanup(func() { r.Exec(ctx, "ufw", "--force", "reset") })

	// The user's own rule: LiteNode must neither take it over nor delete it.
	run("allow", "443/tcp")
	desired := []Rule{
		{Proto: "tcp", Port: 443, Comment: "litenode:node:1"},
		{Proto: "tcp", Port: 2053, Comment: "litenode:node:2"},
		{Route: true, Proto: "tcp", Port: 443, Dest: "203.0.113.10", Comment: "litenode:forward:3"},
		{Route: true, Proto: "udp", Port: 443, Dest: "2001:db8::5", Comment: "litenode:forward:3"},
	}
	if err := Sync(ctx, r, desired); err != nil {
		t.Fatal(err)
	}
	got := read()
	want := []Rule{
		{Proto: "tcp", Port: 443}, // still the user's, without our comment
		{Proto: "tcp", Port: 2053, Comment: "litenode:node:2"},
		{Route: true, Proto: "tcp", Port: 443, Dest: "203.0.113.10", Comment: "litenode:forward:3"},
		{Route: true, Proto: "udp", Port: 443, Dest: "2001:db8::5", Comment: "litenode:forward:3"},
	}
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("missing %+v in %+v", w, got)
		}
	}
	t.Log("ok   added marked rules, left the user's 443/tcp untouched")

	// Second run: nothing to do.
	add, del := Plan(got, desired)
	if len(add)+len(del) != 0 {
		t.Errorf("not idempotent: add %v del %v", add, del)
	}

	// Everything removed from the state: only marked rules go.
	if err := Sync(ctx, r, nil); err != nil {
		t.Fatal(err)
	}
	if got := read(); !slices.Equal(got, []Rule{{Proto: "tcp", Port: 443}}) {
		t.Errorf("after removing all: %+v, want only the user's rule", got)
	}
	t.Log("ok   removed only marked rules (input and route, IPv4 and IPv6)")
}
