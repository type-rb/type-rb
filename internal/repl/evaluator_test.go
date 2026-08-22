package repl

import (
	"bytes"
	"context"
	"errors"
	"math"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

func TestEvaluateFloatClassificationIntrinsics(t *testing.T) {
	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	floatType := types.FromName("Float")
	booleanType := types.FromName("Boolean")
	tests := []struct {
		name  string
		value float64
		want  bool
	}{
		{name: "trb.std.numbers.float_finite", value: 0.25, want: true},
		{name: "trb.std.numbers.float_finite", value: math.Inf(1), want: false},
		{name: "trb.std.numbers.float_infinite", value: math.Inf(-1), want: true},
		{name: "trb.std.numbers.float_nan", value: math.NaN(), want: true},
	}
	for _, test := range tests {
		result, err := evaluator.intrinsic(test.name, []evaluatedArgument{{Value: Value{Type: floatType, Data: test.value}}}, booleanType, nil)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if got, ok := result.Data.(bool); !ok || got != test.want {
			t.Fatalf("%s(%v)=%#v, want %t", test.name, test.value, result.Data, test.want)
		}
	}
}

func TestEvaluateFirstClassFunctionClosure(t *testing.T) {
	integer := types.FromName("Integer")
	functionType := types.FunctionOf([]types.Type{integer}, integer)
	span := token.Span{}
	lambda := &ir.Lambda{
		ExprBase:   ir.NewExprBase(span, functionType),
		Parameters: []ir.Parameter{{Name: "value", Type: integer}},
		ReturnType: integer,
		Body: []ir.Statement{&ir.Return{Value: &ir.Binary{
			ExprBase: ir.NewExprBase(span, integer),
			Left:     &ir.Identifier{ExprBase: ir.NewExprBase(span, integer), Name: "value", Lexical: true},
			Operator: "+",
			Right:    &ir.Identifier{ExprBase: ir.NewExprBase(span, integer), Name: "captured", Lexical: true},
		}}},
	}
	statements := []ir.Statement{
		&ir.Variable{Name: "captured", Type: integer, Value: &ir.Literal{ExprBase: ir.NewExprBase(span, integer), Kind: "integer", Raw: "2"}},
		&ir.Variable{Name: "add", Type: functionType, Value: lambda},
		&ir.ExpressionStatement{Expression: &ir.Call{
			ExprBase:  ir.NewExprBase(span, integer),
			Callee:    &ir.Identifier{ExprBase: ir.NewExprBase(span, functionType), Name: "add", Lexical: true},
			Arguments: []ir.CallArgument{{Value: &ir.Literal{ExprBase: ir.NewExprBase(span, integer), Kind: "integer", Raw: "3"}}},
		}},
	}
	result, err := NewEvaluator(&bytes.Buffer{}, "go").Evaluate(statements, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if result.Value.Data != int64(5) || Inspect(result.Value) != "5" {
		t.Fatalf("result=%#v", result)
	}
}

func TestEvaluateInternalRuntimeFailure(t *testing.T) {
	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	_, err := evaluator.intrinsic(
		"trb.internal.runtime.fail",
		[]evaluatedArgument{{Value: Value{Type: types.FromName("String"), Data: "stopped"}}},
		types.FromName("Response"),
		nil,
	)
	if err == nil || err.Error() != "stopped" {
		t.Fatalf("unexpected runtime failure: %v", err)
	}
}

func TestEvaluatePortableNumericAndMathIntrinsics(t *testing.T) {
	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	integerType := types.FromName("Integer")
	floatType := types.FromName("Float")
	integer := func(value int64) evaluatedArgument {
		return evaluatedArgument{Value: Value{Type: integerType, Data: value}}
	}
	floating := func(value float64) evaluatedArgument {
		return evaluatedArgument{Value: Value{Type: floatType, Data: value}}
	}

	for _, test := range []struct {
		name string
		args []evaluatedArgument
		want int64
	}{
		{name: "trb.std.numbers.integer_min", args: []evaluatedArgument{integer(5), integer(3)}, want: 3},
		{name: "trb.std.numbers.integer_max", args: []evaluatedArgument{integer(5), integer(7)}, want: 7},
		{name: "trb.std.numbers.integer_clamp", args: []evaluatedArgument{integer(12), integer(0), integer(10)}, want: 10},
		{name: "trb.std.numbers.float_floor", args: []evaluatedArgument{floating(-2.75)}, want: -3},
		{name: "trb.std.numbers.float_ceil", args: []evaluatedArgument{floating(-2.75)}, want: -2},
		{name: "trb.std.numbers.float_round", args: []evaluatedArgument{floating(-2.5)}, want: -3},
	} {
		result, err := evaluator.intrinsic(test.name, test.args, integerType, nil)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if got, ok := result.Data.(int64); !ok || got != test.want {
			t.Fatalf("%s result=%#v, want %d", test.name, result.Data, test.want)
		}
	}

	for _, test := range []struct {
		name  string
		value float64
		want  float64
	}{
		{name: "trb.std.math.sqrt", value: 9, want: 3},
		{name: "trb.std.math.exp", value: 0, want: 1},
		{name: "trb.std.math.log", value: 1, want: 0},
		{name: "trb.std.math.log2", value: 8, want: 3},
		{name: "trb.std.math.log10", value: 100, want: 2},
	} {
		result, err := evaluator.intrinsic(test.name, []evaluatedArgument{floating(test.value)}, floatType, nil)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if got, ok := result.Data.(float64); !ok || got != test.want {
			t.Fatalf("%s(%v)=%#v, want %v", test.name, test.value, result.Data, test.want)
		}
	}

	if _, err := evaluator.intrinsic("trb.std.numbers.integer_clamp", []evaluatedArgument{integer(1), integer(2), integer(0)}, integerType, nil); err == nil || err.Error() != "clamp minimum exceeds maximum" {
		t.Fatalf("unexpected clamp error: %v", err)
	}
	if _, err := evaluator.intrinsic("trb.std.numbers.float_floor", []evaluatedArgument{floating(math.Inf(1))}, integerType, nil); err == nil || err.Error() != "Float cannot be converted to Integer" {
		t.Fatalf("unexpected non-finite floor error: %v", err)
	}
}

func TestEvaluatePortableURLComponentHelpers(t *testing.T) {
	if got, want := encodeURLComponent("a b/😀+~"), "a%20b%2F%F0%9F%98%80%2B~"; got != want {
		t.Fatalf("encodeURLComponent()=%q, want %q", got, want)
	}
	for _, test := range []struct {
		input       string
		want        string
		wantKind    string
		wantMessage string
	}{
		{input: "a%20b%2F%F0%9F%98%80%2B~", want: "a b/😀+~"},
		{input: "a+b", want: "a+b"},
		{input: "%", wantKind: "InvalidEscape", wantMessage: "invalid percent escape in URL component"},
		{input: "%GG", wantKind: "InvalidEscape", wantMessage: "invalid percent escape in URL component"},
		{input: "%FF", wantKind: "InvalidUtf8", wantMessage: "decoded URL component is not valid UTF-8"},
	} {
		got, kind, message := decodeURLComponent(test.input)
		if got != test.want || kind != test.wantKind || message != test.wantMessage {
			t.Fatalf("decodeURLComponent(%q)=(%q, %q, %q), want (%q, %q, %q)", test.input, got, kind, message, test.want, test.wantKind, test.wantMessage)
		}
	}
}

func TestEvaluateContextStopsCanceledEvaluation(t *testing.T) {
	integer := types.FromName("Integer")
	statements := []ir.Statement{
		&ir.ExpressionStatement{Expression: &ir.Literal{
			ExprBase: ir.NewExprBase(token.Span{}, integer),
			Kind:     "integer",
			Raw:      "1",
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewEvaluator(&bytes.Buffer{}, "go").EvaluateContext(ctx, statements, "repl")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("EvaluateContext error=%v, want context.Canceled", err)
	}
}

func TestEvaluatePortableRangeIteration(t *testing.T) {
	integer := types.FromName("Integer")
	rangeType := types.Type{Kind: types.Range, Name: "Range", Args: []types.Type{integer}}
	literal := func(raw string) *ir.Literal {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, integer), Kind: "integer", Raw: raw}
	}
	identifier := func(name string) *ir.Identifier {
		return &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, integer), Name: name}
	}
	statements := []ir.Statement{
		&ir.Variable{Name: "sum", Type: integer, Mutable: true, Value: literal("0")},
		&ir.Iterate{
			Source:    &ir.Range{ExprBase: ir.NewExprBase(token.Span{}, rangeType), Start: literal("0"), End: literal("4"), Exclusive: true},
			Operation: "each",
			Bindings:  []ir.IterationBinding{{Name: "value", Type: integer}},
			Body: []ir.Statement{&ir.Assignment{
				Target:   identifier("sum"),
				Operator: "+=",
				Value:    identifier("value"),
			}},
		},
		&ir.ExpressionStatement{Expression: identifier("sum")},
	}
	result, err := NewEvaluator(&bytes.Buffer{}, "go").Evaluate(statements, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display || Inspect(result.Value) != "6" {
		t.Fatalf("unexpected range iteration result: %#v", result)
	}
}

func TestNegativeIndexesCountFromTheEndAndRemainBoundsChecked(t *testing.T) {
	integer := types.FromName("Integer")
	stringType := types.FromName("String")
	arrayType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{integer}}
	array := Value{Type: arrayType, Data: &arrayValue{Items: []Value{
		{Type: integer, Data: int64(10)},
		{Type: integer, Data: int64(20)},
		{Type: integer, Data: int64(30)},
	}}}
	index := func(value int64) Value { return Value{Type: integer, Data: value} }

	for _, test := range []struct {
		position int64
		want     int64
	}{
		{position: -1, want: 30},
		{position: -2, want: 20},
		{position: -3, want: 10},
	} {
		got, err := indexValue(array, index(test.position), integer)
		if err != nil || got.Data != test.want {
			t.Fatalf("array[%d]=%#v, %v; want %d", test.position, got.Data, err, test.want)
		}
	}
	if _, err := indexValue(array, index(-4), integer); err == nil || err.Error() != "array index is out of bounds" {
		t.Fatalf("array[-4] error=%v", err)
	}
	if err := assignIndex(array, index(-1), Value{Type: integer, Data: int64(40)}); err != nil {
		t.Fatal(err)
	}
	if got := array.Data.(*arrayValue).Items[2].Data; got != int64(40) {
		t.Fatalf("array[-1] assignment stored %#v, want 40", got)
	}
	if err := assignIndex(array, index(-4), Value{Type: integer, Data: int64(0)}); err == nil || err.Error() != "array index is out of bounds" {
		t.Fatalf("array[-4] assignment error=%v", err)
	}

	text, err := indexValue(Value{Type: stringType, Data: "A😀"}, index(-1), stringType)
	if err != nil || text.Data != "😀" {
		t.Fatalf("string[-1]=%#v, %v; want emoji", text.Data, err)
	}
	if _, err := indexValue(Value{Type: stringType, Data: "A😀"}, index(-3), stringType); err == nil || err.Error() != "string index is out of bounds" {
		t.Fatalf("string[-3] error=%v", err)
	}
}

