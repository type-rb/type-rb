package packageextension

import (
	"strings"
	"testing"
)

func TestSpecializeCallRejectsUnsupportedProtocolVersion(t *testing.T) {
	_, err := SpecializeCall(SpecializeCallRequest{ProtocolVersion: ProtocolVersion + 1, Provider: "missing"})
	if err == nil || !strings.Contains(err.Error(), "unsupported package extension protocol version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSpecializeCallRequiresRecheckedSourceAndReservedReplacement(t *testing.T) {
	const provider = "test.incomplete-call-specializer"
	RegisterCallProvider(provider, func(SpecializeCallRequest) SpecializeCallResponse {
		return SpecializeCallResponse{ProtocolVersion: ProtocolVersion}
	})
	_, err := SpecializeCall(SpecializeCallRequest{ProtocolVersion: ProtocolVersion, Provider: provider})
	if err == nil || !strings.Contains(err.Error(), "did not return generated TypeRB source") {
		t.Fatalf("unexpected error: %v", err)
	}
}
