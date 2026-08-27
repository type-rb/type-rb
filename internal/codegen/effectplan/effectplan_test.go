package effectplan

import (
	"strings"
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
	holderType := types.FromName("Holder")
	holderConstruction := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, holderType),
		Callee: &ir.Member{
			ExprBase: ir.NewExprBase(token.Span{}, functionType),
			Receiver: &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, holderType), Name: "Holder"},
			Name:     "new",
		},
	}
	holder := &ir.Class{Name: "Holder", Body: []ir.Statement{
		&ir.Field{Name: "@config", Type: configType, Value: classInitializer},
	}}
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
		holder,
		&ir.ExpressionStatement{Expression: holderConstruction},
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
	if !plan.ClassConstructors[holder] || !plan.Calls[holderConstruction] {
		t.Fatal("class field default did not mark the constructor and its call")
	}
}

func TestRecordDefaultsTreatEarlierFunctionFieldsAsSuspendingCandidates(t *testing.T) {
	integer := types.FromName("Integer")
	callback := types.FunctionOf(nil, integer)
	loader := &ir.RecordField{Name: "loader", Type: callback}
	loadCall := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, integer),
		Callee:   &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, callback), Name: "loader", Lexical: true},
	}
	value := &ir.RecordField{Name: "value", Type: integer, Default: loadCall}
	record := &ir.Record{Name: "Config", Body: []ir.Statement{loader, value}}

	plan := Analyze([]*ir.Program{{ModulePath: "main", Statements: []ir.Statement{record}}}, Options{PassToFunctions: true})
	if !plan.RecordDefaultFor(record) || !plan.RecordFieldDefaultFor(value) || !plan.Calls[loadCall] {
		t.Fatalf("function-valued earlier field was not treated as a suspending candidate: %#v", plan)
	}
	if plan.RecordFieldDefaultFor(loader) {
		t.Fatal("required function field was recorded as an effectful default")
	}
}

func TestRecordConstructionEffectsUseOnlyOmittedDefaults(t *testing.T) {
	integer := types.FromName("Integer")
	function := types.FunctionOf(nil, integer)
	effectCall := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, integer),
		Callee: &ir.Identifier{
			ExprBase: ir.NewExprBase(token.Span{}, function), Name: "load_value",
			Reference: &ir.Reference{Runtime: &ir.RuntimeBinding{
				Identity: "example.com/runtime#load", PropagatesExecutionScope: true,
			}},
		},
	}
	pure := &ir.RecordField{Name: "pure", Type: integer, Default: &ir.Literal{
		ExprBase: ir.NewExprBase(token.Span{}, integer), Kind: "integer", Raw: "1",
	}}
	effectful := &ir.RecordField{Name: "effectful", Type: integer, Default: effectCall}
	record := &ir.Record{Name: "Config", Body: []ir.Statement{pure, effectful}}
	construct := func(arguments ...ir.CallArgument) *ir.Call {
		return &ir.Call{
			ExprBase: ir.NewExprBase(token.Span{}, types.FromName("Config")),
			Callee: &ir.Member{
				ExprBase: ir.NewExprBase(token.Span{}, function),
				Receiver: &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, types.FromName("Config")), Name: "Config"},
				Name:     "new",
			},
			Arguments: arguments,
			RecordFields: []ir.RecordFieldContract{
				{Name: "pure", Type: integer, HasDefault: true},
				{Name: "effectful", Type: integer, HasDefault: true},
			},
		}
	}
	omittedEffect := construct()
	effectExplicit := construct(ir.CallArgument{Name: "effectful", Value: &ir.Literal{
		ExprBase: ir.NewExprBase(token.Span{}, integer), Kind: "integer", Raw: "2",
	}})
	allExplicit := construct(
		ir.CallArgument{Name: "effectful", Value: &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, integer), Kind: "integer", Raw: "2"}},
		ir.CallArgument{Name: "pure", Value: &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, integer), Kind: "integer", Raw: "3"}},
	)
	program := &ir.Program{ModulePath: "main", Statements: []ir.Statement{
		record,
		&ir.ExpressionStatement{Expression: omittedEffect},
		&ir.ExpressionStatement{Expression: effectExplicit},
		&ir.ExpressionStatement{Expression: allExplicit},
	}}

	plan := Analyze([]*ir.Program{program}, Options{Runtime: func(binding *ir.RuntimeBinding) bool {
		return binding.PropagatesExecutionScope
	}})
	if !plan.RecordFieldDefaultFor(effectful) || plan.RecordFieldDefaultFor(pure) {
		t.Fatalf("record field effects were not kept separate: %#v", plan.RecordFieldDefaults)
	}
	if !plan.Calls[omittedEffect] || !plan.RecordCallDefaults[omittedEffect] || plan.RecordCallSync[omittedEffect] {
		t.Fatal("omitting the effectful default did not mark the construction")
	}
	for name, call := range map[string]*ir.Call{"effect explicit": effectExplicit, "all explicit": allExplicit} {
		if plan.Calls[call] || plan.RecordCallDefaults[call] || !plan.RecordCallSync[call] {
			t.Errorf("%s did not retain a synchronous construction path", name)
		}
	}
}

