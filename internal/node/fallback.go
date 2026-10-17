package node

// FallbackTier is a named fallback limit offered in the menu (P-009,
// D-053).
type FallbackTier struct {
	ID    string
	Limit FallbackLimit
}

// FallbackTiers are the presets of "modify node > fallback limit", in
// menu order. Strict still starts above typical homepage sizes, so normal
// visits are not throttled (which would reveal the VPS); loose suits
// targets with large homepages.
var FallbackTiers = []FallbackTier{
	{"default", DefaultFallbackLimit},
	{"loose", FallbackLimit{AfterBytes: 50 << 20, BytesPerSec: 5 << 20, BurstBytesPerSec: 20 << 20}},
	{"strict", FallbackLimit{AfterBytes: 5 << 20, BytesPerSec: 512 << 10, BurstBytesPerSec: 2 << 20}},
}
