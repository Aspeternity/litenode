// Package warp manages a Cloudflare WARP tunnel that supplies the exit
// family this host lacks (DESIGN section 19.4, D-133): kernel WireGuard
// with wireguard-tools, keys and registration in the standard library.
package warp

import (
	"bufio"
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Names and locations.
const (
	Interface = "litenode-warp"
	ConfPath  = "/etc/wireguard/" + Interface + ".conf"
	Unit      = "wg-quick@" + Interface + ".service"
	// APIURL registers a device with Cloudflare's consumer WARP service.
	APIURL = "https://api.cloudflareclient.com/v0a2158/reg"
	// ClientVersion is sent as CF-Client-Version, as the app does.
	ClientVersion = "a-6.10-2158"
)

// Ports WARP endpoints listen on, tried in this order.
var Ports = []uint16{2408, 1701, 500, 4500}

// Built-in endpoint addresses, tried after the ones the registration
// returned. Only some combinations of address and port work from a given
// network (D-133).
var (
	BuiltinEndpoints6 = []netip.Addr{netip.MustParseAddr("2606:4700:d0::a29f:c001"), netip.MustParseAddr("2606:4700:d1::a29f:c001")}
	BuiltinEndpoints4 = []netip.Addr{netip.MustParseAddr("162.159.192.1"), netip.MustParseAddr("162.159.193.1")}
)

// Keys is a WireGuard key pair in base64.
type Keys struct {
	Private, Public string
}

// NewKeys generates an X25519 key pair.
func NewKeys(rand io.Reader) (Keys, error) {
	k, err := ecdh.X25519().GenerateKey(rand)
	if err != nil {
		return Keys{}, fmt.Errorf("generate wireguard key: %w", err)
	}
	enc := base64.StdEncoding.EncodeToString
	return Keys{Private: enc(k.Bytes()), Public: enc(k.PublicKey().Bytes())}, nil
}

// RegisterBody is the registration request for public key pub.
func RegisterBody(pub string, now time.Time) []byte {
	b, _ := json.Marshal(map[string]string{
		"key": pub, "install_id": "", "fcm_token": "", "tos": now.UTC().Format(time.RFC3339Nano),
		"model": "PC", "serial_number": "", "locale": "en_US",
	})
	return b
}

// Account is what the registration returns that LiteNode uses.
type Account struct {
	ID, Token string
	// Addr4 and Addr6 are this host's addresses inside the tunnel.
	Addr4, Addr6 netip.Addr
	PeerKey      string
	// Endpoints are the endpoint addresses offered (port not included).
	Endpoints []netip.Addr
	Ports     []uint16
}

// ParseAccount reads a registration response.
func ParseAccount(data []byte) (Account, error) {
	var r struct {
		ID     string `json:"id"`
		Token  string `json:"token"`
		Config struct {
			Peers []struct {
				PublicKey string `json:"public_key"`
				Endpoint  struct {
					V4    string   `json:"v4"`
					V6    string   `json:"v6"`
					Ports []uint16 `json:"ports"`
				} `json:"endpoint"`
			} `json:"peers"`
			Interface struct {
				Addresses struct {
					V4 string `json:"v4"`
					V6 string `json:"v6"`
				} `json:"addresses"`
			} `json:"interface"`
		} `json:"config"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return Account{}, fmt.Errorf("parse warp registration: %w", err)
	}
	if r.ID == "" || r.Token == "" || len(r.Config.Peers) == 0 || r.Config.Peers[0].PublicKey == "" {
		return Account{}, errors.New("parse warp registration: incomplete response")
	}
	a := Account{ID: r.ID, Token: r.Token, PeerKey: r.Config.Peers[0].PublicKey, Ports: r.Config.Peers[0].Endpoint.Ports}
	var err4, err6 error
	a.Addr4, err4 = netip.ParseAddr(r.Config.Interface.Addresses.V4)
	a.Addr6, err6 = netip.ParseAddr(r.Config.Interface.Addresses.V6)
	if err4 != nil || err6 != nil || !a.Addr4.Is4() || !a.Addr6.Is6() {
		return Account{}, errors.New("parse warp registration: bad tunnel addresses")
	}
	ep := r.Config.Peers[0].Endpoint
	for _, s := range []string{ep.V6, ep.V4} {
		// "[2606:4700:d0::a29f:c001]:0" or "162.159.192.1:0"
		if ap, err := netip.ParseAddrPort(s); err == nil {
			a.Endpoints = append(a.Endpoints, ap.Addr())
		} else if ip, err := netip.ParseAddr(s); err == nil {
			a.Endpoints = append(a.Endpoints, ip)
		}
	}
	return a, nil
}

// Candidates lists the endpoints to try, reachable over the family this
// host has (endpointV6), in order: the addresses offered by the
// registration, then the built-in ones, each with every port. avoid (the
// endpoint that just stopped working) goes last.
func Candidates(offered []netip.Addr, ports []uint16, endpointV6 bool, avoid netip.AddrPort) []netip.AddrPort {
	builtin := BuiltinEndpoints4
	if endpointV6 {
		builtin = BuiltinEndpoints6
	}
	var ips []netip.Addr
	for _, ip := range append(slices.Clone(offered), builtin...) {
		if ip.Is6() == endpointV6 && !slices.Contains(ips, ip) {
			ips = append(ips, ip)
		}
	}
	ps := slices.Clone(Ports)
	for _, p := range ports {
		if p != 0 && !slices.Contains(ps, p) {
			ps = append(ps, p)
		}
	}
	var out []netip.AddrPort
	for _, ip := range ips {
		for _, p := range ps {
			if ap := netip.AddrPortFrom(ip, p); ap != avoid {
				out = append(out, ap)
			}
		}
	}
	if avoid.IsValid() && avoid.Addr().Is6() == endpointV6 {
		out = append(out, avoid)
	}
	return out
}

// Conf is LiteNode's WireGuard configuration for WARP. The account ID and
// token are kept in comments of the same 0600 file rather than in the
// state file, so exported configurations never carry them (D-133).
type Conf struct {
	AccountID, Token string
	// IPv6 is the family WARP supplies: false for IPv4 on an IPv6-only
	// host.
	IPv6       bool
	PrivateKey string
	// Address is this host's address inside the tunnel, of family IPv6.
	Address  netip.Addr
	PeerKey  string
	Endpoint netip.AddrPort
}

const (
	keyAccount = "# litenode-warp-account: "
	keyToken   = "# litenode-warp-token: "
	keyFamily  = "# litenode-warp-family: "
)

// FamilyName is "ipv4" or "ipv6".
func FamilyName(ipv6 bool) string {
	if ipv6 {
		return "ipv6"
	}
	return "ipv4"
}

// Render writes the wg-quick file. Only the supplied family is routed
// into the tunnel; wg-quick's policy routing leaves more specific routes
// of the main table alone. No DNS line: the system resolver is not
// touched.
func (c Conf) Render() []byte {
	bits, allowed := 32, "0.0.0.0/0"
	if c.IPv6 {
		bits, allowed = 128, "::/0"
	}
	var b strings.Builder
	b.WriteString("# Cloudflare WARP, managed by LiteNode (system optimization > WARP). Do not edit.\n")
	b.WriteString(keyAccount + c.AccountID + "\n")
	b.WriteString(keyToken + c.Token + "\n")
	b.WriteString(keyFamily + FamilyName(c.IPv6) + "\n")
	b.WriteString("[Interface]\n")
	b.WriteString("PrivateKey = " + c.PrivateKey + "\n")
	b.WriteString("Address = " + netip.PrefixFrom(c.Address, bits).String() + "\n")
	b.WriteString("MTU = 1280\n\n")
	b.WriteString("[Peer]\n")
	b.WriteString("PublicKey = " + c.PeerKey + "\n")
	b.WriteString("AllowedIPs = " + allowed + "\n")
	b.WriteString("Endpoint = " + c.Endpoint.String() + "\n")
	b.WriteString("PersistentKeepalive = 25\n")
	return []byte(b.String())
}

// ParseConf reads a file written by Render.
func ParseConf(data []byte) (Conf, error) {
	var c Conf
	family := ""
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if v, ok := strings.CutPrefix(line, keyAccount); ok {
			c.AccountID = v
			continue
		}
		if v, ok := strings.CutPrefix(line, keyToken); ok {
			c.Token = v
			continue
		}
		if v, ok := strings.CutPrefix(line, keyFamily); ok {
			family = v
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "PrivateKey":
			c.PrivateKey = v
		case "PublicKey":
			c.PeerKey = v
		case "Address":
			if p, err := netip.ParsePrefix(v); err == nil {
				c.Address = p.Addr()
			}
		case "Endpoint":
			if ap, err := netip.ParseAddrPort(v); err == nil {
				c.Endpoint = ap
			}
		}
	}
	if family != "ipv4" && family != "ipv6" || c.PrivateKey == "" || c.PeerKey == "" || !c.Address.IsValid() || !c.Endpoint.IsValid() {
		return Conf{}, errors.New("not a LiteNode WARP configuration")
	}
	c.IPv6 = family == "ipv6"
	return c, nil
}

// ParseHandshake reads "wg show <if> latest-handshakes" ("<key>\t<unix>")
// and returns the latest handshake, zero if none happened yet.
func ParseHandshake(out string) time.Time {
	var latest int64
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		if n, err := strconv.ParseInt(f[1], 10, 64); err == nil && n > latest {
			latest = n
		}
	}
	if latest == 0 {
		return time.Time{}
	}
	return time.Unix(latest, 0)
}
