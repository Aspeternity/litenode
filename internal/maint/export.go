package maint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Aspeternity/litenode/internal/firewall"
	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/node/protocol"
	"github.com/Aspeternity/litenode/internal/node/security"
	"github.com/Aspeternity/litenode/internal/ports"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/xray"
)

// Format marks an export file.
const Format = "litenode-export"

// Errors of Decode.
var (
	ErrNotExport = errors.New("not a LiteNode export file")
	ErrInvalid   = errors.New("invalid export content")
)

// Export is the export file (DESIGN section 16): a subset of the state in
// the same format, so state migrations upgrade it too. It leaves out the
// SSH and system optimization settings, the share addresses and the node
// outbound IP mode, which belong to the machine (P-005, D-086).
type Export struct {
	Format        string `json:"format"`
	SchemaVersion int    `json:"schema_version"`
	// LiteNodeVersion and ExportedAt are informational.
	LiteNodeVersion string            `json:"litenode_version"`
	ExportedAt      string            `json:"exported_at"`
	Nodes           []node.Node       `json:"nodes"`
	Forwards        []forward.Forward `json:"forwards"`
	Firewall        Firewall          `json:"firewall"`
}

// Firewall is the exported part of the firewall: the user's rules, the
// ping setting and the source limits of node and forward rules. Whether
// the firewall is enabled and who may reach SSH stay with the machine.
type Firewall struct {
	BlockPing   bool                 `json:"block_ping,omitempty"`
	Rules       []state.FirewallRule `json:"rules,omitempty"`
	AutoSources map[string][]string  `json:"auto_sources,omitempty"`
}

// sshOwner is the automatic SSH rule's owner in AutoSources.
const sshOwner = "ssh"

// Build makes the export of st.
func Build(st *state.State, version string, now time.Time) Export {
	e := Export{
		Format: Format, SchemaVersion: state.SchemaVersion,
		LiteNodeVersion: version, ExportedAt: now.UTC().Format(time.RFC3339),
		Nodes: slices.Clone(st.Nodes), Forwards: slices.Clone(st.Forwards),
		Firewall: Firewall{BlockPing: st.Firewall.BlockPing, Rules: slices.Clone(st.Firewall.Rules)},
	}
	if e.Nodes == nil {
		e.Nodes = []node.Node{}
	}
	if e.Forwards == nil {
		e.Forwards = []forward.Forward{}
	}
	for owner, src := range st.Firewall.AutoSources {
		if owner == sshOwner {
			continue
		}
		if e.Firewall.AutoSources == nil {
			e.Firewall.AutoSources = map[string][]string{}
		}
		e.Firewall.AutoSources[owner] = slices.Clone(src)
	}
	return e
}

// Encode renders the export file.
func Encode(e Export) ([]byte, error) {
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode export: %w", err)
	}
	return append(data, '\n'), nil
}

