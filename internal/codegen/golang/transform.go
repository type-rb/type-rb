package golang

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) transform(transform *ir.Transform) string {
	if transform.Operation == "concurrent_map" {
		return g.concurrentMap(transform)
	}
	g.temporary++
	suffix := strconv.Itoa(g.temporary)
	items, values := "__trbItems"+suffix, "__trbValues"+suffix
	result, index := "__trbResult"+suffix, "__trbIndex"+suffix
	current := "__trbCurrent" + suffix
	visited := "__trbVisited" + suffix
	item := g.bindingIdentifier(transform.Item)
	if item == "" || item == "_" {
		item = "__trbItem" + suffix
	}
	setup := items + " := " + g.expr(transform.Source) + "; "
	if transform.Operation == "reduce" {
		// The initial argument can mutate the retained source before traversal.
		setup += result + " := " + g.expr(transform.Initial) + "; "
	}
	sourceKind := transform.Source.ExprType().Kind
	if sourceKind == types.Array {
		// Keep the Array identity, reloading its length and storage each time.
		values = g.arrayValues(items)
	} else if sourceKind != types.Range {
		setup += values + " := " + g.iterableValue(items, transform.Source.ExprType()) + "; "
	}
	capacity := "len(" + values + ")"
	header := "for " + index + " := 0; " + index + " < len(" + values + "); " + index + "++ { "
	valueAt := values + "[" + index + "]"
	if sourceKind == types.Range {
		// Range transformations retain bounds; only result collections allocate.
		capacity = "0"
		condition := current + " < " + items + "[1] || (" + current + " == " + items + "[1] && " + items + "[2] == 0)"
		header = "for " + current + ", " + index + " := " + items + "[0], 0; " + condition + "; " + current + ", " + index + " = " + current + "+1, " + index + "+1 { "
		valueAt = current
	}
	loop := header + visited + " := " + valueAt + "; " + item + " := " + visited + "; _ = " + item + "; "
	if transform.WithIndex && transform.Index != "" && transform.Index != "_" {
		binding := g.bindingIdentifier(transform.Index)
		loop += binding + " := " + index + "; _ = " + binding + "; "
	}
	value := g.transformResult(transform)
	wrap := func(body string) string {
		return "func() " + g.goType(transform.ExprType()) + " { " + setup + body + " }()"
	}
	switch transform.Operation {
	case "sort_by", "sort_by_descending":
		g.requireImport("slices", "")
		decorated := "__trbDecorated" + suffix
		keyType := transform.Result.ExprType()
		comparison := g.portableSortComparison("left.key", "right.key", keyType, transform.Operation == "sort_by_descending")
		return wrap("type " + decorated + " struct { value " + g.goType(transform.ItemType) + "; key " + g.goType(keyType) + " }; ordered := make([]" + decorated + ", 0, len(" + values + ")); " + loop + "ordered = append(ordered, " + decorated + "{value: " + visited + ", key: " + value + "}) }; slices.SortStableFunc(ordered, func(left, right " + decorated + ") int { return " + comparison + " }); " + result + " := make(" + g.goArraySliceType(transform.ExprType()) + ", 0, len(ordered)); for _, entry := range ordered { " + result + " = append(" + result + ", entry.value) }; return " + g.arrayReference(result))
	case "map", "select":
		body := result + " := make(" + g.goArraySliceType(transform.ExprType()) + ", 0, " + capacity + "); " + loop
		if transform.Operation == "map" {
			body += result + " = append(" + result + ", " + value + ")"
		} else {
			body += "if " + value + " { " + result + " = append(" + result + ", " + visited + ") }"
		}
		return wrap(body + " }; return " + g.arrayReference(result))
	case "any?", "all?", "none?":
		initial, matched, match := "false", "true", value
		if transform.Operation != "any?" {
			initial, matched = "true", "false"
		}
		if transform.Operation == "all?" {
			match = "!(" + value + ")"
		}
		return wrap(loop + "if " + match + " { return " + matched + " } }; return " + initial)
	case "find":
		found := "&" + visited
		if len(transform.Source.ExprType().Args) > 0 && g.goType(transform.Source.ExprType().Args[0]) == g.goType(transform.ExprType()) {
			found = visited
		}
		return wrap(loop + "if " + value + " { return " + found + " } }; return nil")
	case "find_index":
		return wrap(loop + "if " + value + " { " + result + " := " + index + "; return &" + result + " } }; return nil")
	case "reduce":
		binding := ""
		if accumulator := g.bindingIdentifier(transform.Accumulator); accumulator != "" && accumulator != "_" {
			binding = accumulator + " := " + result + "; _ = " + accumulator + "; "
		}
		return wrap(loop + binding + result + " = " + value + " }; return " + result)
	default:
		return "nil"
	}
}

func (g *generator) transformResult(transform *ir.Transform) string {
	if len(transform.Body) == 0 {
		return g.expr(transform.Result)
	}
	child := *g
	child.b = strings.Builder{}
	child.recordSources = false
	child.indent = 0
	child.line("func() " + child.goType(transform.Result.ExprType()) + " {")
	child.indent++
	child.statements(transform.Body)
	child.line("return " + child.expr(transform.Result))
	child.indent--
	child.line("}()")
	g.temporary = child.temporary
	g.absorbRuntimeRequirements(&child)
	return strings.TrimSpace(child.b.String())
}
