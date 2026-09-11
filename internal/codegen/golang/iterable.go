package golang

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/codegen/naming"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) iterableConversion(conversion *ir.Conversion) string {
	source, target := conversion.Value.ExprType(), conversion.ExprType()
	sourceElement, targetElement := types.FromName("Any"), types.FromName("Any")
	if len(source.Args) == 1 {
		sourceElement = source.Args[0]
	}
	if len(target.Args) == 1 {
		targetElement = target.Args[0]
	}
	element := "__trbElement"
	converted := element
	if targetElement.Nullable && !sourceElement.Nullable {
		value := &ir.Identifier{ExprBase: ir.NewExprBase(conversion.SourceSpan(), sourceElement), Name: element, Lexical: true, Generated: true}
		converted = g.nonNullableToNullableExpr(&ir.Conversion{Value: value}, targetElement)
	} else if targetElement.Kind == types.Float && (sourceElement.Kind == types.Int || sourceElement.Kind == types.IntLiteral) {
		converted = "float64(" + element + ")"
	}
	visit := "if !yield(" + converted + ") { return }; "
	body := ""
	switch source.Kind {
	case types.Range:
		body = "start, end, exclusive := __trbSource[0], __trbSource[1], __trbSource[2] == 1; for " + element + " := start; " + element + " < end; " + element + "++ { " + visit + " }; if !exclusive && start <= end { " + element + " := end; " + visit + " }"
	case types.Array:
		body = "for _, " + element + " := range *__trbSource { " + visit + " }"
	case types.Iterable:
		sequence := "__trbSource"
		if source.Nullable {
			sequence = "*" + sequence
		}
		body = "for " + element + " := range " + sequence + " { " + visit + " }"
	}
	guard := ""
	if source.Nullable {
		guard = "if __trbSource == nil { return nil }; "
	}
	returned := "__trbSequence"
	if target.Nullable {
		returned = "&" + returned
	}
	return "func(__trbSource " + g.goType(source) + ") " + g.goType(target) + " { " + guard + "__trbSequence := func(yield func(" + g.goType(targetElement) + ") bool) { " + body + " }; return " + returned + " }(" + g.expr(conversion.Value) + ")"
}

func (g *generator) iterableIterate(iteration *ir.Iterate) {
	g.temporary++
	suffix := strconv.Itoa(g.temporary)
	items := "__trbIterable" + suffix
	counter := "__trbIndex" + suffix
	g.line("{")
	g.indent++
	g.line(items + " := " + g.expr(iteration.Source))
	sequence := items
	if iteration.Operation == "each_slice" {
		g.iterableRuntime = true
		size := "__trbSize" + suffix
		g.line(size + " := " + g.expr(iteration.SliceSize))
		g.line("if " + size + " <= 0 { panic(\"each_slice size must be greater than zero\") }")
		checkpoint := "func() {}"
		if g.executionActive {
			checkpoint = "func() { if err := __trbScope.Err(); err != nil { panic(err) } }"
		}
		sequence = g.iterableBatchesName() + "(" + items + ", " + size + ", " + checkpoint + ")"
	}
	indexed := iteration.WithIndex && iteration.Bindings[1].Name != "_"
	if indexed {
		g.line(counter + " := 0")
	}
	item := iteration.Bindings[0].Name
	if item == "_" {
		g.line("for range " + sequence + " {")
	} else {
		item = g.bindingIdentifier(item)
		g.line("for " + item + " := range " + sequence + " {")
	}
	g.indent++
	if item != "_" {
		g.line("_ = " + item)
	}
	if indexed {
		index := g.bindingIdentifier(iteration.Bindings[1].Name)
		g.line(index + " := " + counter)
		g.line("_ = " + index)
		g.line(counter + "++")
	}
	g.statements(iteration.Body)
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
}

func (g *generator) iterableBatchesName() string {
	return "trbIterableBatches_" + naming.PrivateSuffix("iterable-batches:"+g.modulePath)
}

func (g *generator) iterableRuntimeSupport() {
	g.line("func " + g.iterableBatchesName() + "[T any](source func(func(T) bool), size int, checkpoint func()) func(func(*[]T) bool) { return func(yield func(*[]T) bool) { chunk := []T{}; for value := range source { checkpoint(); chunk = append(chunk, value); if len(chunk) == size { batch := chunk; if !yield(&batch) { return }; chunk = nil } }; if len(chunk) > 0 { yield(&chunk) } } }")
}
