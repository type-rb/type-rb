package languageservice_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/languageservice"
)

const semanticProgram = `record User
	name: String
	age: Integer
end

def greet(name: String, suffix: String): String
	return "Hello, " + name + suffix
end

def main()
	user := User.new(name: "Ada", age: 37)
	puts(greet(user.name, "!"))
	return
end
`

func TestSemanticHoverUsesCheckedSymbolsAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact := compile(t, mode, semanticProgram)
			service := languageservice.New(mode)
			service.Update([]*ir.Program{artifact.IR}, "repl")

			for _, test := range []struct {
				needle string
				cursor int
				want   string
			}{
				{needle: "greet(user", cursor: len("gr"), want: "greet(name: String, suffix: String): String"},
				{needle: "user.name", cursor: len("user.na"), want: "name: String"},
				{needle: "user := User", cursor: len("us"), want: "user: User"},
				{needle: "User.new", cursor: len("Us"), want: "record User"},
			} {
				start := strings.LastIndex(semanticProgram, test.needle)
				if start < 0 {
					t.Fatalf("missing test source %q", test.needle)
				}
				hover, ok := service.Hover(semanticProgram, start+test.cursor)
				if !ok || hover.Detail != test.want {
					t.Fatalf("Hover(%q)=(%#v, %v), want %q", test.needle, hover, ok, test.want)
				}
			}
		})
	}
}

func TestSemanticSignatureHelpTracksPositionalAndKeywordArguments(t *testing.T) {
	artifact := compile(t, "go", semanticProgram)
	service := languageservice.New("go")
	service.Update([]*ir.Program{artifact.IR}, "repl")

	greetCursor := strings.LastIndex(semanticProgram, `"!"`) + len(`"!"`)
	help, ok := service.Signatures(semanticProgram, greetCursor)
	if !ok || len(help.Signatures) != 1 {
		t.Fatalf("greet signatures=(%#v, %v)", help, ok)
	}
	if help.Signatures[0].Label != "greet(name: String, suffix: String): String" || help.ActiveParameter != 1 {
		t.Fatalf("greet signature=%#v", help)
	}
	if got := help.Signatures[0].Parameters; len(got) != 2 || got[1].Label != "suffix: String" {
		t.Fatalf("greet parameters=%#v", got)
	}

	constructorCursor := strings.Index(semanticProgram, "37") + len("37")
	help, ok = service.Signatures(semanticProgram, constructorCursor)
	if !ok || help.Signatures[0].Label != "new(name: String, age: Integer): User" || help.ActiveParameter != 1 {
		t.Fatalf("constructor signature=(%#v, %v)", help, ok)
	}
}

func TestSemanticQueriesReturnNoResultOutsideSymbolsAndCalls(t *testing.T) {
	service := languageservice.New("go")
	if _, ok := service.Hover("value := 1\n", len("value :")); ok {
		t.Fatal("punctuation unexpectedly returned hover information")
	}
	if _, ok := service.Signatures("value := 1\n", len("value := 1")); ok {
		t.Fatal("non-call unexpectedly returned signature help")
	}
}

func TestDefinitionResolvesImportedMembersAndLexicalBindings(t *testing.T) {
	root := t.TempDir()
	modelPath := filepath.Join(root, "models", "user.trb")
	mainPath := filepath.Join(root, "main.trb")
	modelSource := "record User\n\tname: String\nend\n"
	mainSource := `import { User } from models/user

def user_name(user: User): String
	local_name := user.name
	return local_name
end

def main()
	user := User.new(name: "Ada")
	puts(user_name(user))
	return
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			packageName := ""
			options := compiler.Options{Mode: mode, ModulePath: "main"}
			if mode == "go" {
				packageName = "main"
				options.Package = "main"
				options.GoModule = "example.com/definitions"
			}
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{
				{Filename: modelPath, ModulePath: "models/user", Package: packageName, Source: []byte(modelSource)},
				{Filename: mainPath, ModulePath: "main", Package: packageName, Source: []byte(mainSource)},
			}, options)
			if err != nil {
				t.Fatal(err)
			}
			programs := make([]*ir.Program, 0, len(artifacts))
			for _, artifact := range artifacts {
				programs = append(programs, artifact.IR)
			}
			service := languageservice.New(mode)
			service.Update(programs, "main")

			for _, test := range []struct {
				needle     string
				cursor     int
				wantPath   string
				wantName   string
				wantSource string
			}{
				{needle: "User.new", cursor: len("Us"), wantPath: modelPath, wantName: "User", wantSource: modelSource},
				{needle: "user.name", cursor: len("user.na"), wantPath: modelPath, wantName: "name", wantSource: modelSource},
				{needle: "return local_name", cursor: len("return loc"), wantPath: mainPath, wantName: "local_name", wantSource: mainSource},
				{needle: "local_name := user", cursor: len("local_name := us"), wantPath: mainPath, wantName: "user", wantSource: mainSource},
			} {
				start := strings.Index(mainSource, test.needle)
				definition, ok := service.Definition(mainPath, mainSource, start+test.cursor)
				if !ok || definition.Path != test.wantPath || definition.Name != test.wantName {
					t.Fatalf("Definition(%q)=(%#v, %v), want %s:%s", test.needle, definition, ok, test.wantPath, test.wantName)
				}
				if definition.Range.Start < 0 || definition.Range.End > len(test.wantSource) || definition.ID == "" {
					t.Fatalf("invalid definition location %#v", definition)
				}
				if definition.Path == mainPath && test.wantSource[definition.Range.Start:definition.Range.End] != definition.Name {
					t.Fatalf("lexical definition range=%q, want %q", test.wantSource[definition.Range.Start:definition.Range.End], definition.Name)
				}
			}
		})
	}
}
