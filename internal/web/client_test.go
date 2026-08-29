package web

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
)

func TestBuildBrowserClientGeneratesTypedTypeRBSource(t *testing.T) {
	contracts := parseOpenAPIProgram(t, "contracts/reports", `newtype ReportID = Integer

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

record CreateReportBody
	title: String @json("report_title")
end

record CreateReportInput
	params: ReportParams
	query: ReportQuery
	body: CreateReportBody
end

record CreateReportResponse
	id: ReportID
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

	source, clientIssues, err := BuildBrowserClient(catalog, input, BrowserClientOptions{})
	if err != nil || len(clientIssues) != 0 {
		t.Fatalf("browser client err=%v issues=%#v", err, clientIssues)
	}
	for _, fragment := range []string{
		"import { CreateReportInput, CreateReportResponse, ErrorResponse } from contracts/reports",
		"import { HttpClient, NoBody, RequestError, RequestErrorKind, Response, json_body } from trb/platform/typescript/browser",
		"enum CreateReportEndpointResult",
		"Status202(response: Response<CreateReportResponse>)",
		"Status204(response: Response<NoBody>)",
		"def create_report(input: CreateReportInput, *, headers: Headers = Headers.new(), timeout_milliseconds: Integer? = nil): Result<CreateReportEndpointResult, RequestError>",
		`path := "/" + "reports" + "/" + URL.encode_component(input.params.id.value().to_s())`,
		"if query_value_0 != nil",
		`URL::QueryParameter.new(name: "page", value: query_value_0.to_s())`,
		"input.query.tag.each do |query_item_1|",
		`URL::QueryParameter.new(name: "visibility", value: query_value_2.raw_value())`,
		"body := try json_body(input.body)",
		"raw := try @_http.request(",
		"method: HttpMethod.post(),",
		"decoded := try raw.json<CreateReportResponse>()",
		"decoded := try raw.no_body()",
		"RequestErrorKind::Contract",
	} {
		if !strings.Contains(source, fragment) {
			t.Fatalf("generated source does not contain %q:\n%s", fragment, source)
		}
	}

	repeated, repeatedIssues, err := BuildBrowserClient(catalog, input, BrowserClientOptions{})
	if err != nil || len(repeatedIssues) != 0 || repeated != source {
		t.Fatalf("browser client output is not deterministic: err=%v issues=%#v", err, repeatedIssues)
	}
}

func TestBuildBrowserClientRequiresSharedImportedContractTypes(t *testing.T) {
	program := parseOpenAPIProgram(t, "routes/health", `import { Context, Endpoint, Response, handles, response } from trb/web

record HealthResponse
	ok: Boolean
end

def get(_context: Context): Response
	return Response.new(status: 200)
end

class HealthEndpoint < Endpoint
	handles(get)
	response<HealthResponse>(status: 200)
end
`)
	input, catalog := browserClientTestCatalog(t, []*ast.Program{program}, Route{Method: "GET", Path: "/health", ModulePath: "routes/health", Handler: "get"})
	_, issues, err := BuildBrowserClient(catalog, input, BrowserClientOptions{})
	if err != nil || len(issues) != 1 || !strings.Contains(issues[0].Message, "shared module") {
		t.Fatalf("err=%v issues=%#v", err, issues)
	}
}

func TestBuildBrowserClientRejectsGeneratedNameCollisions(t *testing.T) {
	contracts := parseOpenAPIProgram(t, "contracts/health", `record HealthResponse
	ok: Boolean
end
`)
	route := parseOpenAPIProgram(t, "routes/health", `import { Context, Endpoint, Response, handles, response } from trb/web
import { HealthResponse } from contracts/health

def get(_context: Context): Response
	return Response.new(status: 200)
end

class HealthEndpoint < Endpoint
	handles(get)
	response<HealthResponse>(status: 200)
end
`)
	input, catalog := browserClientTestCatalog(t, []*ast.Program{route, contracts}, Route{Method: "GET", Path: "/health", ModulePath: "routes/health", Handler: "get"})
	_, issues, err := BuildBrowserClient(catalog, input, BrowserClientOptions{Name: "HttpClient"})
	if err != nil || len(issues) == 0 || !strings.Contains(issues[0].Message, "conflicts with import") {
		t.Fatalf("err=%v issues=%#v", err, issues)
	}
}

func TestBuildBrowserClientRejectsConstructorMethodCollision(t *testing.T) {
	contracts := parseOpenAPIProgram(t, "contracts/health", `record HealthResponse
	ok: Boolean
end
`)
	route := parseOpenAPIProgram(t, "routes/health", `import { Context, Endpoint, Response, handles, response } from trb/web
import { HealthResponse } from contracts/health

def get(_context: Context): Response
	return Response.new(status: 200)
end

class InitializeEndpoint < Endpoint
	handles(get)
	response<HealthResponse>(status: 200)
end
`)
	input, catalog := browserClientTestCatalog(t, []*ast.Program{route, contracts}, Route{Method: "GET", Path: "/health", ModulePath: "routes/health", Handler: "get"})
	_, issues, err := BuildBrowserClient(catalog, input, BrowserClientOptions{})
	if err != nil || len(issues) != 1 || !strings.Contains(issues[0].Message, "reserved browser client method initialize") {
		t.Fatalf("err=%v issues=%#v", err, issues)
	}
}

func browserClientTestCatalog(t *testing.T, programs []*ast.Program, route Route) (packageextension.ProjectDeclarationInput, EndpointCatalog) {
	t.Helper()
	input, err := packageextensionhost.ExportProjectDeclarationInput(PackageName, programs, packageextensionhost.ProjectDeclarationInputOptions{})
	if err != nil {
		t.Fatal(err)
	}
	catalog, issues, err := BuildEndpointCatalog(input, []Route{route})
	if err != nil || len(issues) != 0 {
		t.Fatalf("endpoint catalog err=%v issues=%#v", err, issues)
	}
	return input, catalog
}
