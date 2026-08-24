// Package naming contains target-independent helpers for encoding TypeRB
// source names that are not valid identifiers in every backend.
package naming

import (
	"crypto/sha256"
	"encoding/hex"
)

// CallableSuffix returns the source suffix kind and a reversible encoding of
// the complete TypeRB name. Encoding the complete name keeps distinct source
// spellings distinct even when a backend normally normalizes their base name.
func CallableSuffix(name string) (kind, encoded string, ok bool) {
	if len(name) == 0 {
		return "", "", false
	}
	switch name[len(name)-1] {
	case '?':
		kind = "question"
	case '!':
		kind = "bang"
	default:
		return "", "", false
	}
	return kind, hex.EncodeToString([]byte(name)), true
}

// RuntimeBindingIdentifier returns a deterministic private identifier for a
// canonical native runtime binding identity.
func RuntimeBindingIdentifier(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return "__trb_runtime_" + hex.EncodeToString(sum[:])
}

// PrivateSuffix returns a short deterministic suffix for generated identifiers
// whose declarations share a target-language package across source modules.
func PrivateSuffix(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:8])
}
