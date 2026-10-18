// Package state reads and writes the state file with atomic writes, schema
// migration and file locking.
//
// /etc/litenode/state.json is the single source of truth: Xray
// configuration, nftables rules and firewall rules are all generated from it.
package state

import (
	"slices"

	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/node"
)

// SchemaVersion is the current state file format. Bump it only for
// incompatible changes (renaming, retyping or removing a field) and add a
// migration from the previous version. Adding a field whose zero value is
// the correct default does not need a new version.
const SchemaVersion = 1

// Default file locations.
const (
	DefaultPath      = ConfigDir + "/state.json"
	DefaultBackupDir = ConfigDir + "/backups"
	DefaultLockPath  = "/run/litenode.lock"
)

// State is the content of the state file. Feature sections (nodes,
// forwards, firewall) are added by their own tasks.
type State struct {
	SchemaVersion int `json:"schema_version"`
	// NextID is the next stable ID to hand out. It is shared by all kinds
	// of items and never decreases, so IDs are unique and never reused.
	NextID   int64       `json:"next_id"`
	Server   Server      `json:"server"`
	Xray     Xray        `json:"xray"`
	SSH      SSH         `json:"ssh"`
	Firewall Firewall    `json:"firewall"`
	Nodes    []node.Node `json:"nodes"`
	// Forwards are the port forwarding rules (DESIGN section 11).
	Forwards []forward.Forward `json:"forwards"`
	// Sysopt holds the system optimizations LiteNode manages (DESIGN
	// section 15).
	Sysopt Sysopt `json:"sysopt"`
}

// Server holds server-wide settings.
type Server struct {
	// ShareAddresses are the addresses share links may use: detected
	// public IPs and the user's domains. The first is the default (D-068).
	// Empty means not yet detected.
	ShareAddresses []string `json:"share_addresses,omitempty"`
	// LegacyShareAddress is the single address of v0.0.2 state files; it
	// is moved into ShareAddresses when loading.
	LegacyShareAddress string `json:"share_address,omitempty"`
	// ScanWarned is set once the user confirmed the nearby-site scan
	// warning (DESIGN section 10.3, D-058).
	ScanWarned bool `json:"scan_warned,omitempty"`
}

// Xray holds the installed Xray version and whether the user stopped it.
type Xray struct {
	// Version is empty when Xray is not installed.
	Version string `json:"version"`
	// Stopped is set when the user stopped Xray from the menu. Changes
	// then keep it stopped, also across reboots (D-045).
	Stopped bool `json:"stopped"`
}

// SSH holds the sshd settings LiteNode manages, written to
// /etc/ssh/sshd_config.d/00-litenode.conf (DESIGN section 13, D-061). A
// zero field is not managed: LiteNode leaves that setting to the system.
type SSH struct {
	Port         int   `json:"port,omitempty"`
	PubkeyAuth   *bool `json:"pubkey_authentication,omitempty"`
	PasswordAuth *bool `json:"password_authentication,omitempty"`
	// PermitRootLogin is "yes", "prohibit-password" or "no".
	PermitRootLogin string `json:"permit_root_login,omitempty"`
}

// Sysopt holds the system optimizations whose files are generated from
// the state. Settings changed by system commands (swap, DNS, time zone)
// are read from the system instead.
type Sysopt struct {
	// BBR writes BBR and fq to /etc/sysctl.d/99-litenode.conf.
	BBR bool `json:"bbr,omitempty"`
}

// Firewall is the LiteNode firewall (DESIGN section 14). Its automatic
// rules (SSH, nodes, forwards) are not stored: they follow those items.
type Firewall struct {
	Enabled bool `json:"enabled"`
	// BlockPing drops ICMP and ICMPv6 echo requests; ping is allowed by
	// default.
	BlockPing bool `json:"block_ping,omitempty"`
	// Rules are the user's own rules.
	Rules []FirewallRule `json:"rules,omitempty"`
	// AutoSources limits who may reach an automatic rule, by owner
	// ("ssh", "node:<id>", "forward:<id>"). The limit stays when the port
	// changes (P-004).
	AutoSources map[string][]string `json:"auto_sources,omitempty"`
}

// FirewallRule is a port the user opened.
type FirewallRule struct {
	ID int64 `json:"id"`
	// Port is one port ("443") or a range ("10000-10100").
	Port string `json:"port"`
	// Protocol is "tcp", "udp" or "tcp+udp".
	Protocol string `json:"protocol"`
	// Sources are IPv4 / IPv6 addresses or networks allowed in; empty
	// means anyone.
	Sources []string `json:"sources,omitempty"`
	Note    string   `json:"note,omitempty"`
}

// DefaultShareAddress is the address links use unless the user picks
// another, "" if none is set.
func (s Server) DefaultShareAddress() string {
	if len(s.ShareAddresses) == 0 {
		return ""
	}
	return s.ShareAddresses[0]
}

// normalize moves a v0.0.2 single share address into the list and writes
// a missing forward list as [].
func (st *State) normalize() {
	if st.Forwards == nil {
		st.Forwards = []forward.Forward{}
	}
	if a := st.Server.LegacyShareAddress; a != "" {
		if !slices.Contains(st.Server.ShareAddresses, a) {
			st.Server.ShareAddresses = append([]string{a}, st.Server.ShareAddresses...)
		}
		st.Server.LegacyShareAddress = ""
	}
}

// New returns the state used when no state file exists yet.
func New() *State {
	return &State{SchemaVersion: SchemaVersion, NextID: 1, Nodes: []node.Node{}, Forwards: []forward.Forward{}}
}

// AllocID returns a new stable ID.
func (s *State) AllocID() int64 {
	if s.NextID < 1 {
		s.NextID = 1
	}
	id := s.NextID
	s.NextID++
	return id
}
