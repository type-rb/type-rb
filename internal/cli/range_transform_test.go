package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestRangeTransformsStreamAcrossExecutionPaths(t *testing.T) {
	operations := []struct{ name, call, parameters, tail, check string }{
		{"any", "any?", "item", "item == 2", "result == true"},
		{"all", "all?", "item", "item < 2", "result == false"},
		{"none", "none?", "item", "item == 2", "result == false"},
		{"find", "find", "item", "item == 2", "result != nil && result == 2"},
		{"find_index", "find_index", "item", "item == 2", "result != nil && result == 1"},
	}
	var source, calls, want strings.Builder
	source.WriteString(compareArrayValues)
	for _, operation := range operations {
		fmt.Fprintf(&source, "def %s()\nmut seen: Array<Integer> := []\nresult := (1..9007199254740991).%s do |%s|\nseen.push(item)\n%s\nend\nputs(same(seen, [1, 2]))\nputs(%s)\nend\n", operation.name, operation.call, operation.parameters, operation.tail, operation.check)
		fmt.Fprintf(&calls, "%s()\n", operation.name)
		want.WriteString("true\ntrue\n")
	}
	source.WriteString(`def bounds(mut events: Array<Integer>): Range<Integer>
events.push(1)
return -2..1
end

def initial(mut events: Array<Integer>): Integer
events.push(2)
return 10
end

def ordinary()
mut span := -2..1
mapped := span.map.with_index do |item, index|
span = 90..100
position := index
index = 100
item + position
end
puts(same(mapped, [-2, 0, 2, 4]))
selected := (-2...2).select.with_index do |item, index|
item = 99
index = 100
item < index
end
puts(same(selected, [-2, -1, 0, 1]))
mut events: Array<Integer> := []
total := bounds(events).reduce(initial(events)) do |sum, item|
events.push(item)
sum + item
end
puts(total == 8)
puts(same(events, [1, 2, -2, -1, 0, 1]))
empty := (3..1).map { |item| item }
puts(empty.empty?())
excluded := (2...2).select { |item| item == 2 }
puts(excluded.empty?())
sum := (3..1).reduce(initial(events)) { |sum, item| sum + item }
puts(sum == 10)
puts(events.size() == 7)
maximum := (9007199254740991..9007199254740991).map.with_index { |item, index| item - index }
puts(same(maximum, [9007199254740991]))
minimum := (-9007199254740991..-9007199254740990).map { |item| item }
puts(same(minimum, [-9007199254740991, -9007199254740990]))
end
`)
	source.WriteString("def main()\n" + calls.String() + "ordinary()\nend\n")
	want.WriteString(strings.Repeat("true\n", 10))
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

func TestRangeTransformBlockFailureDoesNotMaterializeTheSource(t *testing.T) {
	for _, operation := range []struct{ call, parameters, tail string }{
		{"map", "item", "item"},
		{"select", "item", "item > 0"},
		{"reduce(0)", "sum, item", "sum + item"},
	} {
		t.Run(operation.call, func(t *testing.T) {
			source := fmt.Sprintf("def main()\n_result := (1..9007199254740991).%s do |%s|\nputs(item)\n_failure := [0][item]\n%s\nend\nend\n", operation.call, operation.parameters, operation.tail)
			runPortableExecutionCase(t, source, "1\n", "out of bounds")
		})
	}
}
