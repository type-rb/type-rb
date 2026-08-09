package repl

import (
	"errors"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

type testRuntimeProvider struct {
	configured bool
	closed     bool
	invocation runtimeInvocation
	block      runtimeBlockInvocation
}

func (*testRuntimeProvider) Name() string { return "test" }

func (*testRuntimeProvider) Handles(intrinsic string) bool { return intrinsic == "test.echo" }

func (provider *testRuntimeProvider) Configure(_ []*ir.Program) error {
	provider.configured = true
	return nil
}

func (provider *testRuntimeProvider) Call(_ *Evaluator, invocation runtimeInvocation) (Value, error) {
	provider.invocation = invocation
	if len(invocation.Arguments) != 1 {
		return Value{}, errors.New("test.echo expects one argument")
	}
	return invocation.Arguments[0].Value, nil
}

func (provider *testRuntimeProvider) Block(_ *Evaluator, invocation runtimeBlockInvocation) (Value, error) {
	provider.block = invocation
	return invocation.Evaluate([]Value{{Type: types.FromName("String"), Data: "scoped"}})
}

func (provider *testRuntimeProvider) Close() error {
	provider.closed = true
	return nil
}

func TestRuntimeProviderOwnsStructuredResourceBlock(t *testing.T) {
	provider := &testRuntimeProvider{}
	evaluator := NewEvaluator(nil, "go")
	evaluator.runtimeProviders = []runtimeProvider{provider}
	stringType := types.FromName("String")
	variable := &ir.Variable{Name: "result", Type: stringType}
	result, err := evaluator.Evaluate([]ir.Statement{&ir.StructuredBlock{
		Call: &ir.Call{Callee: &ir.Identifier{
			ExprBase: ir.NewExprBase(token.Span{}, stringType), Name: "resource",
			Reference: &ir.Reference{Intrinsic: "test.echo"},
		}},
		Intrinsic: "test.echo",
		Bindings:  []ir.IterationBinding{{Name: "value", Type: stringType}},
		Value: &ir.Identifier{
			ExprBase: ir.NewExprBase(token.Span{}, stringType), Name: "value", Lexical: true,
		},
		Result: &ir.StructuredBlockResult{Variable: variable, Type: stringType},
	}}, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if result.Value.Data != "scoped" || provider.block.Name != "test.echo" {
		t.Fatalf("structured runtime result=%#v invocation=%#v", result, provider.block)
	}
}

func TestRuntimeProviderOwnsConfigurationInvocationAndLifecycle(t *testing.T) {
	provider := &testRuntimeProvider{}
	evaluator := NewEvaluator(nil, "go")
	evaluator.runtimeProviders = []runtimeProvider{provider}
	if err := evaluator.configureRuntimeProviders([]*ir.Program{{Mode: "go"}}); err != nil {
		t.Fatal(err)
	}
	argument := Value{Type: types.FromName("String"), Data: "value"}
	value, handled, err := evaluator.runtimeCall(runtimeInvocation{
		Name: "test.echo", Arguments: []evaluatedArgument{{Value: argument}},
		Type: types.FromName("String"), MemberName: "echo",
	})
	if err != nil || !handled || value.Data != "value" {
		t.Fatalf("runtime call=(%#v, %t, %v)", value, handled, err)
	}
	if !provider.configured || provider.invocation.MemberName != "echo" {
		t.Fatalf("provider did not receive its configuration and invocation: %#v", provider)
	}
	if err := evaluator.Close(); err != nil {
		t.Fatal(err)
	}
	if !provider.closed {
		t.Fatal("provider was not closed")
	}
}
