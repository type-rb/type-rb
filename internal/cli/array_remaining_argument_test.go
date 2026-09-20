package cli

import (
	"fmt"
	"testing"
)

func TestArrayInsertionsRetainReceiverBeforeArgumentEffects(t *testing.T) {
	for _, method := range []string{"push", "unshift"} {
		for _, scenario := range []struct{ name, effects, push, unshift string }{
			{"rebind", "values = [8, 9]", "1\n7\n3\n8\n", "7\n2\n3\n8\n"},
			{"grow_then_rebind", "values.push(3)\nvalues = [8, 9]", "1\n7\n4\n8\n", "7\n3\n4\n8\n"},
			{"clear_then_rebind", "values.pop()\nvalues.pop()\nvalues = [8, 9]", "7\n7\n1\n8\n", "7\n7\n1\n8\n"},
		} {
			t.Run(method+"/"+scenario.name, func(t *testing.T) {
				source := fmt.Sprintf(`def main()
mut values := [1, 2]
original := values
argument := fn(): Integer
%s
return 7
end
values.%s(argument())
puts(original.first())
puts(original.last())
puts(original.size())
puts(values.first())
end
`, scenario.effects, method)
				want := scenario.push
				if method == "unshift" {
					want = scenario.unshift
				}
				runPortableExecutionCase(t, source, want, "")
			})
		}
	}
}

func TestArrayConcatAndJoinReadStorageAfterArguments(t *testing.T) {
	source := `def main()
mut values := [1, 2]
original := values
argument := fn(): Array<Integer>
values.push(3)
values = [8]
return [9]
end
result := values.concat(argument())
result.each { |value| puts(value) }
puts(original.size())
puts(values.first())
mut words := ["a", "b"]
separator := fn(): String
words.push("c")
words = ["replacement"]
return "-"
end
puts(words.join(separator()))
puts(words.first())
end
`
	runPortableExecutionCase(t, source, "1\n2\n3\n9\n3\n8\na-b-c\nreplacement\n", "")
}

func TestArrayInsertionArgumentsKeepContextualAndOptionalTypes(t *testing.T) {
	source := `def insert(mut values: Array<Integer>, early: Boolean)
values.unshift(if early
return
else
3
end)
end
def main()
mut optional: Array<Integer?> := []
optional.unshift(nil)
optional.push(2)
puts(optional.first() == nil)
mut floats: Array<Float> := []
floats.unshift(1)
floats.push(2)
puts(floats.first() == 1.0)
puts(floats.last() == 2.0)
mut absent: Array<Integer>? := nil
mut calls := 0
argument := fn(): Integer
calls += 1
return 1
end
absent&.push(argument())
absent&.unshift(argument())
puts(calls)
mut values := [1]
insert(values, true)
puts(values.first())
insert(values, false)
puts(values.first())
end
`
	runPortableExecutionCase(t, source, "true\ntrue\ntrue\n0\n1\n3\n", "")
}