// Decode parses and checks an export file, migrating an older schema with
// table (nil: the registered state migrations). A newer file gives a
// *state.TooNewError.
func Decode(data []byte, reg *node.Registry, table map[int]state.Migration) (Export, error) {
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return Export{}, fmt.Errorf("%w: %w", ErrNotExport, err)
	}
	if doc["format"] != Format {
		return Export{}, ErrNotExport
	}
	if err := state.MigrateDocument(doc, table); err != nil {
		return Export{}, err
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return Export{}, fmt.Errorf("encode migrated export: %w", err)
	}
	var e Export
	d := json.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(&e); err != nil {
		return Export{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := Check(e, reg); err != nil {
		return Export{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return e, nil
}

// Check validates the content: every node builds an Xray inbound, forwards
// and firewall rules are well formed, IDs are unique, and no two items
// claim the same port.
func Check(e Export, reg *node.Registry) error {
	ids := map[int64]bool{}
	seen := func(id int64) error {
		if id < 1 || ids[id] {
			return fmt.Errorf("duplicate or invalid id %d", id)
		}
		ids[id] = true
		return nil
	}
	for _, n := range e.Nodes {
		if err := seen(n.ID); err != nil {
			return err
		}
		if n.Port < 1 || n.Port > 65535 {
			return fmt.Errorf("node %d: invalid port %d", n.ID, n.Port)
		}
		if err := checkRemark(n.Remark, node.CheckNodeRemark(e.Nodes, n.Remark, n.ID)); err != nil {
			return fmt.Errorf("node %d: %w", n.ID, err)
		}
		if r := n.Reality; r != nil {
			if pub, err := security.PublicKey(r.PrivateKey); err != nil || pub != r.PublicKey {
				return fmt.Errorf("node %d: reality key pair does not match", n.ID)
			}
		}
		if e := n.VLESSEncryption; e != nil {
			if pub, err := security.PublicKey(e.PrivateKey); err != nil || pub != e.PublicKey {
				return fmt.Errorf("node %d: vless encryption key pair does not match", n.ID)
			}
		}
		if n.TLS != nil {
			if err := security.CheckTLS(n.TLS); err != nil {
				return fmt.Errorf("node %d: %w", n.ID, err)
			}
		}
		for _, c := range n.Clients {
			if err := seen(c.ID); err != nil {
				return err
			}
			switch n.Protocol {
			case "shadowsocks":
				if !protocol.ValidSS2022Key(c.Key) {
					return fmt.Errorf("client %d: invalid shadowsocks key", c.ID)
				}
			case "hysteria":
				if !protocol.ValidHysteriaPassword(c.Key) {
					return fmt.Errorf("client %d: invalid hysteria password", c.ID)
				}
			default:
				if !node.ValidUUID(c.UUID) {
					return fmt.Errorf("client %d: invalid uuid", c.ID)
				}
			}
			if err := checkRemark(c.Remark, node.CheckClientRemark(e.Nodes, c.Remark, c.ID)); err != nil {
				return fmt.Errorf("client %d: %w", c.ID, err)
			}
		}
	}
	// Generating the config checks every node and client (all of them,
	// enabled or not).
	all := slices.Clone(e.Nodes)
	for i := range all {
		all[i].Enabled = true
		all[i].Clients = slices.Clone(all[i].Clients)
		for j := range all[i].Clients {
			all[i].Clients[j].Enabled = true
		}
	}
	if _, err := xray.GenerateConfig(reg, all, ""); err != nil {
		return err
	}
	for _, f := range e.Forwards {
		if err := seen(f.ID); err != nil {
			return err
		}
		if err := forward.Check(f); err != nil {
			return fmt.Errorf("forward %d: %w", f.ID, err)
		}
	}
	scratch := state.New()
	for _, r := range e.Firewall.Rules {
		if err := seen(r.ID); err != nil {
			return err
		}
		if err := firewall.CheckRule(scratch, r, r.ID); err != nil {
			return fmt.Errorf("firewall rule %d: %w", r.ID, err)
		}
		if _, err := nft.NormalizeSources(r.Sources); err != nil {
			return fmt.Errorf("firewall rule %d: %w", r.ID, err)
		}
		scratch.Firewall.Rules = append(scratch.Firewall.Rules, r)
	}
	for owner, src := range e.Firewall.AutoSources {
		if !strings.HasPrefix(owner, "node:") && !strings.HasPrefix(owner, "forward:") {
			return fmt.Errorf("unexpected source limit owner %q", owner)
		}
		if _, err := nft.NormalizeSources(src); err != nil {
			return fmt.Errorf("source limit %s: %w", owner, err)
		}
	}
	claims := ports.StateClaims(e.Nodes, e.Forwards)
	owners := map[ports.Key]string{}
	for _, c := range claims {
		if o, ok := owners[c.Key]; ok && o != c.Owner {
			return fmt.Errorf("%s and %s both use port %d/%s", o, c.Owner, c.Port, c.Proto)
		}
		owners[c.Key] = c.Owner
	}
	return nil
}

func checkRemark(remark string, dup error) error {
	if r, err := node.NormalizeRemark(remark); err != nil || r != remark {
		return fmt.Errorf("invalid remark %q", remark)
	}
	return dup
}

// Replace returns st with its nodes, clients, forwards and firewall rules
// replaced by the export (DESIGN section 16). The machine's own settings
// stay: SSH, system optimizations, share addresses, the Xray version, stop
// flag and outbound IP mode, whether the firewall is enabled and the SSH
// source limit. IDs are kept and next_id moves past them.
func Replace(st *state.State, e Export) *state.State {
	out := *st
	out.Nodes = slices.Clone(e.Nodes)
	out.Forwards = slices.Clone(e.Forwards)
	if out.Forwards == nil {
		out.Forwards = []forward.Forward{}
	}
	out.Firewall = st.Firewall
	out.Firewall.BlockPing = e.Firewall.BlockPing
	out.Firewall.Rules = slices.Clone(e.Firewall.Rules)
	out.Firewall.AutoSources = maps.Clone(e.Firewall.AutoSources)
	if src, ok := st.Firewall.AutoSources[sshOwner]; ok {
		if out.Firewall.AutoSources == nil {
			out.Firewall.AutoSources = map[string][]string{}
		}
		out.Firewall.AutoSources[sshOwner] = src
	}
	maxID := int64(0)
	for _, n := range out.Nodes {
		maxID = max(maxID, n.ID)
		for _, c := range n.Clients {
			maxID = max(maxID, c.ID)
		}
	}
	for _, f := range out.Forwards {
		maxID = max(maxID, f.ID)
	}
	for _, r := range out.Firewall.Rules {
		maxID = max(maxID, r.ID)
	}
	out.NextID = max(st.NextID, maxID+1)
	return &out
}

// Summary counts what an export holds.
type Summary struct {
	Nodes, Clients, Forwards, Rules int
}

// Summarize counts the items of e.
func Summarize(e Export) Summary {
	s := Summary{Nodes: len(e.Nodes), Forwards: len(e.Forwards), Rules: len(e.Firewall.Rules)}
	for _, n := range e.Nodes {
		s.Clients += len(n.Clients)
	}
	return s
}

// SummarizeState counts the same items in the current state.
func SummarizeState(st *state.State) Summary {
	return Summarize(Export{Nodes: st.Nodes, Forwards: st.Forwards, Firewall: Firewall{Rules: st.Firewall.Rules}})
}

// Conflicts lists the imported ports that a program other than LiteNode
// listens on. Ports where the current nodes' Xray listens do not count:
// those nodes are replaced (D-037).
func Conflicts(e Export, current *state.State, listening map[ports.Key]bool) []ports.Claim {
	own := map[ports.Key]bool{}
	for _, n := range current.Nodes {
		if n.Enabled {
			for _, k := range ports.NodeKeys(n, n.Port) {
				own[k] = true
			}
		}
	}
	var out []ports.Claim
	for _, c := range ports.StateClaims(e.Nodes, e.Forwards) {
		if listening[c.Key] && !own[c.Key] {
			out = append(out, c)
		}
	}
	return out
}
