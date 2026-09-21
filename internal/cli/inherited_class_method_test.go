package cli

import "testing"

const inheritedClassMethods = `class Base
def self.number(*, offset: Integer = 1): Integer
return 6 + offset
end
end
class Middle < Base
end
class Child < Middle
end
class Replacement < Base
def self.number(*, offset: Integer = 2): Integer
return 10 + offset
end
end
`

func TestInheritedClassMethodsAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, inheritedClassMethods+`def main()
puts(Child.number())
puts(Child.number(offset: 3))
puts(Replacement.number())
puts(Base.number())
end
`, "7\n9\n12\n7\n", "")
}

func TestInheritedClassMethodsThroughImportedDescendants(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"library/classes.trb": inheritedClassMethods,
		"main.trb": `import { Child as Leaf, Replacement } from library/classes
def main()
puts(Leaf.number())
puts(Replacement.number(offset: 4))
end
`,
	}, "7\n14\n", "")
}

func TestInheritedClassMethodsAcrossModuleBoundaries(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"parent/base.trb": `class Base
def self.number(): Integer
return 17
end
end
`,
		"child/child.trb": `import { Base } from parent/base
class Child < Base
end
`,
		"main.trb": `import { Child } from child/child
def main()
puts(Child.number())
end
`,
	}, "17\n", "")
}
