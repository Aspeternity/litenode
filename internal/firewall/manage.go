package firewall

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/state"
)

// Errors of the rule operations.
var (
	ErrInvalidPorts  = errors.New("invalid ports")
	ErrInvalidSource = errors.New("invalid source")
	ErrRuleExists    = errors.New("rule exists")
	ErrNoRule        = errors.New("no such rule")
)

// Protocols are the protocols a manual rule can open.
var Protocols = []string{"tcp+udp", "tcp", "udp"}

// AutoRuleError is returned when the user tries to change an automatic
// rule other than its sources (P-004).
type AutoRuleError struct{ Entry Entry }

func (e *AutoRuleError) Error() string { return "rule " + e.Entry.Owner + " is managed automatically" }

// IsManual reports whether an owner is one of the user's rules.
func IsManual(owner string) bool { return strings.HasPrefix(owner, "manual:") }

// CheckPorts validates "443" or "10000-10100".
func CheckPorts(s string) error {
	if _, _, err := nft.ParsePorts(s); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPorts, err)
	}
	return nil
}

// ParseSources reads the user's source list (separated by commas or
// spaces) and normalizes it; empty input means anyone.
func ParseSources(in string) ([]string, error) {
	list := strings.FieldsFunc(in, func(r rune) bool { return r == ',' || r == '，' || r == ' ' || r == '\t' })
	out, err := nft.NormalizeSources(list)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSource, err)
	}
	return out, nil
}

// manualIndex finds a manual rule by owner.
func manualIndex(st *state.State, owner string) int {
	return slices.IndexFunc(st.Firewall.Rules, func(r state.FirewallRule) bool { return ManualOwner(r.ID) == owner })
}

// checkRule validates r and that no other manual rule opens the same ports
// and protocol (skipping the rule with ID skip).
func checkRule(st *state.State, r state.FirewallRule, skip int64) error {
	if err := CheckPorts(r.Port); err != nil {
		return err
	}
	if !slices.Contains(Protocols, r.Protocol) {
		return fmt.Errorf("unknown protocol %q", r.Protocol)
	}
	if Duplicate(st, r.Port, r.Protocol, skip) {
		return ErrRuleExists
	}
	return nil
}

// Duplicate reports whether a manual rule other than the one with ID skip
// opens the same ports and protocol.
func Duplicate(st *state.State, ports, proto string, skip int64) bool {
	return slices.ContainsFunc(st.Firewall.Rules, func(o state.FirewallRule) bool {
		return o.ID != skip && o.Port == ports && o.Protocol == proto
	})
}

// AddRule adds a manual rule with a new ID.
func AddRule(st *state.State, r state.FirewallRule) error {
	if err := checkRule(st, r, 0); err != nil {
		return err
	}
	r.ID = st.AllocID()
	st.Firewall.Rules = append(st.Firewall.Rules, r)
	return nil
}

// UpdateRule replaces the manual rule owner with r (its ID is kept).
func UpdateRule(st *state.State, owner string, r state.FirewallRule) error {
	i := manualIndex(st, owner)
	if i < 0 {
		return ErrNoRule
	}
	r.ID = st.Firewall.Rules[i].ID
	if err := checkRule(st, r, r.ID); err != nil {
		return err
	}
	st.Firewall.Rules[i] = r
	return nil
}

// SetSources sets who may reach a rule: a manual rule's sources or an
// automatic rule's limit, kept across port changes (P-004).
func SetSources(st *state.State, owner string, sources []string) error {
	if IsManual(owner) {
		i := manualIndex(st, owner)
		if i < 0 {
			return ErrNoRule
		}
		st.Firewall.Rules[i].Sources = sources
		return nil
	}
	if len(sources) == 0 {
		delete(st.Firewall.AutoSources, owner)
		return nil
	}
	if st.Firewall.AutoSources == nil {
		st.Firewall.AutoSources = map[string][]string{}
	}
	st.Firewall.AutoSources[owner] = sources
	return nil
}

// DeleteRule deletes a manual rule.
func DeleteRule(st *state.State, owner string) error {
	i := manualIndex(st, owner)
	if i < 0 {
		return ErrNoRule
	}
	st.Firewall.Rules = slices.Delete(st.Firewall.Rules, i, i+1)
	return nil
}

