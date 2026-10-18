package ports

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/Aspeternity/litenode/internal/forward"
	"github.com/Aspeternity/litenode/internal/node"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

// Captured from the test VPS: sshd on 61234 (0xEF32) listening, plus
// established connections that must not count.
const procTCP = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:EF32 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 28830 1 0 100 0 0 10 0
   1: FC5817D8:EF32 8FA7F617:C3BC 06 00000000:00000000 03:00000ABF 00000000     0        0 0 3 0
   2: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 51021 4 0
`

const procTCP6 = `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000000:01BB 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 28841 1 0
`

const procUDP = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops
  100: 00000000:01BB 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 1 2 0 0
`

func TestParseProcNet(t *testing.T) {
	if got := ParseProcNet([]byte(procTCP), TCP); !slices.Equal(got, []int{61234, 8080}) {
		t.Errorf("tcp = %v, want [61234 8080] (127.0.0.1 counts, established does not)", got)
	}
	if got := ParseProcNet([]byte(procTCP6), TCP); !slices.Equal(got, []int{443}) {
		t.Errorf("tcp6 = %v", got)
	}
	if got := ParseProcNet([]byte(procUDP), UDP); !slices.Equal(got, []int{443}) {
		t.Errorf("udp = %v", got)
	}
}

func TestListening(t *testing.T) {
	f := systemtest.New()
	f.Files["/proc/net/tcp"] = []byte(procTCP)
	f.Files["/proc/net/udp"] = []byte(procUDP)
	// tcp6 / udp6 missing: IPv6 disabled.
	got, err := Listening(f)
	if err != nil {
		t.Fatal(err)
	}
	want := map[Key]bool{{TCP, 61234}: true, {TCP, 8080}: true, {UDP, 443}: true}
	if len(got) != len(want) {
		t.Errorf("got %v", got)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing %v", k)
		}
	}
}

func TestCheck(t *testing.T) {
	nodes := []node.Node{{ID: 1, Port: 443, Enabled: true}, {ID: 2, Port: 8443, Enabled: false}}
	c := Checker{
		Listening: map[Key]bool{{TCP, 443}: true, {TCP, 22}: true, {UDP, 53}: true, {TCP, 8080}: true},
		Claims:    NodeClaims(nodes),
	}
	tests := []struct {
		name      string
		want      []Key
		self      string
		selfPorts []Key
		conflicts []Conflict
	}{
		{"free port", []Key{{TCP, 2053}}, "", nil, nil},
		{"program listening (even on 127.0.0.1)", []Key{{TCP, 8080}}, "", nil, []Conflict{{Key{TCP, 8080}, ""}}},
		{"disabled node still claims its port", []Key{{TCP, 8443}}, "", nil, []Conflict{{Key{TCP, 8443}, "node:2"}}},
		{"another node", []Key{{TCP, 443}}, "node:2", []Key{{TCP, 8443}}, []Conflict{{Key{TCP, 443}, "node:1"}}},
		{"node keeps its own port", []Key{{TCP, 443}}, "node:1", []Key{{TCP, 443}}, nil},
		{"udp and tcp are separate", []Key{{UDP, 443}, {TCP, 53}}, "", nil, nil},
		{"ssh keeps its own port", []Key{{TCP, 22}}, "ssh", []Key{{TCP, 22}}, nil},
		{"range checked port by port", Range("tcp+udp", 52, 54), "", nil, []Conflict{{Key{UDP, 53}, ""}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.Check(tt.want, tt.self, tt.selfPorts); !slices.Equal(got, tt.conflicts) {
				t.Errorf("got %v, want %v", got, tt.conflicts)
			}
		})
	}
}

func TestRange(t *testing.T) {
	if got := Range("tcp", 10, 11); !slices.Equal(got, []Key{{TCP, 10}, {TCP, 11}}) {
		t.Errorf("tcp = %v", got)
	}
	if got := Range("tcp+udp", 10, 10); !slices.Equal(got, []Key{{TCP, 10}, {UDP, 10}}) {
		t.Errorf("tcp+udp = %v", got)
	}
}

func TestRandomFree(t *testing.T) {
	c := Checker{Listening: map[Key]bool{}, Claims: nil}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 100 {
		p := c.RandomFree(rng)
		if p < 10000 || p > 60000 {
			t.Fatalf("port %d out of range", p)
		}
	}
}

func TestForwardClaims(t *testing.T) {
	fs := []forward.Forward{
		{ID: 5, Protocol: forward.TCPUDP, ListenPort: "30001"},
		{ID: 6, Protocol: forward.UDP, ListenPort: "31000-31001"},
	}
	c := Checker{Claims: StateClaims(nil, fs)}
	got := c.Check([]Key{{TCP, 30001}, {UDP, 30001}, {TCP, 31000}, {UDP, 31001}}, "", nil)
	want := []Conflict{{Key{TCP, 30001}, "forward:5"}, {Key{UDP, 30001}, "forward:5"}, {Key{UDP, 31001}, "forward:6"}}
	if !slices.Equal(got, want) {
		t.Errorf("Check = %v", got)
	}
	// A rule does not conflict with itself.
	if got := c.Check([]Key{{TCP, 30001}}, "forward:5", nil); len(got) != 0 {
		t.Errorf("self conflict %v", got)
	}
}