func TestEvaluatePortableHashIteration(t *testing.T) {
	integer := types.FromName("Integer")
	hashType := types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{integer, integer}}
	literal := func(raw string) *ir.Literal {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, integer), Kind: "integer", Raw: raw}
	}
	identifier := func(name string) *ir.Identifier {
		return &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, integer), Name: name}
	}
	statements := []ir.Statement{
		&ir.Variable{Name: "sum", Type: integer, Mutable: true, Value: literal("0")},
		&ir.Iterate{
			Source: &ir.Hash{ExprBase: ir.NewExprBase(token.Span{}, hashType), Entries: []ir.HashEntry{
				{Key: literal("1"), Value: literal("2")},
				{Key: literal("3"), Value: literal("4")},
			}},
			Operation: "each",
			Bindings: []ir.IterationBinding{
				{Name: "key", Type: integer},
				{Name: "value", Type: integer},
			},
			Body: []ir.Statement{&ir.Assignment{
				Target:   identifier("sum"),
				Operator: "+=",
				Value: &ir.Binary{
					ExprBase: ir.NewExprBase(token.Span{}, integer),
					Left:     identifier("key"),
					Operator: "+",
					Right:    identifier("value"),
				},
			}},
		},
		&ir.ExpressionStatement{Expression: identifier("sum")},
	}
	result, err := NewEvaluator(&bytes.Buffer{}, "go").Evaluate(statements, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display || Inspect(result.Value) != "10" {
		t.Fatalf("unexpected Hash iteration result: %#v", result)
	}
}

