package ssh

import (
	"context"
	"errors"
	"testing"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/state"
)

func TestRollBackLast(t *testing.T) {
	ctx := context.Background()
	f, svc := loginService(t, "port 22\npubkeyauthentication yes\npasswordauthentication no\nkbdinteractiveauthentication no\npermitrootlogin yes\n")
	if _, err := svc.RollBackLast(ctx); !errors.Is(err, ErrNothingToRollBack) {
		t.Fatalf("no backups: %v", err)
	}
	f.Files["/root/.ssh/authorized_keys"] = []byte(readPub(t, "ed25519") + "\n")
	// Change 1: password login off (confirmed).
	if err := svc.SetPasswordAuth(ctx, false); err != nil {
		t.Fatal(err)
	}
	svc.Confirmer.Disarm(ctx)
	// Meanwhile a node is added: rolling back SSH must keep it.
	st, _ := svc.Store.Load()
	st.Nodes = []node.Node{{ID: 9, Port: 443, Enabled: true}}
	svc.Store.Save(st)

	prev, err := svc.RollBackLast(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if prev != (state.SSH{}) {
		t.Errorf("previous = %+v", prev)
	}
	st, _ = svc.Store.Load()
	if st.SSH != (state.SSH{}) || len(st.Nodes) != 1 {
		t.Errorf("after rollback: ssh=%+v nodes=%d", st.SSH, len(st.Nodes))
	}
	if _, ok := f.Files[ConfigPath]; ok {
		t.Error("drop-in still there")
	}
	if _, ok := f.Files["/etc/litenode/rollback/pending"]; !ok {
		t.Error("rollback not under confirm-or-rollback")
	}
	svc.Confirmer.Disarm(ctx)
	// Rolling back again returns to password off.
	prev, err = svc.RollBackLast(ctx)
	if err != nil || prev.PasswordAuth == nil || *prev.PasswordAuth {
		t.Errorf("second rollback: %+v %v", prev, err)
	}
}