func TestNestedEnumCallUsesItsExactDeclarationOwner(t *testing.T) {
	voidType := types.Type{Kind: types.Void, Name: "Void"}
	functionType := types.Type{Kind: types.Function, Args: []types.Type{voidType}}
	effectRoot := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, voidType),
		Callee: &ir.Identifier{
			ExprBase: ir.NewExprBase(token.Span{}, functionType), Name: "effect_root",
			Reference: &ir.Reference{Runtime: &ir.RuntimeBinding{
				Identity: "example.com/runtime#effect", PropagatesExecutionScope: true,
			}},
		},
	}
	effectful := &ir.Method{Name: "effectful", ReturnType: voidType, Body: []ir.Statement{
		&ir.ExpressionStatement{Expression: effectRoot},
	}}
	forwardCall := &ir.EnumCall{
		ExprBase: ir.NewExprBase(token.Span{}, voidType), EnumName: "Status", Owner: "Services::Status", Method: "effectful",
		Receiver: &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, types.FromName("Status")), Name: "self", Lexical: true},
	}
	forwarded := &ir.Method{Name: "forwarded", ReturnType: voidType, Body: []ir.Statement{
		&ir.ExpressionStatement{Expression: forwardCall},
	}}
	program := &ir.Program{ModulePath: "main", Statements: []ir.Statement{
		&ir.Module{Name: "Services", Body: []ir.Statement{
			&ir.Enum{Name: "Status", Body: []ir.Statement{effectful, forwarded}},
		}},
	}}

	plan := Analyze([]*ir.Program{program}, Options{Runtime: func(binding *ir.RuntimeBinding) bool {
		return binding.PropagatesExecutionScope
	}})
	if !plan.Methods[effectful] || !plan.Methods[forwarded] || !plan.EnumCalls[forwardCall] {
		t.Fatalf("nested enum call did not retain its exact effect owner: %#v", plan)
	}
}

