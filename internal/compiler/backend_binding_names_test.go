package compiler

import (
	"os"
	"testing"
)

func TestBackendReservedBindingNamesRunAcrossBackends(t *testing.T) {
	source, err := os.ReadFile("testdata/backend_binding_names.trb")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "backend_binding_names.trb", source, "6\n23\n11\n14\n4\n3\n4\n3\n11\n7\n8")
		})
	}
}