func TestEvaluateEnumAndCase(t *testing.T) {
	enumType := types.FromName("State")
	definition := &ir.Enum{Name: "State", Body: []ir.Statement{
		&ir.EnumMember{Name: "Open"},
		&ir.EnumMember{Name: "Closed"},
	}}
	state := func(member string) *ir.Member {
		return &ir.Member{
			ExprBase:  ir.NewExprBase(token.Span{}, enumType),
			Receiver:  &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, enumType), Name: "State"},
			Name:      member,
			Namespace: true,
		}
	}
	text := func(value string) *ir.Literal {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, types.FromName("String")), Kind: "string", Raw: `"` + value + `"`}
	}

	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	evaluator.LoadDefinitions(&ir.Program{ModulePath: "repl", Statements: []ir.Statement{definition}})
	result, err := evaluator.Evaluate([]ir.Statement{&ir.Case{
		Value: state("Closed"),
		Branches: []ir.CaseBranch{
			{Value: state("Open"), Body: []ir.Statement{&ir.ExpressionStatement{Expression: text("open")}}},
			{Value: state("Closed"), Body: []ir.Statement{&ir.ExpressionStatement{Expression: text("closed")}}},
		},
	}}, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display || Inspect(result.Value) != `"closed"` || result.Value.Type.String() != "String" {
		t.Fatalf("unexpected enum case result: %#v", result)
	}
}

