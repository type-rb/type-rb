package compiler

import (
	"strings"
	"testing"
)

func TestCyclicModuleDiagnosticsAcrossModes(t *testing.T) {
	for _, test := range []struct{ name, a, b, want string }{
		{"values", "import { B } from b\nA: Integer := B\n", "import { A } from a\nB: Integer := A\n", "value initialization cycle"},
		{"transitive read", "import { B } from b\ndef read(): Integer\nreturn B\nend\nA: Integer := read()\n", "import { A } from a\nB: Integer := A\n", "value initialization cycle"},
		{"indirect", "import { B } from b\ndef invoke(callback: () -> Integer): Integer\nreturn callback()\nend\nA := invoke(fn(): Integer\nreturn B\nend)\n", "import { A } from a\nB := 1\ndef later(): Integer\nreturn A\nend\n", "unverified operation"},
		{"inference", "import { B } from b\nA := B\n", "import { A } from a\nB := A\n", "type inference cycle"},
		{"declaration order", "import { B } from b\nA: Integer := B\nLAST := 1\n", "import { LAST } from a\nB := LAST\n", "value initialization cycle"},
		{"aliased inheritance", "import { Node as Parent } from b\nclass Node < Parent\nend\n", "import { Node as Parent } from a\nclass Node < Parent\nend\n", "inheritance cycle"},
		{"aliases", "import { B } from b\nalias A = B\n", "import { A } from a\nalias B = A\n", "type alias cycle"},
		{"record layout", "import { B } from b\nrecord A\nnext: B\nend\n", "import { A } from a\nrecord B\nnext: A\nend\n", "infinite value layout"},
		{"inheritance", "import { B } from b\nclass A < B\nend\n", "import { A } from a\nclass B < A\nend\n", "inheritance cycle"},
	} {
		for _, mode := range []string{"go", "ruby", "typescript", "trb"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				sources := []SourceUnit{{Filename: "/project/a.trb", ModulePath: "a", Source: []byte(test.a)}, {Filename: "/project/b.trb", ModulePath: "b", Source: []byte(test.b)}}
				_, err := AnalyzeProject(sources, Options{Mode: mode, GoModule: "example.com/cycles"})
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("want %q; got %v", test.want, err)
				}
			})
		}
	}
}

func TestImportCyclesRemainForbiddenBetweenCompilationUnits(t *testing.T) {
	sources := []SourceUnit{
		{Filename: "/project/a.trb", ModulePath: "a", CompilationUnit: "first", Source: []byte("import { b } from b\ndef a(): Integer\nreturn b()\nend\n")},
		{Filename: "/project/b.trb", ModulePath: "b", CompilationUnit: "second", Source: []byte("import { a } from a\ndef b(): Integer\nreturn a()\nend\n")},
	}
	_, err := AnalyzeProject(sources, Options{Mode: "go"})
	if err == nil || !strings.Contains(err.Error(), "compilation unit dependency cycle") {
		t.Fatal(err)
	}
}

func TestIncrementalCycleEditInvalidatesInitializerDependencies(t *testing.T) {
	sources := []SourceUnit{
		{Filename: "/project/a.trb", ModulePath: "a", Source: []byte("import { B } from b\nA: Integer := B + 1\n")},
		{Filename: "/project/b.trb", ModulePath: "b", Source: []byte("import { A } from a\nB := 1\ndef later(): Integer\nreturn A\nend\n")},
	}
	analyzer := NewAnalyzer()
	options := Options{Mode: "go", GoModule: "example.com/cycles"}
	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	sources[1].Source = []byte("import { A } from a\nB: Integer := A\n")
	_, incremental := analyzer.AnalyzeProject(sources, options)
	_, fresh := AnalyzeProject(sources, options)
	if incremental == nil || fresh == nil || incremental.Error() != fresh.Error() || !strings.Contains(incremental.Error(), "value initialization cycle") {
		t.Fatalf("incremental=%v fresh=%v", incremental, fresh)
	}
}
