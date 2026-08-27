package codegen_test

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/callsignature"
	golang "github.com/type-rb/type-rb/internal/codegen/golang"
	ruby "github.com/type-rb/type-rb/internal/codegen/ruby"
	typescript "github.com/type-rb/type-rb/internal/codegen/typescript"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

func TestTopLevelEffectfulRecordConstructionUsesRootExecutionScope(t *testing.T) {
	tests := []struct {
		name     string
		generate func(*ir.Program) string
		want     string
	}{
		{name: "go", generate: golang.Generate, want: "Trb__RecordNew__Config(trbcontext.Background()"},
		{name: "ruby", generate: ruby.Generate, want: "Config.__trb_record_new(TrbExecutionScope.root"},
		{name: "typescript", generate: typescript.Generate, want: "__trbRecordNewConfig(undefined"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := test.generate(effectfulRecordProgram(test.name))
			if !strings.Contains(output, test.want) {
				t.Fatalf("generated %s is missing %q:\n%s", test.name, test.want, output)
			}
			if test.name == "ruby" && !strings.Contains(output, "class TrbExecutionScope") {
				t.Fatalf("generated Ruby is missing execution-scope runtime:\n%s", output)
			}
		})
	}
}

func TestEffectfulParameterDefaultsUseTheCalleeExecutionScope(t *testing.T) {
	tests := []struct {
		name     string
		generate func(*ir.Program) string
		want     []string
	}{
		{name: "go", generate: golang.Generate, want: []string{"func Load(__trbScope trbcontext.Context", "Trb__RecordNew__Config(__trbScope", "Load(trbcontext.Background())"}},
		{name: "ruby", generate: ruby.Generate, want: []string{"def load(__trb_scope", "Config.__trb_record_new(__trb_scope", "load.call(TrbExecutionScope.root)"}},
		{name: "typescript", generate: typescript.Generate, want: []string{"export async function load(__trbScope: AbortSignal | undefined, __trbOptional: unknown[])", "(await __trbRecordNewConfig(__trbScope", "(await load(undefined, []))"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := test.generate(effectfulParameterDefaultProgram(test.name))
			for _, want := range test.want {
				if !strings.Contains(output, want) {
					t.Fatalf("generated %s is missing %q:\n%s", test.name, want, output)
				}
			}
		})
	}
}

func TestNestedInitializersUseTheirDeclarationExecutionScopes(t *testing.T) {
	tests := []struct {
		name     string
		generate func(*ir.Program) string
		root     string
		class    []string
	}{
		{name: "go", generate: golang.Generate, root: "Trb__RecordNew__Config(trbcontext.Background()", class: []string{
			"func NewHolder(__trbScope trbcontext.Context)", "Trb__RecordNew__Config(__trbScope",
		}},
		{name: "ruby", generate: ruby.Generate, root: "Config.__trb_record_new(TrbExecutionScope.root", class: []string{
			"def initialize(__trb_scope, ...)", "Config.__trb_record_new(__trb_scope",
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := test.generate(effectfulNestedInitializerProgram(test.name))
			if count := strings.Count(output, test.root); count != 1 {
				t.Fatalf("generated %s root-scope initializer count=%d, want 1:\n%s", test.name, count, output)
			}
			for _, want := range test.class {
				if !strings.Contains(output, want) {
					t.Fatalf("generated %s class initializer is missing %q:\n%s", test.name, want, output)
				}
			}
		})
	}
}

func effectfulRecordProgram(mode string) *ir.Program {
	stringType := types.FromName("String")
	recordType := types.FromName("Config")
	functionType := types.Type{Kind: types.Function, Args: []types.Type{stringType}}
	symbol := "load"
	if mode == "go" {
		symbol = "Load"
	}
	load := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, stringType),
		Callee: &ir.Identifier{
			ExprBase: ir.NewExprBase(token.Span{}, functionType),
			Name:     symbol,
			Reference: &ir.Reference{Runtime: &ir.RuntimeBinding{
				Identity: "example.com/runtime#load", Dependency: "example.com/runtime",
				Module: "example.com/runtime", Symbol: symbol, MaySuspend: true, PropagatesExecutionScope: true,
			}},
		},
	}
	record := &ir.Record{Name: "Config", Body: []ir.Statement{
		&ir.RecordField{Name: "value", Type: stringType, Default: load},
	}}
	constructor := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, recordType),
		Callee: &ir.Member{
			ExprBase: ir.NewExprBase(token.Span{}, functionType),
			Receiver: &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, recordType), Name: "Config"},
			Name:     "new",
		},
		RecordFields: []ir.RecordFieldContract{{Name: "value", Type: stringType, HasDefault: true}},
	}
	return &ir.Program{
		Mode: mode, ModulePath: "main", Package: "main", GoModule: "example.com/application",
		Statements: []ir.Statement{record, &ir.Variable{Name: "CONFIG", Type: recordType, Value: constructor, Constant: true}},
	}
}

func effectfulParameterDefaultProgram(mode string) *ir.Program {
	program := effectfulRecordProgram(mode)
	recordType := types.FromName("Config")
	constructor := program.Statements[1].(*ir.Variable).Value
	method := &ir.Method{
		Name: "load", Parameters: []ir.Parameter{{Name: "config", Type: recordType, Default: constructor}}, ReturnType: recordType,
		Body: []ir.Statement{&ir.Return{Value: &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, recordType), Name: "config", Lexical: true}}},
	}
	call := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, recordType),
		Callee: &ir.Identifier{
			ExprBase: ir.NewExprBase(token.Span{}, types.Type{Kind: types.Function, Args: []types.Type{recordType, recordType}}),
			Name:     "load",
		},
		CallSignature: []callsignature.Parameter{{Kind: callsignature.Positional, Type: recordType, Presence: callsignature.Omittable}},
	}
	program.Statements = []ir.Statement{
		program.Statements[0], method, &ir.Variable{Name: "CONFIG", Type: recordType, Value: call, Constant: true},
	}
	return program
}

func effectfulNestedInitializerProgram(mode string) *ir.Program {
	program := effectfulRecordProgram(mode)
	recordType := types.FromName("Config")
	constructor := program.Statements[1].(*ir.Variable).Value
	program.Statements = []ir.Statement{
		program.Statements[0],
		&ir.Module{Name: "Settings", Body: []ir.Statement{
			&ir.Variable{Name: "CONFIG", Type: recordType, Value: constructor, Constant: true},
		}},
		&ir.Class{Name: "Holder", Body: []ir.Statement{
			&ir.Field{Name: "@config", Type: recordType, Value: constructor},
		}},
	}
	return program
}