func TestEvaluateCaseExpression(t *testing.T) {
	enumType := types.FromName("State")
	stringType := types.FromName("String")
	definition := &ir.Enum{Name: "State", Body: []ir.Statement{
		&ir.EnumMember{Name: "Open"},
		&ir.EnumMember{Name: "Closed"},
	}}
	state := func(member string) *ir.Member {
		return &ir.Member{
			ExprBase:  ir.NewExprBase(token.Span{}, enumType),
			Receiver:  &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, enumType), Name: "State"},
			Name:      member,
			Namespace: true,
		}
	}
	text := func(value string) *ir.Literal {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, stringType), Kind: "string", Raw: `"` + value + `"`}
	}

	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	evaluator.LoadDefinitions(&ir.Program{ModulePath: "repl", Statements: []ir.Statement{definition}})
	caseExpression := &ir.Case{
		ExprBase: ir.NewExprBase(token.Span{}, stringType),
		Value:    state("Closed"),
		Branches: []ir.CaseBranch{
			{Value: state("Open"), Result: text("open")},
			{Value: state("Closed"), Result: text("closed")},
		},
	}
	result, err := evaluator.Evaluate([]ir.Statement{
		&ir.Variable{Name: "message", Type: stringType, Value: caseExpression},
		&ir.ExpressionStatement{Expression: &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, stringType), Name: "message"}},
	}, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display || Inspect(result.Value) != `"closed"` || result.Value.Type.String() != "String" {
		t.Fatalf("unexpected case expression result: %#v", result)
	}
}

