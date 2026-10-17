package ufw

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

// Tuples as written by ufw 0.36.2 on the test VPS. The user's own
// "ufw allow 22/tcp" has no comment.
const userRules = `*filter
### RULES ###

### tuple ### allow tcp 22 0.0.0.0/0 any 0.0.0.0/0 in
-A ufw-user-input -p tcp --dport 22 -j ACCEPT

### tuple ### allow tcp 443 0.0.0.0/0 any 0.0.0.0/0 in comment=6c6974656e6f64653a6e6f64653a33
### tuple ### allow tcp 8443 0.0.0.0/0 any 0.0.0.0/0 in comment=6c6974656e6f64653a6e6f64653a34
### tuple ### route:allow tcp 443 203.0.113.10 any 0.0.0.0/0 in comment=6c6974656e6f64653a666f72776172643a35
### tuple ### deny tcp 25 0.0.0.0/0 any 0.0.0.0/0 in
### tuple ### allow tcp 3306 0.0.0.0/0 any 10.0.0.0/8 in
### END RULES ###
`

const user6Rules = `### tuple ### allow tcp 22 ::/0 any ::/0 in
### tuple ### allow tcp 443 ::/0 any ::/0 in comment=6c6974656e6f64653a6e6f64653a33
### tuple ### allow tcp 8443 ::/0 any ::/0 in comment=6c6974656e6f64653a6e6f64653a34
`

func TestParseRules(t *testing.T) {
	got := ParseRules([]byte(userRules), []byte(user6Rules))
	want := []Rule{
		{Proto: "tcp", Port: 22},
		{Proto: "tcp", Port: 443, Comment: "litenode:node:3"},
		{Proto: "tcp", Port: 8443, Comment: "litenode:node:4"},
		{Route: true, Proto: "tcp", Port: 443, Dest: "203.0.113.10", Comment: "litenode:forward:5"},
	}
	// deny rules and source-restricted rules are not represented.
	if !slices.Equal(got, want) {
		t.Errorf("got\n%v\nwant\n%v", got, want)
	}
}

func TestPlan(t *testing.T) {
	current := ParseRules([]byte(userRules), []byte(user6Rules))
	desired := []Rule{
		{Proto: "tcp", Port: 443, Comment: "litenode:node:3"},   // already there
		{Proto: "tcp", Port: 22, Comment: "litenode:node:9"},    // the user's own rule covers it
		{Proto: "tcp", Port: 2053, Comment: "litenode:node:10"}, // new
	}
	add, del := Plan(current, desired)
	wantAdd := []Rule{{Proto: "tcp", Port: 2053, Comment: "litenode:node:10"}}
	wantDel := []Rule{
		{Route: true, Proto: "tcp", Port: 443, Dest: "203.0.113.10", Comment: "litenode:forward:5"},
		{Proto: "tcp", Port: 8443, Comment: "litenode:node:4"},
	}
	if !slices.Equal(add, wantAdd) {
		t.Errorf("add = %v, want %v", add, wantAdd)
	}
	if !slices.Equal(del, wantDel) {
		t.Errorf("del = %v, want %v (never the user's 22/tcp)", del, wantDel)
	}

	// A port moving from one managed node to another updates the comment.
	add, del = Plan(current, []Rule{{Proto: "tcp", Port: 8443, Comment: "litenode:node:11"}})
	if len(add) != 1 || add[0].Comment != "litenode:node:11" || slices.ContainsFunc(del, func(r Rule) bool { return r.Port == 8443 }) {
		t.Errorf("reassigned port: add %v del %v", add, del)
	}
}

func TestArgs(t *testing.T) {
	tests := []struct {
		r    Rule
		add  bool
		want string
	}{
		{Rule{Proto: "tcp", Port: 443, Comment: "litenode:node:3"}, true, "allow proto tcp to any port 443 comment litenode:node:3"},
		{Rule{Proto: "tcp", Port: 443, Comment: "litenode:node:3"}, false, "delete allow proto tcp to any port 443"},
		{Rule{Route: true, Proto: "udp", Port: 443, Dest: "2001:db8::5", Comment: "litenode:forward:5"}, true,
			"route allow proto udp to 2001:db8::5 port 443 comment litenode:forward:5"},
		{Rule{Route: true, Proto: "tcp", Port: 443, Dest: "203.0.113.10"}, false, "route delete allow proto tcp to 203.0.113.10 port 443"},
	}
	for _, tt := range tests {
		if got := strings.Join(Args(tt.r, tt.add), " "); got != tt.want {
			t.Errorf("Args(%+v, %v) = %q, want %q", tt.r, tt.add, got, tt.want)
		}
	}
}

func TestDesired(t *testing.T) {
	nodes := []node.Node{{ID: 3, Port: 443, Enabled: true}, {ID: 4, Port: 8443, Enabled: false}}
	if got := Desired(nodes); !slices.Equal(got, []Rule{{Proto: "tcp", Port: 443, Comment: "litenode:node:3"}}) {
		t.Errorf("got %v (disabled nodes get no rule)", got)
	}
}

func TestReconcile(t *testing.T) {
	ctx := context.Background()
	desired := []Rule{{Proto: "tcp", Port: 2053, Comment: "litenode:node:10"}}

	f := systemtest.New()
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Status: active\n"}}, "ufw", "status")
	f.Files[UserRules] = []byte(userRules)
	f.Files[User6Rules] = []byte(user6Rules)
	if err := Reconcile(ctx, f, desired, false); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"ufw delete allow proto tcp to any port 443",
		"ufw route delete allow proto tcp to 203.0.113.10 port 443",
		"ufw delete allow proto tcp to any port 8443",
		"ufw allow proto tcp to any port 2053 comment litenode:node:10",
	}
	if got := f.Execs(); !slices.Equal(got, want) {
		t.Errorf("execs =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	for name, f := range map[string]*systemtest.Fake{"inactive": inactive(), "not installed": systemtest.New()} {
		if err := Reconcile(ctx, f, desired, false); err != nil || len(f.Execs()) != 0 {
			t.Errorf("%s: execs %v err %v", name, f.Execs(), err)
		}
	}
	active := systemtest.New()
	active.Respond(systemtest.Response{Result: system.Result{Stdout: "Status: active\n"}}, "ufw", "status")
	if err := Reconcile(ctx, active, desired, true); err != nil || len(active.Execs()) != 0 {
		t.Errorf("LiteNode firewall enabled: execs %v", active.Execs())
	}
}

func inactive() *systemtest.Fake {
	f := systemtest.New()
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Status: inactive\n"}}, "ufw", "status")
	return f
}
