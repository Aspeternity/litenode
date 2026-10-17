// Package firewall implements the LiteNode firewall (DESIGN section 14)
// and inspects firewall rules that other software has installed.
package firewall

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/Aspeternity/litenode/internal/system"
)

// OwnTable is LiteNode's nftables table, ignored when looking for other
// software's rules.
const OwnTable = "litenode"

// ForeignFilter describes inbound filtering installed by something other
// than LiteNode (e.g. the iptables rules of Oracle Cloud images, D-044).
type ForeignFilter struct {
	// Where lists the chains that drop or reject input, such as
	// "ip filter INPUT" or "iptables-legacy INPUT".
	Where []string
}

// Found reports whether any filtering was found.
func (f ForeignFilter) Found() bool { return len(f.Where) > 0 }

// DetectForeignFilter looks for input chains outside LiteNode's table that
// have a drop policy or contain drop / reject rules (including chains they
// jump to). It is a rough check: it does not work out whether a given
// port is blocked, it only says that filtering exists. iptables-nft rules
// appear in the nftables ruleset; iptables-legacy is checked separately
// when installed.
func DetectForeignFilter(ctx context.Context, r system.Runner) ForeignFilter {
	var f ForeignFilter
	if res, err := r.Query(ctx, "nft", "-j", "list", "ruleset"); err == nil {
		f.Where = append(f.Where, nftInputFilters([]byte(res.Stdout))...)
	}
	for _, bin := range []string{"iptables-legacy", "ip6tables-legacy"} {
		res, err := r.Query(ctx, bin, "-S", "INPUT")
		if err == nil && legacyInputFilters(res.Stdout) {
			f.Where = append(f.Where, bin+" INPUT")
		}
	}
	return f
}

type nftChain struct {
	Family, Table, Name string
}

func (c nftChain) String() string { return fmt.Sprintf("%s %s %s", c.Family, c.Table, c.Name) }

// nftInputFilters parses "nft -j list ruleset".
func nftInputFilters(data []byte) []string {
	var doc struct {
		Nftables []map[string]json.RawMessage `json:"nftables"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	type chainInfo struct {
		hook, policy string
	}
	chains := map[nftChain]chainInfo{}
	blocks := map[nftChain]bool{}      // chain has a drop / reject rule
	jumps := map[nftChain][]nftChain{} // chain -> chains it jumps to
	for _, obj := range doc.Nftables {
		if raw, ok := obj["chain"]; ok {
			var c struct {
				Family, Table, Name, Hook, Policy string
			}
			if json.Unmarshal(raw, &c) == nil {
				chains[nftChain{c.Family, c.Table, c.Name}] = chainInfo{c.Hook, c.Policy}
			}
		}
		if raw, ok := obj["rule"]; ok {
			var rule struct {
				Family, Table, Chain string
				Expr                 []map[string]json.RawMessage `json:"expr"`
			}
			if json.Unmarshal(raw, &rule) != nil {
				continue
			}
			ch := nftChain{rule.Family, rule.Table, rule.Chain}
			for _, e := range rule.Expr {
				if _, ok := e["drop"]; ok {
					blocks[ch] = true
				}
				if _, ok := e["reject"]; ok {
					blocks[ch] = true
				}
				if raw, ok := e["xt"]; ok { // iptables-nft extension targets
					var xt struct{ Type, Name string }
					if json.Unmarshal(raw, &xt) == nil && xt.Type == "target" && (xt.Name == "REJECT" || xt.Name == "DROP") {
						blocks[ch] = true
					}
				}
				for _, verb := range []string{"jump", "goto"} {
					if raw, ok := e[verb]; ok {
						var t struct{ Target string }
						if json.Unmarshal(raw, &t) == nil {
							jumps[ch] = append(jumps[ch], nftChain{rule.Family, rule.Table, t.Target})
						}
					}
				}
			}
		}
	}
	var out []string
	for ch, info := range chains {
		if info.hook != "input" || ch.Table == OwnTable {
			continue
		}
		if info.policy == "drop" || reachesBlock(ch, blocks, jumps, map[nftChain]bool{}) {
			out = append(out, ch.String())
		}
	}
	slices.Sort(out)
	return out
}

func reachesBlock(ch nftChain, blocks map[nftChain]bool, jumps map[nftChain][]nftChain, seen map[nftChain]bool) bool {
	if seen[ch] {
		return false
	}
	seen[ch] = true
	if blocks[ch] {
		return true
	}
	for _, next := range jumps[ch] {
		if reachesBlock(next, blocks, jumps, seen) {
			return true
		}
	}
	return false
}

// legacyInputFilters parses "iptables-legacy -S INPUT".
func legacyInputFilters(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		switch {
		case len(f) >= 3 && f[0] == "-P" && f[1] == "INPUT" && f[2] == "DROP":
			return true
		case len(f) >= 2 && f[0] == "-A" && (strings.Contains(line, "-j DROP") || strings.Contains(line, "-j REJECT")):
			return true
		}
	}
	return false
}
