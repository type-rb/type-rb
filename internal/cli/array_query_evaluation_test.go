package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestArrayQueriesObserveArgumentMutations(t *testing.T) {
	operations := []struct{ name, call string }{
		{"include", "include?"},
		{"index", "index"},
		{"count", "count"},
	}
	scenarios := []struct{ name, setup, mutation, target, index, count string }{
		{"growth", "mut values := [1, 2]", "values.push(3)", "3", "2", "1"},
		{"spare_capacity", "mut values := [1, 2, 3, 4]\nvalues.pop()\nvalues.pop()", "values.push(3)", "3", "2", "1"},
		{"shrink", "mut values := [1, 3, 3]", "values.pop()\nvalues.pop()", "3", "nil", "0"},
		{"shift", "mut values := [1, 3, 3]", "values.shift()", "3", "0", "2"},
		{"replace", "mut values := [1, 2]", "values[1] = 3", "3", "1", "1"},
		{"rebind", "mut values := [1, 3]", "values = [3, 3, 3]", "3", "1", "1"},
		{"rebind_and_grow", "mut values := [1, 2]\nmut original := values", "values = [3]\noriginal.push(3)", "3", "2", "1"},
	}
	var source, calls, want strings.Builder
	for _, operation := range operations {
		for _, scenario := range scenarios {
			name := "query_" + operation.name + "_" + scenario.name
			expected := scenario.count
			if operation.name == "index" {
				expected = scenario.index
			} else if operation.name == "include" {
				expected = fmt.Sprint(scenario.count != "0")
			}
			check := "puts(result == " + expected + ")"
			if operation.name == "index" && expected != "nil" {
				check = "if result == nil\nputs(false)\nelse\n" + check + "\nend"
			}
			fmt.Fprintf(&source, `def %s()
%s
mut calls := 0
change := fn(): Integer
calls += 1
%s
return %s
end
result := values.%s(change())
%s
puts(calls == 1)
end
`, name, scenario.setup, scenario.mutation, scenario.target, operation.call, check)
			calls.WriteString(name + "()\n")
			want.WriteString("true\ntrue\n")
		}
	}
	source.WriteString("def main()\n" + calls.String() + "end\n")
	runPortableExecutionCase(t, source.String(), want.String(), "")
}

func TestArrayQueriesRetainSourceScopeAndTypedArguments(t *testing.T) {
	source := `def receiver(mut events: Array<String>, values: Array<String>): Array<String>
events.push("receiver")
return values
end

def argument(mut events: Array<String>, mut values: Array<String>): String
events.push("argument")
values.push("追加")
return "追加"
end

def main()
values := [4, 4, 7]
puts(values.include?(values[0]))
position := values.index(values[2])
if position == nil
puts(false)
else
puts(position == 2)
end
puts(values.count(values[0]) == 2)
flags := [true, false]
puts(flags.include?(false))
flag_position := flags.index(false)
if flag_position == nil
puts(false)
else
puts(flag_position == 1)
end
puts(flags.count(false) == 1)
mut events: Array<String> := []
mut labels := ["before"]
puts(receiver(events, labels).include?(argument(events, labels)))
puts(events.join(",") == "receiver,argument")
missing: Array<String>? := nil
puts(missing&.include?(argument(events, labels)) == nil)
puts(missing&.index(argument(events, labels)) == nil)
puts(missing&.count(argument(events, labels)) == nil)
puts(events.size() == 2)
end
`
	runPortableExecutionCase(t, source, strings.Repeat("true\n", 12), "")
}
