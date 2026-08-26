package web_test

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
	"github.com/type-rb/type-rb/internal/parser"
	webintegration "github.com/type-rb/type-rb/internal/web"
)

func TestGeneratedBrowserClientCompilesForTheBrowserTarget(t *testing.T) {
	contractSource := []byte(`newtype ReportID = Integer

enum Visibility
	Public = "public"
	Private = "private"
end

record ReportParams
	id: ReportID
end

record ReportQuery
	page: Integer?
	tag: Array<String>
	visibility: Visibility?
end

record CreateReportInput
	params: ReportParams
	query: ReportQuery
end

record CreateReportResponse
	id: ReportID
end

record ErrorResponse
	message: String
end
`)
	routeSource := []byte(`import { Context, Endpoint, Response, handles, input, response } from trb/web
import { CreateReportInput, CreateReportResponse, ErrorResponse } from contracts/reports
import { Unit } from trb/std/unit

def get(_context: Context): Response
	return Response.new(status: 200)
end

class GetReportEndpoint < Endpoint
	handles(get)
	input<CreateReportInput>()
	response<CreateReportResponse>(status: 200)
	response<ErrorResponse>(status: 404)
	response<Unit>(status: 204)
end
`)
	contracts := parseBrowserClientProgram(t, "contracts/reports", contractSource)
	route := parseBrowserClientProgram(t, "routes/reports/[id]", routeSource)
	declarations, err := packageextensionhost.ExportProjectDeclarationInput(webintegration.PackageName, []*ast.Program{route, contracts}, packageextensionhost.ProjectDeclarationInputOptions{})
	if err != nil {
		t.Fatal(err)
	}
	catalog, endpointIssues, err := webintegration.BuildEndpointCatalog(declarations, []webintegration.Route{{
		Method: "GET", Path: "/reports/:id", ModulePath: "routes/reports/[id]", Handler: "get",
	}})
	if err != nil || len(endpointIssues) != 0 {
		t.Fatalf("endpoint catalog err=%v issues=%#v", err, endpointIssues)
	}
	generated, clientIssues, err := webintegration.BuildBrowserClient(catalog, declarations, webintegration.BrowserClientOptions{})
	if err != nil || len(clientIssues) != 0 {
		t.Fatalf("browser client err=%v issues=%#v", err, clientIssues)
	}

	_, err = compiler.CompileProject([]compiler.SourceUnit{
		{Filename: "/project/src/contracts/reports.trb", ModulePath: "contracts/reports", Source: contractSource},
		{Filename: "/project/src/generated/api_client.trb", ModulePath: "generated/api_client", Source: []byte(generated)},
	}, compiler.Options{
		Mode: "typescript", TypeScriptRuntime: "browser", ProjectRoot: "/project", SourceRoot: "/project/src",
	})
	if err != nil {
		t.Fatalf("generated browser client did not compile:\n%v\n\n%s", err, generated)
	}
}

func parseBrowserClientProgram(t *testing.T, modulePath string, source []byte) *ast.Program {
	t.Helper()
	program, diagnostics := parser.Parse(source)
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", diagnostics)
	}
	program.ModulePath = modulePath
	return program
}
