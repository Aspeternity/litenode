package ssh

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
)

// Key types accepted in authorized_keys.
var keyTypes = []string{
	"ssh-ed25519",
	"ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521",
	"sk-ssh-ed25519@openssh.com", "sk-ecdsa-sha2-nistp256@openssh.com",
	"ssh-rsa",
}

// keyFields is the number of wire-format strings after the type in each
// type's key data.
var keyFields = map[string]int{
	"ssh-ed25519":                        1, // key
	"ecdsa-sha2-nistp256":                2, // curve, point
	"ecdsa-sha2-nistp384":                2,
	"ecdsa-sha2-nistp521":                2,
	"sk-ssh-ed25519@openssh.com":         2, // key, application
	"sk-ecdsa-sha2-nistp256@openssh.com": 3, // curve, point, application
	"ssh-rsa":                            2, // e, n
}

// MinRSABits is the smallest RSA key accepted (D-063).
const MinRSABits = 2048

// Errors of ParseKey.
var (
	ErrKeyFormat = errors.New("not an OpenSSH public key")
	ErrKeyType   = errors.New("unsupported key type")
	ErrKeyWeak   = errors.New("RSA key too short")
	// ErrPrivateKey: a private key was pasted where a public key was
	// asked for; it is discarded (FIX-04).
	ErrPrivateKey = errors.New("private key pasted")
)

// Key is one public key.
type Key struct {
	Type    string
	Blob    []byte
	Comment string
}

// Line is the key as an authorized_keys line.
func (k Key) Line() string {
	s := k.Type + " " + base64.StdEncoding.EncodeToString(k.Blob)
	if k.Comment != "" {
		s += " " + k.Comment
	}
	return s
}

// Fingerprint is "SHA256:..." as shown by ssh-keygen -l.
func (k Key) Fingerprint() string {
	sum := sha256.Sum256(k.Blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// ShortType is the type without its prefix, e.g. "ed25519" or "rsa".
func (k Key) ShortType() string {
	t := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(k.Type, "sk-"), "ssh-"), "@openssh.com")
	if strings.HasPrefix(t, "ecdsa") {
		return "ecdsa"
	}
	return t
}

// ParseKey parses a pasted public key: "type base64 [comment]", without
// authorized_keys options.
func ParseKey(line string) (Key, error) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 2 {
		return Key{}, ErrKeyFormat
	}
	if !isKeyType(fields[0]) {
		if strings.HasPrefix(fields[0], "ssh-") || strings.HasPrefix(fields[0], "ecdsa-") || strings.HasPrefix(fields[0], "sk-") {
			return Key{}, fmt.Errorf("%w: %s", ErrKeyType, fields[0])
		}
		return Key{}, ErrKeyFormat
	}
	blob, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return Key{}, fmt.Errorf("%w: %v", ErrKeyFormat, err)
	}
	r := bytes.NewReader(blob)
	name, err := readString(r)
	if err != nil || string(name) != fields[0] {
		return Key{}, fmt.Errorf("%w: key data does not match type %s", ErrKeyFormat, fields[0])
	}
	// The key data must be exactly the type's fields, as sshd requires:
	// otherwise a comment run into the data by a broken paste could pass
	// as part of it (FIX-04).
	parts := make([][]byte, keyFields[fields[0]])
	for i := range parts {
		if parts[i], err = readString(r); err != nil {
			return Key{}, fmt.Errorf("%w: %v", ErrKeyFormat, err)
		}
	}
	if r.Len() != 0 {
		return Key{}, fmt.Errorf("%w: trailing key data", ErrKeyFormat)
	}
	if fields[0] == "ssh-rsa" {
		if bits := new(big.Int).SetBytes(parts[1]).BitLen(); bits < MinRSABits {
			return Key{}, fmt.Errorf("%w: %d bits", ErrKeyWeak, bits)
		}
	}
	return Key{Type: fields[0], Blob: blob, Comment: strings.Join(fields[2:], " ")}, nil
}

// Pasted text with more lines than this is not rejoined (a public key is
// at most a dozen lines when wrapped).
const maxPasteLines = 64

