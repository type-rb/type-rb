package bootstrapsnapshot

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
)

const v4Program = `alias Callback = () -> Void

record Message
	text: String
end

enum Choice
	Text(value: String)
	Action(callback: Callback)
end

def main()
	prefix := "hé"
	suffix := "llo"
	append := fn(value: String): String
		return value + suffix
	end
	mut parts := [append(prefix)]
	parts.push(suffix)
	parts[-1] = prefix
	last := parts[-1]
	character := last[-1]
	if parts.size() == 2
		if last.size() == 2
			if character == "é"
				puts(last + suffix)
			end
		end
	end
	message := Message.new(text: last)
	choice := Choice::Text(message.text)
	case choice
	when Choice::Text(value)
		puts(value)
	when Choice::Action(callback)
		callback()
	end
	mut callbacks: Array<Callback> := []
	callback: Callback := fn()
		if callbacks.size() > 0
			puts("cycle")
		end
		return
	end
	callbacks.push(callback)
	callback()
	return
end
`

func TestBuildV4LowersManagedValuesAndLexicalClosures(t *testing.T) {
	artifacts := analyzeV4Program(t, v4Program)
	snapshot, err := BuildV4(artifacts, "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Format != Format || snapshot.Version != Version4 || snapshot.Module != "main" || snapshot.EntryFunction != "main#main" {
		t.Fatalf("unexpected snapshot envelope: %#v", snapshot)
	}
	if len(snapshot.Sources) != 1 || snapshot.Sources[0].Path != "main.trb" || len(snapshot.Functions) != 3 {
		t.Fatalf("unexpected snapshot inputs: %#v", snapshot)
	}
	for _, expected := range []string{
		"String", "Array<String>", "() -> Void", "(String) -> String", "Array<() -> Void>",
		"main#Message", "main#Choice",
	} {
		if !v3HasTypeDefinition(snapshot.Types, expected) {
			t.Fatalf("snapshot is missing type definition %q: %#v", expected, snapshot.Types)
		}
	}
	appendBody := v4FunctionWithSuffix(t, snapshot.Functions, "$lambda0")
	if len(appendBody.Captures) != 1 || appendBody.Captures[0].Type != "String" || len(appendBody.Parameters) != 1 || appendBody.Parameters[0].Type != "String" {
		t.Fatalf("unexpected String closure signature: %#v", appendBody)
	}
	cycleBody := v4FunctionWithSuffix(t, snapshot.Functions, "$lambda1")
	if len(cycleBody.Captures) != 1 || cycleBody.Captures[0].Type != "Array<() -> Void>" {
		t.Fatalf("unexpected managed closure capture: %#v", cycleBody.Captures)
	}

	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := BuildV4(artifacts, "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	repeatedEncoded, err := json.Marshal(repeated)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(repeatedEncoded) {
		t.Fatalf("bootstrap snapshot v4 output is not deterministic:\nfirst:  %s\nsecond: %s", encoded, repeatedEncoded)
	}
	text := string(encoded)
	for _, expected := range []string{
		`"captures":[]`,
		`"kind":"string","id":"String"`,
		`"kind":"array"`,
		`"element":"String"`,
		`"op":"string_literal"`,
		`"op":"string_concat"`,
		`"op":"string_equal"`,
		`"op":"string_size"`,
		`"op":"string_index"`,
		`"op":"write_string"`,
		`"op":"array_construct"`,
		`"op":"array_size"`,
		`"op":"array_get"`,
		`"op":"array_set"`,
		`"op":"array_push"`,
		`"op":"closure_construct"`,
		`"op":"closure_call"`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("snapshot is missing %s:\n%s", expected, text)
		}
	}
	if strings.Contains(text, `"startLine":0`) || strings.Contains(text, `"startColumn":0`) || strings.Contains(text, `"endLine":0`) || strings.Contains(text, `"endColumn":0`) {
		t.Fatalf("snapshot contains a non-positive compiler-generated origin:\n%s", text)
	}
}

