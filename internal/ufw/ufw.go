// Package ufw keeps the ports LiteNode manages open in a user-run ufw
// while the LiteNode firewall is disabled (DESIGN section 2, D-038).
//
// Rules added by LiteNode carry the comment "litenode:<kind>:<id>". Only
// such rules are added or deleted, and an equivalent rule the user already
// has is left alone (ufw ignores comments when comparing rules and would
// otherwise rewrite the user's rule as ours). Syncing reconciles the whole
// set against the state rather than adding and deleting one rule at a
// time.
package ufw

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
)

// Marker prefixes the comments of rules LiteNode manages.
const Marker = "litenode:"

// Rule files written by ufw, with one machine-readable "### tuple ###"
// line per rule (comments hex-encoded).
const (
	UserRules  = "/etc/ufw/user.rules"
	User6Rules = "/etc/ufw/user6.rules"
)

// Rule is an allow rule. Dest "" means any (input rules); route rules
// carry the post-DNAT destination IP.
type Rule struct {
	Route   bool
	Proto   string // tcp, udp or any
	Port    int
	Dest    string
	Comment string
}

type ruleKey struct {
	Route bool
	Proto string
	Port  int
	Dest  string
}

func (r Rule) key() ruleKey { return ruleKey{r.Route, r.Proto, r.Port, r.Dest} }

// Managed reports whether LiteNode added the rule.
func (r Rule) Managed() bool { return strings.HasPrefix(r.Comment, Marker) }

// NodeComment is the comment of a node's rule.
func NodeComment(id int64) string { return Marker + "node:" + strconv.FormatInt(id, 10) }

// SSHComment is the comment of the SSH port's rule.
const SSHComment = Marker + "ssh"

// Desired lists the rules the state calls for: one per enabled node, and
// the SSH port when LiteNode manages it (M2-05). Forwards (M4) are added
// by their task.
func Desired(st *state.State) []Rule {
	var out []Rule
	for _, n := range st.Nodes {
		if n.Enabled {
			out = append(out, Rule{Proto: "tcp", Port: n.Port, Comment: NodeComment(n.ID)})
		}
	}
	if st.SSH.Port > 0 {
		out = append(out, Rule{Proto: "tcp", Port: st.SSH.Port, Comment: SSHComment})
	}
	return out
}

