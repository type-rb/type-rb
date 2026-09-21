package cli

import "testing"

func TestQualifiedClassReturnValuesAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, `module Models
class Answer
def identity(): Answer
return self
end
def value(): Integer
return 42
end
end
end
def main()
puts(Models::Answer.new().identity().value())
end
`, "42\n", "")
}

func TestQualifiedClassConstantOwnersAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, `module Models
class Answer
VALUE := 42
def value(): Integer
return VALUE
end
end
end
def main()
puts(Models::Answer.new().value())
end
`, "42\n", "")
}

func TestQualifiedClassConstantsKeepLexicalOwnersAndCaptures(t *testing.T) {
	runPortableExecutionCase(t, `module Outer
VALUE := 3
module Inner
VALUE := 5
class Base
VALUE := 7
def value(): Integer
return VALUE
end
def captured(): () -> Integer
return fn(): Integer
return VALUE
end
end
end
class Child < Base
VALUE := 11
end
end
end
def main()
child := Outer::Inner::Child.new()
puts(child.value())
reader := child.captured()
puts(reader())
end
`, "7\n7\n", "")
}

func TestImportedQualifiedClassesPreserveReturnsAndConstants(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"models.trb": `module Models
class Answer
VALUE := 42
def identity(): Answer
return self
end
def value(): Integer
return VALUE
end
end
end
`,
		"main.trb": `import { Models as Library } from models
def main()
puts(Library::Answer.new().identity().value())
end
`,
	}, "42\n", "")
}

func TestQualifiedSuperclassThroughNamespaceImportAlias(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"library/base.trb": `module Models
class Base
VALUE := 13
def value(): Integer
return VALUE
end
end
end
`,
		"child/child.trb": `import { Models as Library } from library/base
class Child < Library::Base
end
`,
		"main.trb": `import { Child as Selected } from child/child
def main()
puts(Selected.new().value())
end
`,
	}, "13\n", "")
}
