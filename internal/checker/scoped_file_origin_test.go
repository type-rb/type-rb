package checker_test

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

func TestExternalDeclarationCannotReturnScopedFileAsDirectReceiver(t *testing.T) {
	program := parseDeclarationOnlyProgram(t, `import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def invalid(): Result<String, FileSystemError>
	return external_file().read_text(max_bytes: 1)
end
`, "go", "app/main")
	resolved, resolveDiagnostics := resolver.Resolve(program, resolver.Options{Mode: "go", Filename: "main.trb"})
	if len(resolveDiagnostics) != 0 {
		t.Fatalf("resolve diagnostics: %#v", resolveDiagnostics)
	}
	provider := &resolver.Import{Kind: resolver.ProjectImport, Path: "example/provider", ModulePath: "example/provider/index"}
	exported := &resolver.Export{Name: "external_file", Kind: resolver.FunctionExport, Type: stdlib.FileResourceType()}
	resolved.Symbols["external_file"] = resolver.Binding{Import: provider, Name: "external_file", Export: exported}

	_, diagnostics := checker.Check(program, resolved)
	if !diagnosticsContain(diagnostics, "a scoped resource value requires a checked acquisition or borrow origin") {
		t.Fatalf("check diagnostics = %#v, want external File origin diagnostic", diagnostics)
	}
}

func TestExternalDeclarationCannotReturnCollectionContainingScopedFile(t *testing.T) {
	program := parseDeclarationOnlyProgram(t, `import trb/std/file

def invalid()
	_files := external_files()
	return
end
`, "go", "app/main")
	resolved, resolveDiagnostics := resolver.Resolve(program, resolver.Options{Mode: "go", Filename: "main.trb"})
	if len(resolveDiagnostics) != 0 {
		t.Fatalf("resolve diagnostics: %#v", resolveDiagnostics)
	}
	provider := &resolver.Import{Kind: resolver.ProjectImport, Path: "example/provider", ModulePath: "example/provider/index"}
	fileArray := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{stdlib.FileResourceType()}}
	exported := &resolver.Export{Name: "external_files", Kind: resolver.FunctionExport, Type: fileArray}
	resolved.Symbols["external_files"] = resolver.Binding{Import: provider, Name: "external_files", Export: exported}

	_, diagnostics := checker.Check(program, resolved)
	if !diagnosticsContain(diagnostics, "a scoped resource value requires a checked acquisition or borrow origin") {
		t.Fatalf("check diagnostics = %#v, want nested File origin diagnostic", diagnostics)
	}
}

func TestExternalBlockDeclarationCannotMintScopedFile(t *testing.T) {
	program := parseDeclarationOnlyProgram(t, `import trb/std/file

def invalid()
	borrow() do |_file|
	end
	return
end
`, "go", "app/main")
	resolved, resolveDiagnostics := resolver.Resolve(program, resolver.Options{Mode: "go", Filename: "main.trb"})
	if len(resolveDiagnostics) != 0 {
		t.Fatalf("resolve diagnostics: %#v", resolveDiagnostics)
	}
	provider := &resolver.Import{Kind: resolver.ProjectImport, Path: "example/provider", ModulePath: "example/provider/index"}
	declared := &stdlib.Symbol{
		Name: "borrow", Intrinsic: "example.provider.borrow", Return: types.FromName("Void"),
		Block: &stdlib.Block{Parameters: []types.Type{stdlib.FileResourceType()}, ScopedParameters: []bool{true}},
	}
	resolved.Symbols["borrow"] = resolver.Binding{Import: provider, Name: "borrow", Library: declared}

	_, diagnostics := checker.Check(program, resolved)
	if !diagnosticsContain(diagnostics, "only trusted standard resource acquisition contracts may introduce scoped resource block parameters") {
		t.Fatalf("check diagnostics = %#v, want untrusted block origin diagnostic", diagnostics)
	}
}

func diagnosticsContain(diagnostics []diagnostic.Diagnostic, wanted string) bool {
	for _, item := range diagnostics {
		if strings.Contains(item.Message, wanted) {
			return true
		}
	}
	return false
}
