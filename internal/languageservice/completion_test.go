package languageservice_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/languageservice"
	"github.com/type-rb/type-rb/internal/nativepackage"
	"github.com/type-rb/type-rb/internal/types"
)

func TestBuildContextMatchesBuildContexts(t *testing.T) {
	programs := []*ir.Program{
		{ModulePath: "models/user", SourcePath: "models/user.trb", Statements: []ir.Statement{
			&ir.Record{Name: "User", Body: []ir.Statement{&ir.RecordField{Name: "name", Type: types.FromName("String")}}},
		}},
		{ModulePath: "models/state", SourcePath: "models/state.trb", Statements: []ir.Statement{
			&ir.Enum{Name: "State", Body: []ir.Statement{&ir.EnumMember{Name: "Open"}, &ir.EnumMember{Name: "Closed"}}},
		}},
		{ModulePath: "repl", SourcePath: ".trb-repl.trb", Statements: []ir.Statement{
			&ir.Import{Path: "models/user", Symbols: []string{"User"}},
			&ir.Import{Path: "models/state", Symbols: []string{"State"}},
		}},
	}
	contexts := languageservice.BuildContexts(programs)
	for modulePath, expected := range contexts {
		t.Run(modulePath, func(t *testing.T) {
			actual := languageservice.BuildContext(programs, modulePath)
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("BuildContext(%q) did not match BuildContexts", modulePath)
			}
		})
	}
}

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

enum OrderStatus
	Pending = "PENDING"
	Completed = "COMPLETED"

	def terminal?(): Boolean
		return self == OrderStatus::Completed
	end
end

def greet(name: String): String
	return "Hello, " + name
end

user := User.new("Ada")
str_a := "hello"
numbers := [1, 2, 3]
status := OrderStatus::Pending
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
				{source: "OrderStatus.from", want: "from_raw", insertText: "from_raw", kind: languageservice.CompletionMethod},
				{source: "status.raw", want: "raw_value", insertText: "raw_value()", kind: languageservice.CompletionMethod},
				{source: "status.term", want: "terminal?", insertText: "terminal?()", kind: languageservice.CompletionMethod},
				{source: `"hello".si`, want: "size", insertText: "size()", kind: languageservice.CompletionMethod},
				{source: "str_a.siz", want: "size", insertText: "size()", kind: languageservice.CompletionMethod},
				{source: `["a", "b"].jo`, want: "join", insertText: "join", kind: languageservice.CompletionMethod},
				{source: "numbers.an", want: "any?", insertText: "any?", kind: languageservice.CompletionMethod},
				{source: "numbers.find_", want: "find_index", insertText: "find_index", kind: languageservice.CompletionMethod},
				{source: "numbers.sort_", want: "sort_by", insertText: "sort_by", kind: languageservice.CompletionMethod},
				{source: "numbers.sort_d", want: "sort_descending", insertText: "sort_descending()", kind: languageservice.CompletionMethod},
				{source: "numbers.uni", want: "uniq", insertText: "uniq()", kind: languageservice.CompletionMethod},
				{source: "numbers.con", want: "concat", insertText: "concat", kind: languageservice.CompletionMethod},
				{source: "numbers.concurrent_", want: "concurrent_map", insertText: "concurrent_map", kind: languageservice.CompletionMethod},
				{source: "[1, 2, 3].no", want: "none?", insertText: "none?", kind: languageservice.CompletionMethod},
				{source: "(1..10).to", want: "to_a", insertText: "to_a()", kind: languageservice.CompletionMethod},
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

func TestCompletionOffersResultControlFlowKeywords(t *testing.T) {
	service := languageservice.New("go")
	for _, keyword := range []string{"try", "catch"} {
		item, ok := findCompletion(service.Complete(keyword[:2], 2), keyword)
		if !ok || item.Kind != languageservice.CompletionKeyword {
			t.Errorf("%s completion=(%#v, %v), want keyword", keyword, item, ok)
		}
	}
}

func TestCompletionOffersAliasAndNewtypeKeywords(t *testing.T) {
	service := languageservice.New("go")
	for _, keyword := range []string{"alias", "newtype"} {
		item, ok := findCompletion(service.Complete(keyword[:2], 2), keyword)
		if !ok || item.Kind != languageservice.CompletionKeyword {
			t.Errorf("%s completion=(%#v, %v), want keyword", keyword, item, ok)
		}
	}
	if item, ok := findCompletion(service.Complete("ty", 2), "type"); ok {
		t.Errorf("legacy type completion=%#v, want no keyword", item)
	}
}

