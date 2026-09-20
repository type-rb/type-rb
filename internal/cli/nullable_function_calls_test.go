package cli

import "testing"

func TestNullableFunctionCallsRequireNarrowing(t *testing.T) {
	for _, source := range []string{
		`alias Callback = (Integer) -> Integer
def add(value: Integer): Integer
return value + 1
end
def main()
callback: Callback? := add
puts(callback(3))
end
`,
		`alias Callback = (Integer) -> Integer
def invoke(callback: Callback?): Integer
return callback(3)
end
def main()
puts(invoke(nil))
end
`,
		`alias Callback = (Integer) -> Integer
record Holder
callback: Callback?
end
def main()
item := Holder.new(callback: nil)
puts(item.callback(3))
end
`,
	} {
		runPortableExecutionCase(t, source, "", "nullable function value must be narrowed")
	}
}

func TestNullableFunctionNarrowingAcrossExecutionPaths(t *testing.T) {
	source := `alias Callback = (Integer) -> Integer
def add(value: Integer): Integer
return value + 1
end
def choose(): Callback?
return add
end
def invoke(callback: Callback?): Integer
if callback != nil
return callback(3)
end
return 0
end
def main()
callback := choose()
if callback != nil
puts(callback(5))
end
puts(invoke(add))
puts(invoke(nil))
mut other: Callback? := nil
other = add
puts(other(7))
end
`
	runPortableExecutionCase(t, source, "6\n4\n0\n8\n", "")
}
