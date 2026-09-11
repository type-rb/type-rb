package typescript

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) iterableConversion(conversion *ir.Conversion) string {
	source := conversion.Value.ExprType()
	if source.Kind != types.Range {
		return g.expr(conversion.Value)
	}
	guard := ""
	if source.Nullable {
		guard = "if (source === null) return null; "
	}
	return "((source: " + g.tsType(source) + "): " + g.tsType(conversion.ExprType()) + " => { " + guard + "const [start, end, exclusive] = source; return { *[Symbol.iterator]() { for (let value = start; value < end; value++) yield value; if (!exclusive && start <= end) yield end; } }; })(" + g.expr(conversion.Value) + ")"
}

func (g *generator) iterableIterate(iteration *ir.Iterate) {
	sourceType := iteration.Source.ExprType()
	sourceIdentity := g.expressionTypeIdentity(sourceType, iteration.Source)
	binding := iteration.Bindings[0]
	if len(sourceType.Args) == 1 {
		if iteration.Operation == "each_slice" && len(binding.Type.Args) == 1 {
			itemIdentity := projectTypeScriptTypeIdentity(binding.Type.Args[0], sourceType.Args[0], identityArgument(sourceIdentity, 0))
			g.exactTypes[binding.Name] = identityWithArgument(binding.Type, 0, itemIdentity)
		} else {
			g.exactTypes[binding.Name] = projectTypeScriptTypeIdentity(binding.Type, sourceType.Args[0], identityArgument(sourceIdentity, 0))
		}
	}
	g.temporary++
	suffix := strconv.Itoa(g.temporary)
	items := "__trbIterable" + suffix
	counter := "__trbIndex" + suffix
	g.line("{")
	g.indent++
	g.line("const " + items + " = " + g.expr(iteration.Source) + ";")
	sequence := items
	if iteration.Operation == "each_slice" {
		size := "__trbSize" + suffix
		g.line("const " + size + " = " + g.expr(iteration.SliceSize) + ";")
		g.line("if (" + size + " <= 0) throw new Error(\"each_slice size must be greater than zero\");")
		element := g.tsType(iteration.Source.ExprType().Args[0])
		sequence = "(function* () { let chunk: Array<" + element + "> = []; for (const value of " + items + ") { chunk.push(value); if (chunk.length === " + size + ") { yield chunk; chunk = []; } } if (chunk.length > 0) yield chunk; })()"
	}
	indexed := iteration.WithIndex && iteration.Bindings[1].Name != "_"
	if indexed {
		g.line("let " + counter + " = 0;")
	}
	item := iteration.Bindings[0].Name
	if item == "_" {
		item = "__trbItem" + suffix
	}
	g.line("for (let " + item + " of " + sequence + ") {")
	g.indent++
	g.line("void " + item + ";")
	if indexed {
		index := iteration.Bindings[1].Name
		g.line("let " + index + " = " + counter + "++;")
		g.line("void " + index + ";")
	}
	g.statements(iteration.Body)
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
}
