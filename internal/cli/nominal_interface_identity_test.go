package cli

import "testing"

func TestImportedInterfaceIdentityDoesNotCollideWithLocalClass(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"contracts.trb": `interface Named
name(): String
end
`,
		"main.trb": `import { Named as Contract } from contracts
class Named implements Contract
def name(): String
return "held"
end
end
def display(value: Contract): String
return value.name()
end
def main()
puts(display(Named.new()))
values: Array<Contract> := [Named.new()]
puts(values[0].name())
end
`,
	}, "held\nheld\n", "")
}

func TestNamespacedInterfacesKeepSeparateIdentities(t *testing.T) {
	runPortableExecutionCase(t, `module First
interface Named<T>
name(): T
end
end
module Second
interface Named<T>
name(): T
end
end
class Label implements First::Named<String>
def name(): String
return "first"
end
end
class Count implements Second::Named<Integer>
def name(): Integer
return 7
end
end
def main()
first: First::Named<String> := Label.new()
second: Second::Named<Integer> := Count.new()
puts(first.name())
puts(second.name())
end
`, "first\n7\n", "")
}

func TestImportedGenericClassPreservesImplementedInterfaceIdentity(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"library/factory.trb": `interface Factory<T>
read(): T
end
class Holder<T> implements Factory<T>
@_value: T
def initialize(value: T)
@_value = value
end
def read(): T
return @_value
end
end
`,
		"main.trb": `import { Factory as Contract, Holder as Stored } from library/factory
class Factory
end
class Holder
end
def describe(value: Contract<String>): String
return value.read()
end
def main()
value: Contract<Integer> := Stored<Integer>.new(9)
puts(value.read())
puts(describe(Stored<String>.new("kept")))
end
`,
	}, "9\nkept\n", "")
}

func TestMatchingMembersDoNotConferImportedInterfaceIdentity(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"contracts.trb": `interface Named
name(): String
end
`,
		"main.trb": `import { Named as Contract } from contracts
class Named
def name(): String
return "held"
end
end
def display(value: Contract): String
return value.name()
end
def main()
puts(display(Named.new()))
end
`,
	}, "", "argument 1 to display() has type named, expected")
}

func TestImportedInterfaceEdgesRetainTheirDefiningModule(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"contracts.trb": `interface Factory<T>
read(): T
end
`,
		"models.trb": `import { Factory as Contract } from contracts
alias Source<T> = Contract<T>
class Holder<T> implements Source<T>
@_value: T
def initialize(value: T)
@_value = value
end
def read(): T
return @_value
end
end
`,
		"main.trb": `import { Factory as Reader } from contracts
import { Holder } from models
def display(reader: Reader<String>): String
return reader.read()
end
def main()
reader: Reader<Integer> := Holder<Integer>.new(9)
puts(reader.read())
puts(display(Holder<String>.new("kept")))
end
`,
	}, "9\nkept\n", "")
}

func TestNominalInterfacesRejectOtherContractsAndGenericArguments(t *testing.T) {
	for _, contract := range []string{"Second::Named<Integer>", "First::Named<String>"} {
		t.Run(contract, func(t *testing.T) {
			runPortableExecutionCase(t, `module First
interface Named<T>
name(): T
end
end
module Second
interface Named<T>
name(): T
end
end
class Count implements First::Named<Integer>
def name(): Integer
return 7
end
end
def main()
value: `+contract+` := Count.new()
puts(value.name())
end
`, "", "cannot assign count")
		})
	}
}

func TestInheritedImportedInterfaceIdentity(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"library.trb": `interface Named
name(): String
end
class Parent implements Named
def name(): String
return "inherited"
end
end
class Child < Parent
end
`,
		"main.trb": `import { Child, Named as Contract } from library
def display(value: Contract): String
return value.name()
end
def main()
puts(display(Child.new()))
end
`,
	}, "inherited\n", "")
}

func TestNamespaceImportedInterfaceEdgeIdentity(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"contracts.trb": `module Contracts
interface Named<T>
name(): T
end
end
`,
		"model.trb": `import contracts as Protocols
class Label implements Protocols::Named<String>
def name(): String
return "namespaced"
end
end
`,
		"main.trb": `import { Contracts as Protocols } from contracts
import { Label } from model
def main()
value: Protocols::Named<String> := Label.new()
puts(value.name())
end
`,
	}, "namespaced\n", "")
}

func TestNamespacedInterfaceSignaturesKeepTheirOwnTypeScope(t *testing.T) {
	runPortableExecutionCase(t, `module Contracts
record Payload
value: String
end
interface Named
read(): Payload
end
end
class Label implements Contracts::Named
def read(): Contracts::Payload
return Contracts::Payload.new(value: "scoped")
end
end
def main()
value: Contracts::Named := Label.new()
puts(value.read().value)
end
`, "scoped\n", "")
}

func TestInterfaceDoesNotExposeSameNamedClassMembers(t *testing.T) {
	runPortableExecutionCase(t, `module Contracts
interface Named
name(): String
end
end
class Named implements Contracts::Named
def name(): String
return "held"
end
def hidden(): Integer
return 7
end
end
def main()
value: Contracts::Named := Named.new()
puts(value.hidden())
end
`, "", "has no member hidden")
}
