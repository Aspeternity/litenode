package node

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeRemark(t *testing.T) {
	ok := map[string]string{
		"LA-Reality":                      "LA-Reality",
		"  me  ":                          "me",
		"洛杉矶 节点":                          "洛杉矶 节点",
		"a&b=c #1":                        "a&b=c #1",
		strings.Repeat("节", MaxRemarkLen): strings.Repeat("节", MaxRemarkLen),
	}
	for in, want := range ok {
		if got, err := NormalizeRemark(in); err != nil || got != want {
			t.Errorf("NormalizeRemark(%q) = (%q, %v), want %q", in, got, err, want)
		}
	}
	bad := map[string]error{
		"":                                  ErrRemarkEmpty,
		"   ":                               ErrRemarkEmpty,
		strings.Repeat("x", MaxRemarkLen+1): ErrRemarkTooLong,
		strings.Repeat("节", MaxRemarkLen+1): ErrRemarkTooLong,
		"a\tb":                              ErrRemarkControl,
		"line\nbreak":                       ErrRemarkControl,
		"esc\x1b[31m":                       ErrRemarkControl,
	}
	for in, want := range bad {
		if _, err := NormalizeRemark(in); !errors.Is(err, want) {
			t.Errorf("NormalizeRemark(%q) = %v, want %v", in, err, want)
		}
	}
}

func TestRemarkUniqueness(t *testing.T) {
	nodes := []Node{
		{ID: 1, Remark: "LA-Reality", Clients: []Client{{ID: 2, Remark: "me"}, {ID: 3, Remark: "Phone"}}},
		{ID: 4, Remark: "Tokyo", Clients: []Client{{ID: 5, Remark: "LA-Reality"}}},
	}

	// Node remarks: unique among nodes, case-insensitive.
	if err := CheckNodeRemark(nodes, "la-reality", 0); !errors.Is(err, ErrRemarkDuplicate) {
		t.Errorf("duplicate node remark: %v", err)
	}
	if err := CheckNodeRemark(nodes, "LA-Reality", 1); err != nil {
		t.Errorf("renaming node 1 to its own remark: %v", err)
	}
	if err := CheckNodeRemark(nodes, "me", 0); err != nil {
		t.Errorf("node remark may equal a client remark: %v", err)
	}

	// Client remarks: unique across the clients of all nodes.
	if err := CheckClientRemark(nodes, "PHONE", 0); !errors.Is(err, ErrRemarkDuplicate) {
		t.Errorf("duplicate client remark in another node: %v", err)
	}
	if err := CheckClientRemark(nodes, "la-reality", 0); !errors.Is(err, ErrRemarkDuplicate) {
		t.Errorf("duplicate client remark: %v", err)
	}
	if err := CheckClientRemark(nodes, "Tokyo", 0); err != nil {
		t.Errorf("client remark may equal a node remark: %v", err)
	}
	if err := CheckClientRemark(nodes, "me", 2); err != nil {
		t.Errorf("client 2 keeping its own remark: %v", err)
	}
}
