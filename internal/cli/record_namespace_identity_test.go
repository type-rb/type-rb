package cli

import "testing"

func TestNamespacedRecordsKeepSeparateIdentities(t *testing.T) {
	runPortableExecutionCase(t, `module First
record Entry
value: Integer
end
end
module Second
record Entry
value: String
end
end
def main()
first := First::Entry.new(value: 3)
second := Second::Entry.new(value: "kept")
puts(first.value)
puts(second.value)
end
`, "3\nkept\n", "")
}

func TestNamespacedRecordFieldsAndDefaultsKeepTheirScope(t *testing.T) {
	runPortableExecutionCase(t, `module Outer
module First
Label := "first"
record Entry<T>
value: T
copy: T = value
end
record Envelope
entry: Entry<String> = Entry<String>.new(value: Label)
end
end
module Second
record Entry<T, U>
value: T
extra: U
end
record Envelope
entry: Entry<Integer, String>
end
end
end
module Outer
module First
def self.stored(): Envelope
return Envelope.new()
end
end
end
alias TextEntry = Outer::First::Entry<String>
def main()
first := Outer::First.stored()
second := Outer::Second::Envelope.new(entry: Outer::Second::Entry<Integer, String>.new(value: 9, extra: "second"))
puts(first.entry.copy)
puts(second.entry.value)
puts(second.entry.extra)
puts(TextEntry.new(value: "alias").copy)
end
`, "first\n9\nsecond\nalias\n", "")
}

func TestImportedNamespacedRecordsPreserveIdentity(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"models.trb": `module First
record Entry<T>
value: T
copy: T = value
end
def self.stored(): Entry<String>
return Entry<String>.new(value: "imported")
end
end
module Second
record Entry
value: Integer
end
end
alias TextEntry = First::Entry<String>
`,
		"main.trb": `import { First as Text, Second as Numbers, TextEntry as Stored } from models
record Entry
value: Boolean
end
def read(value: Text::Entry<String>): String
return value.copy
end
def main()
puts(read(Text.stored()))
puts(Stored.new(value: "alias").copy)
puts(Numbers::Entry.new(value: 5).value)
puts(Entry.new(value: true).value)
end
`,
	}, "imported\nalias\n5\ntrue\n", "")
}

func TestNamespacedRecordsRejectMatchingShapes(t *testing.T) {
	runPortableExecutionCase(t, `module First
record Entry
value: Integer
end
end
module Second
record Entry
value: Integer
end
end
def accept(value: First::Entry)
puts(value.value)
end
def main()
accept(Second::Entry.new(value: 2))
end
`, "", "argument 1 to accept() has type entry, expected entry")
}

func TestNamespacedRecordGenericDoesNotUseSameNamedClass(t *testing.T) {
	runPortableExecutionCase(t, `class Entry
def hidden(): Integer
return 0
end
end
module Models
record Entry<T>
value: T
copy: T = value
end
end
def main()
puts(Models::Entry<String>.new(value: "record").copy)
puts(Entry.new().hidden())
end
`, "record\n0\n", "")
}

func TestNamespacedRecordDoesNotExposeSameNamedClassMembers(t *testing.T) {
	runPortableExecutionCase(t, `class Entry
def hidden(): Integer
return 0
end
end
module Models
record Entry
value: Integer
end
end
def main()
puts(Models::Entry.new(value: 7).hidden())
end
`, "", "has no member hidden")
}

func TestNamespacedRecordContractsSupportNestedValues(t *testing.T) {
	runPortableExecutionCase(t, `import trb/std/json
module Inner
record Entry
value: Integer
end
end
module Outer
record Entry
inner: Inner::Entry
end
end
newtype Wrapped = Outer::Entry
def main()
value := Outer::Entry.new(inner: Inner::Entry.new(value: 7))
encoded := JSON.encode(value) catch |_error|
return
end
decoded := JSON.decode<Outer::Entry>(encoded) catch |_error|
return
end
puts(decoded.inner.value)
puts(Wrapped.new(value).value().inner.value)
values := [1].concurrent_map(limit: 1) do |offset|
value.inner.value + offset
end
puts(values[0])
end
`, "7\n7\n8\n", "")
}
