package sysctl

import (
	"testing"

	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

const ccKey = "net.ipv4.tcp_congestion_control"

func TestBootValue(t *testing.T) {
	f := systemtest.New()
	if _, _, ok, err := BootValue(f, ccKey); ok || err != nil {
		t.Fatalf("no files: ok = %v, err = %v", ok, err)
	}

	f.Files["/usr/lib/sysctl.d/50-default.conf"] = []byte("net.ipv4.tcp_congestion_control = reno\n")
	f.Files["/etc/sysctl.d/99-bbr.conf"] = []byte("# BBR\nnet.core.default_qdisc=fq\n-net/ipv4/tcp_congestion_control=bbr\n")
	f.Files["/etc/sysctl.d/99-sysctl.conf"] = []byte("#net.ipv4.tcp_congestion_control = cubic\n")
	f.Files["/etc/sysctl.d/README"] = []byte("net.ipv4.tcp_congestion_control = vegas\n")
	f.Files[ConfigPath] = []byte("net.ipv4.ip_forward = 1\n")
	v, file, ok, err := BootValue(f, ccKey)
	if err != nil || !ok || v != "bbr" || file != "/etc/sysctl.d/99-bbr.conf" {
		t.Errorf("got %q, %q, %v, %v", v, file, ok, err)
	}

	// /etc shadows a file of the same name in /usr/lib.
	f.Files["/etc/sysctl.d/50-default.conf"] = []byte("net.ipv4.tcp_congestion_control = htcp\n")
	delete(f.Files, "/etc/sysctl.d/99-bbr.conf")
	v, file, ok, err = BootValue(f, ccKey)
	if err != nil || !ok || v != "htcp" || file != "/etc/sysctl.d/50-default.conf" {
		t.Errorf("shadowed: got %q, %q, %v, %v", v, file, ok, err)
	}
}

func TestLookup(t *testing.T) {
	data := []byte("; comment\nnet.ipv4.tcp_congestion_control = bbr\nbad line\n  net/ipv4/tcp_congestion_control=cubic  \n")
	if v, ok := lookup(data, ccKey); !ok || v != "cubic" {
		t.Errorf("got %q, %v", v, ok)
	}
	if _, ok := lookup(data, "net.core.default_qdisc"); ok {
		t.Error("found a missing key")
	}
}
