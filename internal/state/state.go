// Package state reads and writes the state file with atomic writes, schema
// migration and file locking.
//
// /etc/litenode/state.json is the single source of truth: Xray
// configuration, nftables rules and firewall rules are all generated from it.
package state

import "github.com/Aspeternity/litenode/internal/node"

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
	NextID int64       `json:"next_id"`
	Server Server      `json:"server"`
	Xray   Xray        `json:"xray"`
	Nodes  []node.Node `json:"nodes"`
}

// Server holds server-wide settings.
type Server struct {
	// ShareAddress is the address used in share links. Empty means not yet
	// chosen.
	ShareAddress string `json:"share_address"`
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

// New returns the state used when no state file exists yet.
func New() *State {
	return &State{SchemaVersion: SchemaVersion, NextID: 1, Nodes: []node.Node{}}
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