func TestCompletionOmitsRemovedSyntaxKeywords(t *testing.T) {
	service := languageservice.New("go")
	for _, keyword := range []string{"attempt", "fails", "and", "or", "not"} {
		if item, ok := findCompletion(service.Complete(keyword[:2], 2), keyword); ok {
			t.Errorf("%s completion=%#v, want no legacy keyword", keyword, item)
		}
	}
}

func TestCompletionOffersOnlyTypesAtTypePositions(t *testing.T) {
	service := languageservice.New("go")
	for _, test := range []struct {
		source string
		want   string
	}{
		{source: "record User\n\tid: Int", want: "Integer"},
		{source: "def render(value: Str", want: "String"},
		{source: "alias Names = Array<Str", want: "String"},
		{source: "newtype UserName = Str", want: "String"},
		{source: "record Box<T>\n\tvalue: T", want: "T"},
		{source: "def identity<T>(value: T", want: "T"},
	} {
		items := service.Complete(test.source, len(test.source))
		if _, ok := findCompletion(items, test.want); !ok {
			t.Errorf("Complete(%q)=%v, want %q", test.source, labels(items), test.want)
		}
		for _, item := range items {
			if item.Kind != languageservice.CompletionType {
				t.Errorf("Complete(%q) included non-type item %#v", test.source, item)
			}
		}
	}
}

func TestCompletionDoesNotTreatValueArgumentsAsTypePositions(t *testing.T) {
	service := languageservice.New("go")
	for _, source := range []string{"send(name: ", "values := {name: "} {
		items := service.Complete(source, len(source))
		if _, ok := findCompletion(items, "puts"); !ok {
			t.Errorf("Complete(%q) incorrectly filtered value completions: %v", source, labels(items))
		}
	}
}

func TestCompletionShowsSourceResultSignature(t *testing.T) {
	artifact := compile(t, "go", `import { Result } from trb/std/result

record LoadError
end

def load_name(): Result<String, LoadError>
	return Result<String, LoadError>::Ok("Ada")
end
`)
	service := languageservice.New("go")
	service.Update([]*ir.Program{artifact.IR}, "repl")
	item, ok := findCompletion(service.Complete("load_", len("load_")), "load_name")
	if !ok {
		t.Fatal("load_name completion is missing")
	}
	if item.Detail != "load_name(): Result<String, LoadError>" {
		t.Fatalf("Result completion detail=%q", item.Detail)
	}
}

