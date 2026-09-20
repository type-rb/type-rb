package cli

import "testing"

func TestNamedFunctionValuesAcrossExecutionPaths(t *testing.T) {
	source := `alias Callback = (Integer) -> Integer

def add(value: Integer): Integer
return value + 2
end
def _private(value: Integer): Integer
return value * 3
end
def apply(callback: (Integer) -> Integer): Integer
return callback(3)
end
def maker(): (Integer) -> Integer
return add
end
def report(value: Integer)
puts(value)
end
def main()
puts(apply(add))
callback := add
puts(callback(4))
puts(maker()(5))
functions := [add, _private]
puts(functions[0](6))
puts(functions[1](6))
optional: Callback? := add
if optional != nil
puts(optional(7))
end
printer := report
printer(10)
shadow := fn(add: (Integer) -> Integer): Integer
return add(2)
end
puts(shadow(_private))
end
`
	runPortableExecutionCase(t, source, "5\n6\n7\n8\n18\n9\n10\n6\n", "")
}

func TestImportedNamedFunctionValuesAcrossExecutionPaths(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"operations.trb": `record Item
value: Integer
end
def add(value: Integer): Integer
return value + 2
end
def make(value: Integer): Item
return Item.new(value: value)
end
def read(value: Item): Integer
return value.value
end
def maker(): (Integer) -> Integer
return add
end
`,
		"main.trb": `import { Item, add as increase, make, read as inspect, maker as choose } from operations
def apply(callback: (Integer) -> Integer): Integer
return callback(3)
end
def factory(): (Integer) -> Item
return make
end
def main()
puts(apply(increase))
callback := increase
puts(callback(4))
maker := factory()
reader := inspect
puts(reader(maker(8)))
puts(choose()(9))
chooser := choose
puts(chooser()(10))
end
`,
	}, "5\n6\n8\n11\n12\n", "")
}

func TestNamedFunctionValuesPreserveResultAndExecutionScope(t *testing.T) {
	source := `import { Result } from trb/std/result
def read(succeed: Boolean): Result<Integer, String>
if succeed
values := [3, 4].concurrent_map(limit: 2) { |value| value + 1 }
return Result<Integer, String>::Ok(values[0] + values[1])
end
return Result<Integer, String>::Err("missing")
end
def invoke(callback: (Boolean) -> Result<Integer, String>, succeed: Boolean): Result<Integer, String>
value := try callback(succeed)
return Result<Integer, String>::Ok(value + 1)
end
def main()
callback := read
first := invoke(callback, true) catch |_error|
-1
end
puts(first)
second := invoke(read, false) catch |error|
puts(error)
12
end
puts(second)
end
`
	runPortableExecutionCase(t, source, "10\nmissing\n12\n", "")
}

func TestNamedFunctionValuesRequireRepresentableSignatures(t *testing.T) {
	for _, test := range []struct{ name, declaration, expression, diagnostic string }{
		{"default", "def source(value: Integer = 1): Integer\nreturn value\nend", "source", "requires an explicit fn wrapper"},
		{"named", "def source(*, value: Integer): Integer\nreturn value\nend", "source", "requires an explicit fn wrapper"},
		{"generic", "def source<T>(value: T): T\nreturn value\nend", "source", "requires an explicit fn wrapper"},
		{"parameter", "def source(value: String): Integer\nreturn value.size()\nend", "source", "cannot assign"},
		{"return", "def source(value: Integer): String\nreturn value.to_s()\nend", "source", "cannot assign"},
		{"arity", "def source(): Integer\nreturn 1\nend", "source", "cannot assign"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := test.declaration + "\ndef main()\ncallback: (Integer) -> Integer := " + test.expression + "\nputs(callback(1))\nend\n"
			runPortableExecutionCase(t, source, "", test.diagnostic)
		})
	}
}

func TestNamedFunctionValueAdaptersAcrossExecutionPaths(t *testing.T) {
	source := `def defaulted(value: Integer = 4): Integer
return value
end
def named(*, value: Integer): Integer
return value + 1
end
def identity<T>(value: T): T
return value
end
def main()
default_callback := fn(): Integer
return defaulted()
end
named_callback := fn(value: Integer): Integer
return named(value: value)
end
generic_callback := fn(value: Integer): Integer
return identity<Integer>(value)
end
puts(default_callback())
puts(named_callback(5))
puts(generic_callback(7))
end
`
	runPortableExecutionCase(t, source, "4\n6\n7\n", "")
}

func TestNamedFunctionValueResultCannotBeIgnored(t *testing.T) {
	source := `import { Result } from trb/std/result
def read(): Result<Integer, String>
return Result<Integer, String>::Ok(1)
end
def main()
callback := read
callback()
end
`
	runPortableExecutionCase(t, source, "", "must be used")
}
