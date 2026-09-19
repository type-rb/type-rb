package cli

import "testing"

func TestEmbeddedCollectionBlocksAcrossExecutionPaths(t *testing.T) {
	source := `def count(values: Array<Integer>): Integer
return values.size()
end

def pair(left: Integer, right: Integer): Integer
return left * 10 + right
end

def marker(mut events: Array<Integer>, value: Integer): Array<Integer>
events.push(value)
return [value]
end

record Box
values: Array<Integer> = [1, 2].map { |item| item + 1 }
end

def main()
puts([1, 2].map { |item| item + 1 }[1])
puts(count([1, 2].map do |item|
# Preserve a real statement body within the call.
if item == 1
item += 1
end
item * 2
end))
puts(([1, 2].reduce(0) { |sum, item| sum + item }) + 7)
rows := [[1].map { |item| item + 1 }, [2].map { |item| item + 1 }]
puts(rows[1][0])
lookup := {"answer" => [40].map { |item| item + 2 }}
puts(lookup["answer"][0])
puts(true ? [1].map { |item| item + 2 }[0] : 0)
puts([1, 2].map { |item| item * 2 }.select { |item| item > 2 }.size())
puts((1..3).reduce(0) do |sum, item|
sum + item
end)
mut events: Array<Integer> := []
puts(pair(marker(events, 1).reduce(0) { |sum, item| sum + item }, marker(events, 2).reduce(0) { |sum, item| sum + item }))
puts(events[0])
puts(events[1])
box := Box.new()
puts(box.values[1])
puts([2].map do |item|; item; end[0])
puts(count([1, 2].map do |item|
inner := [item].map { |value| value + 1 }
inner[0]
end))
end
`
	runPortableExecutionCase(t, source, "3\n2\n10\n3\n42\n3\n1\n6\n12\n1\n2\n3\n2\n2\n", "")
}
