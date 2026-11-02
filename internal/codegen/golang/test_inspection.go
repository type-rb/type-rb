package golang

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

// The underscore prevents a collision with the CamelCase source method mapping.
const testViewMethod = "TrbTest_view_"

func (g *generator) testRecordView(record *ir.Record, name string, fields []*ir.RecordField) {
	if g.testInspection == nil || !g.testInspection.Enabled {
		return
	}
	labels, values := []string{}, []string{}
	for _, field := range fields {
		labels = append(labels, strconv.Quote(field.Name))
		values = append(values, "self."+goIdentifier(field.Name, true))
	}
	g.testView(name+goTypeParameterArguments(record.TypeParameters), g.testInspection.Name(record.Declaration, record.Name), labels, values)
}

func (g *generator) testView(receiver, display string, labels, values []string) {
	g.line("func (self " + receiver + ") " + testViewMethod + "() (string, bool, []string, []any) {")
	g.line("return " + strconv.Quote(display) + ", false, []string{" + strings.Join(labels, ",") + "}, []any{" + strings.Join(values, ",") + "}")
	g.line("}")
}

func (g *generator) testClassView(class *ir.Class, name string, fields []*ir.Field) {
	if g.testInspection == nil || !g.testInspection.Enabled || class.External {
		return
	}
	labels, values := []string{}, []string{}
	for _, field := range fields {
		labels = append(labels, strconv.Quote(strings.TrimPrefix(field.Name, "@")))
		values = append(values, "self."+goFieldName(field.Name))
	}
	g.line("func (self *" + name + goTypeParameterArguments(class.TypeParameters) + ") " + testViewMethod + "() (string, bool, []string, []any) {")
	g.line("names := []string{" + strings.Join(labels, ",") + "}; values := []any{" + strings.Join(values, ",") + "}")
	// Inherited fields retain their source order and are read without user accessors.
	if class.Superclass != nil && g.testInspection.Classes[class.Superclass.ExprType().Declaration] != nil && !g.testInspection.Classes[class.Superclass.ExprType().Declaration].External {
		owner := strings.TrimPrefix(g.goType(class.Superclass.ExprType()), "*")
		if dot := strings.LastIndex(owner, "."); dot >= 0 {
			owner = owner[dot+1:]
		}
		if bracket := strings.Index(owner, "["); bracket >= 0 {
			owner = owner[:bracket]
		}
		g.line("if self." + owner + " != nil { _, _, inheritedNames, inheritedValues := self." + owner + "." + testViewMethod + "(); names = append(inheritedNames, names...); values = append(inheritedValues, values...) }")
	}
	g.line("return " + strconv.Quote(g.testInspection.Name(class.Declaration, class.Name)) + ", false, names, values")
	g.line("}")
}

func (g *generator) testEnumView(enum *ir.Enum, name string) {
	if g.testInspection == nil || !g.testInspection.Enabled {
		return
	}
	payload := enumHasPayload(enum)
	if !payload {
		g.line("func (self " + name + ") TrbTest_scalar_() any {")
		if enum.RawType.Kind != "" {
			g.line("return " + g.goType(enum.RawType) + "(self)")
		} else {
			g.line("switch self {")
			for _, statement := range enum.Body {
				if member, ok := statement.(*ir.EnumMember); ok {
					g.line("case " + goNominalConstantIdentifier(name, member.Name) + ": return " + strconv.Quote(member.Name))
				}
			}
			g.line("}; return \"<invalid enum>\"")
		}
		g.line("}")
		return
	}
	g.line("func (self " + name + goTypeParameterArguments(enum.TypeParameters) + ") " + testViewMethod + "() (string, bool, []string, []any) {")
	subject := "self"
	if payload {
		subject += ".Kind"
	}
	g.line("switch " + subject + " {")
	for _, statement := range enum.Body {
		member, ok := statement.(*ir.EnumMember)
		if !ok {
			continue
		}
		constant := goNominalConstantIdentifier(name, member.Name)
		if payload {
			constant += "Tag"
		}
		values := []string{}
		for _, field := range member.Fields {
			value := "self." + goIdentifier(member.Name, true) + goIdentifier(field.Name, true)
			// Recursive enum fields are represented by an internal pointer; the renderer
			// unwraps it like nullable values without publishing the storage layout.
			values = append(values, value)
		}
		g.line("case " + constant + ": return " + strconv.Quote(g.testInspection.Name(enum.Declaration, enum.Name)+"::"+member.Name) + ", true, nil, []any{" + strings.Join(values, ",") + "}")
	}
	g.line("}")
	g.line("return \"<invalid enum>\", true, nil, nil")
	g.line("}")
}

