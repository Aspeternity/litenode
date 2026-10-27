package egress

import (
	"context"
	"errors"
	"net/netip"
	"sync/atomic"
	"testing"
)

func TestParseTrace(t *testing.T) {
	for body, want := range map[string]Trace{
		"fl=1\nh=1.1.1.1\nip=203.0.113.10\nts=1\nwarp=off\n": {IP: mustAddr("203.0.113.10")},
		"ip=104.28.215.68\nwarp=on\n":                        {IP: mustAddr("104.28.215.68"), WARP: true},
		"ip=2001:db8::1\nwarp=plus\n":                        {IP: mustAddr("2001:db8::1"), WARP: true},
		"ip=::ffff:198.51.100.1\n":                           {IP: mustAddr("198.51.100.1")},
	} {
		if got, ok := ParseTrace([]byte(body)); !ok || got != want {
			t.Errorf("ParseTrace(%q) = %+v, %v; want %+v", body, got, ok, want)
		}
	}
	for _, body := range []string{"<html>", "warp=on\n", "ip=bad\n"} {
		if _, ok := ParseTrace([]byte(body)); ok {
			t.Errorf("ParseTrace(%q) should fail", body)
		}
	}
}

func mustAddr(s string) netip.Addr { return netip.MustParseAddr(s) }

// probe answers the two trace URLs from a map; missing ones fail.
func probe(answers map[string]string, calls *atomic.Int32) *Prober {
	return &Prober{Get: func(_ context.Context, url string) ([]byte, error) {
		calls.Add(1)
		if b, ok := answers[url]; ok {
			return []byte(b), nil
		}
		return nil, errors.New("timeout")
	}}
}

func TestProberStatus(t *testing.T) {
	var calls atomic.Int32
	p := probe(map[string]string{
		TraceURLv4: "ip=104.28.215.68\nwarp=on\n",
		TraceURLv6: "ip=2001:db8::a\nwarp=off\n",
	}, &calls)
	p.Start(t.Context())
	st := p.Status(t.Context())
	if !st.IPv4.OK || !st.IPv4.WARP || st.IPv4.Native() || !st.IPv6.Native() || st.IPv6.IP.String() != "2001:db8::a" {
		t.Errorf("status = %+v", st)
	}
	p.Status(t.Context())
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2 (detected once)", calls.Load())
	}
	p.Reset()
	p.Status(t.Context())
	if calls.Load() != 4 {
		t.Errorf("calls after Reset = %d, want 4", calls.Load())
	}
	if !st.Of(true).Native() || st.Of(false).Native() {
		t.Error("Of")
	}
}

func TestProberNoExit(t *testing.T) {
	var calls atomic.Int32
	// An IPv6 answer on the IPv4 URL (NAT64-like oddities) is ignored.
	st := probe(map[string]string{TraceURLv4: "ip=2001:db8::a\n"}, &calls).Status(t.Context())
	if st.IPv4.OK || st.IPv6.OK {
		t.Errorf("status = %+v", st)
	}
}

func TestStatusCanceled(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	p := &Prober{Get: func(ctx context.Context, _ string) ([]byte, error) {
		<-block
		return nil, errors.New("x")
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if st := p.Status(ctx); st.IPv4.OK || st.IPv6.OK {
		t.Errorf("status = %+v", st)
	}
}
