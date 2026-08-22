package compiler

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/codegen"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/schemalock"
)

func TestAnalyzerReusesOnlyCompilerIdenticalParsedUnits(t *testing.T) {
	analyzer := NewAnalyzer()
	parseCalls := 0
	analyzer.parse = func(source []byte) (*ast.Program, []diagnostic.Diagnostic) {
		parseCalls++
		return parser.Parse(source)
	}
	sources := []SourceUnit{
		{Filename: "/project/src/alpha.trb", ModulePath: "alpha", Package: "main", Source: []byte("def alpha(): Integer\n\treturn 1\nend\n")},
		{Filename: "/project/src/beta.trb", ModulePath: "beta", Package: "main", Source: []byte("def beta(): Integer\n\treturn 2\nend\n")},
	}
	options := Options{Mode: "go", GoModule: "example.com/analyzer", SourceRoot: "/project/src", ProjectRoot: "/project"}

	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if parseCalls != 2 {
		t.Fatalf("initial parse calls=%d, want 2", parseCalls)
	}
	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if parseCalls != 2 {
		t.Fatalf("unchanged parse calls=%d, want 2", parseCalls)
	}

	sources[1].Source = []byte("def beta(): Integer\n\treturn 3\nend\n")
	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if parseCalls != 3 {
		t.Fatalf("one changed unit parse calls=%d, want 3", parseCalls)
	}

	options.Mode = "ruby"
	options.GoModule = ""
	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if parseCalls != 5 {
		t.Fatalf("changed compiler options parse calls=%d, want 5", parseCalls)
	}
}

func TestAnalyzerReusesNormalizedSyntaxDiagnostics(t *testing.T) {
	analyzer := NewAnalyzer()
	parseCalls := 0
	analyzer.parse = func(source []byte) (*ast.Program, []diagnostic.Diagnostic) {
		parseCalls++
		return parser.Parse(source)
	}
	sources := []SourceUnit{{
		Filename: "/project/src/broken.trb", ModulePath: "broken", Package: "main", Source: []byte("def broken(\n"),
	}}
	options := Options{Mode: "go", GoModule: "example.com/analyzer", SourceRoot: "/project/src", ProjectRoot: "/project"}

	for iteration := range 2 {
		diagnostics := compileErrorDiagnostics(t, func() error {
			_, err := analyzer.AnalyzeProject(sources, options)
			return err
		})
		if len(diagnostics) == 0 || diagnostics[0].Path != sources[0].Filename || diagnostics[0].Code != diagnostic.SyntaxError {
			t.Fatalf("unexpected cached diagnostics: %#v", diagnostics)
		}
		if iteration == 0 {
			diagnostics[0].Path = "/mutated/by/caller.trb"
		}
	}
	if parseCalls != 1 {
		t.Fatalf("syntax-error parse calls=%d, want 1", parseCalls)
	}
}

func TestAnalyzerRechecksOnlyChangedInteractiveModule(t *testing.T) {
	analyzer := NewAnalyzer()
	checkCalls := 0
	analyzer.check = func(program *ast.Program, resolution resolver.Result, options checker.Options) (checker.Result, []diagnostic.Diagnostic) {
		checkCalls++
		return checker.CheckWithOptions(program, resolution, options)
	}
	sources := []SourceUnit{
		{Filename: "/project/src/alpha.trb", ModulePath: "alpha", Package: "main", Source: []byte("def alpha(): Integer\n\treturn 1\nend\n")},
		{Filename: "/project/src/beta.trb", ModulePath: "beta", Package: "main", Source: []byte("def beta(): Integer\n\treturn 2\nend\n")},
		{Filename: "/project/src/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main", Source: []byte("")},
	}
	options := Options{
		Mode: "go", GoModule: "example.com/analyzer", SourceRoot: "/project/src", ProjectRoot: "/project",
		AllowUnusedImports: true, InteractiveModule: "__trb_repl__",
	}

	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if checkCalls != 3 {
		t.Fatalf("initial check calls=%d, want 3", checkCalls)
	}

	sources[2].Source = []byte("1 + 2\n")
	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if checkCalls != 4 {
		t.Fatalf("interactive edit check calls=%d, want 4", checkCalls)
	}

	sources[2].Source = []byte("def broken(): Integer\n\treturn \"wrong\"\nend\n")
	if _, err := analyzer.AnalyzeProject(sources, options); err == nil {
		t.Fatal("expected invalid interactive edit to fail")
	}
	if checkCalls != 5 {
		t.Fatalf("invalid interactive edit check calls=%d, want 5", checkCalls)
	}

	sources[2].Source = []byte("3 + 4\n")
	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if checkCalls != 6 {
		t.Fatalf("corrected interactive edit check calls=%d, want 6", checkCalls)
	}

	sources[0].Source = []byte("def alpha(): Integer\n\treturn 9\nend\n")
	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if checkCalls != 7 {
		t.Fatalf("independent project edit check calls=%d, want 7", checkCalls)
	}
}