func TestBuildV4RejectsUnsupportedArrayElements(t *testing.T) {
	artifacts := analyzeV4Program(t, "def main()\n\tvalues := [1.5]\n\tputs(values.size().to_s())\n\treturn\nend\n")
	_, err := BuildV4(artifacts, "/project/src")
	if err == nil || !strings.Contains(err.Error(), "bootstrap snapshot v4 does not support Array element type Float") {
		t.Fatalf("BuildV4() error=%v", err)
	}
}

func TestBuildV4RejectsTaggedArrayElements(t *testing.T) {
	source := `enum Choice
	Item(value: Integer)
end

def main()
	values := [Choice::Item(1)]
	puts(values.size().to_s())
	return
end
`
	_, err := BuildV4(analyzeV4Program(t, source), "/project/src")
	if err == nil || !strings.Contains(err.Error(), "bootstrap snapshot v4 does not support Array element type Choice") {
		t.Fatalf("BuildV4() error=%v", err)
	}
}

func TestBuildV4PreservesBooleanArrayTypesAndCaptures(t *testing.T) {
	source := `alias Predicate = () -> Boolean

record Flags
	values: Array<Boolean>
end

def append(mut values: Array<Boolean>, value: Boolean): Boolean
	values.push(value)
	return values[-1]
end

def main()
	mut values: Array<Boolean> := []
	values.push(false)
	values[-1] = append(values, true)
	mut groups: Array<Array<Array<Boolean>>> := [[[false]], [values]]
	groups[0][0] = values
	flags := Flags.new(values: groups[0][0])
	predicate: Predicate := fn(): Boolean
		return flags.values[0]
	end
	if predicate()
		puts("ok")
	end
	return
end
`
	artifacts := analyzeV4Program(t, source)
	snapshot, err := BuildV4(artifacts, "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	for id, element := range map[string]string{
		"Array<Boolean>":               "Boolean",
		"Array<Array<Boolean>>":        "Array<Boolean>",
		"Array<Array<Array<Boolean>>>": "Array<Array<Boolean>>",
	} {
		found := false
		for _, definition := range snapshot.Types {
			if definition.ID == id {
				found = definition.Kind == "array" && definition.Element != nil && *definition.Element == element
			}
		}
		if !found {
			t.Fatalf("missing exact array definition %s with element %s: %#v", id, element, snapshot.Types)
		}
	}
	appendBody := v4FunctionWithSuffix(t, snapshot.Functions, "#append")
	if appendBody.Parameters[0].Type != "Array<Boolean>" || appendBody.Parameters[1].Type != "Boolean" || appendBody.Result != "Boolean" {
		t.Fatalf("Boolean array signature changed: %#v", appendBody)
	}
	predicate := v4FunctionWithSuffix(t, snapshot.Functions, "$lambda0")
	if len(predicate.Captures) != 1 || predicate.Captures[0].Type != "main#Flags" || predicate.Result != "Boolean" {
		t.Fatalf("Boolean array record capture changed: %#v", predicate)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := BuildV4(artifacts, "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	repeatedEncoded, err := json.Marshal(repeated)
	if err != nil || string(encoded) != string(repeatedEncoded) {
		t.Fatalf("Boolean array snapshot is not deterministic: %v", err)
	}
	for _, operation := range []string{"array_construct", "array_get", "array_set", "array_push", "closure_construct", "closure_call"} {
		if !strings.Contains(string(encoded), `"op":"`+operation+`"`) {
			t.Fatalf("missing %s operation", operation)
		}
	}
}

func TestBuildV4LowersNestedArrays(t *testing.T) {
	source := `def main()
	mut groups: Array<Array<Integer>> := [[1]]
	groups[0].push(2)
	if groups[0][1] == 2
		puts("ok")
	end
	return
end
`
	snapshot, err := BuildV4(analyzeV4Program(t, source), "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Array<Integer>", "Array<Array<Integer>>"} {
		if !v3HasTypeDefinition(snapshot.Types, expected) {
			t.Fatalf("snapshot is missing nested Array definition %q: %#v", expected, snapshot.Types)
		}
	}
	outer := snapshot.Types[0]
	for _, definition := range snapshot.Types {
		if definition.ID == "Array<Array<Integer>>" {
			outer = definition
		}
	}
	if outer.Kind != "array" || outer.Element == nil || *outer.Element != "Array<Integer>" {
		t.Fatalf("nested Array definition = %#v", outer)
	}
}

func TestBuildV4RejectsAssignmentToCapturedBindings(t *testing.T) {
	source := `def main()
	mut count := 0
	increment := fn()
		count += 1
		return
	end
	increment()
	return
end
`
	artifacts := analyzeV4Program(t, source)
	_, err := BuildV4(artifacts, "/project/src")
	if err == nil || !strings.Contains(err.Error(), "bootstrap snapshot v4 does not support assignment to captured binding count") {
		t.Fatalf("BuildV4() error=%v", err)
	}
}

func TestBuildV4ForwardsCapturesToNestedClosures(t *testing.T) {
	source := `alias StringFactory = () -> String
alias FactoryFactory = (String) -> StringFactory

def main()
	prefix := "nested"
	make: FactoryFactory := fn(suffix: String): StringFactory
		return fn(): String
			return prefix + suffix
		end
	end
	factory := make("-closure")
	puts(factory())
	return
end
`
	snapshot, err := BuildV4(analyzeV4Program(t, source), "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	outer := v4FunctionWithSuffix(t, snapshot.Functions, "#main$lambda0")
	if len(outer.Captures) != 1 || outer.Captures[0].Type != "String" || len(outer.Parameters) != 1 {
		t.Fatalf("unexpected outer closure signature: %#v", outer)
	}
	inner := v4FunctionWithSuffix(t, snapshot.Functions, "#main$lambda0$lambda0")
	if len(inner.Captures) != 2 || inner.Captures[0].Type != "String" || inner.Captures[1].Type != "String" {
		t.Fatalf("unexpected nested closure captures: %#v", inner.Captures)
	}
}

func analyzeV4Program(t *testing.T, source string) []*compiler.Artifact {
	t.Helper()
	artifacts, err := compiler.AnalyzeProject(
		[]compiler.SourceUnit{{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte(source)}},
		compiler.Options{Mode: "go", GoModule: "example.com/bootstrap-snapshot", SourceRoot: "/project/src", ProjectRoot: "/project"},
	)
	if err != nil {
		t.Fatal(err)
	}
	return artifacts
}

func TestBuildV4PreservesRecordArrayTypesAndCaptures(t *testing.T) {
	source := `record Row
	id: Integer
	valid: Boolean
end

record Message
	text: String
	rows: Array<Row>
end

alias Rows = Array<Row>
alias ReadRow = () -> Row

def append(mut rows: Rows, row: Row): Row
	rows.push(row)
	return rows[-1]
end

def main()
	mut rows: Rows := []
	rows.push(Row.new(id: 1, valid: false))
	rows[-1] = append(rows, Row.new(id: 2, valid: true))
	mut groups: Array<Array<Array<Row>>> := [[rows]]
	groups[0][0] = rows
	mut messages := [Message.new(text: "rows", rows: groups[0][0])]
	messages.push(Message.new(text: "more", rows: rows))
	read: ReadRow := fn(): Row
		return messages[-1].rows[-1]
	end
	row := read()
	if row.valid
		if row.id == 2
			puts("ok")
		end
	end
	return
end
`
	artifacts := analyzeV4Program(t, source)
	snapshot, err := BuildV4(artifacts, "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	for id, element := range map[string]string{
		"Array<main#Row>":               "main#Row",
		"Array<Array<main#Row>>":        "Array<main#Row>",
		"Array<Array<Array<main#Row>>>": "Array<Array<main#Row>>",
		"Array<main#Message>":           "main#Message",
	} {
		found := false
		for _, definition := range snapshot.Types {
			if definition.ID == id {
				found = definition.Kind == "array" && definition.Element != nil && *definition.Element == element
			}
		}
		if !found {
			t.Fatalf("missing exact array definition %s with element %s: %#v", id, element, snapshot.Types)
		}
	}
	appendBody := v4FunctionWithSuffix(t, snapshot.Functions, "#append")
	if appendBody.Parameters[0].Type != "Array<main#Row>" || appendBody.Parameters[1].Type != "main#Row" || appendBody.Result != "main#Row" {
		t.Fatalf("record array signature changed: %#v", appendBody)
	}
	read := v4FunctionWithSuffix(t, snapshot.Functions, "$lambda0")
	if len(read.Captures) != 1 || read.Captures[0].Type != "Array<main#Message>" || read.Result != "main#Row" {
		t.Fatalf("record array capture changed: %#v", read)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := BuildV4(artifacts, "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	repeatedEncoded, err := json.Marshal(repeated)
	if err != nil || string(encoded) != string(repeatedEncoded) {
		t.Fatalf("record array snapshot is not deterministic: %v", err)
	}
}

func v4FunctionWithSuffix(t *testing.T, functions []FunctionV4, suffix string) FunctionV4 {
	t.Helper()
	for _, function := range functions {
		if strings.HasSuffix(function.ID, suffix) {
			return function
		}
	}
	t.Fatalf("snapshot is missing function with suffix %q: %#v", suffix, functions)
	return FunctionV4{}
}

func TestBuildV4ChecksAndNormalizesAssignmentBeforeRHS(t *testing.T) {
	source := `def index(): Integer
	return -1
end

def grow(mut values: Array<Integer>): Integer
	values.push(3)
	return 9
end

def main()
	mut values := [1, 2]
	values[index()] = grow(values)
	if values[1] == 9
		puts("ok")
	end
	return
end
`
	snapshot, err := BuildV4(analyzeV4Program(t, source), "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	main := v4FunctionWithSuffix(t, snapshot.Functions, "#main")
	calls := map[string]int{}
	var checkedIndex, storedIndex string
	var rhsBlock string
	checkedBlock := ""
	for _, block := range main.Blocks {
		for _, instruction := range block.Instructions {
			switch operation := instruction.(type) {
			case Call:
				calls[operation.Function]++
				if operation.Function == "main#grow" {
					rhsBlock = block.ID
				}
			case ArrayGet:
				if checkedIndex == "" {
					checkedIndex = operation.Index
					checkedBlock = block.ID
				}
			case ArraySet:
				storedIndex = operation.Index
				if rhsBlock != block.ID {
					t.Fatal("assignment store must follow the RHS result")
				}
			}
		}
	}
	if calls["main#index"] != 1 || calls["main#grow"] != 1 {
		t.Fatalf("evaluation counts: %v", calls)
	}
	if checkedIndex == "" || storedIndex == "" || checkedIndex == storedIndex {
		t.Fatal("assignment must retain a normalized position rather than the requested negative index")
	}
	if checkedBlock != main.Entry || rhsBlock == main.Entry {
		t.Fatal("initial bounds check and normalization must precede the RHS")
	}
}

func TestBuildV4PreservesRecursiveRecordArrayDefinitions(t *testing.T) {
	cases := []struct {
		name   string
		source string
		fields map[string]map[string]string
	}{
		{"self", `record Node
	id: Integer
	children: Array<Node>
end

def main()
	mut children: Array<Node> := []
	node := Node.new(id: 42, children: children)
	children.push(node)
	puts("ok")
end
`, map[string]map[string]string{"main#Node": {"id": "Integer", "children": "Array<main#Node>"}}},
		{"mutual", `record Parent
	id: Integer
	children: Array<Child>
end

record Child
	parents: Array<Parent>
end

def main()
	mut parents: Array<Parent> := []
	child := Child.new(parents: parents)
	parent := Parent.new(id: 42, children: [child])
	parents.push(parent)
	puts("ok")
end
`, map[string]map[string]string{
			"main#Parent": {"id": "Integer", "children": "Array<main#Child>"},
			"main#Child":  {"parents": "Array<main#Parent>"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			artifacts := analyzeV4Program(t, tc.source)
			snapshot, err := BuildV4(artifacts, "/project/src")
			if err != nil {
				t.Fatal(err)
			}
			definitions := map[string]TypeDefinition{}
			for _, definition := range snapshot.Types {
				if _, exists := definitions[definition.ID]; exists {
					t.Fatalf("duplicate type %q", definition.ID)
				}
				definitions[definition.ID] = definition
			}
			for id, fields := range tc.fields {
				definition := definitions[id]
				if definition.Kind != "record" || definition.Fields == nil || len(*definition.Fields) != len(fields) {
					t.Fatalf("incomplete recursive record %s: %#v", id, definition)
				}
				for _, field := range *definition.Fields {
					if fields[field.Name] != field.Type {
						t.Fatalf("field identity differs in %s: %#v", id, field)
					}
				}
				array := definitions["Array<"+id+">"]
				if array.Kind != "array" || array.Element == nil || *array.Element != id {
					t.Fatalf("missing recursive Array element %s: %#v", id, array)
				}
			}
			encoded, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			repeated, err := BuildV4(artifacts, "/project/src")
			if err != nil {
				t.Fatal(err)
			}
			repeatedEncoded, err := json.Marshal(repeated)
			if err != nil || string(encoded) != string(repeatedEncoded) {
				t.Fatalf("recursive record Array snapshot is not deterministic: %v", err)
			}
		})
	}
}

func TestBuildV4HashValues(t *testing.T) {
	source := `record Index
 entries: Hash<String, Integer>
end

def key(mut order: Array<Integer>): String
 order.push(1)
 return "same"
end

def value(mut order: Array<Integer>): Integer
 order.push(2)
 return 9
end

def main()
 mut order: Array<Integer> := []
 mut index := Index.new(entries: {"same" => 1, "same" => 2})
 index.entries[key(order)] = value(order)
 if index.entries.key?("same")
  if index.entries["same"] == index.entries.fetch("same")
   puts("ok")
  end
 end
end
`
	snapshot, err := BuildV4(analyzeV4Program(t, source), "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	var main FunctionV4
	for _, function := range snapshot.Functions {
		if function.Name == "main" {
			main = function
		}
	}
	found := map[string]int{}
	calls := []string{}
	for _, block := range main.Blocks {
		for _, instruction := range block.Instructions {
			switch op := instruction.(type) {
			case HashConstruct:
				found[op.Op]++
				if len(op.Keys) != 2 || len(op.Values) != 2 {
					t.Fatalf("literal entries lost: %#v", op)
				}
			case HashGet:
				found[op.Op]++
			case HashContains:
				found[op.Op]++
			case HashSet:
				found[op.Op]++
				if len(calls) != 2 || calls[0] != "main#key" || calls[1] != "main#value" {
					t.Fatalf("assignment evaluation order: %v", calls)
				}
			case Call:
				calls = append(calls, op.Function)
			}
		}
	}
	if found["hash_construct"] != 1 || found["hash_set"] != 1 || found["hash_get"] != 2 || found["hash_contains"] != 1 {
		t.Fatalf("missing Hash operations: %v", found)
	}
	for _, definition := range snapshot.Types {
		if definition.Kind == "hash" {
			if definition.ID != "Hash<String, Integer>" || definition.Key == nil || *definition.Key != "String" || definition.Element == nil || *definition.Element != "Integer" {
				t.Fatalf("incorrect Hash type: %#v", definition)
			}
			return
		}
	}
	t.Fatal("missing Hash type")
}

func TestBuildV4HashSubsetRejectsUnsupportedOperations(t *testing.T) {
	for _, source := range []string{
		"def main()\n values := {1 => 2}\n if values[1] == 2\n puts(\"ok\")\n end\nend\n",
		"def main()\n values := {\"key\" => \"value\"}\n puts(values[\"key\"])\nend\n",
		"def main()\n values := {\"key\" => 2}\n if values.size() == 1\n puts(\"ok\")\n end\nend\n",
	} {
		if _, err := BuildV4(analyzeV4Program(t, source), "/project/src"); err == nil {
			t.Fatalf("unsupported Hash program accepted: %s", source)
		}
	}
	snapshot, err := BuildV4(analyzeV4Program(t, "def main()\n mut values: Hash<String, Integer> := {}\n values[\"key\"] = 2\n if values[\"key\"] == 2\n puts(\"ok\")\n end\nend\n"), "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	for _, function := range snapshot.Functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instructions {
				if op, ok := instruction.(HashConstruct); ok && (op.Keys == nil || op.Values == nil || len(op.Keys) != 0 || len(op.Values) != 0) {
					t.Fatalf("empty Hash lists must be present: %#v", op)
				}
			}
		}
	}
}
