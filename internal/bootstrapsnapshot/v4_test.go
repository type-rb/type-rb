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
