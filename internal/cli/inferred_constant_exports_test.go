package cli

import "testing"

func TestImportedInferredConstantsKeepNominalAndCompositeTypes(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"models/values.trb": `record Box<T>
value: T
end
BOX := Box<Integer>.new(value: 7)
BOXES := [BOX]
TABLE := {"chosen" => BOX}
def maybe(present: Boolean): Box<Integer>?
if present
return BOX
end
return nil
end
OPTIONAL := maybe(true)
CALL := fn(value: Integer): Integer; return value + BOX.value; end
`,
		"consumer.trb": `import { BOX as ITEM, BOXES, TABLE, OPTIONAL, CALL } from models/values
COPIED := ITEM
def report()
puts(COPIED.value + 1)
puts(BOXES[0].value)
puts(TABLE["chosen"].value)
value := OPTIONAL
if value != nil
puts(value.value)
end
puts(CALL(2))
end
`,
		"main.trb": `import { report } from consumer
def main()
report()
end
`,
	}, "8\n7\n7\n7\n9\n", "")
}

func TestImportedModuleInferredConstantsUseDeclarationIdentity(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"models/catalog.trb": `module Catalog
record Box
value: Integer
end
BOX := Box.new(value: 7)
module Inner
TEXT := "inner" + "!"
end
end
`,
		"other.trb": `record Box
value: String
end
BOX := Box.new(value: "other")
`,
		"main.trb": `import models/catalog as Values
import { BOX as OTHER } from other
def main()
puts(Values::BOX.value + 1)
puts(Values::Inner::TEXT + "!")
puts(OTHER.value + "!")
end
`,
	}, "8\ninner!!\nother!\n", "")
}

func TestImportedInferredConstantsRetainTypeAndMutationErrors(t *testing.T) {
	for _, test := range []struct{ source, body, failure string }{
		{"record Box\nvalue: Integer\nend\nBOX := Box.new(value: 7)\n", "puts(ITEM.value + true)", "does not support integer and boolean"},
		{"BOX := [7]\n", "ITEM.push(9)", "mutable"},
		{"BOX := missing()\n", "puts(ITEM)", "missing"},
	} {
		t.Run(test.body, func(t *testing.T) {
			runPortableExecutionFiles(t, map[string]string{
				"values.trb": test.source,
				"main.trb":   "import { BOX as ITEM } from values\ndef main()\n" + test.body + "\nend\n",
			}, "", test.failure)
		})
	}
}
