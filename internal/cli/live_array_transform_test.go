package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestLiveArrayTransformsAcrossExecutionPaths(t *testing.T) {
	operations := []struct{ name, call, parameters, tail, check string }{
		{"map", "map", "item", "item", "same(result, seen)"},
		{"select", "select", "item", "item = 99\ntrue", "same(result, seen)"},
		{"reduce", "reduce(0)", "sum, item", "sum + item", "result == sum_values(seen)"},
		{"any", "any?", "item", "false", "result == false"},
		{"all", "all?", "item", "true", "result == true"},
		{"none", "none?", "item", "false", "result == true"},
		{"find", "find", "item", "false", "result == nil"},
		{"find_index", "find_index", "item", "false", "result == nil"},
		{"sort", "sort_by", "item", "item = 99\n0", "same(result, seen)"},
		{"sort_descending", "sort_by_descending", "item", "item = 99\n0", "same(result, seen)"},
	}
	scenarios := []struct{ name, setup, mutation, seen string }{
		{"growth", "mut values := [1, 2]", "values.push(3)\nvalues[1] = 8", "[1, 8, 3]"},
		{"spare_capacity", "mut values := [1, 2, 3, 4]\nvalues.pop()\nvalues.pop()", "values.push(3)\nvalues[1] = 8", "[1, 8, 3]"},
		{"shortening", "mut values := [1, 2, 3]", "values.pop()\nvalues.pop()", "[1]"},
		{"shift", "mut values := [1, 2, 3]", "values.shift()", "[1, 3]"},
		{"unshift", "mut values := [1, 2]", "values.unshift(9)", "[1, 1, 2]"},
		{"rebinding", "mut values := [1, 2]", "values = [9]", "[1, 2]"},
		{"empty", "mut values: Array<Integer> := []", "values.push(3)", "[]"},
	}
	var source, calls, want strings.Builder
	source.WriteString(compareArrayValues)
	for _, operation := range operations {
		for _, scenario := range scenarios {
			name := operation.name + "_" + scenario.name
			// Most cases traverse a readonly alias. Rebinding exercises the
			// mutable receiver variable itself, which must not retarget traversal.
			alias, receiver := "source := values\n", "source"
			if scenario.name == "rebinding" {
				alias, receiver = "", "values"
			}
			fmt.Fprintf(&source, "def %s()\nputs(%q)\n%s\n%smut seen: Array<Integer> := []\nresult := %s.%s do |%s|\nseen.push(item)\nif seen.size() == 1\n%s\nend\n%s\nend\nputs(same(seen, %s))\nputs(%s)\nend\n", name, name, scenario.setup, alias, receiver, operation.call, operation.parameters, scenario.mutation, operation.tail, scenario.seen, operation.check)
			fmt.Fprintf(&calls, "%s()\n", name)
			want.WriteString(name + "\ntrue\ntrue\n")
		}
	}
	source.WriteString("def main()\n" + calls.String() + "end\n")
	for _, suspend := range []bool{false, true} {
		t.Run(fmt.Sprintf("suspending_%t", suspend), func(t *testing.T) {
			program := source.String()
			if suspend {
				program = strings.ReplaceAll(program, "seen.push(item)", "_checkpoint := [0].concurrent_map { |marker| marker }\nseen.push(item)")
			}
			runPortableExecutionCase(t, program, want.String(), "")
		})
	}
}

func TestLiveArrayTransformRetainedValuesAndShortCircuiting(t *testing.T) {
	source := compareArrayValues + `def main()
mut values := [1, 2]
mapped := values.map.with_index do |item, index|
if index == 0
values.push(3)
values[1] = 8
end
position := index
index = 100
item + position
end
puts(same(mapped, [1, 9, 5]))
values = [1, 2]
selected := values.select.with_index do |item, index|
if index == 0
values.push(3)
values[1] = 8
end
item = 99
index = 100
true
end
puts(same(selected, [1, 8, 3]))
values = [1, 2]
mut visits := 0
found := values.find do |item|
visits += 1
if item == 1
values.push(3)
values.push(4)
end
matched := item == 3
item = 99
matched
end
if found != nil
puts(found == 3)
else
puts(false)
end
puts(visits == 3)
values = [1, 2]
position := values.find_index do |item|
if item == 1
values.push(3)
end
item == 3
end
if position != nil
puts(position == 2)
else
puts(false)
end
values = [1, 2]
answer := values.any? do |item|
if item == 1
values.push(3)
end
item == 3
end
puts(answer)
mut rows := [[1], [2]]
retained := rows.map do |row|
if row[0] == 1
rows.push([3])
rows[1] = [8]
rows[0] = [9]
end
row
end
puts(retained.size() == 3 && retained[0][0] == 1 && retained[1][0] == 8 && retained[2][0] == 3)
mut aliased := rows[1]
aliased.push(10)
puts(same(retained[1], [8, 10]))
end
`
	runPortableExecutionCase(t, source, strings.Repeat("true\n", 8), "")
}

const compareArrayValues = `def sum_values(values: Array<Integer>): Integer
mut result := 0
values.each { |value| result += value }
return result
end

def same(left: Array<Integer>, right: Array<Integer>): Boolean
if left.size() != right.size()
return false
end
mut index := 0
while index < left.size()
if left[index] != right[index]
return false
end
index += 1
end
return true
end

`
