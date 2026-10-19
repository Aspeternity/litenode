package protocol

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/node"
)

// SS2022Method is the only Shadowsocks method LiteNode uses (D-110): AES
// is hardware-accelerated on nearly every VPS and its 16-byte keys keep
// links short.
const SS2022Method = "2022-blake3-aes-128-gcm"

// ss2022KeyLen is the key length of SS2022Method in bytes.
const ss2022KeyLen = 16

// NewSS2022Key returns a random key for SS2022Method, standard base64
// encoded like "openssl rand -base64 16".
func NewSS2022Key(rand io.Reader) (string, error) {
	b := make([]byte, ss2022KeyLen)
	if _, err := io.ReadFull(rand, b); err != nil {
		return "", fmt.Errorf("generate shadowsocks key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// ValidSS2022Key reports whether s is a standard base64 key of the
// length SS2022Method needs.
func ValidSS2022Key(s string) bool {
	b, err := base64.StdEncoding.Strict().DecodeString(s)
	return err == nil && len(b) == ss2022KeyLen
}

// Shadowsocks is Shadowsocks 2022 with one server key and a user key per
// client (DESIGN section 9.8).
type Shadowsocks struct{}

func (Shadowsocks) Name() string  { return "shadowsocks" }
func (Shadowsocks) Label() string { return "Shadowsocks" }

type ssClient struct {
	Password string `json:"password"`
	Email    string `json:"email"`
}

type ssSettings struct {
	Method   string     `json:"method"`
	Password string     `json:"password"`
	Clients  []ssClient `json:"clients"`
	Network  string     `json:"network"`
}

// ssConfig returns the node's settings after checking the method and the
// server key.
func ssConfig(n node.Node) (*node.Shadowsocks, error) {
	s := n.Shadowsocks
	if s == nil {
		return nil, errors.New("shadowsocks settings missing")
	}
	if s.Method != SS2022Method {
		return nil, fmt.Errorf("unsupported shadowsocks method %q", s.Method)
	}
	if !ValidSS2022Key(s.ServerKey) {
		return nil, errors.New("invalid shadowsocks server key")
	}
	return s, nil
}

// InboundSettings lists the enabled clients with their user keys and the
// client remark as the Xray email. The inbound serves TCP and UDP.
func (Shadowsocks) InboundSettings(n node.Node) (any, error) {
	s, err := ssConfig(n)
	if err != nil {
		return nil, err
	}
	out := ssSettings{Method: s.Method, Password: s.ServerKey, Clients: []ssClient{}, Network: "tcp,udp"}
	for _, c := range n.Clients {
		if !c.Enabled {
			continue
		}
		if !ValidSS2022Key(c.Key) {
			return nil, fmt.Errorf("client %d: invalid shadowsocks key", c.ID)
		}
		out.Clients = append(out.Clients, ssClient{Password: c.Key, Email: c.Remark})
	}
	return out, nil
}

// ShareLink builds a SIP002 link (DESIGN section 9.8). The 2022 user info
// "<method>:<server key>:<user key>" is percent-encoded, not base64:
//
//	ss://2022-blake3-aes-128-gcm:<server key>%3A<user key>@<host>:<port>#<remark>
//
// The transport and security parameters do not apply to Shadowsocks links
// and are ignored.
func (Shadowsocks) ShareLink(n node.Node, c node.Client, host string, port int, _ []node.Param) (string, error) {
	s, err := ssConfig(n)
	if err != nil {
		return "", err
	}
	if !ValidSS2022Key(c.Key) {
		return "", fmt.Errorf("client %d: invalid shadowsocks key", c.ID)
	}
	var b strings.Builder
	b.WriteString("ss://")
	b.WriteString(s.Method)
	b.WriteByte(':')
	b.WriteString(url.QueryEscape(s.ServerKey + ":" + c.Key))
	b.WriteByte('@')
	b.WriteString(net.JoinHostPort(host, strconv.Itoa(port)))
	b.WriteByte('#')
	b.WriteString(EscapeRemark(c.Remark))
	return b.String(), nil
}
