package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestSafeTransformsAcrossTargetsAndREPL(t *testing.T) {
	operations := []struct{ name, call, parameters, value, inspect, present string }{
		{"map", "map", "value", "value + 1", "puts(result[0])", "3"},
		{"indexed_map", "map.with_index", "value, index", "value + index", "puts(result[1])", "2"},
		{"select", "select", "value", "value > 1", "puts(result.size())", "1"},
		{"reduce", "reduce(0)", "sum, value", "sum + value", "puts(result)", "3"},
		{"any", "any?", "value", "value > 1", "puts(result)", "true"},
		{"all", "all?", "value", "value > 0", "puts(result)", "true"},
		{"none", "none?", "value", "value < 0", "puts(result)", "true"},
		{"find", "find", "value", "value == 1", "puts(result)", "1"},
		{"find_index", "find_index", "value", "value == 1", "puts(result)", "1"},
		{"sort_by", "sort_by", "value", "value", "puts(result[0])", "1"},
		{"sort_by_descending", "sort_by_descending", "value", "value", "puts(result[0])", "2"},
	}
	var source, calls, want strings.Builder
	for _, operation := range operations {
		fmt.Fprintf(&source, `def probe_%s(values: Array<Integer>?)
mut visits := 0
result := values&.%s do |%s|
visits += 1
%s
end
puts(result == nil)
if result != nil
%s
end
puts(visits)
end
`, operation.name, operation.call, operation.parameters, operation.value, operation.inspect)
		fmt.Fprintf(&calls, "probe_%s(nil)\nprobe_%s([2, 1])\n", operation.name, operation.name)
		visits := "2"
		if operation.name == "any" {
			visits = "1"
		}
		want.WriteString("true\n0\nfalse\n" + operation.present + "\n" + visits + "\n")
	}
	source.WriteString("def main()\n" + calls.String() + "end\n")
	for _, suspending := range []bool{false, true} {
		t.Run(fmt.Sprintf("suspending_%t", suspending), func(t *testing.T) {
			program := source.String()
			if suspending {
				program = strings.ReplaceAll(program, "visits += 1", "_checkpoint := [0].concurrent_map { |marker| marker }\nvisits += 1")
			}
			runPortableExecutionCase(t, program, want.String(), "")
		})
	}
}

func TestSafeIterationSkipsArgumentsAndKeepsControlTransfers(t *testing.T) {
	source := `def mark(mut log: Array<Integer>, value: Integer): Integer
log.push(value)
return value
end

def values(mut log: Array<Integer>, present: Boolean): Array<Integer>?
log.push(7)
if present
return [1, 2, 3]
end
return nil
end

def walk(values: Array<Integer>?): Integer
values&.each.with_index do |value, index|
next if index == 0
return value
end
return 9
end

def inspect_collections(array: Array<Integer>?, range: Range<Integer>?, hash: Hash<String, Integer>?)
mut visits := 0
array&.each { |value| visits += value }
range&.each { |value| visits += value }
hash&.each { |_key, value| visits += value }
puts(visits)
end

def batches(values: Array<Integer>?, mut log: Array<Integer>)
values&.each_slice(mark(log, 2)) do |batch|
puts(batch.size())
end
end

def main()
inspect_collections(nil, nil, nil)
inspect_collections([1], (1..2), {"a": 4})
puts(walk(nil))
puts(walk([1, 2, 3]))
mut log: Array<Integer> := []
missing := values(log, false)&.reduce(mark(log, 5)) { |sum, value| sum + value }
puts(missing == nil)
puts(log.size())
present := values(log, true)&.reduce(mark(log, 5)) { |sum, value| sum + value }
if present != nil
puts(present)
end
puts(log.size())
batches(nil, log)
puts(log.size())
batches([1, 2, 3], log)
puts(log.size())
values(log, false)&.each_slice(mark(log, 2)) { |batch| puts(batch.size()) }
puts(log.size())
nonnullable := [1, 2]&.map { |value| value + 1 }
puts(nonnullable.size())
end
`
	runPortableExecutionCase(t, source, "0\n8\n9\n2\ntrue\n1\n11\n3\n3\n2\n1\n4\n5\n2\n", "")
}

