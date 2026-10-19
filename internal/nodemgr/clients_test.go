package nodemgr

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/xray"
)

// clientsInConfig returns the emails of the clients of the first inbound.
func (fx *fixture) clientsInConfig(t *testing.T) []string {
	t.Helper()
	var cfg struct {
		Inbounds []struct {
			Settings struct {
				Clients []struct {
					Email string `json:"email"`
					ID    string `json:"id"`
				} `json:"clients"`
			} `json:"settings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(fx.f.Files[xray.ConfigPath], &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Inbounds) == 0 {
		return nil
	}
	var out []string
	for _, c := range cfg.Inbounds[0].Settings.Clients {
		out = append(out, c.Email+"="+c.ID)
	}
	return out
}

func TestClients(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	n := fx.add(t, 443, "LA")
	other := fx.add(t, 2053, "Tokyo")

	st := fx.state(t)
	if got := DefaultClientRemark(st, st.Nodes[0]); got != "LA-2" {
		t.Errorf("default remark = %q, want LA-2", got)
	}
	c, err := fx.svc.AddClient(ctx, n.ID, "phone")
	if err != nil {
		t.Fatal(err)
	}
	if c.Flow != node.FlowVision || !node.ValidUUID(c.UUID) || !c.Enabled {
		t.Errorf("new client = %+v (flow copied from the node's clients, D-056)", c)
	}
	if _, err := fx.svc.AddClient(ctx, other.ID, "PHONE"); !errors.Is(err, node.ErrRemarkDuplicate) {
		t.Errorf("client remark must be unique across nodes: %v", err)
	}
	if got := fx.clientsInConfig(t); len(got) != 2 {
		t.Errorf("config clients = %v", got)
	}

	// Resetting one UUID leaves the others alone.
	firstUUID := fx.state(t).Nodes[0].Clients[0].UUID
	if err := fx.svc.ResetClientCredential(ctx, n.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	cl := fx.state(t).Nodes[0].Clients
	if cl[0].UUID != firstUUID || cl[1].UUID == c.UUID {
		t.Error("reset changed the wrong client")
	}

	// A disabled client disappears from the Xray config.
	if err := fx.svc.SetClientEnabled(ctx, n.ID, c.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := fx.clientsInConfig(t); len(got) != 1 {
		t.Errorf("disabled client still in config: %v", got)
	}

	if err := fx.svc.SetClientRemark(ctx, n.ID, c.ID, "tokyo"); !errors.Is(err, node.ErrRemarkDuplicate) {
		t.Errorf("rename to another client's remark: %v", err)
	}
	if err := fx.svc.SetClientRemark(ctx, n.ID, c.ID, "ipad"); err != nil || fx.state(t).Nodes[0].Clients[1].Remark != "ipad" {
		t.Errorf("rename: %v", err)
	}

	if err := fx.svc.DeleteClient(ctx, n.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	// Deleting the last client keeps the node without an inbound (D-060).
	last := fx.state(t).Nodes[0].Clients[0].ID
	if err := fx.svc.DeleteClient(ctx, n.ID, last); err != nil {
		t.Fatalf("deleting the last client: %v", err)
	}
	if st := fx.state(t); st.Nodes[0].ID != n.ID || len(st.Nodes[0].Clients) != 0 || st.Nodes[0].Port != n.Port {
		t.Fatalf("node after deleting its last client: %+v", st.Nodes[0])
	}
	if got := fx.inbounds(t); slices.Contains(got, n.Port) {
		t.Errorf("node without clients still has an inbound: %v", got)
	}
	// Its next client takes the preset's flow and brings the inbound back.
	again, err := fx.svc.AddClient(ctx, n.ID, "again")
	if err != nil {
		t.Fatal(err)
	}
	if again.Flow != node.FlowVision {
		t.Errorf("flow of a client added to an empty node = %q", again.Flow)
	}
	if got := fx.inbounds(t); !slices.Contains(got, n.Port) {
		t.Errorf("inbounds after adding a client = %v", got)
	}
	if err := fx.svc.ResetClientCredential(ctx, n.ID, 9999); !errors.Is(err, ErrClientNotFound) {
		t.Errorf("unknown client: %v", err)
	}
}

func TestDefaultClientRemarkFitsLimit(t *testing.T) {
	fx := newFixture(t)
	long := "abcdefghijklmnopqrstuvwxyz012345" // 32 characters
	n := fx.add(t, 443, long)
	st := fx.state(t)
	r := DefaultClientRemark(st, st.Nodes[0])
	if len([]rune(r)) > node.MaxRemarkLen || node.CheckClientRemark(st.Nodes, r, 0) != nil {
		t.Errorf("default remark %q too long or not unique", r)
	}
	if _, err := fx.svc.AddClient(context.Background(), n.ID, r); err != nil {
		t.Errorf("default remark rejected: %v", err)
	}
}
