package cli

import "testing"

func TestModuleMemberIdentityAcrossTargetsAndREPL(t *testing.T) {
	source := `module First
LABEL := "first"
def self.label(): String
return LABEL
end
def self.describe(prefix: String = First.label()): String
return prefix + "!"
end
def self._private_value(): Integer
return 7
end
def self.public_value(): Integer
return First._private_value()
end
end

module Second
def self.label(): String
return "second"
end
end

module Outer
module Inner
LABEL := "nested"
def self.label(): String
return LABEL
end
end
end

def label(): String
return "top"
end

def main()
puts(First.describe())
puts(Second.label())
puts(Outer::Inner.label())
puts(First::LABEL)
puts(Outer::Inner::LABEL)
puts(label())
puts(First.public_value())
end
`
	runPortableExecutionCase(t, source, "first!\nsecond\nnested\nfirst\nnested\ntop\n7\n", "")
}

func TestModulePrivateMembersAreRejectedAcrossTargetsAndREPL(t *testing.T) {
	source := `module Secret
def self._value(): Integer
return 7
end
end
def main()
puts(Secret._value())
end
`
	runPortableExecutionCase(t, source, "", "private member _value cannot be accessed externally")
}

func TestModuleMemberIdentityThroughImportAliases(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"library/settings.trb": `module Settings
LABEL := "setting"
def self.label(): String
return LABEL
end
def self._label(): String
return "private"
end
def self.secret_label(): String
return Settings._label()
end
module Inner
LABEL := "inner"
def self.label(): String
return LABEL
end
end
end
`,
		"consumer.trb": `import library/settings as Config
def report(): String
return Config.label() + ":" + Config::LABEL + ":" + Config::Inner.label() + ":" + Config::Inner::LABEL + ":" + Config.secret_label()
end
`,
		"main.trb": `import { report } from consumer
def main()
puts(report())
end
`,
	}, "setting:setting:inner:inner:private\n", "")
}

func TestModuleConstantsKeepInitializationAndTypeIdentity(t *testing.T) {
	source := `def mark(value: String): String
puts(value)
return value
end
module Values
FIRST := mark("first")
SECOND := mark(Values::FIRST + " second")
def self.read(): String
return SECOND
end
THIRD := mark(Values.read())
record Item
text: String
end
end
def main()
item := Values::Item.new(text: Values::THIRD)
puts(item.text)
puts(Values::FIRST.size())
end
`
	runPortableExecutionCase(t, source, "first\nfirst second\nfirst second\nfirst second\n5\n", "")
}

func TestModuleConstantTypesAreChecked(t *testing.T) {
	source := `module Values
TEXT := "text"
end
def main()
value: Integer := Values::TEXT
puts(value)
end
`
	runPortableExecutionCase(t, source, "", "cannot assign")
}

func TestModuleConstantCollectionsRemainImmutable(t *testing.T) {
	source := `module Values
ITEMS := [1, 2]
end
def main()
Values::ITEMS.push(3)
end
`
	runPortableExecutionCase(t, source, "", "immutable")
}

func TestNestedModuleOwnerIsNotAValue(t *testing.T) {
	source := `module Outer
module Inner
end
end
def main()
puts(Outer::Inner)
end
`
	runPortableExecutionCase(t, source, "", "declaration")
}
