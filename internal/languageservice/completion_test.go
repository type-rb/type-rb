package languageservice_test

import (
	"slices"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/languageservice"
)

const completionProgram = `class User
	@_name: String

	def initialize(name: String)
		@_name = name
		return
	end

	def name(): String
		return @_name
	end
end

enum State
	Open
	Closed
end

def greet(name: String): String
	return "Hello, " + name
end

user := User.new("Ada")
str_a := "hello"
`

func TestCompletionUsesCheckedContextAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact := compile(t, mode, completionProgram)
			service := languageservice.New(mode)
			service.Update([]*ir.Program{artifact.IR}, "repl")

			for _, test := range []struct {
				source     string
				want       string
				insertText string
				kind       languageservice.CompletionKind
			}{
				{source: "gre", want: "greet", insertText: "greet", kind: languageservice.CompletionFunction},
				{source: "user.na", want: "name", insertText: "name()", kind: languageservice.CompletionMethod},
				{source: "State::Cl", want: "Closed", insertText: "Closed", kind: languageservice.CompletionEnumMember},
				{source: `"hello".si`, want: "size", insertText: "size()", kind: languageservice.CompletionMethod},
				{source: "str_a.siz", want: "size", insertText: "size()", kind: languageservice.CompletionMethod},
				{source: `["a", "b"].jo`, want: "join", insertText: "join", kind: languageservice.CompletionMethod},
				{source: "0.25.to_", want: "to_i", insertText: "to_i()", kind: languageservice.CompletionMethod},
				{source: "0.25.to_", want: "to_s", insertText: "to_s()", kind: languageservice.CompletionMethod},
				{source: "1.pos", want: "positive?", insertText: "positive?()", kind: languageservice.CompletionMethod},
				{source: "1.mi", want: "min", insertText: "min", kind: languageservice.CompletionMethod},
				{source: "0.25.fin", want: "finite?", insertText: "finite?()", kind: languageservice.CompletionMethod},
				{source: "0.25.flo", want: "floor", insertText: "floor()", kind: languageservice.CompletionMethod},
				{source: "true.to_", want: "to_s", insertText: "to_s()", kind: languageservice.CompletionMethod},
			} {
				items := service.Complete(test.source, len(test.source))
				item, ok := findCompletion(items, test.want)
				if !ok {
					t.Fatalf("Complete(%q)=%v, want %q", test.source, labels(items), test.want)
				}
				if item.Kind != test.kind {
					t.Errorf("Complete(%q) kind=%q, want %q", test.source, item.Kind, test.kind)
				}
				if item.InsertText != test.insertText {
					t.Errorf("Complete(%q) insert text=%q, want %q", test.source, item.InsertText, test.insertText)
				}
				if got := test.source[item.Replacement.Start:len(test.source)]; got == "" {
					t.Errorf("Complete(%q) returned empty replacement prefix", test.source)
				}
			}
		})
	}
}

func TestCompletionHandlesIncompleteFunctionParameters(t *testing.T) {
	service := languageservice.New("go")
	source := "def welcome(name: String)\n\tna"
	items := service.Complete(source, len(source))
	item, ok := findCompletion(items, "name")
	if !ok {
		t.Fatalf("completion labels=%v, want name", labels(items))
	}
	if item.Kind != languageservice.CompletionParameter || item.Detail != "String" {
		t.Fatalf("name completion=%#v", item)
	}
}

func TestCompletionIncludesExplicitImportedNamesAndNamespaces(t *testing.T) {
	artifacts, err := compiler.CompileProject([]compiler.SourceUnit{
		{Filename: "models/user.trb", ModulePath: "models/user", Source: []byte("record User\n\tname: String\nend\n")},
		{Filename: "models/state.trb", ModulePath: "models/state", Source: []byte("enum State\n\tOpen\n\tClosed\nend\n")},
		{Filename: ".trb-repl.trb", ModulePath: "repl", Source: []byte("import { User } from models/user\nimport models/state as states\nimport { Result } from trb/std/result\nimport trb/std/strings\n")},
	}, compiler.Options{Mode: "go", Package: "main", ModulePath: "repl", AllowUnusedImports: true})
	if err != nil {
		t.Fatal(err)
	}
	programs := make([]*ir.Program, 0, len(artifacts))
	for _, artifact := range artifacts {
		programs = append(programs, artifact.IR)
	}
	service := languageservice.New("go")
	service.Update(programs, "repl")

	if _, ok := findCompletion(service.Complete("Us", 2), "User"); !ok {
		t.Fatal("named project import was not completed")
	}
	if _, ok := findCompletion(service.Complete("strings.up", len("strings.up")), "uppercase"); !ok {
		t.Fatal("standard package namespace member was not completed")
	}
	if _, ok := findCompletion(service.Complete("Result::O", len("Result::O")), "Ok"); !ok {
		t.Fatal("imported enum member was not completed")
	}
	if _, ok := findCompletion(service.Complete("states::State::O", len("states::State::O")), "Open"); !ok {
		t.Fatal("aliased project namespace member was not completed")
	}

	withoutImport := languageservice.New("go")
	programs = append(programs, &ir.Program{Mode: "go", ModulePath: "blank"})
	withoutImport.Update(programs, "blank")
	if _, ok := findCompletion(withoutImport.Complete("Us", 2), "User"); ok {
		t.Fatal("unimported project name was completed")
	}
}

func TestCompletionDoesNotExposeImplicitRuntimeImports(t *testing.T) {
	artifact := compile(t, "go", "missing := [1].try_fetch(9)\n")
	service := languageservice.New("go")
	service.Update([]*ir.Program{artifact.IR}, "repl")
	for _, name := range []string{"Result", "IndexLookupError", "KeyLookupError", "NumberParseError", "HexDecodeError"} {
		if _, ok := findCompletion(service.Complete(name[:3], 3), name); ok {
			t.Fatalf("implicit runtime dependency %s was exposed as a source import", name)
		}
	}
}

func TestCompletionReplacesTheIdentifierAroundTheCursor(t *testing.T) {
	service := languageservice.New("go")
	items := service.Complete("retxx", 3)
	item, ok := findCompletion(items, "return")
	if !ok {
		t.Fatalf("completion labels=%v, want return", labels(items))
	}
	if item.Replacement != (languageservice.OffsetRange{Start: 0, End: 5}) {
		t.Fatalf("replacement=%#v", item.Replacement)
	}
}

func compile(t *testing.T, mode, source string) *compiler.Artifact {
	t.Helper()
	options := compiler.Options{Mode: mode, ModulePath: "repl", AllowUnusedImports: true}
	if mode == "go" {
		options.Package = "main"
	}
	artifact, err := compiler.CompileWithOptions(".trb-repl.trb", []byte(source), options)
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func findCompletion(items []languageservice.CompletionItem, label string) (languageservice.CompletionItem, bool) {
	for _, item := range items {
		if item.Label == label {
			return item, true
		}
	}
	return languageservice.CompletionItem{}, false
}

func labels(items []languageservice.CompletionItem) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Label)
	}
	slices.Sort(result)
	return result
}
