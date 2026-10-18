package sysopt

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/sysctl"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

func TestSetBBR(t *testing.T) {
	ctx := context.Background()
	f, s := service(t)
	f.OnCommand("sysctl -p "+sysctl.ConfigPath, func() { f.Files[ccPath] = []byte("bbr\n") })
	if err := s.SetBBR(ctx, true); err != nil {
		t.Fatal(err)
	}
	conf := string(f.Files[sysctl.ConfigPath])
	if !strings.Contains(conf, "net.core.default_qdisc = fq\n") || !strings.Contains(conf, "net.ipv4.tcp_congestion_control = bbr\n") {
		t.Errorf("sysctl file:\n%s", conf)
	}
	if b, err := s.BBRStatus(); err != nil || !b.Active || !b.Managed {
		t.Errorf("status = %+v, %v", b, err)
	}

	if err := s.SetBBR(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Files[sysctl.ConfigPath]; ok {
		t.Error("sysctl file kept without settings")
	}
	if !slices.Contains(f.Execs(), "sysctl -w net.ipv4.tcp_congestion_control=cubic") {
		t.Errorf("execs = %q", f.Execs())
	}
	if st, _ := s.Store.Load(); st.Sysopt.BBR {
		t.Error("state still has BBR")
	}
}

func TestSetBBRFailureRestores(t *testing.T) {
	f, s := service(t)
	f.Respond(systemtest.Response{Err: errors.New("invalid argument")}, "sysctl", "-p", sysctl.ConfigPath)
	if err := s.SetBBR(context.Background(), true); err == nil {
		t.Fatal("no error")
	}
	if _, ok := f.Files[sysctl.ConfigPath]; ok {
		t.Error("sysctl file not restored")
	}
	if st, _ := s.Store.Load(); st.Sysopt.BBR {
		t.Error("state not restored")
	}
}
