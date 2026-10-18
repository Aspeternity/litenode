package sysopt

import (
	"testing"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

const ccPath = "/proc/sys/net/ipv4/tcp_congestion_control"

func service(t *testing.T) (*systemtest.Fake, *Service) {
	t.Helper()
	f := systemtest.New()
	f.Files[ccPath] = []byte("cubic\n")
	store := state.NewStore(f)
	if err := store.Save(state.New()); err != nil {
		t.Fatal(err)
	}
	return f, &Service{Runner: f, Store: store, Changes: change.NewManager(f, store)}
}