// ParseRules reads the tuples of user.rules and user6.rules. Input rules
// to any destination appear in both files and are merged.
func ParseRules(files ...[]byte) []Rule {
	var out []Rule
	seen := map[Rule]bool{}
	for _, data := range files {
		sc := bufio.NewScanner(bytes.NewReader(data))
		for sc.Scan() {
			rest, ok := strings.CutPrefix(sc.Text(), "### tuple ### ")
			if !ok {
				continue
			}
			r, ok := parseTuple(rest)
			if ok && !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	return out
}

// parseTuple parses "<action> <proto> <dport> <dst> <sport> <src> <dir>
// [comment=<hex>]". Only plain allow rules from anywhere, with a single
// port, are represented; anything else is ignored.
func parseTuple(s string) (Rule, bool) {
	f := strings.Fields(s)
	if len(f) < 7 {
		return Rule{}, false
	}
	var r Rule
	switch f[0] {
	case "allow":
	case "route:allow":
		r.Route = true
	default:
		return Rule{}, false
	}
	port, err := strconv.Atoi(f[2])
	if err != nil || f[4] != "any" || (f[5] != "0.0.0.0/0" && f[5] != "::/0") || !strings.HasPrefix(f[6], "in") {
		return Rule{}, false
	}
	r.Proto, r.Port = f[1], port
	if f[3] != "0.0.0.0/0" && f[3] != "::/0" {
		r.Dest = f[3]
	}
	for _, extra := range f[7:] {
		if h, ok := strings.CutPrefix(extra, "comment="); ok {
			if b, err := hex.DecodeString(h); err == nil {
				r.Comment = string(b)
			}
		}
	}
	return r, true
}

// Plan compares the current rules with the desired ones. It deletes
// managed rules that are no longer wanted and adds wanted rules that are
// missing, except where the user has an equivalent unmanaged rule.
func Plan(current, desired []Rule) (add, del []Rule) {
	wanted := map[ruleKey]Rule{}
	for _, d := range desired {
		wanted[d.key()] = d
	}
	have := map[ruleKey]Rule{}
	for _, c := range current {
		have[c.key()] = c
	}
	for _, c := range current {
		if c.Managed() {
			if _, ok := wanted[c.key()]; !ok {
				del = append(del, c)
			}
		}
	}
	for _, d := range desired {
		c, exists := have[d.key()]
		switch {
		case !exists:
			add = append(add, d)
		case !c.Managed():
			// The user's own rule already opens the port; never take it over.
		case c.Comment != d.Comment:
			add = append(add, d) // re-adding updates the comment
		}
	}
	slices.SortFunc(add, compareRules)
	slices.SortFunc(del, compareRules)
	return add, del
}

func compareRules(a, b Rule) int {
	if a.Port != b.Port {
		return a.Port - b.Port
	}
	return strings.Compare(a.Proto+a.Dest, b.Proto+b.Dest)
}

// Args returns the ufw command line that adds or deletes r.
func Args(r Rule, add bool) []string {
	var args []string
	if r.Route {
		args = append(args, "route")
	}
	if !add {
		args = append(args, "delete")
	}
	args = append(args, "allow", "proto", r.Proto, "to")
	if r.Dest == "" {
		args = append(args, "any")
	} else {
		args = append(args, r.Dest)
	}
	args = append(args, "port", strconv.Itoa(r.Port))
	if add {
		args = append(args, "comment", r.Comment)
	}
	return args
}

// Active reports whether ufw is installed and enabled.
func Active(ctx context.Context, r system.Runner) bool {
	res, err := r.Query(ctx, "ufw", "status")
	return err == nil && strings.Contains(res.Stdout, "Status: active")
}

// Current reads the rules from ufw's rule files.
func Current(r system.Runner) ([]Rule, error) {
	var files [][]byte
	for _, p := range []string{UserRules, User6Rules} {
		data, err := r.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		files = append(files, data)
	}
	return ParseRules(files...), nil
}

// Reconcile brings ufw in line with desired. It does nothing unless ufw is
// active and the LiteNode firewall is disabled.
func Reconcile(ctx context.Context, r system.Runner, desired []Rule, liteNodeFirewall bool) error {
	if liteNodeFirewall || !Active(ctx, r) {
		return nil
	}
	return Sync(ctx, r, desired)
}

// Sync applies Plan to ufw's rules whether or not ufw is active (ufw
// accepts rule changes while disabled).
func Sync(ctx context.Context, r system.Runner, desired []Rule) error {
	current, err := Current(r)
	if err != nil {
		return err
	}
	add, del := Plan(current, desired)
	var errs []error
	for _, d := range del {
		if _, err := r.Exec(ctx, "ufw", Args(d, false)...); err != nil {
			errs = append(errs, fmt.Errorf("delete ufw rule %s: %w", d.Comment, err))
		}
	}
	for _, a := range add {
		if _, err := r.Exec(ctx, "ufw", Args(a, true)...); err != nil {
			errs = append(errs, fmt.Errorf("add ufw rule %s: %w", a.Comment, err))
		}
	}
	return errors.Join(errs...)
}

// UserRule is a rule the user added, found because it allows a port.
type UserRule struct {
	// Display is how the rule is shown, e.g. "22/tcp" or "OpenSSH".
	Display string
	// Delete is the ufw command line that deletes it.
	Delete []string
}

// PortRules finds the user's simple allow rules for port (from anywhere
// to any address, one port, or an application profile such as OpenSSH),
// for the user to delete after the port moved (D-038, D-065). Rules
// LiteNode manages are left out: reconciliation handles them. other
// counts rules for the port that are more complex (source limits,
// several ports), which are left for the user.
func PortRules(r system.Runner, port int) (simple []UserRule, other int, err error) {
	var files [][]byte
	for _, p := range []string{UserRules, User6Rules} {
		data, err := r.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, 0, fmt.Errorf("read %s: %w", p, err)
		}
		files = append(files, data)
	}
	seen := map[string]bool{}
	for _, data := range files {
		sc := bufio.NewScanner(bytes.NewReader(data))
		for sc.Scan() {
			t, ok := strings.CutPrefix(sc.Text(), "### tuple ### ")
			if !ok {
				continue
			}
			rule, match, simpleRule := portTuple(t, port)
			if !match {
				continue
			}
			if !simpleRule {
				// The IPv4 and IPv6 copies of a rule count once.
				key := "other:" + strings.ReplaceAll(t, "::/0", "0.0.0.0/0")
				if !seen[key] {
					seen[key] = true
					other++
				}
				continue
			}
			if !seen[rule.Display] {
				seen[rule.Display] = true
				simple = append(simple, rule)
			}
		}
	}
	return simple, other, nil
}

// portTuple reads "<action> <proto> <dport> <dst> <sport> <src> [<dapp>
// <sapp>] <dir> [comment=<hex>]" and reports whether it allows port and
// whether it is simple enough to delete for the user.
func portTuple(t string, port int) (UserRule, bool, bool) {
	f := strings.Fields(t)
	if len(f) < 7 || f[0] != "allow" {
		return UserRule{}, false, false
	}
	for _, extra := range f[6:] {
		if h, ok := strings.CutPrefix(extra, "comment="); ok {
			if b, err := hex.DecodeString(h); err == nil && strings.HasPrefix(string(b), Marker) {
				return UserRule{}, false, false
			}
		}
	}
	p := strconv.Itoa(port)
	ports := strings.Split(f[2], ",")
	if !slices.Contains(ports, p) {
		return UserRule{}, false, false
	}
	anyDst := f[3] == "0.0.0.0/0" || f[3] == "::/0"
	anySrc := (f[5] == "0.0.0.0/0" || f[5] == "::/0") && f[4] == "any"
	if !anyDst || !anySrc {
		return UserRule{}, true, false
	}
	// Application profile: dapp is not "in"/"out" and not "-".
	if len(f) >= 9 && f[6] != "-" && !strings.HasPrefix(f[6], "in") && !strings.HasPrefix(f[6], "out") {
		if !strings.HasPrefix(f[8], "in") {
			return UserRule{}, true, false
		}
		app := strings.ReplaceAll(f[6], "%20", " ")
		return UserRule{Display: app, Delete: []string{"delete", "allow", app}}, true, true
	}
	if len(ports) != 1 || !strings.HasPrefix(f[6], "in") {
		return UserRule{}, true, false
	}
	spec := p
	if f[1] != "any" {
		spec += "/" + f[1]
	}
	return UserRule{Display: spec, Delete: []string{"delete", "allow", spec}}, true, true
}

// DeleteUserRule deletes a rule the user agreed to delete.
func DeleteUserRule(ctx context.Context, r system.Runner, rule UserRule) error {
	if _, err := r.Exec(ctx, "ufw", rule.Delete...); err != nil {
		return fmt.Errorf("delete ufw rule %s: %w", rule.Display, err)
	}
	return nil
}