func TestCompletionIncludesExplicitImportedNamesAndDeclarationRoots(t *testing.T) {
	artifacts, err := compiler.CompileProject([]compiler.SourceUnit{
		{Filename: "models/user.trb", ModulePath: "models/user", Source: []byte("record User\n\tname: String\nend\n")},
		{Filename: "models/state.trb", ModulePath: "models/state", Source: []byte("enum State\n\tOpen\n\tClosed\nend\n")},
		{Filename: ".trb-repl.trb", ModulePath: "repl", Source: []byte("import { User } from models/user\nimport { State as WorkflowState } from models/state\nimport { Result } from trb/std/result\nimport { Date } from trb/std/time\nimport trb/std/strings\n")},
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
	if _, ok := findCompletion(service.Complete("Strings.up", len("Strings.up")), "uppercase"); !ok {
		t.Fatal("standard package root member was not completed")
	}
	if _, ok := findCompletion(service.Complete("Result::O", len("Result::O")), "Ok"); !ok {
		t.Fatal("imported enum member was not completed")
	}
	if _, ok := findCompletion(service.Complete("Date", len("Date")), "Date"); !ok {
		t.Fatal("imported standard type was not completed")
	}
	if _, ok := findCompletion(service.Complete("Date.pa", len("Date.pa")), "parse"); !ok {
		t.Fatal("imported standard type member was not completed")
	}
	if _, ok := findCompletion(service.Complete("WorkflowState::O", len("WorkflowState::O")), "Open"); !ok {
		t.Fatal("aliased project declaration member was not completed")
	}

	withoutImport := languageservice.New("go")
	programs = append(programs, &ir.Program{Mode: "go", ModulePath: "blank"})
	withoutImport.Update(programs, "blank")
	if _, ok := findCompletion(withoutImport.Complete("Us", 2), "User"); ok {
		t.Fatal("unimported project name was completed")
	}
}

func TestCompletionUsesIndexedNativePackageContracts(t *testing.T) {
	propsName := "Native_react_spinners_ClipLoaderProps"
	catalog := &nativepackage.Catalog{
		FormatVersion: nativepackage.FormatVersion,
		Dependencies:  map[string]string{"react-spinners": "^0.17.0"},
		Modules: map[string]nativepackage.Module{
			"react-spinners": {
				Exports: map[string]nativepackage.Export{
					"ClipLoader": {
						Kind:       "component",
						Type:       nativepackage.Type{Kind: "named", Name: "ReactNode"},
						Parameters: []nativepackage.Type{{Kind: "named", Name: propsName}},
						Required:   1,
					},
					"identity": {
						Kind: "function", Type: nativepackage.Type{Kind: "named", Name: "T"},
						TypeParameters: []string{"T"}, Parameters: []nativepackage.Type{{Kind: "named", Name: "T"}}, Required: 1,
					},
				},
				Records: map[string]nativepackage.Export{
					propsName: {Kind: "record", Type: nativepackage.Type{Kind: "named", Name: propsName}},
				},
			},
		},
	}
	artifacts, err := compiler.CompileProject([]compiler.SourceUnit{{
		Filename: ".trb-repl.trb", ModulePath: "repl", Source: []byte("import { ClipLoader, identity } from react-spinners\n"),
	}}, compiler.Options{Mode: "typescript", ModulePath: "repl", AllowUnusedImports: true, NativePackages: catalog})
	if err != nil {
		t.Fatal(err)
	}
	service := languageservice.New("typescript")
	service.Update([]*ir.Program{artifacts[0].IR}, "repl")
	item, ok := findCompletion(service.Complete("Clip", len("Clip")), "ClipLoader")
	if !ok {
		t.Fatal("native package export was not completed")
	}
	if item.Kind != languageservice.CompletionFunction || item.InsertText != "ClipLoader" {
		t.Fatalf("native package completion=%#v", item)
	}
	if item.Detail != "ClipLoader(Native_react_spinners_ClipLoaderProps): ReactNode" {
		t.Fatalf("native package detail=%q", item.Detail)
	}
	item, ok = findCompletion(service.Complete("iden", len("iden")), "identity")
	if !ok {
		t.Fatal("generic native package export was not completed")
	}
	if item.InsertText != "identity" || item.Detail != "identity<T>(T): T" {
		t.Fatalf("generic native package completion=%#v", item)
	}
}

func TestCompletionInstantiatesGenericUnionAliasMembers(t *testing.T) {
	typeParameter := types.FromName("TData")
	named := func(name string, arguments ...types.Type) types.Type {
		return types.Type{Kind: types.Named, Name: name, Args: arguments}
	}
	program := &ir.Program{Mode: "typescript", ModulePath: "app", Statements: []ir.Statement{
		&ir.Record{Name: "Page", Body: []ir.Statement{
			&ir.RecordField{Name: "total", Type: types.FromName("Integer")},
		}},
		&ir.Record{Name: "PendingResult", TypeParameters: []string{"TData"}, Body: []ir.Statement{
			&ir.RecordField{Name: "status", Type: types.FromName(`"pending"`)},
		}},
		&ir.Record{Name: "SuccessResult", TypeParameters: []string{"TData"}, Body: []ir.Statement{
			&ir.RecordField{Name: "status", Type: types.FromName(`"success"`)},
			&ir.RecordField{Name: "data", Type: typeParameter},
		}},
		&ir.TypeAlias{
			Name:           "QueryResult",
			TypeParameters: []string{"TData"},
			Target: types.UnionOf(
				named("PendingResult", typeParameter),
				named("SuccessResult", typeParameter),
			),
		},
		&ir.Method{
			Name:           "useQuery",
			TypeParameters: []string{"TData"},
			ReturnType:     named("QueryResult", typeParameter),
		},
	}}
	service := languageservice.New("typescript")
	service.Update([]*ir.Program{program}, "app")

	bare := "query := useQuery<Page>()\nquery."
	bareItems := service.Complete(bare, len(bare))
	if _, ok := findCompletion(bareItems, "status"); !ok {
		t.Fatalf("generic union completion labels=%v, want status", labels(bareItems))
	}
	if _, ok := findCompletion(bareItems, "data"); ok {
		t.Fatalf("generic union completion labels=%v, data is not common to every alternative", labels(bareItems))
	}

	narrowed := "query := useQuery<Page>()\nquery.data."
	narrowedItems := service.Complete(narrowed, len(narrowed))
	if _, ok := findCompletion(narrowedItems, "total"); !ok {
		t.Fatalf("instantiated nested member completion labels=%v, want total", labels(narrowedItems))
	}
}

func TestCompletionDoesNotExposeImplicitRuntimeImports(t *testing.T) {
	artifact := compile(t, "go", "missing := [1].try_fetch(9)\n")
	service := languageservice.New("go")
	service.Update([]*ir.Program{artifact.IR}, "repl")
	for _, name := range []string{"Result", "IndexLookupError", "SliceRangeError", "KeyLookupError", "NumberParseError", "Hex::DecodeError", "Base64::DecodeError"} {
		if _, ok := findCompletion(service.Complete(name[:3], 3), name); ok {
			t.Fatalf("implicit runtime dependency %s was exposed as a source import", name)
		}
	}
}

func TestCompletionOffersImportCandidatesWithoutChangingCheckedContext(t *testing.T) {
	service := languageservice.New("go")
	service.SetCandidates(languageservice.Context{Symbols: []languageservice.Symbol{{
		Name: "Date", Kind: languageservice.CompletionType, Detail: "trb/std/time",
		Members: []languageservice.Symbol{{Name: "parse", Kind: languageservice.CompletionMethod, Detail: "parse(value: String): Date"}},
	}}})
	if _, ok := findCompletion(service.Complete("Date", len("Date")), "Date"); !ok {
		t.Fatal("standard import candidate was not completed")
	}
	if _, ok := findCompletion(service.Complete("Date.pa", len("Date.pa")), "parse"); !ok {
		t.Fatal("standard import candidate members were not completed")
	}

	service.Update([]*ir.Program{{Mode: "go", ModulePath: "repl", Statements: []ir.Statement{
		&ir.Record{Name: "Date"},
	}}}, "repl")
	items := service.Complete("Date.pa", len("Date.pa"))
	if _, ok := findCompletion(items, "parse"); ok {
		t.Fatal("import candidate was not shadowed by the checked session declaration")
	}
}

func TestCompletionAddsCanonicalStandardImport(t *testing.T) {
	service := languageservice.New("go")
	service.SetCandidates(languageservice.StandardImportCandidates("go"))
	item, ok := findCompletion(service.Complete("# Values\nRes", len("# Values\nRes")), "Result")
	if !ok {
		t.Fatal("Result import candidate was not completed")
	}
	want := languageservice.TextEdit{
		Range:   languageservice.OffsetRange{Start: len("# Values\n"), End: len("# Values\n")},
		NewText: "import trb/std/result\n",
	}
	if len(item.AdditionalEdits) != 1 || item.AdditionalEdits[0] != want {
		t.Fatalf("additional edits=%#v, want %#v", item.AdditionalEdits, want)
	}

	source := "import { Date } from trb/std/time\nRes"
	item, ok = findCompletion(service.Complete(source, len(source)), "Result")
	if !ok || len(item.AdditionalEdits) != 1 {
		t.Fatalf("Result completion=%#v, ok=%v", item, ok)
	}
	if got := item.AdditionalEdits[0]; got.Range.Start != len("import { Date } from trb/std/time\n") || got.NewText != "import trb/std/result\n" {
		t.Fatalf("additional edit=%#v", got)
	}
}

func TestCompletionOffersStandardPackageRootsAndNamedFunctions(t *testing.T) {
	service := languageservice.New("go")
	service.SetCandidates(languageservice.StandardImportCandidates("go"))

	math, ok := findCompletion(service.Complete("Mat", len("Mat")), "Math")
	if !ok || len(math.AdditionalEdits) != 1 || math.AdditionalEdits[0].NewText != "import trb/std/math\n" {
		t.Fatalf("Math completion=%#v, ok=%v", math, ok)
	}
	sqrt, ok := findCompletion(service.Complete("Math.sq", len("Math.sq")), "sqrt")
	if !ok || len(sqrt.AdditionalEdits) != 1 || sqrt.AdditionalEdits[0].NewText != "import trb/std/math\n" {
		t.Fatalf("Math.sqrt completion=%#v, ok=%v", sqrt, ok)
	}
	describe, ok := findCompletion(service.Complete("desc", len("desc")), "describe")
	if !ok || len(describe.AdditionalEdits) != 1 || describe.AdditionalEdits[0].NewText != "import { describe } from trb/std/test\n" {
		t.Fatalf("describe completion=%#v, ok=%v", describe, ok)
	}
	date, ok := findCompletion(service.Complete("Dat", len("Dat")), "Date")
	if !ok || len(date.AdditionalEdits) != 1 || date.AdditionalEdits[0].NewText != "import { Date } from trb/std/time\n" {
		t.Fatalf("Date completion=%#v, ok=%v", date, ok)
	}
}

func TestCompletionMergesAnExistingNamedImport(t *testing.T) {
	service := languageservice.New("go")
	service.SetCandidates(languageservice.Context{Symbols: []languageservice.Symbol{{
		Name: "Result", Kind: languageservice.CompletionType,
		Import: &languageservice.Import{Path: "trb/std/result", Symbol: "Result"},
	}}})
	source := "import { Other } from trb/std/result\nRes"
	item, ok := findCompletion(service.Complete(source, len(source)), "Result")
	if !ok || len(item.AdditionalEdits) != 1 {
		t.Fatalf("Result completion=%#v, ok=%v", item, ok)
	}
	if got := item.AdditionalEdits[0]; got.NewText != ", Result" || source[got.Range.Start:got.Range.End] != "" {
		t.Fatalf("additional edit=%#v", got)
	}
}

func TestCompletionKeepsAmbiguousImportOriginsDistinct(t *testing.T) {
	service := languageservice.New("go")
	service.SetCandidates(languageservice.Context{Symbols: []languageservice.Symbol{
		{
			Name: "sha256", Kind: languageservice.CompletionFunction, Detail: "sha256(value: Bytes): Bytes — trb/std/digest",
			Import: &languageservice.Import{Path: "trb/std/digest", Symbol: "sha256"},
		},
		{
			Name: "sha256", Kind: languageservice.CompletionFunction, Detail: "sha256(key: Bytes, value: Bytes): Bytes — trb/std/hmac",
			Import: &languageservice.Import{Path: "trb/std/hmac", Symbol: "sha256"},
		},
	}})
	items := service.Complete("sha", len("sha"))
	matched := []languageservice.CompletionItem{}
	for _, item := range items {
		if item.Label == "sha256" {
			matched = append(matched, item)
		}
	}
	if len(matched) != 2 {
		t.Fatalf("sha256 candidates=%#v, want two origins", matched)
	}
	byImport := map[string]languageservice.CompletionItem{}
	for _, item := range matched {
		if item.InsertText != "sha256" || len(item.AdditionalEdits) != 1 {
			t.Fatalf("ambiguous completion=%#v", item)
		}
		byImport[item.AdditionalEdits[0].NewText] = item
	}
	if _, ok := byImport["import { sha256 } from trb/std/digest\n"]; !ok {
		t.Fatalf("digest candidate missing: %#v", matched)
	}
	if _, ok := byImport["import { sha256 } from trb/std/hmac\n"]; !ok {
		t.Fatalf("hmac candidate missing: %#v", matched)
	}
}

func TestCompletionAddsPackageImport(t *testing.T) {
	service := languageservice.New("go")
	service.SetCandidates(languageservice.Context{Symbols: []languageservice.Symbol{{
		Name: "math", Kind: languageservice.CompletionModule, Detail: "trb/std/math",
		Import: &languageservice.Import{Path: "trb/std/math"},
	}}})
	item, ok := findCompletion(service.Complete("ma", len("ma")), "math")
	if !ok || item.InsertText != "math" || len(item.AdditionalEdits) != 1 || item.AdditionalEdits[0].NewText != "import trb/std/math\n" {
		t.Fatalf("package completion=%#v, ok=%v", item, ok)
	}
}

func TestCompletionCandidateRepairsAStaleCheckedImport(t *testing.T) {
	checked := languageservice.Context{Symbols: []languageservice.Symbol{{
		Name: "Result", Kind: languageservice.CompletionType, Detail: "stale checked import",
	}}}
	candidates := languageservice.Context{Symbols: []languageservice.Symbol{{
		Name: "Result", Kind: languageservice.CompletionType,
		Import: &languageservice.Import{Path: "trb/std/result"},
	}}}
	items := languageservice.Complete(languageservice.CompletionRequest{
		Source: "Res", Cursor: 3, Mode: "go", Context: checked, Candidates: candidates, RepairImports: true,
	})
	item, ok := findCompletion(items, "Result")
	if !ok || len(item.AdditionalEdits) != 1 || item.AdditionalEdits[0].NewText != "import trb/std/result\n" {
		t.Fatalf("repaired Result completion=%#v, ok=%v", item, ok)
	}
}

func TestCompletionImportRepairKeepsCurrentLocalSymbols(t *testing.T) {
	candidates := languageservice.Context{Symbols: []languageservice.Symbol{
		{Name: "result", Kind: languageservice.CompletionFunction, Import: &languageservice.Import{Path: "support/result", Symbol: "result"}},
		{Name: "RESULT", Kind: languageservice.CompletionConstant, Import: &languageservice.Import{Path: "support/result", Symbol: "RESULT"}},
	}}
	for _, test := range []struct {
		name   string
		source string
		label  string
	}{
		{name: "local", source: "result := \"local\"\nres", label: "result"},
		{name: "parameter", source: "def run(result: String)\n\tres\nend", label: "result"},
		{name: "constant", source: "RESULT := \"local\"\nRES", label: "RESULT"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cursor := strings.LastIndex(test.source, test.label[:3]) + 3
			items := languageservice.Complete(languageservice.CompletionRequest{
				Source: test.source, Cursor: cursor, Mode: "go", Candidates: candidates, RepairImports: true,
			})
			item, ok := findCompletion(items, test.label)
			if !ok || len(item.AdditionalEdits) != 0 {
				t.Fatalf("current %s completion=%#v, ok=%v", test.name, item, ok)
			}
		})
	}
}

func TestCompletionUsesCandidateCalleeArgumentMetadata(t *testing.T) {
	candidates := languageservice.Context{Symbols: []languageservice.Symbol{
		{
			Name: "configure", Kind: languageservice.CompletionFunction,
			Call: &languageservice.CallInfo{ParameterCount: 1, Parameters: []languageservice.CallParameter{{
				Name: "mode", LiteralValues: []string{"fast", "safe"},
			}}},
		},
		{
			Name: "associate", Kind: languageservice.CompletionFunction,
			Call: &languageservice.CallInfo{ParameterCount: 1, Parameters: []languageservice.CallParameter{{
				Name: "target", ReferenceScopes: []languageservice.ReferenceScope{{
					Owner: "Owner", Range: languageservice.OffsetRange{Start: 0, End: 1000},
					Symbols: []languageservice.Symbol{{Name: "Target", Kind: languageservice.CompletionType}},
				}},
			}}},
		},
	}}

	literalSource := `configure("fa`
	literal, ok := findCompletion(languageservice.Complete(languageservice.CompletionRequest{
		Source: literalSource, Cursor: len(literalSource), Mode: "go", Candidates: candidates,
	}), "fast")
	if !ok || literal.Kind != languageservice.CompletionValue {
		t.Fatalf("candidate callee literal completion=%#v, ok=%v", literal, ok)
	}

	referenceSource := "class Owner\n\tassociate(Tar)\nend\n"
	referenceCursor := strings.Index(referenceSource, "Tar") + len("Tar")
	reference, ok := findCompletion(languageservice.Complete(languageservice.CompletionRequest{
		Source: referenceSource, Cursor: referenceCursor, Mode: "go", Candidates: candidates,
	}), "Target")
	if !ok || reference.Kind != languageservice.CompletionType {
		t.Fatalf("candidate callee reference completion=%#v, ok=%v", reference, ok)
	}
}

func TestProjectImportCandidatesOmitAmbiguousAndSameModuleNames(t *testing.T) {
	programs := []*ir.Program{
		{ModulePath: "app/main"},
		{ModulePath: "models/user", Statements: []ir.Statement{&ir.Record{Name: "User"}, &ir.Record{Name: "Unique"}}},
		{ModulePath: "admin/user", Statements: []ir.Statement{&ir.Record{Name: "User"}}},
		{ModulePath: "models/account", Statements: []ir.Statement{&ir.Record{Name: "Account"}}},
		{ModulePath: "helpers/render", Statements: []ir.Statement{&ir.Method{Name: "render"}}},
		{ModulePath: "shared/ui/DataTable/index", Statements: []ir.Statement{&ir.Method{Name: "DataTable"}}},
	}
	project := languageservice.BuildProjectImportCandidates(programs)
	candidates := project.ForModule("app/main")
	service := languageservice.New("go")
	service.SetCandidates(candidates)
	if _, ok := findCompletion(service.Complete("Us", 2), "User"); ok {
		t.Fatal("ambiguous User candidate was completed")
	}
	item, ok := findCompletion(service.Complete("Uni", 3), "Unique")
	if !ok || item.AdditionalEdits[0].NewText != "import { Unique } from models/user\n" {
		t.Fatalf("Unique completion=%#v, ok=%v", item, ok)
	}
	item, ok = findCompletion(service.Complete("DataT", 5), "DataTable")
	if !ok || item.AdditionalEdits[0].NewText != "import { DataTable } from shared/ui/DataTable\n" {
		t.Fatalf("index-module completion=%#v, ok=%v", item, ok)
	}
	item, ok = findCompletion(service.Complete("Acc", 3), "Account")
	if !ok || item.AdditionalEdits[0].NewText != "import models/account\n" {
		t.Fatalf("declaration-root completion=%#v, ok=%v", item, ok)
	}
	item, ok = findCompletion(service.Complete("rend", 4), "render")
	if !ok || item.AdditionalEdits[0].NewText != "import { render } from helpers/render\n" {
		t.Fatalf("root-matching function completion=%#v, ok=%v", item, ok)
	}

	service.SetCandidates(project.ForModule("models/user"))
	if _, ok := findCompletion(service.Complete("Uni", 3), "Unique"); ok {
		t.Fatal("same-module Unique candidate was completed")
	}
	item, ok = findCompletion(service.Complete("Us", 2), "User")
	if !ok || len(item.AdditionalEdits) != 1 || item.AdditionalEdits[0].NewText != "import admin/user\n" {
		t.Fatalf("external User completion after excluding current module=%#v, ok=%v", item, ok)
	}

	service.SetCandidates(project.ForModule("shared/ui/DataTable/index"))
	if _, ok := findCompletion(service.Complete("DataT", 5), "DataTable"); ok {
		t.Fatal("shortened index-module import candidate included its own declaration")
	}
}

func TestProjectImportCandidatesKeepIndexWhenShortPathNamesAnotherModule(t *testing.T) {
	programs := []*ir.Program{
		{ModulePath: "app/main"},
		{ModulePath: "shared/ui/DataTable", Statements: []ir.Statement{&ir.Method{Name: "DirectDataTable"}}},
		{ModulePath: "shared/ui/DataTable/index", Statements: []ir.Statement{&ir.Method{Name: "IndexedDataTable"}}},
	}
	service := languageservice.New("typescript")
	service.SetCandidates(languageservice.BuildProjectImportCandidates(programs).ForModule("app/main"))

	direct, ok := findCompletion(service.Complete("Direct", len("Direct")), "DirectDataTable")
	if !ok || direct.AdditionalEdits[0].NewText != "import { DirectDataTable } from shared/ui/DataTable\n" {
		t.Fatalf("direct-module completion=%#v, ok=%v", direct, ok)
	}
	indexed, ok := findCompletion(service.Complete("Indexed", len("Indexed")), "IndexedDataTable")
	if !ok || indexed.AdditionalEdits[0].NewText != "import { IndexedDataTable } from shared/ui/DataTable/index\n" {
		t.Fatalf("ambiguous index-module completion=%#v, ok=%v", indexed, ok)
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
