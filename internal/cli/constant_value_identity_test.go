package cli

import "testing"

func TestConstantScalarCopiesRemainMutable(t *testing.T) {
	runPortableExecutionCase(t, `module Values
COUNT := 3
TEXT := "old"
FLAG := true
end
def main()
mut count := Values::COUNT
mut text := Values::TEXT
mut flag := Values::FLAG
count += 1
text = "new"
flag = false
puts(count)
puts(text)
puts(flag)
puts(Values::COUNT)
puts(Values::TEXT)
puts(Values::FLAG)
end
`, "4\nnew\nfalse\n3\nold\ntrue\n", "")
}

func TestReopenedModuleConstantsKeepTheirCheckedTypes(t *testing.T) {
	runPortableExecutionCase(t, `module Values
FIRST := 1
end
module Values
SECOND := FIRST + 1
end
def main()
puts(Values::FIRST)
puts(Values::SECOND)
end
`, "1\n2\n", "")
}

func TestConstantNamesDoNotCollideWithTypesOrMethods(t *testing.T) {
	runPortableExecutionCase(t, `module Values
record Box
value: Integer
end
BOX := Box.new(value: 7)
def self.box(): Integer
return BOX.value
end
end
def main()
puts(Values::BOX.value)
puts(Values.box())
end
`, "7\n7\n", "")
}

func TestImportedConstantAliasesAndInitializationDependencies(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"values.trb": `def mark(): Integer
puts("dependency")
return 7
end
COUNT := mark()
`,
		"consumer.trb": `import { COUNT as LIMIT } from values
def mark(value: Integer): Integer
puts("consumer")
return value + 1
end
TOTAL := mark(LIMIT)
def report(): Integer
return TOTAL
end
`,
		"main.trb": `import { report } from consumer
def main()
puts(report())
puts(report())
end
`,
	}, "dependency\nconsumer\n8\n8\n", "")
}

func TestImportedConstantsKeepCanonicalNamesAcrossPackages(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"models/values.trb": `record Box
value: Integer
end
BOX: Box := Box.new(value: 7)
`,
		"left.trb":  "COUNT := 3\n",
		"right.trb": "COUNT := 4\n",
		"consumer.trb": `import { BOX as ITEM } from models/values
import { COUNT as LEFT } from left
import { COUNT as RIGHT } from right
def report(): Integer
return ITEM.value + LEFT + RIGHT
end
`,
		"main.trb": `import { report } from consumer
def main()
puts(report())
end
`,
	}, "14\n", "")
}
