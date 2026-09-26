package formatter

import (
	"bytes"
	"testing"
)

func TestFormatKeepsUnarySignsWithTheirOperands(t *testing.T) {
	source := []byte("a[- 1]\na[+ 1]\na[-(offset+1)]\nx := - 1\ny := + value\nz := left - - right\nw := left + + right\nnested := - -1\nreturn - value\n")
	want := "a[-1]\na[+1]\na[-(offset + 1)]\nx := -1\ny := +value\nz := left - -right\nw := left + +right\nnested := - -1\nreturn -value\n"
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unary sign formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("unary sign formatting is not idempotent:\n%s\ndiagnostics=%v", formattedAgain, diagnostics)
	}
}
