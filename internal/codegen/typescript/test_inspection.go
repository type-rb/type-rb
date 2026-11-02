package typescript

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

// Shared by copied lambda generators; every requested helper is emitted once.
type testViewRegistry struct {
	names  map[string]string
	bodies []string
}

const testViewMap = `((globalThis as any)[Symbol.for("type-rb.test.views")] ??= new WeakMap<object, () => any>())`

func (g *generator) expr(expression ir.Expression) string {
	value := g.exprRaw(expression)
	if expression == nil || g.testInspection == nil || !g.testInspection.Enabled {
		return value
	}
	// A declaration reference is a type/constructor, never an object to inspect.
	declaration := ir.ExpressionDeclaration(expression)
	if declaration.Kind.IsType() {
		switch node := expression.(type) {
		case *ir.Member:
			if node.Name == declaration.LeafName() {
				return value
			}
		default:
			return value
		}
	}
	return g.testValue(expression.ExprType(), value)
}

func testTypeKey(typ types.Type) string {
	result := string(typ.Kind) + ":" + typ.Name + ":" + typ.Declaration.Key()
	for _, argument := range typ.Args {
		result += "[" + testTypeKey(argument) + "]"
	}
	return result
}

func testSubstitute(typ types.Type, bindings map[string]types.Type) types.Type {
	if typ.Kind == types.Named && typ.Declaration.Empty() {
		if value, ok := bindings[typ.Name]; ok {
			value.Nullable = value.Nullable || typ.Nullable
			return value
		}
	}
	args := make([]types.Type, len(typ.Args))
	for i, arg := range typ.Args {
		args[i] = testSubstitute(arg, bindings)
	}
	typ.Args = args
	return typ
}

func (g *generator) testValue(typ types.Type, value string) string {
	if g.testInspection == nil || !g.testInspection.Enabled {
		return value
	}
	record := g.testInspection.Records[typ.Declaration]
	enum := g.testInspection.Enums[typ.Declaration]
	class := g.testInspection.Classes[typ.Declaration]
	collection := typ.Kind == types.Array || typ.Kind == types.Hash || typ.Kind == types.Range
	if record == nil && (enum == nil || !enumHasPayload(enum)) && class == nil && !collection {
		return value
	}
	if class != nil && class.External {
		return value
	}
	key := testTypeKey(typ)
	if name, ok := g.testViews.names[key]; ok {
		return name + "(" + value + ")"
	}
	name := "__trbTestView" + strconv.Itoa(len(g.testViews.names))
	g.testViews.names[key] = name
	bindings := map[string]types.Type{}
	parameters := []string{}
	if record != nil {
		parameters = record.TypeParameters
	}
	if enum != nil {
		parameters = enum.TypeParameters
	}
	if class != nil {
		parameters = class.TypeParameters
	}
	for index, parameter := range parameters {
		if index < len(typ.Args) {
			bindings[parameter] = typ.Args[index]
		}
	}
	labels, values := []string{}, []string{}
	display := g.testInspection.Name(typ.Declaration, typ.Name)
	body := ""
	if record != nil {
		for _, statement := range record.Body {
			if field, ok := statement.(*ir.RecordField); ok {
				labels = append(labels, strconv.Quote(field.Name))
				values = append(values, g.testValue(testSubstitute(field.Type, bindings), "value["+strconv.Quote(field.Name)+"]"))
			}
		}
		body = "return [" + strconv.Quote(display) + ", 'record', [" + strings.Join(labels, ",") + "], [" + strings.Join(values, ",") + "]];"
	} else if class != nil {
		// The most-specific constructor value is registered before a later upcast.
		var fields func(*ir.Class, map[string]types.Type)
		fields = func(owner *ir.Class, bindings map[string]types.Type) {
			if owner.Superclass != nil {
				parentType := testSubstitute(owner.Superclass.ExprType(), bindings)
				if parent := g.testInspection.Classes[parentType.Declaration]; parent != nil && !parent.External {
					parentBindings := map[string]types.Type{}
					for index, parameter := range parent.TypeParameters {
						if index < len(parentType.Args) {
							parentBindings[parameter] = parentType.Args[index]
						}
					}
					fields(parent, parentBindings)
				}
			}
			for _, statement := range owner.Body {
				if field, ok := statement.(*ir.Field); ok {
					fieldName := strings.TrimPrefix(field.Name, "@")
					labels = append(labels, strconv.Quote(fieldName))
					values = append(values, g.testValue(testSubstitute(field.Type, bindings), "value["+strconv.Quote("__trb_"+fieldName)+"]"))
				}
			}
		}
		fields(class, bindings)
		body = "return [" + strconv.Quote(display) + ", 'record', [" + strings.Join(labels, ",") + "], [" + strings.Join(values, ",") + "]];"
	} else if enum != nil {
		body = "switch(value.kind) {"
		for _, statement := range enum.Body {
			if member, ok := statement.(*ir.EnumMember); ok {
				values = []string{}
				for _, field := range member.Fields {
					values = append(values, g.testValue(testSubstitute(field.Type, bindings), "value["+strconv.Quote(field.Name)+"]"))
				}
				body += "case " + strconv.Quote(member.Name) + ": return [" + strconv.Quote(display+"::"+member.Name) + ", 'variant', [], [" + strings.Join(values, ",") + "]];"
			}
		}
		body += "default: return ['<invalid enum>', 'variant', [], []];}"
	} else if typ.Kind == types.Range {
		body = "return ['', 'range', [], value];"
	} else if typ.Kind == types.Array && len(typ.Args) == 1 {
		body = "return ['', 'array', [], value.slice(0,16).map((item: any) => " + g.testValue(typ.Args[0], "item") + "), value.length];"
	} else if typ.Kind == types.Hash && len(typ.Args) == 2 {
		keyValue := "key"
		if typ.Args[0].Kind == types.Int {
			keyValue = "Number(key)"
		}
		body = "return ['', 'hash', [], Object.keys(value).map(key => [" + keyValue + ", () => " + g.testValue(typ.Args[1], "value[key]") + "])];"
	}
	helper := "function " + name + "<T>(input: T): T { if (typeof input !== 'object' || input === null) return input; const value = input as any; const views = " + testViewMap + "; if (!views.has(value)) views.set(value, () => {" + body + "}); return input; }"
	g.testViews.bodies = append(g.testViews.bodies, helper)
	return name + "(" + value + ")"
}

