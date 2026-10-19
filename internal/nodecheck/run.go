package nodecheck

import (
	"context"
	"fmt"
	"strings"

	"github.com/Aspeternity/litenode/internal/firewall"
	"github.com/Aspeternity/litenode/internal/nft"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/ufw"
)

// removeScript deletes the probe table whether or not it exists.
const removeScript = "table " + nft.ProbeTable + "\ndelete table " + nft.ProbeTable + "\n"

// Start loads the probe table counting ps. It is not persistent
// configuration, so it bypasses the change flow (DESIGN 9.9).
func Start(ctx context.Context, r system.Runner, ps []nft.ProbePort) error {
	script, err := nft.GenerateProbe(ps)
	if err != nil {
		return err
	}
	if _, err := r.ExecInput(ctx, script, "nft", "-f", "-"); err != nil {
		return fmt.Errorf("load probe table: %w", err)
	}
	return nil
}

// Read returns the current counts by protocol.
func Read(ctx context.Context, r system.Runner) (map[string]nft.ProbeCount, error) {
	res, err := r.Query(ctx, "nft", "-j", "list", "table", "inet", nft.ProbeTableName)
	if err != nil {
		return nil, fmt.Errorf("read probe table: %w", err)
	}
	return nft.ParseProbe([]byte(res.Stdout))
}

// Stop removes the probe table.
func Stop(ctx context.Context, r system.Runner) error {
	if _, err := r.ExecInput(ctx, []byte(removeScript), "nft", "-f", "-"); err != nil {
		return fmt.Errorf("remove probe table: %w", err)
	}
	return nil
}

// RemoveLeftover removes a probe table left by a check that was killed.
// It does nothing (and logs nothing) when nft is missing or there is no
// such table.
func RemoveLeftover(ctx context.Context, r system.Runner) error {
	res, err := r.Query(ctx, "nft", "list", "tables")
	if err != nil || !hasProbeTable(res.Stdout) {
		return nil
	}
	return Stop(ctx, r)
}

func hasProbeTable(listing string) bool {
	for _, line := range strings.Split(listing, "\n") {
		if strings.TrimSpace(line) == "table "+nft.ProbeTable {
			return true
		}
	}
	return false
}

// DetectEnv reads the local firewalls for Conclude. Other inbound
// filtering is only looked for when neither ufw nor the LiteNode firewall
// is on (firewallEnabled comes from the state).
func DetectEnv(ctx context.Context, r system.Runner, firewallEnabled bool) Env {
	env := Env{UFW: ufw.Active(ctx, r), Firewall: firewallEnabled}
	if !env.UFW && !env.Firewall {
		env.Foreign = firewall.DetectForeignFilter(ctx, r).Where
	}
	return env
}