func (g *generator) testInspectionRuntime() {
	for _, name := range []string{"reflect", "strconv", "strings", "sort", "math"} {
		g.requireImport(name, name)
	}
	g.line(`type trbTestView interface { TrbTest_view_() (string, bool, []string, []any) }
type trbTestVisit struct { kind reflect.Kind; typ reflect.Type; pointer uintptr }
func trbTestQuote(value string) string {
 count := 0; truncated := false
 var output strings.Builder; output.WriteByte('"')
 for _, point := range value {
  if count == 128 { truncated = true; break }; count++
  switch point {
  case '"', '\\': output.WriteByte('\\'); output.WriteRune(point)
  case '\b': output.WriteString("\\b")
  case '\f': output.WriteString("\\f")
  case '\n': output.WriteString("\\n")
  case '\r': output.WriteString("\\r")
  case '\t': output.WriteString("\\t")
  default: if point < 32 { output.WriteString("\\u00"); output.WriteByte("0123456789abcdef"[point / 16]); output.WriteByte("0123456789abcdef"[point % 16]) } else { output.WriteRune(point) }
  }
 }
 output.WriteByte('"'); if truncated { output.WriteString("...") }; return output.String()
}
func trbTestInspect(input any) string {
 active := map[trbTestVisit]bool{}; budget := 256
 var inspect func(reflect.Value, int) string
 inspect = func(value reflect.Value, depth int) string {
  if !value.IsValid() { return "nil" }
  for value.Kind() == reflect.Interface { if value.IsNil() { return "nil" }; value = value.Elem() }
  kind := value.Kind()
  if value.CanInterface() { if scalar, ok := value.Interface().(interface { TrbTest_scalar_() any }); ok { return inspect(reflect.ValueOf(scalar.TrbTest_scalar_()),depth) } }
  if kind == reflect.Ptr && value.IsNil() { return "nil" }
  if budget <= 0 || depth >= 8 { return "..." }
  if kind == reflect.Ptr || kind == reflect.Map || kind == reflect.Slice {
   if value.IsNil() && kind == reflect.Ptr { return "nil" }
   if !value.IsNil() {
    key := trbTestVisit{kind, value.Type(), uintptr(value.UnsafePointer())}
    if active[key] { budget--; return "<cycle>" }; active[key] = true; defer delete(active,key)
   }
  }
  if kind == reflect.Ptr {
   nominal := false; if value.CanInterface() { _, nominal = value.Interface().(trbTestView) }; if !nominal { return inspect(value.Elem(),depth) }
  }
  if budget <= 0 || depth >= 8 { return "..." }; budget--
  if value.CanInterface() { if view, ok := value.Interface().(trbTestView); ok {
   name, variant, labels, values := view.TrbTest_view_()
   if variant && len(values) == 0 { return name }
   parts := []string{}; limit := len(values); if limit > 16 { limit = 16 }
   for index := 0; index < limit; index++ { item := inspect(reflect.ValueOf(values[index]),depth+1); if !variant { item = labels[index]+": "+item }; parts = append(parts,item) }
   if limit < len(values) { parts = append(parts,"...") }; return name+"("+strings.Join(parts,", ")+")"
  } }
  switch kind {
  case reflect.Ptr: return inspect(value.Elem(),depth)
  case reflect.Bool: return strconv.FormatBool(value.Bool())
  case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64: return strconv.FormatInt(value.Int(),10)
  case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64: return strconv.FormatUint(value.Uint(),10)
  case reflect.Float32, reflect.Float64:
   number := value.Float(); if math.IsNaN(number) { return "NaN" }; if math.IsInf(number,1) { return "Infinity" }; if math.IsInf(number,-1) { return "-Infinity" }; if number == 0 && math.Signbit(number) { return "-0" }; return strconv.FormatFloat(number,'f',-1,64)
  case reflect.String: return trbTestQuote(value.String())
  case reflect.Func: return "<function>"
  case reflect.Slice, reflect.Array:
   if kind == reflect.Array && value.Type() == reflect.TypeOf([3]int{}) { operator := ".."; if value.Index(2).Int() != 0 { operator = "..." }; return strconv.FormatInt(value.Index(0).Int(),10)+operator+strconv.FormatInt(value.Index(1).Int(),10) }
   parts := []string{}; limit := value.Len(); if limit > 16 { limit = 16 }
   for index := 0; index < limit; index++ { parts = append(parts,inspect(value.Index(index),depth+1)) }
   if limit < value.Len() { parts = append(parts,"...") }; prefix := "["; if value.Type().Elem().Kind() == reflect.Uint8 { prefix = "Bytes[" }; return prefix+strings.Join(parts,", ")+"]"
  case reflect.Map:
   // Stable order is independent from map iteration and target field names.
   type entry struct { key reflect.Value; value reflect.Value }; entries := []entry{}
   for _, key := range value.MapKeys() { entries = append(entries,entry{key,value.MapIndex(key)}) }
   sort.Slice(entries,func(i,j int) bool { if entries[i].key.Kind() == reflect.String { return entries[i].key.String() < entries[j].key.String() }; return entries[i].key.Int() < entries[j].key.Int() })
   parts := []string{}; limit := len(entries); if limit > 16 { limit = 16 }
   for _, entry := range entries[:limit] { parts = append(parts,trbTestInspect(entry.key.Interface())+" => "+inspect(entry.value,depth+1)) }
   if limit < len(entries) { parts = append(parts,"...") }; return "{"+strings.Join(parts,", ")+"}"
  default: return "<value>"
  }
 }
 return inspect(reflect.ValueOf(input),0)
}`)
}