func TestUpdateProjectDefinitionsRetainsUnchangedModulesAndRelinksSuperclasses(t *testing.T) {
	parent := &ir.Program{ModulePath: "models/parent", Statements: []ir.Statement{
		&ir.Class{Name: "Parent"},
	}}
	child := &ir.Program{ModulePath: "models/child", Statements: []ir.Statement{
		&ir.Class{Name: "Child", Superclass: &ir.Identifier{Name: "Parent"}},
	}}
	unchanged := &ir.Program{ModulePath: "models/profile", Statements: []ir.Statement{
		&ir.Record{Name: "Profile"},
	}}
	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	initial := []*ir.Program{parent, child, unchanged}
	if err := evaluator.LoadProject(initial, "__trb_repl__"); err != nil {
		t.Fatal(err)
	}
	initialParent := evaluator.definitions[symbolKey(parent.ModulePath, "Parent")].(*classDefinition)
	initialChild := evaluator.definitions[symbolKey(child.ModulePath, "Child")].(*classDefinition)
	initialProfile := evaluator.definitions[symbolKey(unchanged.ModulePath, "Profile")]
	if initialChild.Superclass != initialParent {
		t.Fatal("initial project definitions did not link the superclass")
	}

	updatedParent := &ir.Program{ModulePath: parent.ModulePath, Statements: []ir.Statement{
		&ir.Class{Name: "Parent", Body: []ir.Statement{&ir.Method{Name: "updated"}}},
	}}
	updated := []*ir.Program{updatedParent, child, unchanged}
	evaluator.updateProjectDefinitions(initial, updated, "__trb_repl__")
	currentParent := evaluator.definitions[symbolKey(parent.ModulePath, "Parent")].(*classDefinition)
	currentChild := evaluator.definitions[symbolKey(child.ModulePath, "Child")].(*classDefinition)
	if currentParent == initialParent || currentChild != initialChild || currentChild.Superclass != currentParent {
		t.Fatalf("definitions were not updated incrementally: parent=%p child=%p superclass=%p", currentParent, currentChild, currentChild.Superclass)
	}
	if evaluator.definitions[symbolKey(unchanged.ModulePath, "Profile")] != initialProfile {
		t.Fatal("unchanged module definitions were rebuilt")
	}

	withoutParent := []*ir.Program{child, unchanged}
	evaluator.updateProjectDefinitions(updated, withoutParent, "__trb_repl__")
	if _, exists := evaluator.definitions[symbolKey(parent.ModulePath, "Parent")]; exists {
		t.Fatal("removed module definition remained available")
	}
	if currentChild.Superclass != nil {
		t.Fatal("class retained a removed superclass definition")
	}
}

func TestEvaluateLiteralCaseAlternatives(t *testing.T) {
	stringType := types.FromName("String")
	literal := func(value string) *ir.Literal {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, stringType), Kind: "string", Raw: `"` + value + `"`}
	}
	caseExpression := &ir.Case{
		ExprBase: ir.NewExprBase(token.Span{}, stringType),
		Value:    literal("receipt_detail"),
		Branches: []ir.CaseBranch{{
			Value:        literal("receipts"),
			Alternatives: []ir.Expression{literal("receipt_detail")},
			Result:       literal("receipts"),
		}},
		ElseResult: literal("other"),
		HasElse:    true,
	}
	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	result, err := evaluator.Evaluate([]ir.Statement{&ir.ExpressionStatement{Expression: caseExpression}}, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display || Inspect(result.Value) != `"receipts"` {
		t.Fatalf("unexpected literal case result: %#v", result)
	}
}

func TestEvaluateIfExpression(t *testing.T) {
	booleanType := types.FromName("Boolean")
	stringType := types.FromName("String")
	literal := func(kind, raw string, typ types.Type) *ir.Literal {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, typ), Kind: kind, Raw: raw}
	}
	ifExpression := &ir.If{
		ExprBase:   ir.NewExprBase(token.Span{}, stringType),
		Condition:  literal("boolean", "false", booleanType),
		ThenResult: literal("string", `"on"`, stringType),
		ElseIf: []ir.IfBranch{{
			Condition: literal("boolean", "true", booleanType),
			Result:    literal("string", `"secondary"`, stringType),
		}},
		ElseResult: literal("string", `"off"`, stringType),
		HasElse:    true,
	}

	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	result, err := evaluator.Evaluate([]ir.Statement{
		&ir.Variable{Name: "message", Type: stringType, Value: ifExpression},
		&ir.ExpressionStatement{Expression: &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, stringType), Name: "message"}},
	}, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display || Inspect(result.Value) != `"secondary"` || result.Value.Type.String() != "String" {
		t.Fatalf("unexpected if expression result: %#v", result)
	}
}

