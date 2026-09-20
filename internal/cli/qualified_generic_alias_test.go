package cli

import "testing"

func TestQualifiedGenericRecordAliasConstructionAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, `module Outer
record Box<T>
value: T
copy: T = value
end
alias Holder<T> = Box<T>
end
def main()
direct := Outer::Box<Integer>.new(value: 3)
aliased := Outer::Holder<Integer>.new(value: 7)
puts(direct.copy)
puts(aliased.copy)
end
`, "3\n7\n", "")
}

func TestImportedQualifiedGenericRecordAliasConstructionAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"models/catalog.trb": `module Catalog
module Inner
record Box<T>
value: T
copy: T = value
end
alias Holder<T> = Box<T>
alias Wrapped<T> = Holder<Array<T>>
end
end
`,
		"main.trb": `import models/catalog as Items
record Box
value: Integer
end
def main()
first := Items::Inner::Holder<String>.new(value: "kept")
second := Items::Inner::Wrapped<Integer>.new(value: [7, 8])
puts(first.copy)
puts(second.copy[1])
local := Box.new(value: 11)
puts(local.value)
end
`,
	}, "kept\n8\n11\n", "")
}

func TestQualifiedGenericRecordAliasRejectsInvalidFields(t *testing.T) {
	for _, value := range []string{`"wrong"`, `[7]`} {
		t.Run(value, func(t *testing.T) {
			runPortableExecutionCase(t, `module Outer
record Box<T>
value: T
end
alias Holder<T> = Box<T>
end
def main()
value := Outer::Holder<Integer>.new(value: `+value+`)
puts(value.value)
end
`, "", "expected integer")
		})
	}
}
