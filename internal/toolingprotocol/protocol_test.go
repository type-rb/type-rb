package toolingprotocol

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/compilerservice"
	"github.com/type-rb/type-rb/internal/token"
)

func TestBuildExposesAuthoredSourcesModulesAndTypedDeclarations(t *testing.T) {
	root := t.TempDir()
	modelsSource := "record User\n\tname: String\nend\n\nenum Status\n\tReady = \"ready\"\n\tFailed = \"failed\"\nend\n\ninterface Loader<T>\n\tload(id: Integer): T\nend\n\nclass Service\n\t@_name: String\n\n\tdef initialize(name: String)\n\t\t@_name = name\n\t\treturn\n\tend\n\n\tdef name(): String\n\t\treturn @_name\n\tend\n\n\tdef self.kind(): String\n\t\treturn \"service\"\n\tend\nend\n\ntype UserID = Integer\n\nDEFAULT_NAME := \"guest\"\n\ndef make_user(name: String = DEFAULT_NAME): User\n\treturn User.new(name: name)\nend\n"
	units := []compiler.SourceUnit{
		{
			Filename: filepath.Join(root, "main.trb"), ModulePath: "main", Package: "main",
			Source: []byte("import { User, make_user } from ./models\n\ndef main()\n\tuser: User := make_user()\n\tputs(user.name)\n\treturn\nend\n"),
		},
		{
			Filename: filepath.Join(root, "models.trb"), ModulePath: "models", Package: "main", Source: []byte(modelsSource),
			CompilerGeneratedSources: []compiler.CompilerGeneratedSource{{
				ID: "generated-helper", Source: []byte("def __trb_generated(): Integer\n\treturn 1\nend\n"),
				Origin: token.Span{Start: token.Position{Line: 1, Column: 1}, End: token.Position{Line: 1, Column: 1}},
			}},
		},
	}
	snapshot := compilerservice.New(units, compiler.Options{Mode: "go", GoModule: "example.com/tooling"}).Analyze()
	if snapshot.HasErrors() {
		t.Fatalf("unexpected diagnostics: %#v", snapshot.Diagnostics)
	}

	report := Build(BuildOptions{CompilerVersion: "test-version", Mode: "go"}, units, snapshot)
	if report.ProtocolVersion != ProtocolVersion || report.CompilerVersion != "test-version" || report.Mode != "go" {
		t.Fatalf("unexpected report metadata: %#v", report)
	}
	if len(report.Sources) != 2 || len(report.Modules) != 2 || report.Sources[0].Path != filepath.Join(root, "main.trb") || report.Modules[0].ModulePath != "main" {
		t.Fatalf("sources and modules are not deterministic: %#v %#v", report.Sources, report.Modules)
	}
	if report.Sources[1].Encoding != "utf-8" || strings.Contains(report.Sources[1].Content, "__trb_generated") {
		t.Fatalf("source snapshot exposed compiler-generated source: %#v", report.Sources[1])
	}
	if len(report.Modules[0].Imports) != 1 || report.Modules[0].Imports[0].Path != "./models" || report.Modules[0].Imports[0].ModulePath != "models" {
		t.Fatalf("unexpected module import: %#v", report.Modules[0].Imports)
	}
	if _, ok := findDeclaration(report, DeclarationFunction, "__trb_generated"); ok {
		t.Fatal("declaration snapshot exposed a compiler-generated helper")
	}

	makeUser := requireDeclaration(t, report, DeclarationFunction, "make_user")
	if makeUser.ReturnType == nil || makeUser.ReturnType.Kind != "named" || makeUser.ReturnType.Name != "User" || len(makeUser.Parameters) != 1 || !makeUser.Parameters[0].Optional {
		t.Fatalf("unexpected function declaration: %#v", makeUser)
	}
	service := requireDeclaration(t, report, DeclarationClass, "Service")
	privateField := requireDeclaration(t, report, DeclarationField, "@_name")
	if privateField.OwnerID != service.ID || privateField.Visibility != "private" || privateField.Type == nil || privateField.Type.Name != "String" {
		t.Fatalf("unexpected private field declaration: %#v", privateField)
	}
	classMethod := requireDeclaration(t, report, DeclarationMethod, "kind")
	if !classMethod.ClassMember || classMethod.OwnerID != service.ID || !strings.Contains(classMethod.ID, ":class:") {
		t.Fatalf("unexpected class method declaration: %#v", classMethod)
	}
	status := requireDeclaration(t, report, DeclarationEnum, "Status")
	if status.RawType == nil || status.RawType.Name != "String" {
		t.Fatalf("unexpected raw enum declaration: %#v", status)
	}
	if report.Summary.Errors != 0 || len(report.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostic report: %#v", report)
	}
}

func TestBuildRetainsInputsAndDiagnosticsWithoutCheckedArtifacts(t *testing.T) {
	root := t.TempDir()
	unit := compiler.SourceUnit{
		Filename: filepath.Join(root, "broken.trb"), ModulePath: "broken", Package: "main",
		Source: []byte("def answer(): Integer\n\treturn \"wrong\"\nend\n"),
	}
	snapshot := compilerservice.New([]compiler.SourceUnit{unit}, compiler.Options{Mode: "typescript"}).Analyze()
	if !snapshot.HasErrors() {
		t.Fatal("invalid source unexpectedly produced a successful snapshot")
	}
	report := Build(BuildOptions{CompilerVersion: "test-version", Mode: "typescript"}, []compiler.SourceUnit{unit}, snapshot)
	if len(report.Sources) != 1 || report.Sources[0].Encoding != "utf-8" || report.Sources[0].Content != string(unit.Source) || len(report.Modules) != 1 || report.Modules[0].ModulePath != "broken" {
		t.Fatalf("invalid snapshot lost its inputs: %#v", report)
	}
	if len(report.Declarations) != 0 || report.Summary.Errors != 1 || len(report.Diagnostics) != 1 || report.Diagnostics[0].Location == nil {
		t.Fatalf("unexpected invalid report: %#v", report)
	}
}

func TestBuildPreservesInvalidUTF8SourceAsBase64(t *testing.T) {
	root := t.TempDir()
	unit := compiler.SourceUnit{Filename: filepath.Join(root, "invalid.trb"), ModulePath: "invalid", Source: []byte{0xff}}
	snapshot := compilerservice.New([]compiler.SourceUnit{unit}, compiler.Options{Mode: "go", GoModule: "example.com/tooling"}).Analyze()
	if !snapshot.HasErrors() {
		t.Fatal("invalid UTF-8 source unexpectedly produced a successful snapshot")
	}
	report := Build(BuildOptions{CompilerVersion: "test-version", Mode: "go"}, []compiler.SourceUnit{unit}, snapshot)
	if len(report.Sources) != 1 || report.Sources[0].Encoding != "base64" || report.Sources[0].Content != "/w==" {
		t.Fatalf("invalid UTF-8 bytes were not preserved: %#v", report.Sources)
	}
}

func requireDeclaration(t *testing.T, report Report, kind DeclarationKind, name string) Declaration {
	t.Helper()
	declaration, ok := findDeclaration(report, kind, name)
	if !ok {
		t.Fatalf("missing %s declaration %s: %#v", kind, name, report.Declarations)
	}
	return declaration
}

func findDeclaration(report Report, kind DeclarationKind, name string) (Declaration, bool) {
	for _, declaration := range report.Declarations {
		if declaration.Kind == kind && declaration.Name == name {
			return declaration, true
		}
	}
	return Declaration{}, false
}
