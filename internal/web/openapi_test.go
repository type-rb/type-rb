package web

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
	"github.com/type-rb/type-rb/internal/parser"
)

func TestBuildOpenAPIGeneratesTypedEndpointDocument(t *testing.T) {
	contracts := parseOpenAPIProgram(t, "contracts/reports", `import { Date, Instant } from trb/std/time

newtype ReportID = Integer
alias ReportTitle = String

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
	published: Boolean
	created_on: Date?
	visibility: Visibility?
end

record CreateReportBody
	title: ReportTitle @json("report_title")
end

record CreateReportInput
	params: ReportParams
	query: ReportQuery
	body: CreateReportBody
end

record CreateReportResponse
	id: ReportID
	created_at: Instant
end

record ErrorResponse
	message: String
end
`)
	route := parseOpenAPIProgram(t, "routes/reports/[id]", `import { Context, Endpoint, Response, handles, input, response } from trb/web
import { CreateReportInput, CreateReportResponse, ErrorResponse } from contracts/reports
import { Unit } from trb/std/unit

def post(_context: Context): Response
	return Response.new(status: 202)
end

class CreateReportEndpoint < Endpoint
	handles(post)
	input<CreateReportInput>()
	response<CreateReportResponse>(status: 202)
	response<ErrorResponse>(status: 400)
	response<Unit>(status: 204)
end
`)
	input, err := packageextensionhost.ExportProjectDeclarationInput(PackageName, []*ast.Program{route, contracts}, packageextensionhost.ProjectDeclarationInputOptions{})
	if err != nil {
		t.Fatal(err)
	}
	catalog, issues, err := BuildEndpointCatalog(input, []Route{{
		Method: "POST", Path: "/reports/:id", ModulePath: "routes/reports/[id]", Handler: "post",
	}})
	if err != nil || len(issues) != 0 {
		t.Fatalf("endpoint catalog err=%v issues=%#v", err, issues)
	}

	document, openAPIIssues, err := BuildOpenAPI(catalog, input, OpenAPIOptions{Title: "Reports API", Version: "1.2.3"})
	if err != nil || len(openAPIIssues) != 0 {
		t.Fatalf("OpenAPI err=%v issues=%#v", err, openAPIIssues)
	}
	if document.OpenAPI != "3.1.0" || document.Info.Title != "Reports API" || document.Info.Version != "1.2.3" {
		t.Fatalf("unexpected OpenAPI metadata: %#v", document)
	}
	operation := document.Paths["/reports/{id}"]["post"]
	if operation.OperationID != "CreateReportEndpoint" || len(operation.Parameters) != 6 || operation.RequestBody == nil || len(operation.Responses) != 3 {
		t.Fatalf("unexpected operation: %#v", operation)
	}
	if parameter := operation.Parameters[0]; parameter.Name != "id" || parameter.In != "path" || !parameter.Required || parameter.Schema.Ref != "#/components/schemas/ReportID" {
		t.Fatalf("unexpected path parameter: %#v", parameter)
	}
	if parameter := operation.Parameters[1]; parameter.Name != "page" || parameter.Required || len(parameter.Schema.AnyOf) != 2 {
		t.Fatalf("unexpected nullable query parameter: %#v", parameter)
	}
	if parameter := operation.Parameters[2]; parameter.Name != "tag" || parameter.Required || parameter.Schema.Type != "array" {
		t.Fatalf("unexpected repeated query parameter: %#v", parameter)
	}
	if parameter := operation.Parameters[3]; parameter.Name != "published" || !parameter.Required || parameter.Schema.Type != "boolean" {
		t.Fatalf("unexpected required query parameter: %#v", parameter)
	}
	if response := operation.Responses["204"]; response.Description != "No Content" || len(response.Content) != 0 {
		t.Fatalf("unexpected no-content response: %#v", response)
	}
	if operation.RequestBody.Content["application/json"].Schema.Ref != "#/components/schemas/CreateReportBody" {
		t.Fatalf("unexpected request body: %#v", operation.RequestBody)
	}
	components := document.Components.Schemas
	if components["CreateReportBody"].Properties["report_title"].Type != "string" || len(components["CreateReportBody"].Required) != 1 {
		t.Fatalf("JSON field mapping was not preserved: %#v", components["CreateReportBody"])
	}
	if components["CreateReportResponse"].Properties["created_at"].Format != "date-time" {
		t.Fatalf("portable time format was not preserved: %#v", components["CreateReportResponse"])
	}
	if len(components["Visibility"].Enum) != 2 || components["Visibility"].Enum[0] != "public" {
		t.Fatalf("raw enum was not preserved: %#v", components["Visibility"])
	}
	if components["ReportID"].Type != "integer" || components["ReportID"].Minimum == nil || components["ReportID"].Maximum == nil {
		t.Fatalf("newtype representation was not preserved: %#v", components["ReportID"])
	}

	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"CreateReportInput", "ReportParams", "ReportQuery"} {
		if strings.Contains(string(encoded), `\"`+forbidden+`\"`) {
			t.Fatalf("bind envelope component %s leaked into OpenAPI: %s", forbidden, encoded)
		}
	}
}

