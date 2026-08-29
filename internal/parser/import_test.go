package parser

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestParseNamedImportAliasesAndActivation(t *testing.T) {
	program, diagnostics := Parse([]byte("import { JSON, Json as LegacyJson } from vendor/json\nactivate trb/platform/ruby/native\n"))
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", diagnostics)
	}
	imported, ok := program.Statements[0].(*ast.ImportStatement)
	if !ok || len(imported.Symbols) != 2 || imported.Symbols[0] != "JSON" || imported.SymbolAliases["Json"] != "LegacyJson" {
		t.Fatalf("named import = %#v", program.Statements[0])
	}
	activated, ok := program.Statements[1].(*ast.ActivateStatement)
	if !ok || activated.Path != "trb/platform/ruby/native" {
		t.Fatalf("activation = %#v", program.Statements[1])
	}
}

func TestActivateRemainsContextualOutsideTheTopLevelCommandForm(t *testing.T) {
	program, diagnostics := Parse([]byte("activate(user)\ndef run()\n\tactivate user\nend\n"))
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", diagnostics)
	}
	if _, activation := program.Statements[0].(*ast.ActivateStatement); activation {
		t.Fatal("parenthesized call parsed as activation")
	}
	method := program.Statements[1].(*ast.MethodStatement)
	if _, activation := method.Body[0].(*ast.ActivateStatement); activation {
		t.Fatal("method-body call parsed as activation")
	}
}

func TestParseRejectsEmptyNamedImport(t *testing.T) {
	_, diagnostics := Parse([]byte("import {} from trb/std/json\n"))
	if len(diagnostics) == 0 || diagnostics[0].Message != "named import requires at least one declaration" {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}