func TestAnalyzerReusesORMDeclarationsUntilProviderInputsChange(t *testing.T) {
	for _, mode := range []string{"ruby", "go", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			lockPath := filepath.Join(root, "schema.lock.json")
			lock := schemalock.New("sqlite")
			lock.Tables["products"] = schemalock.Table{Columns: map[string]schemalock.Column{
				"id": {Type: "Integer", PrimaryKey: true},
			}}
			if err := lock.Write(lockPath); err != nil {
				t.Fatal(err)
			}
			sources := []SourceUnit{
				{
					Filename: filepath.Join(root, "src", "models", "product.trb"), ModulePath: "models/product", Package: "models",
					Source: []byte("import { Model } from trb/orm\n\nclass Product < Model\nend\n"),
				},
				{Filename: filepath.Join(root, "src", ".trb-repl.trb"), ModulePath: "__trb_repl__", Package: "main"},
			}
			options := Options{
				Mode: mode, GoModule: "example.com/provider-reuse", RubyLoader: "require_relative", TypeScriptRuntime: "bun",
				SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
				AllowUnusedImports: true, InteractiveModule: "__trb_repl__",
				PackageOptions: map[string][]byte{
					"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3","schemaLock":"schema.lock.json"}`),
				},
			}
			analyzer := NewAnalyzer()
			if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
				t.Fatal(err)
			}
			initialDeclarations := analyzer.state.declarations

			sources[1].Source = []byte("1 + 2\n")
			if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
				t.Fatal(err)
			}
			if analyzer.state.declarations != initialDeclarations {
				t.Fatal("interactive expression edit reloaded unchanged ORM declarations")
			}

			lock.Tables["products"] = schemalock.Table{Columns: map[string]schemalock.Column{
				"id":   {Type: "Integer", PrimaryKey: true},
				"name": {Type: "String"},
			}}
			if err := lock.Write(lockPath); err != nil {
				t.Fatal(err)
			}
			sources[1].Source = []byte("import { Product } from models/product\n\ndef product_name(product: Product): String\n\treturn product.name\nend\n")
			if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
				t.Fatal(err)
			}
			if analyzer.state.declarations == initialDeclarations {
				t.Fatal("ORM schema lock edit reused stale declarations")
			}
		})
	}
}

func TestAnalyzerRechecksChangedProjectDependencies(t *testing.T) {
	analyzer := NewAnalyzer()
	checkCalls := 0
	analyzer.check = func(program *ast.Program, resolution resolver.Result, options checker.Options) (checker.Result, []diagnostic.Diagnostic) {
		checkCalls++
		return checker.CheckWithOptions(program, resolution, options)
	}
	sources := []SourceUnit{
		{Filename: "/project/src/alpha.trb", ModulePath: "alpha", Package: "main", Source: []byte("def alpha(): Integer\n\treturn 1\nend\n")},
		{Filename: "/project/src/middle.trb", ModulePath: "middle", Package: "main", Source: []byte("import { alpha } from alpha\ndef middle(): Integer\n\treturn alpha()\nend\n")},
		{Filename: "/project/src/top.trb", ModulePath: "top", Package: "main", Source: []byte("import { middle } from middle\ndef top(): Integer\n\treturn middle()\nend\n")},
		{Filename: "/project/src/unrelated.trb", ModulePath: "unrelated", Package: "main", Source: []byte("def unrelated(): Integer\n\treturn 4\nend\n")},
	}
	options := Options{Mode: "go", GoModule: "example.com/analyzer", SourceRoot: "/project/src", ProjectRoot: "/project"}

	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if checkCalls != 4 {
		t.Fatalf("initial check calls=%d, want 4", checkCalls)
	}

	sources[0].Source = []byte("def alpha(): Integer\n\treturn 2\nend\n")
	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if checkCalls != 5 {
		t.Fatalf("body-only edit check calls=%d, want 5", checkCalls)
	}

	sources[0].Source = []byte("def alpha(increment: Integer = 0): Integer\n\treturn 2 + increment\nend\n")
	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if checkCalls != 8 {
		t.Fatalf("signature edit check calls=%d, want 8", checkCalls)
	}

	sources[0].Source = []byte("def alpha(): String\n\treturn \"wrong\"\nend\n")
	if _, err := analyzer.AnalyzeProject(sources, options); err == nil {
		t.Fatal("expected incompatible dependency edit to fail")
	}
	if checkCalls != 11 {
		t.Fatalf("invalid signature edit check calls=%d, want 11", checkCalls)
	}

	sources[0].Source = []byte("def alpha(increment: Integer = 0): Integer\n\treturn 3 + increment\nend\n")
	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if checkCalls != 12 {
		t.Fatalf("corrected body edit check calls=%d, want 12", checkCalls)
	}
}

func TestAnalyzerFallsBackWhenInteractiveCompilerDependenciesChange(t *testing.T) {
	analyzer := NewAnalyzer()
	checkCalls := 0
	analyzer.check = func(program *ast.Program, resolution resolver.Result, options checker.Options) (checker.Result, []diagnostic.Diagnostic) {
		checkCalls++
		return checker.CheckWithOptions(program, resolution, options)
	}
	sources := []SourceUnit{
		{Filename: "/project/src/alpha.trb", ModulePath: "alpha", Package: "main", Source: []byte("def alpha(): Integer\n\treturn 1\nend\n")},
		{Filename: "/project/src/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main", Source: []byte("")},
	}
	options := Options{
		Mode: "go", GoModule: "example.com/analyzer", SourceRoot: "/project/src", ProjectRoot: "/project",
		AllowUnusedImports: true, InteractiveModule: "__trb_repl__",
	}

	initial, err := analyzer.AnalyzeProject(sources, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(initial) != 2 || checkCalls != 2 {
		t.Fatalf("initial artifacts=%d check calls=%d, want 2 and 2", len(initial), checkCalls)
	}

	sources[1].Source = []byte("import { Date } from trb/std/time\nDate.parse(\"2026-08-11\")\n")
	withTime, err := analyzer.AnalyzeProject(sources, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(withTime) <= len(initial) || checkCalls <= 3 {
		t.Fatalf("dependency edit artifacts=%d check calls=%d, want a full analysis", len(withTime), checkCalls)
	}

	beforeRemoval := checkCalls
	sources[1].Source = []byte("1 + 2\n")
	afterRemoval, err := analyzer.AnalyzeProject(sources, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterRemoval) != len(initial) || checkCalls-beforeRemoval <= 1 {
		t.Fatalf("dependency removal artifacts=%d check call delta=%d, want 2 and a full analysis", len(afterRemoval), checkCalls-beforeRemoval)
	}
}

func TestAnalyzerInteractiveArtifactsMatchFullCompilation(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			sources := []SourceUnit{
				{Filename: "/project/src/alpha.trb", ModulePath: "alpha", Package: "main", Source: []byte("def alpha(value: Integer): Integer\n\treturn value + 1\nend\n")},
				{Filename: "/project/src/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main", Source: []byte("")},
			}
			options := Options{
				Mode: mode, GoModule: "example.com/analyzer", RubyLoader: "require_relative", TypeScriptRuntime: "node",
				SourceRoot: "/project/src", ProjectRoot: "/project", AllowUnusedImports: true, InteractiveModule: "__trb_repl__",
			}
			analyzer := NewAnalyzer()
			if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
				t.Fatal(err)
			}
			sources[1].Source = []byte("import { alpha } from alpha\nalpha(2)\n")
			incremental, err := analyzer.AnalyzeProject(sources, options)
			if err != nil {
				t.Fatal(err)
			}
			requireAnalysisMatchesFullCompilation(t, incremental, sources, options)
		})
	}
}

func TestAnalyzerProjectArtifactsMatchFullCompilation(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			sources := []SourceUnit{
				{Filename: "/project/src/alpha.trb", ModulePath: "alpha", Package: "main", Source: []byte("def alpha(): Integer\n\treturn 1\nend\n")},
				{Filename: "/project/src/consumer.trb", ModulePath: "consumer", Package: "main", Source: []byte("import { alpha } from alpha\ndef consumer(): Integer\n\treturn alpha()\nend\n")},
			}
			options := Options{
				Mode: mode, GoModule: "example.com/analyzer", RubyLoader: "require_relative", TypeScriptRuntime: "node",
				SourceRoot: "/project/src", ProjectRoot: "/project",
			}
			analyzer := NewAnalyzer()
			if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
				t.Fatal(err)
			}
			sources[0].Source = []byte("def alpha(): Integer\n\treturn 2\nend\n")
			incremental, err := analyzer.AnalyzeProject(sources, options)
			if err != nil {
				t.Fatal(err)
			}
			requireAnalysisMatchesFullCompilation(t, incremental, sources, options)
		})
	}
}

func requireAnalysisMatchesFullCompilation(t *testing.T, incremental []*Artifact, sources []SourceUnit, options Options) {
	t.Helper()
	full, err := CompileProject(sources, options)
	if err != nil {
		t.Fatal(err)
	}
	incrementalPrograms := make([]*ir.Program, len(incremental))
	for index, artifact := range incremental {
		incrementalPrograms[index] = artifact.IR
	}
	outputs, err := codegen.GenerateProject(incrementalPrograms)
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) != len(full) {
		t.Fatalf("incremental outputs=%d, full artifacts=%d", len(outputs), len(full))
	}
	for index := range outputs {
		if got, want := string(outputs[index].Output), string(full[index].Output); got != want {
			t.Fatalf("artifact %d output differs after incremental analysis:\ngot:\n%s\nwant:\n%s", index, got, want)
		}
	}
}

func BenchmarkProjectAnalysisAfterOneFileEdit(b *testing.B) {
	const modules = 64
	base := make([]SourceUnit, 0, modules)
	for index := 0; index < modules; index++ {
		base = append(base, SourceUnit{
			Filename:   fmt.Sprintf("/project/src/module_%03d.trb", index),
			ModulePath: fmt.Sprintf("module_%03d", index),
			Package:    "main",
			Source:     []byte(fmt.Sprintf("def value_%03d(): Integer\n\treturn %d\nend\n", index, index)),
		})
	}
	options := Options{Mode: "go", GoModule: "example.com/analyzer", SourceRoot: "/project/src", ProjectRoot: "/project"}

	b.Run("one-shot", func(b *testing.B) {
		sources := append([]SourceUnit(nil), base...)
		iteration := 0
		for b.Loop() {
			iteration++
			sources[modules-1].Source = []byte(fmt.Sprintf("def value_063(): Integer\n\treturn %d\nend\n", iteration))
			if _, err := AnalyzeProject(sources, options); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("reused-dependencies", func(b *testing.B) {
		sources := append([]SourceUnit(nil), base...)
		analyzer := NewAnalyzer()
		if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
			b.Fatal(err)
		}
		iteration := 0
		b.ResetTimer()
		for b.Loop() {
			iteration++
			sources[modules-1].Source = []byte(fmt.Sprintf("def value_063(): Integer\n\treturn %d\nend\n", iteration))
			if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkInteractiveProjectAnalysis(b *testing.B) {
	const modules = 64
	base := make([]SourceUnit, 0, modules+1)
	for index := 0; index < modules; index++ {
		base = append(base, SourceUnit{
			Filename:   fmt.Sprintf("/project/src/module_%03d.trb", index),
			ModulePath: fmt.Sprintf("module_%03d", index),
			Package:    "main",
			Source:     []byte(fmt.Sprintf("def value_%03d(): Integer\n\treturn %d\nend\n", index, index)),
		})
	}
	base = append(base, SourceUnit{
		Filename: "/project/src/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main", Source: []byte("0 + 1\n"),
	})
	options := Options{
		Mode: "go", GoModule: "example.com/analyzer", SourceRoot: "/project/src", ProjectRoot: "/project",
		AllowUnusedImports: true, InteractiveModule: "__trb_repl__",
	}

	b.Run("one-shot", func(b *testing.B) {
		sources := append([]SourceUnit(nil), base...)
		iteration := 0
		for b.Loop() {
			iteration++
			sources[modules].Source = []byte(fmt.Sprintf("%d + 1\n", iteration))
			if _, err := AnalyzeProject(sources, options); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("reused-project-semantics", func(b *testing.B) {
		sources := append([]SourceUnit(nil), base...)
		analyzer := NewAnalyzer()
		if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
			b.Fatal(err)
		}
		iteration := 0
		b.ResetTimer()
		for b.Loop() {
			iteration++
			sources[modules].Source = []byte(fmt.Sprintf("%d + 1\n", iteration))
			if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
				b.Fatal(err)
			}
		}
	})
}