func TestBuildOpenAPIRejectsRecursiveResponseRecords(t *testing.T) {
	program := parseOpenAPIProgram(t, "routes/nodes", `import { Context, Endpoint, Response, handles, response } from trb/web

record Node
	child: Node?
end

def get(_context: Context): Response
	return Response.new(status: 200)
end

class NodeEndpoint < Endpoint
	handles(get)
	response<Node>(status: 200)
end
`)
	input, catalog := openAPITestCatalog(t, program, Route{Method: "GET", Path: "/nodes", ModulePath: "routes/nodes", Handler: "get"})
	_, issues, err := BuildOpenAPI(catalog, input, OpenAPIOptions{Title: "Nodes", Version: "1.0.0"})
	if err != nil || len(issues) != 1 || !strings.Contains(issues[0].Message, "recursive OpenAPI record Node") {
		t.Fatalf("err=%v issues=%#v", err, issues)
	}
}

func TestBuildOpenAPIRejectsCatchAllRoutes(t *testing.T) {
	program := parseOpenAPIProgram(t, "routes/files/[...path]", `import { Context, Endpoint, Response, handles, response } from trb/web
import { Unit } from trb/std/unit

def get(_context: Context): Response
	return Response.new(status: 204)
end

class FilesEndpoint < Endpoint
	handles(get)
	response<Unit>(status: 204)
end
`)
	input, catalog := openAPITestCatalog(t, program, Route{Method: "GET", Path: "/files/*path", ModulePath: "routes/files/[...path]", Handler: "get"})
	_, issues, err := BuildOpenAPI(catalog, input, OpenAPIOptions{Title: "Files", Version: "1.0.0"})
	if err != nil || len(issues) != 1 || !strings.Contains(issues[0].Message, "catch-all route") {
		t.Fatalf("err=%v issues=%#v", err, issues)
	}
}

func TestBuildOpenAPIRejectsBodyForNoContentStatus(t *testing.T) {
	program := parseOpenAPIProgram(t, "routes/health", `import { Context, Endpoint, Response, handles, response } from trb/web

record Payload
	ok: Boolean
end

def get(_context: Context): Response
	return Response.new(status: 204)
end

class HealthEndpoint < Endpoint
	handles(get)
	response<Payload>(status: 204)
end
`)
	input, catalog := openAPITestCatalog(t, program, Route{Method: "GET", Path: "/health", ModulePath: "routes/health", Handler: "get"})
	_, issues, err := BuildOpenAPI(catalog, input, OpenAPIOptions{Title: "Health", Version: "1.0.0"})
	if err != nil || len(issues) != 1 || !strings.Contains(issues[0].Message, "cannot declare a response body") {
		t.Fatalf("err=%v issues=%#v", err, issues)
	}
}

func openAPITestCatalog(t *testing.T, program *ast.Program, route Route) (packageextension.ProjectDeclarationInput, EndpointCatalog) {
	t.Helper()
	input, err := packageextensionhost.ExportProjectDeclarationInput(PackageName, []*ast.Program{program}, packageextensionhost.ProjectDeclarationInputOptions{})
	if err != nil {
		t.Fatal(err)
	}
	catalog, issues, err := BuildEndpointCatalog(input, []Route{route})
	if err != nil || len(issues) != 0 {
		t.Fatalf("endpoint catalog err=%v issues=%#v", err, issues)
	}
	return input, catalog
}

func parseOpenAPIProgram(t *testing.T, modulePath, source string) *ast.Program {
	t.Helper()
	program, diagnostics := parser.Parse([]byte(source))
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", diagnostics)
	}
	program.ModulePath = modulePath
	return program
}
