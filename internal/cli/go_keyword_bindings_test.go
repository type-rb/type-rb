package cli

import "testing"

func TestGoKeywordBindingsPreserveScopesAcrossBackendsAndREPL(t *testing.T) {
	source := `record Value
range: Integer
end
def _range(range: Integer): Integer
return range + 1
end
def add(*, range: Integer, map: Integer): Integer
return range + map
end
def main()
mut range := (1..2)
trb_keyword_range := 30
mut map := 0
range.to_a().each do |range|
map += range
end
callback := fn(range: Integer): Integer
return range + map
end
puts(callback(4))
puts(add(range: 5, map: 6))
puts(_range(7))
puts(Value.new(range: 9).range)
puts(trb_keyword_range)
range = (3..4)
range.each { |range| puts(range) }
{a: 10}.each { |_key, map| puts(map) }
puts(map)
end
`
	runPortableExecutionCase(t, source, "7\n11\n8\n9\n30\n3\n4\n10\n3\n", "")
}

func TestIterationBindingsShadowReceiversAcrossBackendsAndREPL(t *testing.T) {
	source := `def main()
values := [10, 20]
values.each { |values| puts(values) }
values.each.with_index { |values, index| puts(values + index) }
values.each.with_index { |item, values| puts(item + values) }
entries := {a: 3}
entries.each do |entries, value|
puts(entries)
puts(value)
end
numbers := {4 => 5}
numbers.each { |key, numbers| puts(key + numbers) }
mut live := [1]
mut peer := live
mut visits := 0
live.each do |live|
visits += 1
if live == 1
peer.push(2)
end
end
puts(visits)
mut calls := 0
source := fn(): Array<Integer>
calls += 1
return [7]
end
source().each { |source| puts(source) }
puts(calls)
end
`
	runPortableExecutionCase(t, source, "10\n20\n10\n21\n10\n21\na\n3\n9\n2\n7\n1\n", "")
}
