package node

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxRemarkLen is the maximum remark length in characters.
const MaxRemarkLen = 32

// Remark errors, mapped to user-facing text by the menu.
var (
	ErrRemarkEmpty     = errors.New("remark is empty")
	ErrRemarkTooLong   = errors.New("remark is too long")
	ErrRemarkControl   = errors.New("remark contains control characters")
	ErrRemarkDuplicate = errors.New("remark already in use")
)

// NormalizeRemark trims surrounding white space and checks the format:
// 1 to MaxRemarkLen characters, no control characters.
func NormalizeRemark(s string) (string, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return "", ErrRemarkEmpty
	case utf8.RuneCountInString(s) > MaxRemarkLen:
		return "", ErrRemarkTooLong
	case strings.ContainsFunc(s, unicode.IsControl):
		return "", ErrRemarkControl
	}
	return s, nil
}

// sameRemark compares remarks case-insensitively, so "LA" and "la" count
// as the same.
func sameRemark(a, b string) bool {
	return strings.EqualFold(a, b)
}

// CheckNodeRemark reports ErrRemarkDuplicate if another node (ID other
// than exceptID) already uses remark.
func CheckNodeRemark(nodes []Node, remark string, exceptID int64) error {
	for _, n := range nodes {
		if n.ID != exceptID && sameRemark(n.Remark, remark) {
			return ErrRemarkDuplicate
		}
	}
	return nil
}

// CheckClientRemark reports ErrRemarkDuplicate if another client of any
// node (ID other than exceptID) already uses remark. Client remarks are
// the Xray "email" and must be unique across all nodes.
func CheckClientRemark(nodes []Node, remark string, exceptID int64) error {
	for _, n := range nodes {
		for _, c := range n.Clients {
			if c.ID != exceptID && sameRemark(c.Remark, remark) {
				return ErrRemarkDuplicate
			}
		}
	}
	return nil
}
