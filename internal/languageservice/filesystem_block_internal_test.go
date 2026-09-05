package languageservice

import (
	"testing"

	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

func TestCompilerBlockParameterInferenceTypesScopedFileBinding(t *testing.T) {
	context := StandardImportCandidates("go")
	source := "import trb/std/path\nimport trb/std/file\n\ndef inspect(path: Path)\n\tFile.open(path) do |file|\n\t\tfile"
	symbols := lexicalSymbols(source, len(source), context)
	for _, symbol := range symbols {
		if symbol.Name == "file" {
			if symbol.Kind != CompletionParameter || symbol.Type.String() != "File" {
				t.Fatalf("scoped binding=%#v", symbol)
			}
			return
		}
	}
	t.Fatalf("scoped file binding is missing: %#v", symbols)
}

func TestCompilerBlockParameterInferenceDoesNotEscapeClosedScope(t *testing.T) {
	context := StandardImportCandidates("go")
	source := "File.open(Path.new(path)) do |file|\n\tfile.read(max_bytes: 1)\nend\nfile"
	for _, symbol := range lexicalSymbols(source, len(source), context) {
		if symbol.Name == "file" && symbol.Kind == CompletionParameter {
			t.Fatalf("closed scoped binding escaped into completion: %#v", symbol)
		}
	}
}

func TestUnrelatedFileDeclarationDoesNotReceiveScopedFileMethods(t *testing.T) {
	unrelated := types.Type{
		Kind:        types.Named,
		Name:        "File",
		Declaration: identity.Declaration{Module: "example/file", Name: "File", Kind: identity.Class},
	}
	if methods := stdlib.ReceiverMethods(unrelated); len(methods) != 0 {
		t.Fatalf("unrelated File received compiler-owned methods: %#v", methods)
	}
	if members := receiverMembers(unrelated, emptyContext()); len(members) != 0 {
		t.Fatalf("completion leaked compiler-owned File methods: %#v", members)
	}
}
