package repl

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func compileRangeSession(t *testing.T, mode, source string) (*Evaluator, *ir.Program) {
	t.Helper()
	unit := compiler.SourceUnit{Filename: "/project/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main", Source: []byte(source)}
	artifacts, err := compiler.CompileProject([]compiler.SourceUnit{unit}, compiler.Options{
		Mode: mode, GoModule: "example.com/range-repl", RubyLoader: "require_relative", InteractiveModule: unit.ModulePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	var programs []*ir.Program
	var session *ir.Program
	for _, artifact := range artifacts {
		programs = append(programs, artifact.IR)
		if artifact.IR.ModulePath == unit.ModulePath {
			session = artifact.IR
		}
	}
	if session == nil {
		t.Fatal("missing interactive session")
	}
	evaluator := NewEvaluator(&bytes.Buffer{}, mode)
	t.Cleanup(func() { _ = evaluator.Close() })
	if err := evaluator.LoadProject(programs, unit.ModulePath); err != nil {
		t.Fatal(err)
	}
	evaluator.LoadDefinitions(session)
	return evaluator, session
}

func TestReplRangeIterationHasNoFixedLimit(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			e, session := compileRangeSession(t, mode, `mut count := 0
(0..1_000_000).each { |_| count += 1 }
mut loops := 0
while loops < 1_000_001
	loops += 1
end
mut first := -1
(0..9007199254740991).each.with_index do |value, index|
	first = value + index
	break
end
mut last := -1
(9007199254740990..9007199254740991).each_slice(3) do |part|
	last = part[-1]
end
[count, loops, first, last]
`)
			result, err := e.Evaluate(session.Statements, session.ModulePath)
			if err != nil {
				t.Fatal(err)
			}
			if got := Inspect(result.Value); got != "[1000001, 1000001, 0, 9007199254740991]" {
				t.Fatal(got)
			}
		})
	}
}

func TestReplRangeEndpointsRetainEvaluatedStart(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			e, session := compileRangeSession(t, mode, `mut start := 0
mut calls := 0
finish := fn(): Integer
	calls += 1
	start = 9
	return 2
end
mut values: Array<Integer> := []
(start..finish()).each { |value| values.push(value) }
start = 1
(start...finish()).each { |value| values.push(value) }
start = 3
values.push((start..finish()).to_a().size())
values.push(start)
values.push(calls)
values
`)
			result, err := e.Evaluate(session.Statements, session.ModulePath)
			if err != nil {
				t.Fatal(err)
			}
			if got := Inspect(result.Value); got != "[0, 1, 2, 1, 0, 9, 3]" {
				t.Fatal(got)
			}
		})
	}
}

func TestReplMillionEntryHashAndRangeMaterialization(t *testing.T) {
	e, session := compileRangeSession(t, "go", `mut h := {}
(0..1_000_000).each { |i| h[i] = i * i }
[h.size(), h[1_000_000], (0..1_000_000).to_a().size()]
`)
	result, err := e.Evaluate(session.Statements, session.ModulePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := Inspect(result.Value); got != "[1000001, 1000000000000, 1000001]" {
		t.Fatal(got)
	}
}

func TestReplCancelsActiveRangeAndWhile(t *testing.T) {
	for name, source := range map[string]string{
		"range": "(0..9007199254740991).each { |_| }\n",
		"batch": "(0..9007199254740991).each_slice(9007199254740991) { |_| }\n",
		"while": "while true\nend\n",
	} {
		t.Run(name, func(t *testing.T) {
			e, session := compileRangeSession(t, "go", source)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			_, err := e.EvaluateContext(ctx, session.Statements, session.ModulePath)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("got %v, want active cancellation", err)
			}
			// Cancellation is scoped to this submission; the evaluator remains usable.
			if _, err := e.Evaluate(nil, session.ModulePath); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReplRangeMaterializationCanBeCancelled(t *testing.T) {
	e := NewEvaluator(&bytes.Buffer{}, "go")
	t.Cleanup(func() { _ = e.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	e.context = ctx
	_, err := e.materializeIterable(Value{Data: &rangeValue{Start: 0, End: types.MaxPortableInteger}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want cancellation during materialization", err)
	}
}

func TestReplIterableBoundariesRetainRepeatableSources(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			e, session := compileRangeSession(t, mode, `def retain<T>(values: Iterable<T>): Iterable<T>
	return values
end
def first(values: Iterable<Integer>): Integer
	values.each do |value|
		return value
	end
	return -1
end
def last_batch(values: Iterable<Integer>): Integer
	mut result := -1
	values.each_slice(2).with_index do |part, index|
		next if index == 0
		result = part[-1]
	end
	return result
end
values := retain<Integer>(0..9007199254740991)
mut items := [1, 2]
retained := retain<Integer>(items)
items.push(3)
items[0] = 4
items = [99]
[first(values), first(values), first(retained), last_batch(retained)]
`)
			result, err := e.Evaluate(session.Statements, session.ModulePath)
			if err != nil {
				t.Fatal(err)
			}
			if got := Inspect(result.Value); got != "[0, 0, 4, 3]" {
				t.Fatal(got)
			}
		})
	}
}
