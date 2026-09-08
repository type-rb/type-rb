package compiler

import (
	"strings"
	"testing"
)

func TestImportedRecordContractsPreserveNominalIdentity(t *testing.T) {
	model := SourceUnit{Filename: "model.trb", ModulePath: "model", Source: []byte(`record Entry
id: Integer
end
def entry(): Entry
return Entry.new(id: 1)
end
def entries(): Array<Entry>
return [entry()]
end
def entries_for<T>(_value: T): Array<Entry>
return entries()
end
def accept(value: Entry)
puts(value.id)
end
def accept_all(values: Array<Entry>)
puts(values.size())
end
`)}
	cases := []struct{ name, symbol, body string }{
		{"scalar return", "entry", "consume(entry())"},
		{"array return", "entries", "consume_all(entries())"},
		{"generic return", "entries_for", "consume_all(entries_for<Integer>(1))"},
		{"scalar argument", "accept", "accept(Entry.new(id: 2))"},
		{"array argument", "accept_all", "accept_all([Entry.new(id: 2)])"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode+"/same declaration", func(t *testing.T) {
			main := SourceUnit{Filename: "main.trb", ModulePath: "main", Source: []byte(`import { Entry as Item, entry, entries, entries_for, accept, accept_all } from model
def main()
value: Item := entry()
accept(value)
accept_all(entries())
accept_all(entries_for<Integer>(1))
end
`)}
			if _, err := CompileProject([]SourceUnit{model, main}, Options{Mode: mode, SourceRoot: "/project", ProjectRoot: "/project"}); err != nil {
				t.Fatal(err)
			}
		})
		for _, tc := range cases {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				main := SourceUnit{Filename: "main.trb", ModulePath: "main", Source: []byte(`import { ` + tc.symbol + ` } from model
record Entry
id: Integer
end
def consume(value: Entry)
puts(value.id)
end
def consume_all(values: Array<Entry>)
puts(values.size())
end
def main()
` + tc.body + "\nend\n")}
				_, err := CompileProject([]SourceUnit{model, main}, Options{Mode: mode, SourceRoot: "/project", ProjectRoot: "/project"})
				if err == nil || !strings.Contains(err.Error(), "expected") {
					t.Fatalf("expected nominal mismatch, got %v", err)
				}
			})
		}
	}
}