func TestInheritedAndOverriddenMethodsShareEffectABIs(t *testing.T) {
	stringType := types.FromName("String")
	functionType := types.Type{Kind: types.Function, Args: []types.Type{stringType, stringType}}
	effectDefault := func() ir.Expression {
		return &ir.Call{
			ExprBase: ir.NewExprBase(token.Span{}, stringType),
			Callee: &ir.Identifier{
				ExprBase: ir.NewExprBase(token.Span{}, functionType), Name: "load_default",
				Reference: &ir.Reference{Runtime: &ir.RuntimeBinding{
					Identity: "example.com/runtime#load", PropagatesExecutionScope: true,
				}},
			},
		}
	}
	pureDefault := func() ir.Expression {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, stringType), Raw: `"pure"`}
	}
	method := func(defaultValue ir.Expression) *ir.Method {
		return &ir.Method{
			Name: "load", Parameters: []ir.Parameter{{Name: "value", Type: stringType, Default: defaultValue}}, ReturnType: stringType,
			Body: []ir.Statement{&ir.Return{Value: &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, stringType), Name: "value", Lexical: true}}},
		}
	}
	superclass := func(name string) ir.Expression {
		return &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, types.Type{}), Name: name}
	}
	call := func(owner string) *ir.Call {
		return &ir.Call{
			ExprBase: ir.NewExprBase(token.Span{}, stringType),
			Callee: &ir.Member{
				ExprBase: ir.NewExprBase(token.Span{}, functionType),
				Receiver: &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, types.FromName(owner)), Name: strings.ToLower(owner), Lexical: true},
				Name:     "load",
			},
			CallSignature: []callsignature.Parameter{{Kind: callsignature.Positional, Type: stringType, Presence: callsignature.Omittable}},
		}
	}

	inheritedBase := method(effectDefault())
	inheritedCall := call("InheritedChild")
	pureBase := method(pureDefault())
	effectOverride := method(effectDefault())
	pureBaseCall := call("PureBase")
	effectChildCall := call("EffectChild")
	effectBase := method(effectDefault())
	pureOverride := method(pureDefault())
	pureChildCall := call("PureChild")
	program := &ir.Program{ModulePath: "main", Statements: []ir.Statement{
		&ir.Class{Name: "InheritedBase", Body: []ir.Statement{inheritedBase}},
		&ir.Class{Name: "InheritedChild", Superclass: superclass("InheritedBase")},
		&ir.Class{Name: "PureBase", Body: []ir.Statement{pureBase}},
		&ir.Class{Name: "EffectChild", Superclass: superclass("PureBase"), Body: []ir.Statement{effectOverride}},
		&ir.Class{Name: "EffectBase", Body: []ir.Statement{effectBase}},
		&ir.Class{Name: "PureChild", Superclass: superclass("EffectBase"), Body: []ir.Statement{pureOverride}},
		&ir.ExpressionStatement{Expression: inheritedCall},
		&ir.ExpressionStatement{Expression: pureBaseCall},
		&ir.ExpressionStatement{Expression: effectChildCall},
		&ir.ExpressionStatement{Expression: pureChildCall},
	}}

	plan := Analyze([]*ir.Program{program}, Options{Runtime: func(binding *ir.RuntimeBinding) bool {
		return binding.PropagatesExecutionScope
	}})
	for name, candidate := range map[string]*ir.Method{
		"inherited base":  inheritedBase,
		"pure base":       pureBase,
		"effect override": effectOverride,
		"effect base":     effectBase,
		"pure override":   pureOverride,
	} {
		if !plan.Methods[candidate] || !plan.ParameterDefaults[candidate] {
			t.Errorf("%s did not receive the shared effect ABI", name)
		}
	}
	for name, candidate := range map[string]*ir.Call{
		"inherited call":     inheritedCall,
		"pure base call":     pureBaseCall,
		"effect child call":  effectChildCall,
		"pure override call": pureChildCall,
	} {
		if !plan.Calls[candidate] || !plan.CallParameterDefaults[candidate] {
			t.Errorf("%s did not use the shared effect ABI", name)
		}
	}
}

