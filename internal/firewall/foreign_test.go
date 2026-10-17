package firewall

import (
	"context"
	"slices"
	"testing"

	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

// oracleLike is "nft -j list ruleset" captured on the test VPS after
// "iptables -A INPUT -p tcp --dport 9 -j REJECT" (iptables-nft).
const oracleLike = `{"nftables": [{"metainfo": {"version": "1.0.6", "json_schema_version": 1}}, {"table": {"family": "ip", "name": "filter", "handle": 1}}, {"chain": {"family": "ip", "table": "filter", "name": "INPUT", "handle": 34, "type": "filter", "hook": "input", "prio": 0, "policy": "accept"}}, {"rule": {"family": "ip", "table": "filter", "chain": "INPUT", "handle": 35, "expr": [{"match": {"op": "==", "left": {"payload": {"protocol": "tcp", "field": "dport"}}, "right": 9}}, {"counter": {"packets": 0, "bytes": 0}}, {"xt": {"type": "target", "name": "REJECT"}}]}}, {"table": {"family": "ip6", "name": "filter", "handle": 2}}]}`

const clean = `{"nftables": [{"metainfo": {"version": "1.0.6"}}, {"table": {"family": "ip", "name": "filter", "handle": 1}}, {"chain": {"family": "ip", "table": "filter", "name": "INPUT", "hook": "input", "policy": "accept"}}]}`

const dropPolicy = `{"nftables": [{"chain": {"family": "inet", "table": "filter", "name": "input", "hook": "input", "policy": "drop"}}]}`

// Our own table and non-input chains are ignored; a jump to a chain with
// a drop counts.
const mixed = `{"nftables": [
 {"chain": {"family": "inet", "table": "litenode", "name": "input", "hook": "input", "policy": "drop"}},
 {"chain": {"family": "ip", "table": "filter", "name": "FORWARD", "hook": "forward", "policy": "drop"}},
 {"chain": {"family": "ip", "table": "filter", "name": "INPUT", "hook": "input", "policy": "accept"}},
 {"chain": {"family": "ip", "table": "filter", "name": "custom"}},
 {"rule": {"family": "ip", "table": "filter", "chain": "INPUT", "expr": [{"jump": {"target": "custom"}}]}},
 {"rule": {"family": "ip", "table": "filter", "chain": "custom", "expr": [{"drop": null}]}}
]}`

func detect(nft, legacy string) ForeignFilter {
	f := systemtest.New()
	f.Respond(systemtest.Response{Result: system.Result{Stdout: nft}}, "nft", "-j", "list", "ruleset")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: legacy}}, "iptables-legacy", "-S", "INPUT")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "-P INPUT ACCEPT\n"}}, "ip6tables-legacy", "-S", "INPUT")
	return DetectForeignFilter(context.Background(), f)
}

func TestDetectForeignFilter(t *testing.T) {
	tests := []struct {
		name, nft, legacy string
		want              []string
	}{
		{"oracle-like iptables-nft reject", oracleLike, "-P INPUT ACCEPT\n", []string{"ip filter INPUT"}},
		{"clean", clean, "-P INPUT ACCEPT\n", nil},
		{"drop policy", dropPolicy, "", []string{"inet filter input"}},
		{"jump to dropping chain, own table ignored", mixed, "", []string{"ip filter INPUT"}},
		{"iptables-legacy reject", clean, "-P INPUT ACCEPT\n-A INPUT -p tcp -j REJECT --reject-with icmp-host-prohibited\n", []string{"iptables-legacy INPUT"}},
		{"iptables-legacy drop policy", "", "-P INPUT DROP\n", []string{"iptables-legacy INPUT"}},
		{"garbage", "not json", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detect(tt.nft, tt.legacy)
			if !slices.Equal(got.Where, tt.want) || got.Found() != (tt.want != nil) {
				t.Errorf("got %v, want %v", got.Where, tt.want)
			}
		})
	}
}

func TestDetectForeignFilterWithoutTools(t *testing.T) {
	f := systemtest.New() // every query answers empty
	if got := DetectForeignFilter(context.Background(), f); got.Found() {
		t.Errorf("found %v without any rules", got.Where)
	}
}
