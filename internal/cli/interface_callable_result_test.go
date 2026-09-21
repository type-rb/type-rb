package cli

import "testing"

const interfaceCallableResults = `interface Factory<T>
reader(value: T): () -> T
mapper(): (Integer) -> Integer
end
class Holder<T> implements Factory<T>
def reader(value: T): () -> T
return fn(): T; return value; end
end
def mapper(): (Integer) -> Integer
return fn(value: Integer): Integer; return value * 2; end
end
end
`

func TestInterfaceMethodsPreserveCallableResults(t *testing.T) {
	runPortableExecutionCase(t, interfaceCallableResults+`def main()
factory: Factory<Integer> := Holder<Integer>.new()
read := factory.reader(21)
double := factory.mapper()
puts(read())
puts(double(4))
text: Factory<String> := Holder<String>.new()
read_text := text.reader("kept")
puts(read_text())
end
`, "21\n8\nkept\n", "")
}

func TestImportedInterfaceMethodsPreserveCallableResults(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"library/factory.trb": interfaceCallableResults,
		"main.trb": `import { Factory as Contract } from library/factory
class Local implements Contract<Integer>
def reader(value: Integer): () -> Integer
return fn(): Integer; return value; end
end
def mapper(): (Integer) -> Integer
return fn(value: Integer): Integer; return value * 2; end
end
end
def main()
factory: Contract<Integer> := Local.new()
read := factory.reader(21)
double := factory.mapper()
puts(read())
puts(double(4))
end
`,
	}, "21\n8\n", "")
}

func TestInterfaceMethodAndReturnedFunctionCheckSeparateArguments(t *testing.T) {
	for _, tc := range []struct{ source, diagnostic string }{
		{`factory.reader()`, "missing required argument"},
		{`factory.reader("wrong")`, "argument 1 to reader() has type string, expected integer"},
		{`read := factory.reader(21)
read(1)`, "fn() expects 0..0 arguments, got 1"},
		{`double := factory.mapper()
double("wrong")`, "argument 1 to fn() has type string, expected integer"},
	} {
		runPortableExecutionCase(t, interfaceCallableResults+`def main()
factory: Factory<Integer> := Holder<Integer>.new()
`+tc.source+"\nend\n", "", tc.diagnostic)
	}
}

func TestImportedCallableFieldsKeepFunctionInvocation(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"library/callback.trb": `record Callback
apply: (Integer) -> String
end
class Stored
@run: () -> Integer
def initialize()
@run = fn(): Integer; return 7; end
end
end
`,
		"main.trb": `import { Callback, Stored } from library/callback
def main()
render := fn(value: Integer): String; return value.to_s(); end
callback := Callback.new(apply: render)
puts(callback.apply(3))
puts(Stored.new().run())
end
`,
	}, "3\n7\n", "")
}
