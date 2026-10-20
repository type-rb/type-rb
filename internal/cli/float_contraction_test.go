package cli

import (
	"os"
	"strings"
	"testing"
)

func TestFloatContractionContractAcrossBackendsAndREPL(t *testing.T) {
	source, err := os.ReadFile("../compiler/testdata/float_contraction.trb")
	if err != nil {
		t.Fatal(err)
	}
	// Every backend may choose fused or separately rounded arithmetic. The
	// program also checks effects, independently observed products and cases
	// where reassociation or ignoring special values would be incorrect.
	runPortableExecutionCase(t, string(source), strings.Repeat("true\n", 21), "")
}
