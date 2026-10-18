package nft

import (
	"flag"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := "testdata/" + name + ".nft"
	if *update {
		os.WriteFile(path, got, 0o644)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("%s:\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

func TestGenerateGolden(t *testing.T) {
	for name, c := range map[string]Config{
		"empty": {},
		"firewall": {Firewall: &Firewall{Rules: []Rule{
			{Ports: "22222", Proto: "tcp", Comment: "litenode:ssh"},
			{Ports: "443", Proto: "tcp", Sources: []string{"203.0.113.7", "198.51.100.0/24", "2001:db8::/32"}, Comment: "litenode:node:3"},
			{Ports: "30000-30010", Proto: "tcp+udp", Comment: "litenode:manual:9"},
		}}},
		"no-ping": {Firewall: &Firewall{BlockPing: true, Rules: []Rule{{Ports: "22", Proto: "tcp", Comment: "litenode:ssh"}}}},
	} {
		got, err := Generate(c)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		golden(t, name, got)
	}
}

func TestGenerateRejects(t *testing.T) {
	for _, r := range []Rule{
		{Ports: "0", Proto: "tcp"}, {Ports: "70000", Proto: "tcp"}, {Ports: "20-10", Proto: "tcp"},
		{Ports: "22", Proto: "icmp"}, {Ports: "22", Proto: "tcp", Sources: []string{"nope"}}, {Ports: "022", Proto: "tcp"},
	} {
		if _, err := Generate(Config{Firewall: &Firewall{Rules: []Rule{r}}}); err == nil {
			t.Errorf("accepted %+v", r)
		}
	}
}

func TestParseSource(t *testing.T) {
	for in, want := range map[string]string{"1.2.3.4": "1.2.3.4", "10.1.2.3/8": "10.0.0.0/8", "::ffff:1.2.3.4": "1.2.3.4", "2001:db8::1/32": "2001:db8::/32"} {
		p, err := ParseSource(in)
		if err != nil || sourceString(p) != want {
			t.Errorf("%s: %v %v", in, sourceString(p), err)
		}
	}
}