// sshAccess describes who may reach the SSH ports: for each port, the
// rules that open it over TCP, with their sources. A change that alters it
// can cut off SSH and must be confirmed (DESIGN section 14, D-073).
func sshAccess(st *state.State, sshPorts []int) []string {
	var out []string
	for _, p := range sshPorts {
		for _, e := range Entries(st, sshPorts) {
			from, to, err := nft.ParsePorts(e.Ports)
			if err != nil || p < from || p > to || e.Proto == "udp" || IsForward(e.Owner) {
				continue
			}
			out = append(out, strconv.Itoa(p)+" "+e.Owner+" "+e.Proto+" "+strings.Join(e.Sources, ","))
		}
	}
	slices.Sort(out)
	return out
}

// AffectsSSH reports whether going from before to after changes who may
// reach the SSH ports.
func AffectsSSH(before, after *state.State, sshPorts []int) bool {
	if before.Firewall.Enabled != after.Firewall.Enabled {
		return true
	}
	return !slices.Equal(sshAccess(before, sshPorts), sshAccess(after, sshPorts))
}

// Report is what "check and repair" found (DESIGN section 14).
type Report struct {
	// NotLoaded means the table or its input chain is missing.
	NotLoaded bool
	// BaseDiffers means the default policy or the rules without owner
	// (replies, IPv6 neighbor discovery, ping ...) differ.
	BaseDiffers bool
	// Missing, Extra and Changed are owners whose rules are not loaded,
	// loaded but not in the state, or loaded differently.
	Missing, Extra, Changed []string
}

// OK reports whether the loaded rules are as the state calls for.
func (r Report) OK() bool {
	return !r.NotLoaded && !r.BaseDiffers && len(r.Missing)+len(r.Extra)+len(r.Changed) == 0
}

// Compare compares the input chain the state calls for (want) with the
// loaded one (got, nil when not loaded).
func Compare(want *nft.Firewall, got *nft.Chain) (Report, error) {
	var rep Report
	if got == nil {
		rep.NotLoaded = true
		return rep, nil
	}
	lines, err := nft.InputRules(want)
	if err != nil {
		return rep, err
	}
	wantBy, order, err := byOwner(lines)
	if err != nil {
		return rep, err
	}
	gotBy, gotOrder, err := byOwner(got.Rules)
	if err != nil {
		return rep, err
	}
	rep.BaseDiffers = got.Policy != "drop" || !sameRules(wantBy[""], gotBy[""])
	for _, o := range order {
		switch {
		case o == "":
		case gotBy[o] == nil:
			rep.Missing = append(rep.Missing, o)
		case !sameRules(wantBy[o], gotBy[o]):
			rep.Changed = append(rep.Changed, o)
		}
	}
	for _, o := range gotOrder {
		if o != "" && wantBy[o] == nil {
			rep.Extra = append(rep.Extra, o)
		}
	}
	return rep, nil
}

// byOwner groups rule lines by owner ("" for rules without a LiteNode
// comment), keeping the order owners first appear in.
func byOwner(lines []string) (map[string][]nft.ListedRule, []string, error) {
	m := map[string][]nft.ListedRule{}
	var order []string
	for _, l := range lines {
		r, err := nft.ParseRule(l)
		if err != nil {
			return nil, nil, err
		}
		o, _ := strings.CutPrefix(r.Comment, "litenode:")
		if _, seen := m[o]; !seen {
			order = append(order, o)
		}
		m[o] = append(m[o], r)
	}
	return m, order, nil
}

// sameRules reports whether two lists hold the same rules in any order.
func sameRules(a, b []nft.ListedRule) bool {
	if len(a) != len(b) {
		return false
	}
	used := make([]bool, len(b))
	for _, x := range a {
		i := slices.IndexFunc(b, func(y nft.ListedRule) bool { return x.Equal(y) })
		for i >= 0 && used[i] {
			j := slices.IndexFunc(b[i+1:], func(y nft.ListedRule) bool { return x.Equal(y) })
			if j < 0 {
				i = -1
			} else {
				i += j + 1
			}
		}
		if i < 0 {
			return false
		}
		used[i] = true
	}
	return true
}
