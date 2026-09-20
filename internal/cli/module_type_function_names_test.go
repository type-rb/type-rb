package cli

import "testing"

func TestModuleMethodsDoNotCollideWithOwnedTypes(t *testing.T) {
	source := `module Outer
record Item
value: Integer
end
def self.item(): Item
return Item.new(value: 7)
end
module Inner
record Box
value: String
end
def self.box(): Box
return Box.new(value: "nested")
end
end
end
def main()
puts(Outer.item().value)
puts(Outer::Inner.box().value)
end
`
	runPortableExecutionCase(t, source, "7\nnested\n", "")
}

func TestImportedModuleMethodsDoNotCollideWithOwnedTypes(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"models/items.trb": `module Items
record Item
value: Integer
end
def self.item(): Item
return Item.new(value: 9)
end
end
`,
		"consumer.trb": `import models/items as Catalog
def report(): Integer
return Catalog.item().value
end
`,
		"main.trb": `import { report } from consumer
def main()
puts(report())
end
`,
	}, "9\n", "")
}