func TestSafeConcurrentMapSkipsLimitAndPreservesNullableResult(t *testing.T) {
	source := `def limit(mut calls: Array<Integer>): Integer
calls[0] += 1
return 2
end

def execute(values: Array<Integer>?, mut calls: Array<Integer>)
result := values&.concurrent_map(limit: limit(calls)) { |value| value + 1 }
puts(result == nil)
if result != nil
puts(result[0])
puts(result[1])
end
end

def main()
mut calls := [0]
execute(nil, calls)
puts(calls[0])
execute([3, 1], calls)
puts(calls[0])
end
`
	runPortableExecutionCase(t, source, "true\n0\nfalse\n4\n2\n1\n", "")
}

func TestSafeIterationWithNullableContainerLiterals(t *testing.T) {
	runPortableExecutionCase(t, `def inspect_hash(value: Hash<String, Integer>?)
if value != nil
puts(value.size())
value&.each { |_key, item| puts(item) }
end
end

def inspect_arrays(values: Array<Array<Integer>?>?)
if values != nil
values&.each do |value|
if value != nil
puts(value.size())
end
end
end
end

def make_hash(): Hash<String, Integer>?
return {"a": 7}
end

def main()
inspect_hash({})
inspect_hash({"a": 5})
inspect_hash(make_hash())
value: Hash<String, Integer>? := {"a": 9}
inspect_hash(value)
inspect_arrays([[], [1, 2], nil])
end
`, "0\n1\n5\n1\n7\n1\n9\n0\n2\n", "")
}

func TestSafeIterationChainingGenericsAndRetainedReceivers(t *testing.T) {
	runPortableExecutionCase(t, `def copy_items<T>(values: Array<T>?): Array<T>?
return values&.map { |value| value }
end

def count_values(values: Array<Integer>?): Integer?
return values&.map() { |value| value + 1 }&.size()
end

def sum_range(values: Range<Integer>?): Integer?
return values&.reduce(0) { |sum, value| sum + value }
end

def keep(values: Array<Integer>?): Array<Integer?>?
return values&.map do |value|
optional: Integer? := value
optional
end
end

def main()
puts(copy_items<String>(nil) == nil)
words := copy_items<String>(["one", "two"])
if words != nil
puts(words.join("|"))
end
puts(count_values(nil) == nil)
count := count_values([1, 2])
if count != nil
puts(count)
end
puts(sum_range(nil) == nil)
sum := sum_range(1..3)
if sum != nil
puts(sum)
end
items := keep([1])
if items != nil
item := items[0]
if item != nil
puts(item)
end
end
mut values: Array<Integer>? := [1, 2]
mut calls := 0
result := values&.map.with_index do |value, index|
calls += 1
values = [9]
value + index
end
if result != nil
puts(result[0])
puts(result[1])
end
puts(calls)
end
`, "true\none|two\ntrue\n2\ntrue\n6\n1\n1\n3\n2\n", "")
}

func TestSafeIterationRejectsInvalidNullableUses(t *testing.T) {
	for _, test := range []struct{ body, diagnostic string }{
		{"values: Array<Integer>? := nil\nvalues.map { |value| value }", "nullable iteration source requires safe navigation"},
		{"values: Array<Integer>? := nil\nvalues.each { |value| puts(value) }", "nullable iteration source requires safe navigation"},
		{"values: Array<Integer>? := nil\nvalues&.each.with_index { |value, index| puts(value + index) }\nresult := values&.map { |value| value }\nputs(result.size())", "has no member"},
		{"values: Array<Integer>? := nil\nresult := values&.each { |value| puts(value) }", "void"},
		{"[1].map&.with_index { |value, index| value + index }", "safe navigation must precede"},
		{"[1].each&.with_index() { |value, index| puts(value + index) }", "safe navigation must precede"},
		{"values: Array<Integer>? := nil\nvalues&.sort_by { |_value| true }", "portable natural order"},
	} {
		t.Run(test.diagnostic+test.body, func(t *testing.T) {
			runPortableExecutionCase(t, "def main()\n"+test.body+"\nend\n", "", test.diagnostic)
		})
	}
}

func TestSafeIterationNestedWorkersPreserveOuterBindings(t *testing.T) {
	runPortableExecutionCase(t, `def main()
value := 9
error := "kept"
marker := 8
values: Array<Integer>? := [1, 2]
result := values&.map do |value|
collected := [3, 4].concurrent_map { |value| value + 1 }
value + collected[0]
end
if result != nil
puts(result[0])
puts(result[1])
end
puts(value)
puts(error)
puts(marker)
end
`, "5\n6\n9\nkept\n8\n", "")
}
