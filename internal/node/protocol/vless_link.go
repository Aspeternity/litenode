package protocol

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/node"
)

// ShareLink builds a VLESS link (DESIGN section 9.5):
//
//	vless://<uuid>@<host>:<port>?encryption=none&flow=...&<params>#<remark>
func (VLESS) ShareLink(_ node.Node, c node.Client, host string, port int, params []node.Param) (string, error) {
	q := []node.Param{{Key: "encryption", Value: "none"}}
	if c.Flow != "" {
		q = append(q, node.Param{Key: "flow", Value: c.Flow})
	}
	q = append(q, params...)
	var b strings.Builder
	b.WriteString("vless://")
	b.WriteString(c.UUID)
	b.WriteByte('@')
	b.WriteString(net.JoinHostPort(host, strconv.Itoa(port)))
	b.WriteByte('?')
	for i, p := range q {
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

// EscapeRemark percent-encodes a remark for the URL fragment. Spaces
// become %20, not "+", which clients would show literally.
func EscapeRemark(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}
