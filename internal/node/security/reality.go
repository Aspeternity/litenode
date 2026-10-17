// Package security contains node security layer implementations such as
// REALITY, TLS and none.
package security

import (
	"errors"

	"github.com/Aspeternity/litenode/internal/node"
)

// Reality is the REALITY security layer.
type Reality struct{}

func (Reality) Name() string  { return "reality" }
func (Reality) Label() string { return "Reality" }

type limitFallback struct {
	AfterBytes       int64 `json:"afterBytes"`
	BytesPerSec      int64 `json:"bytesPerSec"`
	BurstBytesPerSec int64 `json:"burstBytesPerSec"`
}

type realitySettings struct {
	Show                  bool           `json:"show"`
	Target                string         `json:"target"`
	ServerNames           []string       `json:"serverNames"`
	PrivateKey            string         `json:"privateKey"`
	ShortIDs              []string       `json:"shortIds"`
	LimitFallbackUpload   *limitFallback `json:"limitFallbackUpload,omitempty"`
	LimitFallbackDownload *limitFallback `json:"limitFallbackDownload,omitempty"`
}

// StreamSecurity returns the server side of REALITY. The fingerprint and
// public key are client settings and only appear in share links. The
// fallback limit (DESIGN section 10.4) applies the same values to upload
// and download; a zero limit is left out.
func (Reality) StreamSecurity(n node.Node) (string, string, any, error) {
	r := n.Reality
	if r == nil {
		return "", "", nil, errors.New("reality settings missing")
	}
	s := realitySettings{
		Target:      r.Target,
		ServerNames: r.ServerNames,
		PrivateKey:  r.PrivateKey,
		ShortIDs:    r.ShortIDs,
	}
	if l := r.FallbackLimit; l != (node.FallbackLimit{}) {
		lf := &limitFallback{AfterBytes: l.AfterBytes, BytesPerSec: l.BytesPerSec, BurstBytesPerSec: l.BurstBytesPerSec}
		s.LimitFallbackUpload, s.LimitFallbackDownload = lf, lf
	}
	return "reality", "realitySettings", s, nil
}
