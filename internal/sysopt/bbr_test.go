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
	if _, err := s.SetBBR(ctx, true); err != nil {
		t.Fatal(err)
	}
	conf := string(f.Files[sysctl.ConfigPath])
	if !strings.Contains(conf, "net.core.default_qdisc = fq\n") || !strings.Contains(conf, "net.ipv4.tcp_congestion_control = bbr\n") {
		t.Errorf("sysctl file:\n%s", conf)
	}
	if b, err := s.BBRStatus(); err != nil || !b.Active || !b.Managed {
		t.Errorf("status = %+v, %v", b, err)
	}

	if keptBy, err := s.SetBBR(ctx, false); err != nil || keptBy != "" {
		t.Fatalf("keptBy = %q, err = %v", keptBy, err)
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
	if _, err := s.SetBBR(context.Background(), true); err == nil {
		t.Fatal("no error")
	}
	if _, ok := f.Files[sysctl.ConfigPath]; ok {
		t.Error("sysctl file not restored")
	}
	if st, _ := s.Store.Load(); st.Sysopt.BBR {
		t.Error("state not restored")
	}
}

// Turning BBR off sets the congestion control the other sysctl files
// give at boot instead of cubic.
func TestSetBBROffKeepsSystemValue(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, conf, want, keptBy string
	}{
		{"bbr", "net.ipv4.tcp_congestion_control = bbr\n", "", "/etc/sysctl.d/99-bbr.conf"},
		{"htcp", "net.ipv4.tcp_congestion_control = htcp\n", "sysctl -w net.ipv4.tcp_congestion_control=htcp", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, s := service(t)
			f.Files["/etc/sysctl.d/99-bbr.conf"] = []byte(tc.conf)
			f.OnCommand("sysctl -p "+sysctl.ConfigPath, func() { f.Files[ccPath] = []byte("bbr\n") })
			if _, err := s.SetBBR(ctx, true); err != nil {
				t.Fatal(err)
			}
			keptBy, err := s.SetBBR(ctx, false)
			if err != nil || keptBy != tc.keptBy {
				t.Errorf("keptBy = %q, err = %v", keptBy, err)
			}
			var writes []string
			for _, e := range f.Execs() {
				if strings.HasPrefix(e, "sysctl -w") {
					writes = append(writes, e)
				}
			}
			if tc.want == "" && len(writes) != 0 || tc.want != "" && !slices.Equal(writes, []string{tc.want}) {
				t.Errorf("sysctl -w calls = %q", writes)
			}
		})
	}
}
