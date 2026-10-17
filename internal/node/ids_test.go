package node

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestNewUUID(t *testing.T) {
	got, err := NewUUID(bytes.NewReader(bytes.Repeat([]byte{0xff}, 16)))
	if err != nil || got != "ffffffff-ffff-4fff-bfff-ffffffffffff" {
		t.Errorf("NewUUID = (%q, %v)", got, err)
	}
	seen := map[string]bool{}
	for range 100 {
		u, err := NewUUID(rand.Reader)
		if err != nil || !ValidUUID(u) || u[14] != '4' || !bytes.ContainsRune([]byte("89ab"), rune(u[19])) {
			t.Fatalf("bad uuid %q (%v)", u, err)
		}
		if seen[u] {
			t.Fatal("duplicate uuid")
		}
		seen[u] = true
	}
}

func TestValidUUID(t *testing.T) {
	for s, want := range map[string]bool{
		"5783a3e7-e373-51cd-8642-c83782b807c5": true,
		"5783A3E7-E373-51CD-8642-C83782B807C5": false,
		"5783a3e7e37351cd8642c83782b807c5":     false,
		"":                                     false,
	} {
		if ValidUUID(s) != want {
			t.Errorf("ValidUUID(%q) = %v", s, !want)
		}
	}
}