func TestClassConstructorsUseTheirOwnModuleQualifiedInitializerEffects(t *testing.T) {
	integerType := types.FromName("Integer")
	functionType := types.Type{Kind: types.Function, Args: []types.Type{integerType}}
	effectCall := func() *ir.Call {
		return &ir.Call{
			ExprBase: ir.NewExprBase(token.Span{}, integerType),
			Callee: &ir.Identifier{
				ExprBase: ir.NewExprBase(token.Span{}, functionType), Name: "load_value",
				Reference: &ir.Reference{Runtime: &ir.RuntimeBinding{
					Identity: "example.com/runtime#load", PropagatesExecutionScope: true,
				}},
			},
		}
	}
	pureDefault := func() ir.Expression {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, integerType), Raw: "1"}
	}
	initializer := func(defaultValue ir.Expression, body []ir.Statement) *ir.Method {
		parameters := []ir.Parameter{}
		if defaultValue != nil {
			parameters = append(parameters, ir.Parameter{Name: "value", Type: integerType, Default: defaultValue})
		}
		return &ir.Method{Name: "initialize", Parameters: parameters, Body: body}
	}
	constructorCall := func(module, name string, optional bool) *ir.Call {
		classType := types.FromName(name)
		call := &ir.Call{
			ExprBase: ir.NewExprBase(token.Span{}, classType),
			Callee: &ir.Member{
				ExprBase: ir.NewExprBase(token.Span{}, functionType),
				Receiver: &ir.Identifier{
					ExprBase: ir.NewExprBase(token.Span{}, classType), Name: name,
					Reference: &ir.Reference{Package: module, Symbol: name},
				},
				Name: "new",
			},
		}
		if optional {
			call.CallSignature = []callsignature.Parameter{{
				Kind: callsignature.Positional, Type: integerType, Presence: callsignature.Omittable,
			}}
		}
		return call
	}

	effectDefaultInitializer := initializer(effectCall(), nil)
	pureInitializer := initializer(pureDefault(), nil)
	effectBodyInitializer := initializer(nil, []ir.Statement{
		&ir.ExpressionStatement{Expression: effectCall()},
	})
	effectParentInitializer := initializer(effectCall(), nil)
	pureChildInitializer := initializer(pureDefault(), nil)
	effectDefaultCall := constructorCall("models/effect", "Box", true)
	pureCall := constructorCall("models/pure", "Box", true)
	effectBodyCall := constructorCall("models/body", "BodyBox", false)
	pureChildCall := constructorCall("main", "PureChild", true)
	programs := []*ir.Program{
		{ModulePath: "models/effect", Statements: []ir.Statement{
			&ir.Class{Name: "Box", Body: []ir.Statement{effectDefaultInitializer}},
		}},
		{ModulePath: "models/pure", Statements: []ir.Statement{
			&ir.Class{Name: "Box", Body: []ir.Statement{pureInitializer}},
		}},
		{ModulePath: "models/body", Statements: []ir.Statement{
			&ir.Class{Name: "BodyBox", Body: []ir.Statement{effectBodyInitializer}},
		}},
		{ModulePath: "main", Statements: []ir.Statement{
			&ir.Class{Name: "EffectParent", Body: []ir.Statement{effectParentInitializer}},
			&ir.Class{
				Name: "PureChild",
				Superclass: &ir.Identifier{
					ExprBase: ir.NewExprBase(token.Span{}, types.Type{}), Name: "EffectParent",
				},
				Body: []ir.Statement{pureChildInitializer},
			},
			&ir.ExpressionStatement{Expression: effectDefaultCall},
			&ir.ExpressionStatement{Expression: pureCall},
			&ir.ExpressionStatement{Expression: effectBodyCall},
			&ir.ExpressionStatement{Expression: pureChildCall},
		}},
	}

	plan := Analyze(programs, Options{Runtime: func(binding *ir.RuntimeBinding) bool {
		return binding.PropagatesExecutionScope
	}})
	if !plan.Methods[effectDefaultInitializer] || !plan.ParameterDefaults[effectDefaultInitializer] {
		t.Fatal("effectful constructor default did not mark its initializer ABI")
	}
	if plan.Methods[pureInitializer] || plan.ParameterDefaults[pureInitializer] {
		t.Fatal("same-named class in another module inherited the effectful constructor ABI")
	}
	if !plan.Methods[effectBodyInitializer] || plan.ParameterDefaults[effectBodyInitializer] {
		t.Fatal("effectful constructor body did not mark only its execution-scope ABI")
	}
	if !plan.Calls[effectDefaultCall] || !plan.CallParameterDefaults[effectDefaultCall] {
		t.Fatal("imported constructor call did not use its effectful default ABI")
	}
	if plan.Calls[pureCall] || plan.CallParameterDefaults[pureCall] {
		t.Fatal("imported pure constructor call used another module's effect ABI")
	}
	if !plan.Calls[effectBodyCall] || plan.CallParameterDefaults[effectBodyCall] {
		t.Fatal("imported constructor call did not use only its effectful body ABI")
	}
	if !plan.Methods[effectParentInitializer] || !plan.ParameterDefaults[effectParentInitializer] {
		t.Fatal("effectful parent initializer was not analyzed")
	}
	if plan.Methods[pureChildInitializer] || plan.ParameterDefaults[pureChildInitializer] || plan.Calls[pureChildCall] || plan.CallParameterDefaults[pureChildCall] {
		t.Fatal("effectful parent initializer propagated into the child's own constructor")
	}
}

func TestTransparentAliasCanonicalizationIsCycleSafe(t *testing.T) {
	program := &ir.Program{ModulePath: "contracts", Statements: []ir.Statement{
		&ir.TypeAlias{Name: "First", Target: types.FromName("Second")},
		&ir.TypeAlias{Name: "Second", Target: types.FromName("First")},
		&ir.Interface{Name: "Worker", Methods: []*ir.Method{{Name: "values"}}},
		&ir.Class{Name: "Implementation", Implements: []types.Type{types.FromName("First")}},
	}}

	plan := Analyze([]*ir.Program{program}, Options{})
	if plan == nil {
		t.Fatal("Analyze returned a nil plan for a cyclic hand-built alias graph")
	}
}

