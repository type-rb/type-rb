package cli

import "testing"

func TestConcreteGenericEnumAliasMethodsAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, `enum Box<T>
Value(value: T)
def echo(value: T): T
return value
end
end
alias TextBox = Box<String>
def main()
puts(TextBox::Value("held").echo("ok"))
end
`, "ok\n", "")
}

func TestGenericEnumAliasMethodsKeepSelfDefaultsAndClosures(t *testing.T) {
	runPortableExecutionCase(t, `enum Box<T>
Value(value: T)
Other(value: T)
def _get(fallback: T): T
case self
when Box::Value(value)
return value
when Box::Other(_value)
return fallback
end
end
def reader(value: T, *, fallback: T = value): () -> T
return fn(): T; return _get(fallback); end
end
end
alias Container<T> = Box<T>
alias TextBox = Container<String>
alias TextAgain = TextBox
def main()
first := TextAgain::Value("held").reader("unused")
second := Container<Integer>::Other(0).reader(2, fallback: 7)
third := TextBox::Other("unused").reader("default")
puts(first())
puts(second())
puts(third())
end
`, "held\n7\ndefault\n", "")
}

func TestGenericEnumAliasNullableMethodsStayLazy(t *testing.T) {
	runPortableExecutionCase(t, `enum Box<T>
Value(value: T)
def echo(value: T): T
return value
end
end
alias TextBox = Box<String>
def argument(): String
puts("argument")
return "ok"
end
def read(value: TextBox?): String?
return value&.echo(argument())
end
def main()
puts(read(nil) == nil)
value := read(TextBox::Value("held"))
if value != nil
puts(value)
end
end
`, "true\nargument\nok\n", "")
}

func TestImportedGenericEnumAliasMethodsKeepCanonicalOwner(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"boxes.trb": `module Boxes
enum Box<T>
Value(value: T)
def get(fallback: T): T
case self
when Box::Value(value)
return value
end
end
end
alias Container<T> = Box<T>
alias TextBox = Container<String>
end
`,
		"consumer.trb": `import boxes as Wire
def report()
puts(Wire::TextBox::Value("imported").get("unused"))
puts(Wire::Container<Integer>::Value(7).get(0))
end
`,
		"main.trb": `import { report } from consumer
enum Box<T>
Value(value: T)
def get(fallback: T): T
return fallback
end
end
def main()
report()
puts(Box<String>::Value("local").get("own"))
end
`,
	}, "imported\n7\nown\n", "")
}

func TestGenericEnumAliasMethodsRejectWrongArgumentsAndPrivateAccess(t *testing.T) {
	for _, test := range []struct{ expression, failure string }{
		{`puts(TextBox::Value("held").echo(1))`, "expected string"},
		{`puts(TextBox::Value("held")._hidden("value"))`, "private"},
	} {
		t.Run(test.expression, func(t *testing.T) {
			runPortableExecutionCase(t, `enum Box<T>
Value(value: T)
def echo(value: T): T
return value
end
def _hidden(value: T): T
return value
end
end
alias TextBox = Box<String>
def main()
`+test.expression+"\nend\n", "", test.failure)
		})
	}
}
