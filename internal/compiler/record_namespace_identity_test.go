package compiler

import (
	"strings"
	"testing"
)

func TestNamespacedRecordDiagnosticsAcrossModes(t *testing.T) {
	const prefix = `module First
record Entry<T>
value: T
end
end
module Second
record Entry
value: String
end
end
`
	for _, tc := range []struct{ name, body, message string }{
		{"missing field", "Second::Entry.new()", "missing record field value"},
		{"wrong field", "Second::Entry.new(value: 1)", "expected String"},
		{"unknown field", "First::Entry<Integer>.new(other: 2)", "has no field other"},
		{"wrong generic field", "First::Entry<Integer>.new(value: \"bad\")", "expected Integer"},
		{"missing type argument", "First::Entry.new(value: 2)", "expects 1 type argument(s), got 0"},
		{"extra type argument", "First::Entry<Integer, String>.new(value: 2)", "expects 1 type argument(s), got 2"},
		{"non-generic record", "Second::Entry<Integer>.new(value: 2)", "not a generic declaration"},
		{"readonly field", "mut value := Second::Entry.new(value: \"kept\")\nvalue.value = \"changed\"", "field value is readonly"},
		{"nominal assignment", "value: Second::Entry := First::Entry<String>.new(value: \"kept\")\nputs(value.value)", "cannot assign"},
		{"ambiguous constructor", "Entry.new(value: 2)", "ambiguous"},
	} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				_, err := Compile("records.trb", []byte(prefix+"def main()\n"+tc.body+"\nend\n"), mode)
				if err == nil || !strings.Contains(err.Error(), tc.message) {
					t.Fatalf("got %v, want %q", err, tc.message)
				}
			})
		}
	}
}

func TestNamespacedRecordsRejectDuplicateDeclaration(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := Compile("duplicate.trb", []byte(`module First
record Entry
value: Integer
end
end
module First
record Entry
value: String
end
end
`), mode)
		if err == nil || !strings.Contains(err.Error(), "already declared as record") {
			t.Fatalf("%s: duplicate declaration: %v", mode, err)
		}
	}
}

func TestNamespacedRecordRejectsOtherTypeWithSameQualifiedName(t *testing.T) {
	const record = "record Entry\nvalue: Integer\nend\n"
	for _, other := range []string{
		"class Entry\nend\n",
		"interface Entry\nread(): Integer\nend\n",
		"alias Entry = Integer\n",
		"newtype Entry = Integer\n",
		"enum Entry\nReady\nend\n",
	} {
		for _, body := range []string{record + other, other + record} {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				source := "module Models\n" + body + "end\n"
				_, err := Compile("duplicate.trb", []byte(source), mode)
				if err == nil || !strings.Contains(err.Error(), "already declared as") {
					t.Errorf("%s: duplicate declaration: %v\n%s", mode, err, source)
				}
			}
		}
	}
}

func TestNamespacedRecordApplicationsProduceValidTypeScript(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{
		{Filename: "/project/models.trb", ModulePath: "models", Source: []byte(`module First
record Entry<T>
value: T
copy: T = value
end
record Envelope
entry: Entry<String>
end
def self.stored(): Envelope
return Envelope.new(entry: Entry<String>.new(value: "kept"))
end
end
module Second
record Entry
value: Integer
end
end
alias TextEntry = First::Entry<String>
`)},
		{Filename: "/project/main.trb", ModulePath: "main", Source: []byte(`import { First as Text, Second as Numbers, TextEntry as Stored } from models
record Entry
value: Boolean
end
def read(value: Text::Envelope): String
return value.entry.copy
end
def main()
puts(read(Text.stored()))
puts(Stored.new(value: "alias").copy)
puts(Numbers::Entry.new(value: 5).value)
puts(Entry.new(value: true).value)
end
`)},
	}, Options{Mode: "typescript", SourceRoot: "/project", ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	checkTypeScriptArtifacts(t, artifacts, "record_namespaces")
}
