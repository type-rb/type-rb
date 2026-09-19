package typescript

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

func (g *generator) transform(transform *ir.Transform) string {
	if transform.Operation == "concurrent_map" {
		return g.concurrentMap(transform)
	}
	return g.imperativeTransform(transform, g.suspension != nil && g.suspension.Expressions[transform])
}

func (g *generator) imperativeTransform(transform *ir.Transform, suspends bool) string {
	child := *g
	child.b = strings.Builder{}
	child.sourceRecorder = nil
	child.indent = 0
	child.exactTypes = cloneTypeScriptTypeIdentities(g.exactTypes)
	child.temporary++
	suffix := strconv.Itoa(child.temporary)
	items := "__trbItems" + suffix
	result := "__trbResult" + suffix
	index := "__trbIndex" + suffix
	visited := "__trbVisited" + suffix
	item := tsBindingName(transform.Item)
	if item == "" {
		item = "__trbItem" + suffix
	}
	sourceType := transform.Source.ExprType()
	sourceIdentity := child.expressionTypeIdentity(sourceType, transform.Source)
	itemIdentity := projectTypeScriptTypeIdentity(transform.ItemType, sourceType, sourceIdentity)
	child.exactTypes[transform.Item] = cloneTypeScriptTypeIdentity(itemIdentity)
	success := transform.ExprType()
	successIdentity := child.expressionTypeIdentity(success, transform)
	complete := func(value string) string { return value }
	if suspends {
		child.line("(async (): Promise<" + child.tsTypeWithIdentity(success, successIdentity) + "> => {")
	} else {
		child.line("((): " + child.tsTypeWithIdentity(success, successIdentity) + " => {")
	}
	child.indent++
	child.line("const " + items + " = " + child.iterableExpr(transform.Source) + ";")

	emitBindings := func(includeIndex bool) {
		child.line("const " + visited + " = " + items + "[" + index + "]!;")
		child.line("let " + item + " = " + visited + ";")
		if includeIndex {
			name := tsBindingName(transform.Index)
			if name == "" {
				name = "__trbSourceIndex" + suffix
			}
			child.line("let " + name + " = " + index + ";")
		}
	}
	emitValue := func() string {
		child.statements(transform.Body)
		return child.expr(transform.Result)
	}

	switch transform.Operation {
	case "sort_by", "sort_by_descending":
		keyType := child.expressionType(transform.Result.ExprType(), transform.Result)
		itemType := child.tsTypeWithIdentity(transform.ItemType, itemIdentity)
		decorated := "__trbDecorated" + suffix
		child.line("const " + decorated + ": Array<{ value: " + itemType + "; key: " + keyType + "; index: number }> = [];")
		child.line("for (let " + index + " = 0; " + index + " < " + items + ".length; " + index + " += 1) {")
		child.indent++
		emitBindings(false)
		value := emitValue()
		child.line(decorated + ".push({ value: " + visited + ", key: " + value + ", index: " + index + " });")
		child.indent--
		child.line("}")
		comparison := tsPortableSortComparison("left.key", "right.key", transform.Result.ExprType(), transform.Operation == "sort_by_descending")
		child.line(decorated + ".sort((left, right) => { const compared = " + comparison + "; return compared === 0 ? left.index - right.index : compared; });")
		child.line("return " + complete(decorated+".map((entry) => entry.value)") + ";")
	case "map", "select":
		child.line("const " + result + ": " + child.tsTypeWithIdentity(success, successIdentity) + " = [];")
		child.line("for (let " + index + " = 0; " + index + " < " + items + ".length; " + index + " += 1) {")
		child.indent++
		emitBindings(transform.WithIndex)
		value := emitValue()
		if transform.Operation == "map" {
			child.line(result + ".push(" + value + ");")
		} else {
			child.line("if (" + value + ") " + result + ".push(" + visited + ");")
		}
		child.indent--
		child.line("}")
		child.line("return " + complete(result) + ";")
	case "any?", "all?", "none?", "find", "find_index":
		child.line("for (let " + index + " = 0; " + index + " < " + items + ".length; " + index + " += 1) {")
		child.indent++
		emitBindings(false)
		value := emitValue()
		switch transform.Operation {
		case "any?":
			child.line("if (" + value + ") return " + complete("true") + ";")
		case "all?":
			child.line("if (!(" + value + ")) return " + complete("false") + ";")
		case "none?":
			child.line("if (" + value + ") return " + complete("false") + ";")
		case "find":
			child.line("if (" + value + ") return " + complete(visited) + ";")
		case "find_index":
			child.line("if (" + value + ") return " + complete(index) + ";")
		}
		child.indent--
		child.line("}")
		switch transform.Operation {
		case "any?":
			child.line("return " + complete("false") + ";")
		case "all?", "none?":
			child.line("return " + complete("true") + ";")
		default:
			child.line("return " + complete("null") + ";")
		}
	case "reduce":
		child.line("let " + result + " = " + child.expr(transform.Initial) + ";")
		child.line("for (let " + index + " = 0; " + index + " < " + items + ".length; " + index + " += 1) {")
		child.indent++
		emitBindings(false)
		accumulator := tsBindingName(transform.Accumulator)
		if accumulator == "" {
			accumulator = "__trbAccumulator" + suffix
		}
		child.line("let " + accumulator + " = " + result + ";")
		value := emitValue()
		child.line(result + " = " + value + ";")
		child.indent--
		child.line("}")
		child.line("return " + complete(result) + ";")
	default:
		child.line("throw new Error(\"unsupported TypeRB collection transformation\");")
	}
	child.indent--
	child.line("})()")
	g.temporary = child.temporary
	generated := strings.TrimSpace(child.b.String())
	if suspends {
		return "(await " + generated + ")"
	}
	return generated
}
