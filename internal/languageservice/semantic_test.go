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

func TestSemanticHoverUsesAssignmentNarrowedNullableType(t *testing.T) {
	const source = `def display_name(mut name: String?): String
	if name == nil
		return "anonymous"
	end
	name = name.strip().downcase()
	return name
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact := compile(t, mode, source)
		service := languageservice.New(mode)
		service.Update([]*ir.Program{artifact.IR}, "repl")
		start := strings.LastIndex(source, "name")
		hover, ok := service.Hover(source, start+len("na"))
		if !ok || hover.Detail != "name: String" {
			t.Fatalf("%s assignment-narrowed hover=(%#v, %v), want name: String", mode, hover, ok)
		}
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

func TestDefinitionResolvesImportedJSXComponentUse(t *testing.T) {
	root := t.TempDir()
	pagePath := filepath.Join(root, "features", "insurers", "components", "InsurerPage", "index.trb")
	entryPath := filepath.Join(root, "routes", "insurers.trb")
	pageSource := `import { ReactNode } from trb/platform/typescript/react

def InsurerPage(): ReactNode
	return <p>Ready</p>
end
`
	modulePath := "features/insurers/components/InsurerPage"
	moduleSpecifier := "./" + modulePath + ".trb"
	entrySource := `import { InsurerPage } from "./features/insurers/components/InsurerPage.trb"
import { ReactNode } from trb/platform/typescript/react

def InsurerListRoutePage(): ReactNode
	return <InsurerPage />
end
`
	artifacts, err := compiler.CompileProject([]compiler.SourceUnit{
		{Filename: pagePath, ModulePath: "features/insurers/components/InsurerPage/index", Source: []byte(pageSource)},
		{Filename: entryPath, ModulePath: "routes/insurers", Source: []byte(entrySource)},
	}, compiler.Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err != nil {
		t.Fatal(err)
	}
	programs := make([]*ir.Program, 0, len(artifacts))
	for _, artifact := range artifacts {
		programs = append(programs, artifact.IR)
	}
	service := languageservice.New("typescript")
	service.Update(programs, "routes/insurers")

	pathStart := strings.Index(entrySource, moduleSpecifier)
	for cursor := pathStart; cursor < pathStart+len(moduleSpecifier); cursor++ {
		definition, ok := service.Definition(entryPath, entrySource, cursor)
		if !ok || definition.Path != pagePath || definition.Origin == nil {
			t.Fatalf("module definition at offset %d=(%#v, %v), want %s", cursor-pathStart, definition, ok, pagePath)
		}
		if *definition.Origin != (languageservice.OffsetRange{Start: pathStart - 1, End: pathStart + len(moduleSpecifier) + 1}) {
			t.Fatalf("module origin=%#v, want complete import path", definition.Origin)
		}
	}

	cursor := strings.LastIndex(entrySource, "InsurerPage") + len("Insurer")
	definition, ok := service.Definition(entryPath, entrySource, cursor)
	if !ok || definition.Path != pagePath || definition.Name != "InsurerPage" {
		t.Fatalf("JSX definition=(%#v, %v), want %s:InsurerPage", definition, ok, pagePath)
	}
	if got := pageSource[definition.Range.Start:definition.Range.End]; !strings.Contains(got, "def InsurerPage") {
		t.Fatalf("JSX definition range=%q, want InsurerPage declaration", got)
	}
}

func TestImplementationsResolveInterfaceTypesAndMethods(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "renderers.trb")
	source := `interface Renderer
	render(input: String): String
end

class HTMLRenderer implements Renderer
	def render(input: String): String
		return "<p>" + input + "</p>"
	end
end

class TextRenderer implements Renderer
	def render(input: String): String
		return input
	end
end
`
	artifact := compile(t, "go", source)
	artifact.IR.SourcePath = path
	service := languageservice.New("go")
	service.Update([]*ir.Program{artifact.IR}, "repl")

	methodCursor := strings.Index(source, "render(input") + len("render")
	methods, ok := service.Implementations(path, source, methodCursor)
	if !ok || len(methods) != 2 {
		t.Fatalf("method implementations=(%#v, %v), want two", methods, ok)
	}
	for _, implementation := range methods {
		if implementation.Path != path || implementation.Name != "render" {
			t.Fatalf("method implementation=%#v", implementation)
		}
	}

	typeCursor := strings.Index(source, "Renderer") + len("Ren")
	types, ok := service.Implementations(path, source, typeCursor)
	if !ok || len(types) != 2 || types[0].Name != "HTMLRenderer" || types[1].Name != "TextRenderer" {
		t.Fatalf("type implementations=(%#v, %v), want HTMLRenderer and TextRenderer", types, ok)
	}
}

func TestImplementationsResolveImportedInterfaceMethods(t *testing.T) {
	root := t.TempDir()
	contractPath := filepath.Join(root, "contracts", "renderer.trb")
	htmlPath := filepath.Join(root, "renderers", "html.trb")
	textPath := filepath.Join(root, "renderers", "text.trb")
	contractSource := "interface Renderer\n\trender(input: String): String\nend\n"
	implementationSource := func(name, body string) string {
		return "import { Renderer } from contracts/renderer\n\nclass " + name + " implements Renderer\n\tdef render(input: String): String\n\t\treturn " + body + "\n\tend\nend\n"
	}
	artifacts, err := compiler.CompileProject([]compiler.SourceUnit{
		{Filename: contractPath, ModulePath: "contracts/renderer", Package: "main", Source: []byte(contractSource)},
		{Filename: htmlPath, ModulePath: "renderers/html", Package: "main", Source: []byte(implementationSource("HTMLRenderer", `"<p>" + input + "</p>"`))},
		{Filename: textPath, ModulePath: "renderers/text", Package: "main", Source: []byte(implementationSource("TextRenderer", "input"))},
	}, compiler.Options{Mode: "go", GoModule: "example.com/implementations"})
	if err != nil {
		t.Fatal(err)
	}
	programs := make([]*ir.Program, 0, len(artifacts))
	for _, artifact := range artifacts {
		programs = append(programs, artifact.IR)
	}
	service := languageservice.New("go")
	service.Update(programs, "contracts/renderer")
	methodCursor := strings.Index(contractSource, "render") + len("render")
	implementations, ok := service.Implementations(contractPath, contractSource, methodCursor)
	if !ok || len(implementations) != 2 || implementations[0].Path != htmlPath || implementations[1].Path != textPath {
		t.Fatalf("imported method implementations=(%#v, %v)", implementations, ok)
	}
}

func TestReferencesUseCheckedIdentityAcrossProjectFiles(t *testing.T) {
	root := t.TempDir()
	modelPath := filepath.Join(root, "models", "user.trb")
	mainPath := filepath.Join(root, "main.trb")
	modelSource := "record User\n\tname: String\nend\n\nrecord Product\n\tname: String\nend\n"
	mainSource := `import { Product, User } from models/user

def user_name(user: User): String
	local_name := user.name
	return local_name
end

def main()
	user := User.new(name: "Ada")
	product := Product.new(name: "Book")
	puts(user_name(user))
	puts(product.name)
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
				options.GoModule = "example.com/references"
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
			documents := []languageservice.SemanticDocument{
				{Path: modelPath, Source: modelSource, Mode: mode, Context: languageservice.BuildContext(programs, "models/user")},
				{Path: mainPath, Source: mainSource, Mode: mode, Context: languageservice.BuildContext(programs, "main")},
			}

			fieldCursor := strings.Index(mainSource, "user.name") + len("user.na")
			fieldReferences, ok := languageservice.References(languageservice.SemanticRequest{
				Path: mainPath, Source: mainSource, Cursor: fieldCursor, Mode: mode, Context: documents[1].Context,
			}, documents, true)
			if !ok || len(fieldReferences) != 3 {
				t.Fatalf("field references=(%#v, %v), want declaration, member, and constructor keyword", fieldReferences, ok)
			}
			if declarationCount(fieldReferences, modelPath) != 1 {
				t.Fatalf("field references have no unique model declaration: %#v", fieldReferences)
			}

			typeCursor := strings.LastIndex(mainSource, "User.new") + len("Us")
			typeReferences, ok := languageservice.References(languageservice.SemanticRequest{
				Path: mainPath, Source: mainSource, Cursor: typeCursor, Mode: mode, Context: documents[1].Context,
			}, documents, true)
			if !ok || len(typeReferences) != 4 || declarationCount(typeReferences, modelPath) != 1 {
				t.Fatalf("type references=(%#v, %v), want four checked occurrences", typeReferences, ok)
			}
			constructorCursor := strings.LastIndex(mainSource, "User.new") + len("User.ne")
			if references, renameable := languageservice.References(languageservice.SemanticRequest{
				Path: mainPath, Source: mainSource, Cursor: constructorCursor, Mode: mode, Context: documents[1].Context,
			}, documents, true); renameable {
				t.Fatalf("generated constructor is renameable: %#v", references)
			}

			localCursor := strings.Index(mainSource, "return local_name") + len("return loc")
			localReferences, ok := languageservice.References(languageservice.SemanticRequest{
				Path: mainPath, Source: mainSource, Cursor: localCursor, Mode: mode, Context: documents[1].Context,
			}, documents, false)
			if !ok || len(localReferences) != 1 || localReferences[0].Declaration {
				t.Fatalf("local references=(%#v, %v), want one use without declaration", localReferences, ok)
			}
		})
	}
}

