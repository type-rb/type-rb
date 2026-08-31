package checker_test

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/codegen"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/lower"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/resolver"
)

func TestClassBodyDeclarationRuleIsCheckedAndOmittedByEveryBackend(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			provider := parseDeclarationOnlyProgram(t, `class ContractBase
end

def declare<T>(_target: Any)
	return
end
`, mode, "test/provider")
			catalog, catalogDiagnostics := resolver.NewCatalog([]resolver.Module{{
				Path: "test/provider", Filename: "provider.trb", Program: provider,
			}})
			if len(catalogDiagnostics) != 0 {
				t.Fatalf("catalog diagnostics: %#v", catalogDiagnostics)
			}
			program := parseDeclarationOnlyProgram(t, `import { ContractBase, declare } from test/provider

class Input
end

class Endpoint < ContractBase
	declare<Input>(Input)
end
`, mode, "app/endpoint")
			declarations := declaration.NewCatalog()
			declarations.ClassBodyDeclarationRules = []declaration.ClassBodyDeclarationRule{{
				Package: "test/provider", Function: "declare",
				Owner: declaration.DeclarationReference{ModulePath: "app/endpoint", Name: "Endpoint"},
			}}
			resolved, resolveDiagnostics := resolver.Resolve(program, resolver.Options{
				Mode: mode, Filename: "endpoint.trb", Catalog: catalog, Declarations: declarations,
			})
			if len(resolveDiagnostics) != 0 {
				t.Fatalf("resolve diagnostics: %#v", resolveDiagnostics)
			}
			checked, checkDiagnostics := checker.Check(program, resolved)
			if len(checkDiagnostics) != 0 {
				t.Fatalf("check diagnostics: %#v", checkDiagnostics)
			}
			endpoint := program.Statements[2].(*ast.ClassStatement)
			call := endpoint.Body[0].(*ast.ExpressionStatement).Expression.(*ast.CallExpression)
			if !checked.DeclarationOnlyCalls[call] {
				t.Fatal("direct generic class-body call was not marked declaration-only")
			}
			lowered := lower.Program(checked)
			loweredEndpoint := lowered.Statements[2].(*ir.Class)
			loweredCall := loweredEndpoint.Body[0].(*ir.ExpressionStatement).Expression.(*ir.Call)
			if !loweredCall.DeclarationOnly {
				t.Fatal("declaration-only call did not cross typed IR")
			}
			generated, err := codegen.Generate(lowered)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(generated.Output), "declare<Input>()") || strings.Contains(string(generated.Output), "declare()") {
				t.Fatalf("declaration-only call reached %s runtime output:\n%s", mode, generated.Output)
			}
		})
	}
}

func TestClassBodyDeclarationRuleDoesNotApplyInsideMethods(t *testing.T) {
	provider := parseDeclarationOnlyProgram(t, `class ContractBase
end

def declare()
	return
end
`, "ruby", "test/provider")
	catalog, catalogDiagnostics := resolver.NewCatalog([]resolver.Module{{Path: "test/provider", Filename: "provider.trb", Program: provider}})
	if len(catalogDiagnostics) != 0 {
		t.Fatalf("catalog diagnostics: %#v", catalogDiagnostics)
	}
	program := parseDeclarationOnlyProgram(t, `import { ContractBase, declare } from test/provider

class Endpoint < ContractBase
	def execute()
		declare()
		return
	end
end

class OtherEndpoint < ContractBase
	declare()
end
`, "ruby", "app/endpoint")
	declarations := declaration.NewCatalog()
	declarations.ClassBodyDeclarationRules = []declaration.ClassBodyDeclarationRule{{
		Package: "test/provider", Function: "declare",
		Owner: declaration.DeclarationReference{ModulePath: "app/endpoint", Name: "Endpoint"},
	}}
	resolved, resolveDiagnostics := resolver.Resolve(program, resolver.Options{
		Mode: "ruby", Filename: "endpoint.trb", Catalog: catalog, Declarations: declarations,
	})
	if len(resolveDiagnostics) != 0 {
		t.Fatalf("resolve diagnostics: %#v", resolveDiagnostics)
	}
	checked, checkDiagnostics := checker.Check(program, resolved)
	if len(checkDiagnostics) != 0 {
		t.Fatalf("check diagnostics: %#v", checkDiagnostics)
	}
	endpoint := program.Statements[1].(*ast.ClassStatement)
	method := endpoint.Body[0].(*ast.MethodStatement)
	call := method.Body[0].(*ast.ExpressionStatement).Expression.(*ast.CallExpression)
	if checked.DeclarationOnlyCalls[call] {
		t.Fatal("method call was incorrectly treated as a class-body declaration")
	}
	otherEndpoint := program.Statements[2].(*ast.ClassStatement)
	otherCall := otherEndpoint.Body[0].(*ast.ExpressionStatement).Expression.(*ast.CallExpression)
	if checked.DeclarationOnlyCalls[otherCall] {
		t.Fatal("class-body declaration rule matched a different project class")
	}
}

func parseDeclarationOnlyProgram(t *testing.T, source, mode, modulePath string) *ast.Program {
	t.Helper()
	program, diagnostics := parser.Parse([]byte(source))
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", diagnostics)
	}
	program.Mode = mode
	program.ModulePath = modulePath
	program.TypeScriptRuntime = "node"
	return program
}
