package protocol

import (
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/node"
)

// hysteriaPasswordLen is the length of a Hysteria2 password in bytes; it
// is hex encoded so links need no escaping.
const hysteriaPasswordLen = 16

// NewHysteriaPassword returns a random client password: 32 lowercase hex
// digits.
func NewHysteriaPassword(rand io.Reader) (string, error) {
	b := make([]byte, hysteriaPasswordLen)
	if _, err := io.ReadFull(rand, b); err != nil {
		return "", fmt.Errorf("generate hysteria password: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ValidHysteriaPassword reports whether s has the form NewHysteriaPassword
// generates.
func ValidHysteriaPassword(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == hysteriaPasswordLen && strings.ToLower(s) == s
}

// Hysteria is Hysteria2 with a password per client (DESIGN section 9.10).
// Its name is Xray's protocol name; the version is always 2.
type Hysteria struct{}

func (Hysteria) Name() string  { return "hysteria" }
func (Hysteria) Label() string { return "Hysteria2" }

type hysteriaClient struct {
	Auth  string `json:"auth"`
	Email string `json:"email"`
}

type hysteriaSettings struct {
	Version int              `json:"version"`
	Clients []hysteriaClient `json:"clients"`
}

// InboundSettings lists the enabled clients with their passwords and the
// client remark as the Xray email. Xray offers UDP relay whenever clients
// are listed.
func (Hysteria) InboundSettings(n node.Node) (any, error) {
	out := hysteriaSettings{Version: 2, Clients: []hysteriaClient{}}
	for _, c := range n.Clients {
		if !c.Enabled {
			continue
		}
		if !ValidHysteriaPassword(c.Key) {
			return nil, fmt.Errorf("client %d: invalid hysteria password", c.ID)
		}
		out.Clients = append(out.Clients, hysteriaClient{Auth: c.Key, Email: c.Remark})
	}
	return out, nil
}

// ShareLink builds a Hysteria2 URI with the parameters of the security
// layer (sni, pinSHA256):
//
//	hysteria2://<password>@<host>:<port>/?sni=<sni>&pinSHA256=<hex>#<remark>
func (Hysteria) ShareLink(n node.Node, c node.Client, host string, port int, params []node.Param) (string, error) {
	if !ValidHysteriaPassword(c.Key) {
		return "", fmt.Errorf("client %d: invalid hysteria password", c.ID)
	}
	var b strings.Builder
	b.WriteString("hysteria2://")
	b.WriteString(c.Key)
	b.WriteByte('@')
	b.WriteString(net.JoinHostPort(host, strconv.Itoa(port)))
	b.WriteString("/?")
	for i, p := range params {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(p.Key))
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(p.Value))
	}
	b.WriteByte('#')
	b.WriteString(EscapeRemark(c.Remark))
	return b.String(), nil
}
