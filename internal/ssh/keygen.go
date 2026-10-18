package ssh

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"io"
)

// GenerateKeyPair creates an ed25519 key pair (DESIGN section 13). The
// private key is in OpenSSH format, unencrypted: it is shown once and
// never stored on the server.
func GenerateKeyPair(rand io.Reader, comment string) (Key, []byte, error) {
	pub, priv, err := ed25519.GenerateKey(rand)
	if err != nil {
		return Key{}, nil, err
	}
	var blob bytes.Buffer
	writeString(&blob, []byte("ssh-ed25519"))
	writeString(&blob, pub)

	var check [4]byte
	if _, err := io.ReadFull(rand, check[:]); err != nil {
		return Key{}, nil, err
	}
	// openssh-key-v1 private section: two equal check words, the key,
	// the comment, then padding 1, 2, 3... to the cipher block size (8
	// for "none").
	var sec bytes.Buffer
	sec.Write(check[:])
	sec.Write(check[:])
	writeString(&sec, []byte("ssh-ed25519"))
	writeString(&sec, pub)
	writeString(&sec, priv) // seed followed by the public key
	writeString(&sec, []byte(comment))
	for i := byte(1); sec.Len()%8 != 0; i++ {
		sec.WriteByte(i)
	}

	var out bytes.Buffer
	out.WriteString("openssh-key-v1\x00")
	writeString(&out, []byte("none")) // cipher
	writeString(&out, []byte("none")) // kdf
	writeString(&out, nil)            // kdf options
	binary.Write(&out, binary.BigEndian, uint32(1))
	writeString(&out, blob.Bytes())
	writeString(&out, sec.Bytes())

	key := Key{Type: "ssh-ed25519", Blob: blob.Bytes(), Comment: comment}
	return key, pemOpenSSH(out.Bytes()), nil
}

// pemOpenSSH armors the key as ssh-keygen does: base64 in lines of 70.
func pemOpenSSH(data []byte) []byte {
	enc := base64.StdEncoding.EncodeToString(data)
	var b bytes.Buffer
	b.WriteString("-----BEGIN OPENSSH PRIVATE KEY-----\n")
	for len(enc) > 70 {
		b.WriteString(enc[:70] + "\n")
		enc = enc[70:]
	}
	b.WriteString(enc + "\n-----END OPENSSH PRIVATE KEY-----\n")
	return b.Bytes()
}

func writeString(b *bytes.Buffer, s []byte) {
	binary.Write(b, binary.BigEndian, uint32(len(s)))
	b.Write(s)
}