func declarationCount(references []languageservice.ReferenceInfo, path string) int {
	count := 0
	for _, reference := range references {
		if reference.Declaration && reference.Path == path {
			count++
		}
	}
	return count
}

func TestReferencesKeepShadowedBlockBindingsSeparate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.trb")
	source := `def main()
	value := 1
	[1, 2].each do |value|
		puts(value)
	end
	puts(value)
	return
end
`
	document := languageservice.SemanticDocument{Path: path, Source: source, Mode: "go", Context: languageservice.Context{TypeMembers: map[string][]languageservice.Symbol{}}}

	outerCursor := strings.LastIndex(source, "puts(value)") + len("puts(val")
	outer, ok := languageservice.References(languageservice.SemanticRequest{
		Path: path, Source: source, Cursor: outerCursor, Mode: "go", Context: document.Context,
	}, []languageservice.SemanticDocument{document}, true)
	if !ok || len(outer) != 2 {
		t.Fatalf("outer references=(%#v, %v), want declaration and final use", outer, ok)
	}

	innerCursor := strings.Index(source, "puts(value)") + len("puts(val")
	inner, ok := languageservice.References(languageservice.SemanticRequest{
		Path: path, Source: source, Cursor: innerCursor, Mode: "go", Context: document.Context,
	}, []languageservice.SemanticDocument{document}, true)
	if !ok || len(inner) != 2 {
		t.Fatalf("inner references=(%#v, %v), want block parameter and block use", inner, ok)
	}
	if outer[0].ID == inner[0].ID {
		t.Fatalf("shadowed bindings share identity %q", outer[0].ID)
	}
}

