package effectplan

import (
	"testing"

	"github.com/type-rb/type-rb/internal/callsignature"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

func TestEffectsIncludeParameterDefaultsAndNestedInitializers(t *testing.T) {
	stringType := types.FromName("String")
	configType := types.FromName("Config")
	functionType := types.Type{Kind: types.Function, Args: []types.Type{stringType}}
	loadDefault := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, stringType),
		Callee: &ir.Identifier{
			ExprBase: ir.NewExprBase(token.Span{}, functionType), Name: "load_default",
			Reference: &ir.Reference{Runtime: &ir.RuntimeBinding{
				Identity: "example.com/runtime#load", PropagatesExecutionScope: true,
			}},
		},
	}
	record := &ir.Record{Name: "Config", Body: []ir.Statement{
		&ir.RecordField{Name: "value", Type: stringType, Default: loadDefault},
	}}
	newConfig := func() *ir.Call {
		return &ir.Call{
			ExprBase: ir.NewExprBase(token.Span{}, configType),
			Callee: &ir.Member{
				ExprBase: ir.NewExprBase(token.Span{}, functionType),
				Receiver: &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, configType), Name: "Config"},
				Name:     "new",
			},
			RecordFields: []ir.RecordFieldContract{{Name: "value", Type: stringType, HasDefault: true}},
		}
	}
	parameterDefault := newConfig()
	moduleInitializer := newConfig()
	nestedModuleInitializer := newConfig()
	classInitializer := newConfig()
	method := &ir.Method{
		Name: "load", Parameters: []ir.Parameter{{Name: "config", Type: configType, Default: parameterDefault}}, ReturnType: configType,
		Body: []ir.Statement{&ir.Return{Value: &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, configType), Name: "config", Lexical: true}}},
	}
	loadCall := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, configType),
		Callee:   &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, types.Type{Kind: types.Function, Args: []types.Type{configType, configType}}), Name: "load"},
		CallSignature: []callsignature.Parameter{{
			Kind: callsignature.Positional, Type: configType, Presence: callsignature.Omittable,
		}},
	}
	program := &ir.Program{ModulePath: "main", Statements: []ir.Statement{
		record,
		method,
		&ir.Variable{Name: "LOADED", Type: configType, Value: loadCall, Constant: true},
		&ir.Module{Name: "Settings", Body: []ir.Statement{
			&ir.Variable{Name: "CONFIG", Type: configType, Value: moduleInitializer, Constant: true},
			&ir.Module{Name: "Nested", Body: []ir.Statement{
				&ir.Variable{Name: "CONFIG", Type: configType, Value: nestedModuleInitializer, Constant: true},
			}},
		}},
		&ir.Class{Name: "Holder", Body: []ir.Statement{
			&ir.Field{Name: "@config", Type: configType, Value: classInitializer},
		}},
	}}

	plan := Analyze([]*ir.Program{program}, Options{Runtime: func(binding *ir.RuntimeBinding) bool {
		return binding.PropagatesExecutionScope
	}})
	if !plan.Methods[method] || !plan.ParameterDefaults[method] {
		t.Fatalf("parameter default did not mark its method: %#v", plan)
	}
	for name, call := range map[string]*ir.Call{
		"parameter default":         parameterDefault,
		"module initializer":        moduleInitializer,
		"nested module initializer": nestedModuleInitializer,
		"class initializer":         classInitializer,
	} {
		if !plan.Calls[call] || !plan.Expressions[call] {
			t.Errorf("%s was not recorded as effectful", name)
		}
	}
	if !plan.Calls[loadCall] || !plan.CallParameterDefaults[loadCall] {
		t.Fatalf("call of method with an effectful default was not recorded")
	}
}
