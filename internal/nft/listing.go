package nft

import (
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

// InputRules are the rule lines of the input chain for f, as Generate
// writes them.
func InputRules(f *Firewall) ([]string, error) { return inputRules(f) }

// Chain is the input chain as "nft list chain" prints it.
type Chain struct {
	// Policy is "drop" or "accept".
	Policy string
	// Rules are the rule lines, without indentation.
	Rules []string
}

// ParseChain reads the output of "nft list chain <table> <chain>".
func ParseChain(listing string) (Chain, error) {
	var c Chain
	found := false
	for _, l := range strings.Split(listing, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "type filter hook"):
			found = true
			if _, p, ok := strings.Cut(l, "policy "); ok {
				c.Policy = strings.TrimSuffix(p, ";")
			}
		case !found, l == "", l == "}", strings.HasPrefix(l, "table "), strings.HasPrefix(l, "chain "):
		default:
			c.Rules = append(c.Rules, l)
		}
	}
	if !found {
		return c, fmt.Errorf("no base chain in nft listing")
	}
	return c, nil
}

// ListedRule is a rule line split into what "check and repair" compares
// (D-073). nft reorders set elements, prints one-element sets without
// braces and merges overlapping or adjacent source networks, so lines are
// compared by meaning, not by text.
type ListedRule struct {
	Comment string
	// Family is "ip" or "ip6" when the rule has sources, "" otherwise.
	Family  string
	Sources []netip.Prefix
	// Match is the rest of the rule, set elements sorted.
	Match string
}

// ParseRule reads one rule line written by Generate or printed by nft.
func ParseRule(line string) (ListedRule, error) {
	var r ListedRule
	if i := strings.Index(line, ` comment "`); i >= 0 {
		c, err := strconv.Unquote(line[i+len(" comment "):])
		if err != nil {
			return r, fmt.Errorf("rule comment in %q: %w", line, err)
		}
		r.Comment, line = c, line[:i]
	}
	for _, fam := range []string{"ip", "ip6"} {
		rest, ok := strings.CutPrefix(line, fam+" saddr ")
		if !ok {
			continue
		}
		elems, after, err := cutElements(rest)
		if err != nil {
			return r, fmt.Errorf("sources in %q: %w", line, err)
		}
		for _, e := range elems {
			p, err := ParseSource(e)
			if err != nil {
				return r, err
			}
			r.Sources = append(r.Sources, p)
		}
		r.Family, line = fam, after
	}
	r.Match = sortSets(line)
	return r, nil
}

// cutElements reads "{ a, b } rest" or "a rest".
func cutElements(s string) (elems []string, rest string, err error) {
	if body, ok := strings.CutPrefix(s, "{ "); ok {
		in, after, ok := strings.Cut(body, " }")
		if !ok {
			return nil, "", fmt.Errorf("unclosed set")
		}
		return strings.Split(in, ", "), strings.TrimSpace(after), nil
	}
	e, after, _ := strings.Cut(s, " ")
	return []string{e}, after, nil
}

// sortSets sorts the elements of every "{ ... }" in s.
func sortSets(s string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, "{ ")
		if i < 0 {
			break
		}
		j := strings.Index(s[i:], " }")
		if j < 0 {
			break
		}
		elems := strings.Split(s[i+2:i+j], ", ")
		slices.Sort(elems)
		b.WriteString(s[:i] + "{ " + strings.Join(elems, ", ") + " }")
		s = s[i+j+2:]
	}
	b.WriteString(s)
	return b.String()
}

// Equal reports whether two rules match the same packets the same way.
func (r ListedRule) Equal(o ListedRule) bool {
	return r.Comment == o.Comment && r.Family == o.Family && r.Match == o.Match && SameAddresses(r.Sources, o.Sources)
}

// SameAddresses reports whether two lists of networks cover the same
// addresses.
func SameAddresses(a, b []netip.Prefix) bool {
	return slices.EqualFunc(addrRanges(a), addrRanges(b), func(x, y addrRange) bool { return x == y })
}

type addrRange struct{ from, to netip.Addr }

// addrRanges turns networks into sorted, merged address ranges.
func addrRanges(ps []netip.Prefix) []addrRange {
	var rs []addrRange
	for _, p := range ps {
		p = p.Masked()
		rs = append(rs, addrRange{p.Addr(), lastAddr(p)})
	}
	slices.SortFunc(rs, func(x, y addrRange) int { return x.from.Compare(y.from) })
	var out []addrRange
	for _, r := range rs {
		if n := len(out); n > 0 {
			last := &out[n-1]
			// Merge overlapping or adjacent ranges of the same family.
			if last.to.Is4() == r.from.Is4() && (r.from.Compare(last.to) <= 0 || last.to.Next() == r.from) {
				if r.to.Compare(last.to) > 0 {
					last.to = r.to
				}
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

func lastAddr(p netip.Prefix) netip.Addr {
	b := p.Addr().AsSlice()
	for i := p.Bits(); i < len(b)*8; i++ {
		b[i/8] |= 0x80 >> (i % 8)
	}
	a, _ := netip.AddrFromSlice(b)
	return a
}

// NormalizeSources parses sources, drops duplicates and networks covered
// by others, and returns them sorted, IPv4 first.
func NormalizeSources(in []string) ([]string, error) {
	var ps []netip.Prefix
	for _, s := range in {
		p, err := ParseSource(s)
		if err != nil {
			return nil, err
		}
		ps = append(ps, p)
	}
	slices.SortFunc(ps, func(x, y netip.Prefix) int {
		if c := x.Addr().Compare(y.Addr()); c != 0 {
			return c
		}
		return x.Bits() - y.Bits()
	})
	var out []string
	var kept []netip.Prefix
	for _, p := range ps {
		if slices.ContainsFunc(kept, func(k netip.Prefix) bool { return k.Bits() <= p.Bits() && k.Contains(p.Addr()) }) {
			continue
		}
		kept = append(kept, p)
		out = append(out, sourceString(p))
	}
	return out, nil
}
