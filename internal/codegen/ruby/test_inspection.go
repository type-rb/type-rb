package ruby

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

func (g *generator) testView(display string, variant bool, labels, values []string) {
	g.line("def __trb_test_view__; ["+strconv.Quote(display)+", "+strconv.FormatBool(variant)+", ["+strings.Join(labels, ",")+"], ["+strings.Join(values, ",")+"]]; end", "")
}

func (g *generator) testRecordView(record *ir.Record, name string, fields []*ir.RecordField) {
	if g.testInspection == nil || !g.testInspection.Enabled {
		return
	}
	labels, values := []string{}, []string{}
	for _, field := range fields {
		labels = append(labels, strconv.Quote(field.Name))
		values = append(values, "self."+field.Name)
	}
	g.line(name+".class_eval do", "")
	g.indent++
	g.testView(g.testInspection.Name(record.Declaration, record.Name), false, labels, values)
	g.indent--
	g.line("end", "")
}

func (g *generator) testClassView(class *ir.Class, fields []*ir.Field) {
	if g.testInspection == nil || !g.testInspection.Enabled {
		return
	}
	labels, values := []string{}, []string{}
	for _, field := range fields {
		labels = append(labels, strconv.Quote(strings.TrimPrefix(field.Name, "@")))
		values = append(values, "@"+strings.TrimPrefix(field.Name, "@"))
	}
	g.line("def __trb_test_view__", "")
	g.indent++
	g.line("names = ["+strings.Join(labels, ",")+"]; values = ["+strings.Join(values, ",")+"]", "")
	if class.Superclass != nil {
		g.line("if defined?(super); inherited = super; names = inherited[2] + names; values = inherited[3] + values; end", "")
	}
	g.line("["+strconv.Quote(g.testInspection.Name(class.Declaration, class.Name))+", false, names, values]", "")
	g.indent--
	g.line("end", "")
}

func (g *generator) testEnumViews(enum *ir.Enum) {
	if g.testInspection == nil || !g.testInspection.Enabled {
		return
	}
	name := g.declarationName(enum.Name, enum.Declaration)
	if !enumHasPayload(enum) {
		value := "name.to_s"
		if enum.RawType.Kind != "" {
			value = "raw_value"
		}
		g.line(name+".class_eval { def __trb_test_scalar__; "+value+"; end }", "")
		return
	}
	for _, statement := range enum.Body {
		member, ok := statement.(*ir.EnumMember)
		if !ok {
			continue
		}
		values := []string{}
		for _, field := range member.Fields {
			values = append(values, "self."+field.Name)
		}
		target := name + "::" + member.Name
		if !enumHasPayload(enum) || len(member.Fields) == 0 {
			target += ".class"
		}
		g.line(target+".class_eval do", "")
		g.indent++
		g.testView(g.testInspection.Name(enum.Declaration, enum.Name)+"::"+member.Name, true, nil, values)
		g.indent--
		g.line("end", "")
	}
}

func (g *generator) testInspectionRuntime() {
	g.line(`def trb_test_inspect(value, active = {}, budget = [256], depth = 0)
  return "nil" if value.nil?
  return trb_test_inspect(value.__trb_test_scalar__, active, budget, depth) if value.respond_to?(:__trb_test_scalar__)
  return "..." if budget[0] <= 0 || depth >= 8
  budget[0] -= 1
  compound = value.is_a?(Array) || value.is_a?(Hash) || value.respond_to?(:__trb_test_view__)
  if compound
    return "<cycle>" if active[value.object_id]
    active[value.object_id] = true
  end
  begin
    if value.respond_to?(:__trb_test_view__)
      name, variant, labels, values = value.__trb_test_view__
      return name if variant && values.empty?
      parts = values.first(16).each_with_index.map { |item,index| text = trb_test_inspect(item,active,budget,depth+1); variant ? text : labels[index] + ": " + text }
      parts << "..." if values.length > 16
      return name + "(" + parts.join(", ") + ")"
    end
    case value
    when true, false, Integer
      value.to_s
    when Float
      return "NaN" if value.nan?
      return value.positive? ? "Infinity" : "-Infinity" if value.infinite?
      return "-0" if value.zero? && (1.0 / value).negative?
      text = value.to_s
      if text.include?("e")
        mantissa, exponent = text.split("e"); negative = mantissa.start_with?("-"); mantissa = mantissa.delete_prefix("-")
        whole, fraction = mantissa.split("."); digits = whole + (fraction || ""); position = whole.length + exponent.to_i
        text = position <= 0 ? "0." + "0" * -position + digits : (position >= digits.length ? digits + "0" * (position - digits.length) : digits[0...position] + "." + digits[position..])
        text = "-" + text if negative
      end
      text = text.sub(/0+$/, "").sub(/\.$/, "") if text.include?(".")
      text
    when String
      if value.encoding == Encoding::BINARY
        parts = value.byteslice(0,16).bytes.map { |item| trb_test_inspect(item,active,budget,depth+1) }; parts << "..." if value.bytesize > 16
        return "Bytes[" + parts.join(", ") + "]"
      end
      points = value.scrub.each_char.take(129)
      JSON.generate(points.first(128).join) + (points.length > 128 ? "..." : "")
    when Symbol
      points = value.to_s.each_char.take(129)
      JSON.generate(points.first(128).join) + (points.length > 128 ? "..." : "")
    when Range
      value.begin.to_s + (value.exclude_end? ? "..." : "..") + value.end.to_s
    when Array
      parts = value.first(16).map { |item| trb_test_inspect(item,active,budget,depth+1) }; parts << "..." if value.length > 16
      "[" + parts.join(", ") + "]"
    when Hash
      entries = value.sort_by { |key,_item| key }
      parts = entries.first(16).map { |key,item| trb_test_inspect(key) + " => " + trb_test_inspect(item,active,budget,depth+1) }; parts << "..." if entries.length > 16
      "{" + parts.join(", ") + "}"
    when Proc
      "<function>"
    else
      "<value>"
    end
  ensure
    active.delete(value.object_id) if compound
  end
end`, "")
}
