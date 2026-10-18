package ssh

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os"
	"strings"
	"testing"
)

func readPub(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func TestParseKeyRealKeys(t *testing.T) {
	// Fingerprints from ssh-keygen -lf.
	for name, want := range map[string]struct{ fp, typ, short string }{
		"ed25519": {"SHA256:lpC5s49LE3yha+tpRyTosMU8dLYae6I+sNQQ+WRVueI", "ssh-ed25519", "ed25519"},
		"rsa":     {"SHA256:sUx22/3EzEd1nbx+VU3N2sqtp6iqqzMvbon3RslFhF8", "ssh-rsa", "rsa"},
		"ecdsa":   {"SHA256:5XTvJyQJwiWFdtCk/I5daz5XnpsIUdjS5GT0VEz2ldo", "ecdsa-sha2-nistp384", "ecdsa"},
	} {
		line := readPub(t, name)
		k, err := ParseKey(line)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if k.Fingerprint() != want.fp || k.Type != want.typ || k.ShortType() != want.short || k.Comment != "test-"+name {
			t.Errorf("%s: %s %s %s %q", name, k.Fingerprint(), k.Type, k.ShortType(), k.Comment)
		}
		if k.Line() != line {
			t.Errorf("%s: Line() = %q", name, k.Line())
		}
	}
}

func TestParseKeyRejects(t *testing.T) {
	ed := readPub(t, "ed25519")
	fields := strings.Fields(ed)
	blob, _ := base64.StdEncoding.DecodeString(fields[1])
	for name, tc := range map[string]struct {
		line string
		want error
	}{
		"rsa 1024":       {readPub(t, "rsa1024"), ErrKeyWeak},
		"dss":            {"ssh-dss AAAAB3NzaC1kc3M= x", ErrKeyType},
		"empty":          {"", ErrKeyFormat},
		"private key":    {"-----BEGIN OPENSSH PRIVATE KEY-----", ErrKeyFormat},
		"bad base64":     {"ssh-ed25519 !!!", ErrKeyFormat},
		"type mismatch":  {"ssh-rsa " + fields[1], ErrKeyFormat},
		"truncated blob": {"ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob[:5]), ErrKeyFormat},
		"options":        {`from="1.2.3.4" ` + ed, ErrKeyFormat},
	} {
		if _, err := ParseKey(tc.line); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", name, err, tc.want)
		}
	}
}

func TestAuthorizedKeys(t *testing.T) {
	ed, rsa := readPub(t, "ed25519"), readPub(t, "rsa")
	edKey, _ := ParseKey(ed)
	rsaKey, _ := ParseKey(rsa)
	file := "# my keys\n" + `from="10.0.0.0/8",no-pty ` + ed + "\n" + "garbage line\n" + ed + " duplicate\n"
	ak := ParseAuthorizedKeys([]byte(file))
	keys := ak.Keys()
	if len(keys) != 1 || keys[0].Fingerprint() != edKey.Fingerprint() {
		t.Fatalf("keys = %v", keys)
	}
	// The same key with another comment is not added again.
	edOther := edKey
	edOther.Comment = "laptop"
	if n := ak.Add(edOther, rsaKey); n != 1 {
		t.Errorf("added %d, want 1", n)
	}
	want := file + rsa + "\n"
	if got := string(ak.Bytes()); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	// Removing a key removes every line holding it, options included,
	// and keeps comments and unknown lines.
	if !ak.Remove(edKey.Fingerprint()) || ak.Remove("SHA256:nope") {
		t.Error("Remove result")
	}
	if got, want := string(ak.Bytes()), "# my keys\ngarbage line\n"+rsa+"\n"; got != want {
		t.Errorf("after remove:\n%s\nwant:\n%s", got, want)
	}
	if got := ParseAuthorizedKeys(nil).Bytes(); len(got) != 0 {
		t.Errorf("empty file = %q", got)
	}
}

// TestGenerateKeyPair decodes the private key file and checks that it
// holds a working key matching the public key.
func TestGenerateKeyPair(t *testing.T) {
	pub, priv, err := GenerateKeyPair(rand.Reader, "litenode@test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseKey(pub.Line()); err != nil {
		t.Fatalf("public key: %v", err)
	}
	s := string(priv)
	if !strings.HasPrefix(s, "-----BEGIN OPENSSH PRIVATE KEY-----\n") || !strings.HasSuffix(s, "\n-----END OPENSSH PRIVATE KEY-----\n") {
		t.Fatalf("armor:\n%s", s)
	}
	body := strings.TrimSuffix(strings.TrimPrefix(s, "-----BEGIN OPENSSH PRIVATE KEY-----\n"), "-----END OPENSSH PRIVATE KEY-----\n")
	for _, l := range strings.Split(strings.TrimSpace(body), "\n") {
		if len(l) > 70 {
			t.Errorf("line longer than 70: %d", len(l))
		}
	}
	data, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\n", ""))
	if err != nil {
		t.Fatal(err)
	}
	magic := "openssh-key-v1\x00"
	if !strings.HasPrefix(string(data), magic) {
		t.Fatal("magic")
	}
	r := bytes.NewReader(data[len(magic):])
	str := func() []byte {
		b, err := readString(r)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if string(str()) != "none" || string(str()) != "none" || len(str()) != 0 {
		t.Fatal("cipher / kdf")
	}
	var n uint32
	binary.Read(r, binary.BigEndian, &n)
	if n != 1 || !bytes.Equal(str(), pub.Blob) {
		t.Fatal("public key section")
	}
	sec := str()
	if len(sec)%8 != 0 {
		t.Errorf("private section length %d", len(sec))
	}
	sr := bytes.NewReader(sec)
	var c1, c2 uint32
	binary.Read(sr, binary.BigEndian, &c1)
	binary.Read(sr, binary.BigEndian, &c2)
	r = sr
	if c1 != c2 || string(str()) != "ssh-ed25519" {
		t.Fatal("check words / type")
	}
	pk, sk := str(), str()
	if string(str()) != "litenode@test" {
		t.Error("comment")
	}
	msg := []byte("hello")
	if !ed25519.Verify(pk, msg, ed25519.Sign(ed25519.PrivateKey(sk), msg)) {
		t.Error("private key does not match public key")
	}
}