func TestNestedNamespacesKeepSameLeafMethodEffectsSeparate(t *testing.T) {
	integer := types.FromName("Integer")
	effectCall := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, integer),
		Callee: &ir.Identifier{
			ExprBase: ir.NewExprBase(token.Span{}, types.Type{Kind: types.Function, Args: []types.Type{integer}}),
			Name:     "load_value",
			Reference: &ir.Reference{Runtime: &ir.RuntimeBinding{
				Identity: "example.com/runtime#load", PropagatesExecutionScope: true,
			}},
		},
	}
	effectful := &ir.Method{Name: "value", ReturnType: integer, Body: []ir.Statement{&ir.Return{Value: effectCall}}}
	pure := &ir.Method{Name: "value", ReturnType: integer, Body: []ir.Statement{
		&ir.Return{Value: &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, integer), Kind: "integer", Raw: "1"}},
	}}
	program := &ir.Program{ModulePath: "main", Statements: []ir.Statement{
		&ir.Module{Name: "Left", Body: []ir.Statement{&ir.Class{Name: "Worker", Body: []ir.Statement{effectful}}}},
		&ir.Module{Name: "Right", Body: []ir.Statement{&ir.Class{Name: "Worker", Body: []ir.Statement{pure}}}},
	}}

	plan := Analyze([]*ir.Program{program}, Options{Runtime: func(binding *ir.RuntimeBinding) bool {
		return binding.PropagatesExecutionScope
	}})
	if !plan.Methods[effectful] {
		t.Fatal("effectful nested method was not recorded")
	}
	if plan.Methods[pure] {
		t.Fatal("same-leaf method in another nested namespace inherited the effect ABI")
	}
}

func TestNestedNamespacesKeepSameLeafRecordDefaultsSeparate(t *testing.T) {
	integer := types.FromName("Integer")
	effectCall := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, integer),
		Callee: &ir.Identifier{
			ExprBase: ir.NewExprBase(token.Span{}, types.Type{Kind: types.Function, Args: []types.Type{integer}}),
			Name:     "load_value",
			Reference: &ir.Reference{Runtime: &ir.RuntimeBinding{
				Identity: "example.com/runtime#load", PropagatesExecutionScope: true,
			}},
		},
	}
	left := &ir.Record{Name: "Entry", Body: []ir.Statement{
		&ir.RecordField{Name: "value", Type: integer, Default: effectCall},
	}}
	right := &ir.Record{Name: "Entry", Body: []ir.Statement{
		&ir.RecordField{Name: "value", Type: integer, Default: &ir.Literal{
			ExprBase: ir.NewExprBase(token.Span{}, integer), Kind: "integer", Raw: "1",
		}},
	}}
	program := &ir.Program{ModulePath: "main", Statements: []ir.Statement{
		&ir.Module{Name: "Left", Body: []ir.Statement{left}},
		&ir.Module{Name: "Right", Body: []ir.Statement{right}},
	}}

	plan := Analyze([]*ir.Program{program}, Options{Runtime: func(binding *ir.RuntimeBinding) bool {
		return binding.PropagatesExecutionScope
	}})
	if !plan.RecordDefaultFor(left) || !plan.RecordDefault("main", "Left::Entry") {
		t.Fatal("effectful nested record default was not recorded under its qualified identity")
	}
	if plan.RecordDefaultFor(right) || plan.RecordDefault("main", "Right::Entry") {
		t.Fatal("same-leaf record in another nested namespace inherited the effect ABI")
	}
}

func TestModuleInitializerKeepsTopLevelUnqualifiedFunctionIdentity(t *testing.T) {
	integer := types.FromName("Integer")
	function := types.FunctionOf(nil, integer)
	effectCall := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, integer),
		Callee: &ir.Identifier{
			ExprBase: ir.NewExprBase(token.Span{}, function), Name: "operation",
			Reference: &ir.Reference{Runtime: &ir.RuntimeBinding{
				Identity: "example.com/runtime#load", PropagatesExecutionScope: true,
			}},
		},
	}
	pure := &ir.Method{Name: "value", ReturnType: integer}
	moduleMethod := &ir.Method{Name: "value", ReturnType: integer, Body: []ir.Statement{
		&ir.Return{Value: effectCall},
	}}
	initializerCall := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, integer),
		Callee:   &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, function), Name: "value"},
	}
	program := &ir.Program{ModulePath: "main", Statements: []ir.Statement{
		pure,
		&ir.Module{Name: "Settings", Body: []ir.Statement{
			moduleMethod,
			&ir.Variable{Name: "VALUE", Type: integer, Value: initializerCall, Constant: true},
		}},
	}}

	plan := Analyze([]*ir.Program{program}, Options{Runtime: func(binding *ir.RuntimeBinding) bool {
		return binding.PropagatesExecutionScope
	}})
	if !plan.Methods[moduleMethod] {
		t.Fatal("effectful module method was not recorded")
	}
	if plan.Methods[pure] || plan.Calls[initializerCall] {
		t.Fatal("module initializer confused an unqualified top-level call with a same-named module method")
	}
}