func TestEvaluateNonNullableToNullableConversion(t *testing.T) {
	stringType := types.FromName("String")
	nullableString := stringType
	nullableString.Nullable = true
	conversion := &ir.Conversion{
		ExprBase: ir.NewExprBase(token.Span{}, nullableString),
		Kind:     ir.NonNullableToNullableConversion,
		Value:    &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, stringType), Kind: "string", Raw: `"Ada"`},
	}

	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	result, err := evaluator.Evaluate([]ir.Statement{&ir.ExpressionStatement{Expression: conversion}}, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display || Inspect(result.Value) != `"Ada"` || result.Value.Type.String() != "String?" {
		t.Fatalf("unexpected nullable conversion result: %#v", result)
	}
}

func TestEvaluateNullableToNonNullableConversion(t *testing.T) {
	stringType := types.FromName("String")
	nullableString := stringType
	nullableString.Nullable = true
	conversion := &ir.Conversion{
		ExprBase: ir.NewExprBase(token.Span{}, stringType),
		Kind:     ir.NullableToNonNullableConversion,
		Value:    &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, nullableString), Name: "name"},
	}

	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	evaluator.moduleValue[symbolKey("repl", "name")] = Value{Type: nullableString, Data: "Ada"}
	result, err := evaluator.Evaluate([]ir.Statement{&ir.ExpressionStatement{Expression: conversion}}, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display || Inspect(result.Value) != `"Ada"` || result.Value.Type.String() != "String" {
		t.Fatalf("unexpected nullable unwrap result: %#v", result)
	}
}

func TestEvaluateNullableRecordFieldToNonNullableConversion(t *testing.T) {
	stringType := types.FromName("String")
	nullableString := stringType
	nullableString.Nullable = true
	profileType := types.FromName("Profile")
	definition := &recordDefinition{Module: "repl", Node: &ir.Record{Name: "Profile"}}
	conversion := &ir.Conversion{
		ExprBase: ir.NewExprBase(token.Span{}, stringType),
		Kind:     ir.NullableToNonNullableConversion,
		Value: &ir.Member{
			ExprBase: ir.NewExprBase(token.Span{}, nullableString),
			Receiver: &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, profileType), Name: "profile"},
			Name:     "nickname",
		},
	}

	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	evaluator.moduleValue[symbolKey("repl", "profile")] = Value{Type: profileType, Data: &recordInstance{
		Definition: definition,
		Fields:     map[string]Value{"nickname": {Type: nullableString, Data: "Ada"}},
	}}
	result, err := evaluator.Evaluate([]ir.Statement{&ir.ExpressionStatement{Expression: conversion}}, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display || Inspect(result.Value) != `"Ada"` || result.Value.Type.String() != "String" {
		t.Fatalf("unexpected nullable field unwrap result: %#v", result)
	}
}

func TestEvaluateDivergingIfExpressionPropagatesReturn(t *testing.T) {
	booleanType := types.FromName("Boolean")
	stringType := types.FromName("String")
	literal := func(kind, raw string, typ types.Type) *ir.Literal {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, typ), Kind: kind, Raw: raw}
	}
	ifExpression := &ir.If{
		ExprBase:     ir.NewExprBase(token.Span{}, stringType),
		Condition:    literal("boolean", "false", booleanType),
		ThenResult:   literal("string", `"value"`, stringType),
		Else:         []ir.Statement{&ir.Return{Value: literal("string", `"returned"`, stringType)}},
		ElseDiverges: true,
		HasElse:      true,
	}

	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	result, err := evaluator.Evaluate([]ir.Statement{
		&ir.ExpressionStatement{Expression: ifExpression},
		&ir.ExpressionStatement{Expression: literal("string", `"unreachable"`, stringType)},
	}, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if result.Display || Inspect(result.Value) != `"returned"` || result.Value.Type.String() != "String" {
		t.Fatalf("unexpected propagated return: %#v", result)
	}
}