func TestResultCatchBindingUsesItsOwnLexicalScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.trb")
	source := `value := load() catch |error|
	puts(error)
end
puts(error)
`
	context := languageservice.Context{TypeMembers: map[string][]languageservice.Symbol{}}
	bindingStart := strings.Index(source, "error")
	insideCursor := strings.Index(source[bindingStart+len("error"):], "error") + bindingStart + len("error") + 1
	definition, ok := languageservice.Definition(languageservice.SemanticRequest{
		Path: path, Source: source, Cursor: insideCursor, Mode: "go", Context: context,
	})
	if !ok || definition.Range.Start != bindingStart || definition.Name != "error" {
		t.Fatalf("inside definition=(%#v, %v), want catch binding at %d", definition, ok, bindingStart)
	}

	outsideCursor := strings.LastIndex(source, "error") + 1
	if definition, ok := languageservice.Definition(languageservice.SemanticRequest{
		Path: path, Source: source, Cursor: outsideCursor, Mode: "go", Context: context,
	}); ok {
		t.Fatalf("catch binding escaped its body: %#v", definition)
	}
}

func TestReferencesRenameClassFieldWithoutRemovingInstanceMarker(t *testing.T) {
	source := `class Box
	readonly @label: String := "items"
end

def main()
	box := Box.new()
	puts(box.label)
	return
end
`
	artifact := compile(t, "go", source)
	path := artifact.IR.SourcePath
	context := languageservice.BuildContext([]*ir.Program{artifact.IR}, "repl")
	document := languageservice.SemanticDocument{Path: path, Source: source, Mode: "go", Context: context}
	cursor := strings.Index(source, "box.label") + len("box.lab")
	references, ok := languageservice.References(languageservice.SemanticRequest{
		Path: path, Source: source, Cursor: cursor, Mode: "go", Context: context,
	}, []languageservice.SemanticDocument{document}, true)
	if !ok || len(references) != 2 {
		t.Fatalf("class field references=(%#v, %v)", references, ok)
	}
	declarationStart := strings.Index(source, "@label") + 1
	if references[0].Range.Start != declarationStart || source[references[0].Range.Start:references[0].Range.End] != "label" {
		t.Fatalf("class field declaration range=%#v", references[0])
	}
}
