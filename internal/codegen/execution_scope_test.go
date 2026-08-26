package codegen_test

import (
	"strings"
	"testing"

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
		{name: "go", generate: golang.Generate, want: "TrbRecordNewConfig(trbcontext.Background()"},
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