func (g *generator) testViewSupport() {
	if g.testViews == nil {
		return
	}
	for _, body := range g.testViews.bodies {
		g.line(body)
	}
}

func (g *generator) testInspectionRuntime() {
	g.line(`function trbTestInspect(input: any): string {
 const active = new Set<object>(); let budget = 256;
 const inspect = (value: any, depth: number): string => {
  if (value === null || value === undefined) return "nil";
  if (budget <= 0 || depth >= 8) return "..."; budget--;
  if (typeof value === "string") { const points: string[] = []; for (const point of value) { points.push(point); if (points.length > 128) break; } return JSON.stringify(points.slice(0,128).join("")) + (points.length > 128 ? "..." : ""); }
  if (typeof value === "boolean") return String(value);
  if (typeof value === "number") {
   if (Number.isNaN(value)) return "NaN"; if (value === Infinity) return "Infinity"; if (value === -Infinity) return "-Infinity"; if (Object.is(value,-0)) return "-0";
   const text = String(value); if (!/[eE]/.test(text)) return text;
   const [mantissa,exp] = text.toLowerCase().split("e"); const negative = mantissa!.startsWith("-"); const [whole,fraction = ""] = (negative ? mantissa!.slice(1) : mantissa!).split("."); const digits = whole! + fraction; const position = whole!.length + Number(exp);
   return (negative ? "-" : "") + (position <= 0 ? "0." + "0".repeat(-position) + digits : position >= digits.length ? digits + "0".repeat(position-digits.length) : digits.slice(0,position) + "." + digits.slice(position));
  }
  if (typeof value === "function") return "<function>";
  if (typeof value !== "object") return "<value>";
  if (active.has(value)) return "<cycle>"; active.add(value);
  try {
   const views = (globalThis as any)[Symbol.for("type-rb.test.views")] as WeakMap<object, () => any> | undefined;
   const view = views?.get(value)?.();
   if (view?.[1] === 'range') return String(value[0]) + (value[2] ? "..." : "..") + String(value[1]);
   if (view && (view[1] === 'record' || view[1] === 'variant')) {
    const [name,kind,labels,values] = view; if (kind === 'variant' && values.length === 0) return name;
    const parts = values.slice(0,16).map((item: any,index: number) => (kind === 'record' ? labels[index]+": " : "") + inspect(item,depth+1));
    if (values.length > 16) parts.push("..."); return name+"("+parts.join(", ")+")";
   }
   if (Array.isArray(value) || value instanceof Uint8Array) {
    const items = view?.[1] === 'array' ? view[3] : Array.from(value.slice(0,16));
    const parts = items.map((item: any) => inspect(item,depth+1)); if (value.length > 16) parts.push("...");
    return (value instanceof Uint8Array ? "Bytes[" : "[") + parts.join(", ") + "]";
   }
   if (view?.[1] !== 'hash') return "<value>";
   const entries: [string | number, () => any][] = view[3];
   const ordered = entries.sort((a,b) => {
    if (typeof a[0] === 'number' && typeof b[0] === 'number') return a[0]-b[0];
    const left = Array.from(String(a[0]), c => c.codePointAt(0)!); const right = Array.from(String(b[0]), c => c.codePointAt(0)!);
    for (let index=0;index<Math.min(left.length,right.length);index++) { if(left[index]!==right[index]) return left[index]!-right[index]!; }
    return left.length-right.length;
   });
   const parts = ordered.slice(0,16).map(([key,get]) => trbTestInspect(key)+" => "+inspect(get(),depth+1)); if (ordered.length > 16) parts.push("...");
   return "{"+parts.join(", ")+"}";
  } finally { active.delete(value); }
 };
 return inspect(input,0);
}`)
}
