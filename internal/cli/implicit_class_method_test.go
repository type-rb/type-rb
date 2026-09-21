package cli

import "testing"

const implicitClassMethods = `class Secret
@_value: Integer
def initialize(value: Integer)
@_value = value
end
def _read(*, extra: Integer = 0): Integer
return @_value + extra
end
def value(*, extra: Integer = 0): Integer
return _read(extra: extra)
end
def reader(): () -> Integer
return fn(): Integer; return _read(); end
end
def shadow(): Integer
_read := fn(): Integer; return 99; end
return _read()
end
def self._number(): Integer
return 7
end
def self.number(): Integer
return _number()
end
end
`

func TestImplicitClassMethodReceiversAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, implicitClassMethods+`def main()
value := Secret.new(9)
puts(value.value(extra: 1))
read := value.reader()
puts(read())
puts(value.shadow())
puts(Secret.number())
end
`, "10\n9\n99\n7\n", "")
}

func TestImportedImplicitClassMethodsKeepTheirReceiver(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"library/secret.trb": implicitClassMethods,
		"main.trb": `import { Secret as Hidden } from library/secret
def main()
value := Hidden.new(12)
puts(value.value())
read := value.reader()
puts(read())
puts(Hidden.number())
end
`,
	}, "12\n12\n7\n", "")
}

func TestInheritedImplicitClassMethodReceivers(t *testing.T) {
	runPortableExecutionCase(t, `class Base
def _number(): Integer
return 17
end
def value(): Integer
return _number()
end
end
class Child < Base
end
def main()
puts(Child.new().value())
end
`, "17\n", "")
}

func TestClassCallableFieldsAndMethodResultsStayDistinct(t *testing.T) {
	runPortableExecutionCase(t, `class Holder
@work: () -> Integer
def initialize()
@work = fn(): Integer; return 21; end
end
def reader(): () -> Integer
return @work
end
end
def main()
value := Holder.new()
puts(value.work())
read := value.reader()
puts(read())
end
`, "21\n21\n", "")
}
