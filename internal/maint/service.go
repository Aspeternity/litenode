package maint

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/firewall"
	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/forwardmgr"
	"github.com/Aspeternity/litenode/internal/nodemgr"
	"github.com/Aspeternity/litenode/internal/ports"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/sysctl"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/ufw"
	"github.com/Aspeternity/litenode/internal/xray"
)

// ExportFileMode is the export file's mode: it holds private keys.
const ExportFileMode = 0o600

// Service runs the maintenance operations (DESIGN section 16).
type Service struct {
	Runner   system.Runner
	Store    *state.Store
	Nodes    *nodemgr.Service
	Forwards *forwardmgr.Service
	// Confirmer applies an import that changes who may reach SSH through
	// the LiteNode firewall.
	Confirmer *change.Confirmer
	Version   string
	Now       func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// DefaultExportPath is the suggested export file name.
func (s *Service) DefaultExportPath() string {
	return "/root/litenode-export-" + s.now().Format("20060102-150405") + ".json"
}

// Export writes the export file to path with mode 0600.
func (s *Service) Export(path string) (Summary, error) {
	st, err := s.Store.Load()
	if err != nil {
		return Summary{}, err
	}
	e := Build(st, s.Version, s.now())
	data, err := Encode(e)
	if err != nil {
		return Summary{}, err
	}
	if err := s.Runner.WriteFile(path, data, ExportFileMode); err != nil {
		return Summary{}, fmt.Errorf("write export file: %w", err)
	}
	return Summarize(e), nil
}

// Read loads and checks an export file.
func (s *Service) Read(path string) (Export, error) {
	data, err := s.Runner.ReadFile(path)
	if err != nil {
		return Export{}, fmt.Errorf("read export file: %w", err)
	}
	return Decode(data, s.Nodes.Registry, nil)
}

// PortsTakenError lists imported ports other programs listen on.
type PortsTakenError struct {
	Claims []ports.Claim
}

func (e *PortsTakenError) Error() string { return fmt.Sprintf("ports taken: %v", e.Claims) }

// Import replaces the nodes, clients, forwards and firewall rules with e
// as one change of every kind they touch (DESIGN section 16); the change
// backs up the current state first. Ports another program listens on stop
// the import with *PortsTakenError. When the LiteNode firewall is on and
// the import changes who may reach SSH, the change waits for
// confirmation and Import reports confirm = true: the caller asks the
// user and disarms.
func (s *Service) Import(ctx context.Context, e Export) (confirm bool, err error) {
	st, err := s.Store.Load()
	if err != nil {
		return false, err
	}
	listening, err := ports.Listening(s.Runner)
	if err != nil {
		return false, err
	}
	if c := Conflicts(e, st, listening); len(c) > 0 {
		return false, &PortsTakenError{Claims: c}
	}
	next := Replace(st, e)
	if len(next.Nodes) > 0 {
		if err := s.Nodes.EnsureXray(ctx, next); err != nil {
			return false, err
		}
	}
	if slices.ContainsFunc(next.Forwards, func(f forward.Forward) bool { return f.Enabled }) || next.Firewall.Enabled {
		if err := firewall.EnsureNft(ctx, s.Runner); err != nil {
			return false, err
		}
	}
	parts, err := s.parts(ctx, next)
	if err != nil {
		return false, err
	}
	ch := change.Change{Parts: parts, State: next}
	if next.Firewall.Enabled {
		sshPorts, err := (&firewall.Service{Store: s.Store, Confirmer: s.Confirmer}).SSHPorts(ctx, next)
		if err != nil {
			return false, err
		}
		confirm = firewall.AffectsSSH(st, next, sshPorts)
	}
	if confirm {
		_, err = s.Confirmer.Apply(ctx, ch)
	} else {
		_, err = s.Confirmer.Manager.Apply(ctx, ch)
	}
	if err != nil {
		return false, err
	}
	if err := s.Forwards.SyncTimer(ctx, next); err != nil {
		return confirm, &forwardmgr.TimerError{Err: err}
	}
	return confirm, nil
}

// parts are the changes an import makes, in apply order: kernel
// forwarding, Xray, the nftables table, then ufw.
func (s *Service) parts(ctx context.Context, st *state.State) ([]change.Part, error) {
	var parts []change.Part
	env, err := sysctl.ReadEnv(s.Runner)
	if err != nil {
		return nil, err
	}
	f, ok, err := sysctl.ConfigFile(s.Runner, sysctl.Desired(st, env))
	if err != nil {
		return nil, err
	}
	if ok {
		parts = append(parts, change.Part{Kind: sysctl.Kind(), Files: []change.File{f}})
	}
	if _, installed := s.Nodes.Xray.Installed(ctx); installed {
		x, err := xray.ConfigFile(s.Nodes.Registry, st)
		if err != nil {
			return nil, err
		}
		parts = append(parts, change.Part{Kind: xray.Kind(s.Store, s.Nodes.Xray), Files: []change.File{x}})
	}
	return append(parts, change.Part{Kind: firewall.SyncKind(s.Store)}, change.Part{Kind: ufw.Kind(s.Store)}), nil
}
