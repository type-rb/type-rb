package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestArrayLookupsRetainReceiverBeforeArguments(t *testing.T) {
	scenarios := []struct{ name, setup, mutation, limit string }{
		{"rebind", "mut values := [1, 2]", "values = [8, 9, 10]", "2"},
		{"growth", "mut values := [1, 2]", "values.push(3)", "3"},
		{"rebind_and_grow", "mut values := [1, 2]\nmut original := values", "values = [8]\noriginal.push(3)", "3"},
		{"shrink", "mut values := [1, 2, 3]", "values.pop()", "2"},
	}
	var source, calls, want strings.Builder
	source.WriteString("import trb/std/result\n")
	for _, method := range []string{"slice", "try_slice", "try_fetch"} {
		for _, scenario := range scenarios {
			name := "lookup_" + method + "_" + scenario.name
			argument := "0...change()"
			check := "puts(result.size() == " + scenario.limit + ")\nputs(result.first() == 1)"
			if method == "try_fetch" {
				argument = "change() - 1"
				check = "puts(result == " + scenario.limit + ")"
			}
			call := "values." + method + "(" + argument + ")"
			if method == "slice" {
				check = "result := " + call + "\n" + check
			} else {
				check = "case " + call + "\nwhen Result::Ok(result)\n" + check + "\nwhen Result::Err(error)\nputs(error.message)\nend"
			}
			fmt.Fprintf(&source, `def %s()
%s
mut calls := 0
change := fn(): Integer
calls += 1
%s
return %s
end
%s
puts(calls == 1)
end
`, name, scenario.setup, scenario.mutation, scenario.limit, check)
			calls.WriteString(name + "()\n")
			want.WriteString("true\ntrue\n")
			if method != "try_fetch" {
				want.WriteString("true\n")
			}
		}
	}
	source.WriteString("def main()\n" + calls.String() + "end\n")
	runPortableExecutionCase(t, source.String(), want.String(), "")
}

func TestArrayLookupErrorsAndOptionalReceiversRetainIdentity(t *testing.T) {
	source := `import trb/std/result

def main()
mut values := [1, 2]
original := values
mut calls := 0
change := fn(): Integer
calls += 1
values = [8, 9, 10]
return 3
end
case values.try_slice(0...change())
when Result::Ok(_result)
puts(false)
when Result::Err(error)
puts(error.size == 2)
puts(error.finish == 3)
end
values = original
case values.try_fetch(change() - 1)
when Result::Ok(_result)
puts(false)
when Result::Err(error)
puts(error.size == 2)
puts(error.index == 2)
end
missing: Array<Integer>? := nil
puts(missing&.slice(0...change()) == nil)
puts(missing&.try_slice(0...change()) == nil)
puts(missing&.try_fetch(change()) == nil)
puts(calls == 2)
puts(values.slice(0...values.size()).last() == 10)
end
`
	runPortableExecutionCase(t, source, strings.Repeat("true\n", 9), "")
}