// ParsePastedKey parses a public key pasted as one or more lines (a key
// wrapped by the terminal or editor it was copied from, FIX-04). The
// lines are rejoined without spaces, or else with a space at one or two
// of the breaks (where the type, data and comment meet), whichever
// parses first; the strict key data check makes the join unambiguous.
// A private key is refused with ErrPrivateKey.
func ParsePastedKey(text string) (Key, error) {
	if strings.Contains(text, "PRIVATE KEY") || strings.Contains(text, "PuTTY-User-Key-File") {
		return Key{}, ErrPrivateKey
	}
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	k, firstErr := ParseKey(strings.Join(lines, ""))
	if firstErr == nil || len(lines) < 2 || len(lines) > maxPasteLines {
		return k, firstErr
	}
	join := func(spaces ...int) string {
		var b strings.Builder
		for i, l := range lines {
			if i > 0 && slices.Contains(spaces, i) {
				b.WriteByte(' ')
			}
			b.WriteString(l)
		}
		return b.String()
	}
	// A join that gets past the format check (e.g. a short RSA key) gives
	// the more useful error.
	best := firstErr
	try := func(spaces ...int) (Key, bool) {
		k, err := ParseKey(join(spaces...))
		if err != nil && !errors.Is(err, ErrKeyFormat) && errors.Is(best, ErrKeyFormat) {
			best = err
		}
		return k, err == nil
	}
	for i := 1; i < len(lines); i++ {
		if k, ok := try(i); ok {
			return k, nil
		}
	}
	for i := 1; i < len(lines); i++ {
		for j := i + 1; j < len(lines); j++ {
			if k, ok := try(i, j); ok {
				return k, nil
			}
		}
	}
	return Key{}, best
}

func isKeyType(t string) bool {
	for _, k := range keyTypes {
		if k == t {
			return true
		}
	}
	return false
}

// readString reads an SSH wire-format string (uint32 length, bytes).
func readString(r *bytes.Reader) ([]byte, error) {
	var n uint32
	if err := binary.Read(r, binary.BigEndian, &n); err != nil {
		return nil, err
	}
	if int64(n) > int64(r.Len()) {
		return nil, errors.New("truncated key data")
	}
	b := make([]byte, n)
	r.Read(b)
	return b, nil
}

// AuthorizedKeys is an authorized_keys file. Lines are kept as they are
// (comments, options, keys LiteNode cannot parse); keys are found in them.
type AuthorizedKeys struct {
	lines []string
}

// ParseAuthorizedKeys reads an authorized_keys file.
func ParseAuthorizedKeys(data []byte) *AuthorizedKeys {
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return &AuthorizedKeys{}
	}
	return &AuthorizedKeys{lines: strings.Split(text, "\n")}
}

// lineKey finds the key in an authorized_keys line, which may start with
// options such as from="...".
func lineKey(line string) (Key, bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return Key{}, false
	}
	fields := strings.Fields(t)
	for i := range fields {
		if isKeyType(fields[i]) {
			if k, err := ParseKey(strings.Join(fields[i:], " ")); err == nil {
				return k, true
			}
			return Key{}, false
		}
	}
	return Key{}, false
}

// Keys lists the keys in file order, each once.
func (a *AuthorizedKeys) Keys() []Key {
	var out []Key
	seen := map[string]bool{}
	for _, l := range a.lines {
		if k, ok := lineKey(l); ok && !seen[string(k.Blob)] {
			seen[string(k.Blob)] = true
			out = append(out, k)
		}
	}
	return out
}

// Has reports whether the file already has k (same key data, whatever
// the comment).
func (a *AuthorizedKeys) Has(k Key) bool {
	for _, x := range a.Keys() {
		if bytes.Equal(x.Blob, k.Blob) {
			return true
		}
	}
	return false
}

// Add appends the keys not yet present and returns how many were added.
func (a *AuthorizedKeys) Add(keys ...Key) int {
	n := 0
	for _, k := range keys {
		if !a.Has(k) {
			a.lines = append(a.lines, k.Line())
			n++
		}
	}
	return n
}

// Remove deletes every line holding the key with the given fingerprint
// and reports whether there was one.
func (a *AuthorizedKeys) Remove(fingerprint string) bool {
	var kept []string
	found := false
	for _, l := range a.lines {
		if k, ok := lineKey(l); ok && k.Fingerprint() == fingerprint {
			found = true
			continue
		}
		kept = append(kept, l)
	}
	a.lines = kept
	return found
}

// Bytes is the file content.
func (a *AuthorizedKeys) Bytes() []byte {
	if len(a.lines) == 0 {
		return []byte{}
	}
	return []byte(strings.Join(a.lines, "\n") + "\n")
}
