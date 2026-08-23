package naming

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestCallableSuffixEncodesTheCompleteSourceName(t *testing.T) {
	tests := []struct {
		name string
		kind string
	}{
		{name: "ready?", kind: "question"},
		{name: "ready_now?", kind: "question"},
		{name: "save!", kind: "bang"},
	}
	seen := map[string]bool{}
	for _, test := range tests {
		kind, encoded, ok := CallableSuffix(test.name)
		if !ok || kind != test.kind {
			t.Fatalf("CallableSuffix(%q) = %q, %q, %t", test.name, kind, encoded, ok)
		}
		decoded, err := hex.DecodeString(encoded)
		if err != nil || string(decoded) != test.name {
			t.Fatalf("encoding for %q is not reversible: decoded=%q err=%v", test.name, decoded, err)
		}
		key := kind + ":" + encoded
		if seen[key] {
			t.Fatalf("encoding for %q collided at %q", test.name, key)
		}
		seen[key] = true
	}
}

func TestCallableSuffixRejectsOrdinaryNames(t *testing.T) {
	if _, _, ok := CallableSuffix("ready"); ok {
		t.Fatal("ordinary name was treated as suffixed")
	}
}

func TestRuntimeBindingIdentifierUsesTheCompleteDigest(t *testing.T) {
	first := RuntimeBindingIdentifier("github.com/acme/runtime/native#invoke")
	second := RuntimeBindingIdentifier("github.com/acme/runtime/native#other")
	if !strings.HasPrefix(first, "__trb_runtime_") || len(strings.TrimPrefix(first, "__trb_runtime_")) != 64 {
		t.Fatalf("runtime identifier does not contain a complete SHA-256 digest: %q", first)
	}
	if first == second || first != RuntimeBindingIdentifier("github.com/acme/runtime/native#invoke") {
		t.Fatalf("runtime identifiers must be deterministic and distinct: %q, %q", first, second)
	}
}
