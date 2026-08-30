package compiler

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	webintegration "github.com/type-rb/type-rb/internal/web"
)

func TestCompileProjectAttachesWebRouteManifestToMain(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename:   "/project/src/main.trb",
			ModulePath: "main",
			Package:    "main",
			Source: []byte(`import trb/web

def main()
	Web.serve(Web::ServerConfig.new(host: "127.0.0.1", port: 4100, body_limit_bytes: 2048, shutdown_timeout_milliseconds: 500))
	return
end
`),
		},
		{
			Filename:   "/project/src/routes/todos/[id].trb",
			ModulePath: "routes/todos/[id]",
			Package:    "todos",
			Source: []byte(`import { Context, Response } from trb/web
import { Result } from trb/std/result

record TodoRequest
	title: String
end

record TodoResponse
	id: String
	title: String
end

def post(context: Context): Response
	id := context.path_value("id")
	case context.request.json<TodoRequest>()
	when Result::Ok(payload)
		return Response.json(TodoResponse.new(id: id, title: payload.title), 201)
	when Result::Err(_error)
		return Response.json(TodoResponse.new(id: id, title: "invalid"), 400)
	end
end
`),
		},
		{
			Filename:   "/project/src/routes/_middleware.trb",
			ModulePath: "routes/_middleware",
			Package:    "routes",
			Source: []byte(`import { Context, Next, Response } from trb/web

def call(context: Context, next_handler: Next): Response
	return next_handler.call(context)
end
`),
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject(sources, Options{Mode: mode, GoModule: "example.com/web-routes", RubyLoader: "require_relative", SourceRoot: "/project/src", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			main := artifactForModule(artifacts, "main")
			if main == nil {
				t.Fatal("main artifact was not generated")
			}
			manifest := webintegration.ManifestFrom(main.IR.Extensions)
			if manifest == nil || len(manifest.Routes) != 1 {
				t.Fatalf("unexpected web manifest: %#v", manifest)
			}
			if len(manifest.Middlewares) != 1 {
				t.Fatalf("unexpected middleware manifest: %#v", manifest.Middlewares)
			}
			route := manifest.Routes[0]
			if route.Method != "POST" || route.Path != "/todos/:id" || route.ModulePath != "routes/todos/[id]" || route.Handler != "post" || route.TargetHandler != "trb_web_route_0" {
				t.Fatalf("unexpected route: %#v", route)
			}
			if len(route.PathParameters) != 1 || route.PathParameters[0] != "id" {
				t.Fatalf("unexpected path parameters: %#v", route.PathParameters)
			}
			if len(route.Middlewares) != 1 || route.Middlewares[0].ModulePath != "routes/_middleware" || route.Middlewares[0].TargetHandler != "trb_web_middleware_0" {
				t.Fatalf("unexpected route middleware chain: %#v", route.Middlewares)
			}
			for _, artifact := range artifacts {
				if artifact.IR.ModulePath == "routes/todos/[id]" {
					assertWebHandlerTarget(t, mode, artifact)
				}
				if artifact.IR.ModulePath == "routes/_middleware" {
					assertWebMiddlewareTarget(t, mode, artifact)
				}
				if artifact.IR.ModulePath != "main" && webintegration.ManifestFrom(artifact.IR.Extensions) != nil {
					t.Fatalf("route manifest was attached to %s", artifact.IR.ModulePath)
				}
			}
			assertWebServerTarget(t, mode, main)
		})
	}
}

func TestCompileProjectAttachesTypedWebEndpointCatalogAcrossModes(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
			Source: []byte("import trb/web\n\ndef main()\n\tWeb.serve()\n\treturn\nend\n"),
		},
		{
			Filename: "/project/src/contracts/reports.trb", ModulePath: "contracts/reports", Package: "contracts",
			Source: []byte(`record CreateReportBody
	title: String
end

record CreateReportInput
	body: CreateReportBody
end

record CreateReportResponse
	id: Integer
end

record ErrorResponse
	message: String
end
`),
		},
		{
			Filename: "/project/src/routes/reports.trb", ModulePath: "routes/reports", Package: "routes",
			Source: []byte(`import { Context, Endpoint, Response, handles, input, response } from trb/web
import { CreateReportInput, CreateReportResponse, ErrorResponse } from contracts/reports

def post(_context: Context): Response
	return Response.json(CreateReportResponse.new(id: 42), 202)
end

class CreateReportEndpoint < Endpoint
	handles(post)
	input<CreateReportInput>()
	response<CreateReportResponse>(status: 202)
	response<ErrorResponse>(status: 400)
end
`),
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject(sources, Options{
				Mode: mode, GoModule: "example.com/web-contract", RubyLoader: "require_relative",
				SourceRoot: "/project/src", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			manifest := webintegration.ManifestFrom(artifactForModule(artifacts, "main").IR.Extensions)
			if manifest == nil || manifest.EndpointCatalog.ProtocolVersion != webintegration.EndpointCatalogProtocolVersion || len(manifest.EndpointCatalog.Endpoints) != 1 {
				t.Fatalf("unexpected endpoint catalog: %#v", manifest)
			}
			endpoint := manifest.EndpointCatalog.Endpoints[0]
			if endpoint.Name != "CreateReportEndpoint" || endpoint.ModulePath != "routes/reports" || endpoint.Handler != "post" || endpoint.Method != "POST" || endpoint.Path != "/reports" {
				t.Fatalf("unexpected endpoint: %#v", endpoint)
			}
			if endpoint.Input == nil || endpoint.Input.Authored.Name != "CreateReportInput" || endpoint.Input.Authored.Definition == nil || endpoint.Input.Authored.Definition.ModulePath != "contracts/reports" {
				t.Fatalf("unexpected endpoint input: %#v", endpoint.Input)
			}
			if len(endpoint.Responses) != 2 || endpoint.Responses[0].Status != 202 || endpoint.Responses[0].Type.Authored.Name != "CreateReportResponse" || endpoint.Responses[0].Type.Authored.Definition == nil || endpoint.Responses[0].Type.Authored.Definition.ModulePath != "contracts/reports" || endpoint.Responses[1].Status != 400 || endpoint.Responses[1].Type.Authored.Name != "ErrorResponse" {
				t.Fatalf("unexpected endpoint responses: %#v", endpoint.Responses)
			}
			route := artifactForModule(artifacts, "routes/reports")
			if route == nil {
				t.Fatal("route artifact was not generated")
			}
			assertEndpointCallsAreDeclarationOnly(t, route)
			output := string(route.Output)
			if strings.Contains(output, "handles(post") || strings.Contains(output, "response<CreateReportResponse") || strings.Contains(output, "input<CreateReportInput") {
				t.Fatalf("declaration-only endpoint calls reached %s output:\n%s", mode, output)
			}
		})
	}
}

func assertEndpointCallsAreDeclarationOnly(t *testing.T, artifact *Artifact) {
	t.Helper()
	for _, statement := range artifact.IR.Statements {
		class, ok := statement.(*ir.Class)
		if !ok || class.Name != "CreateReportEndpoint" {
			continue
		}
		if len(class.Body) != 4 {
			t.Fatalf("endpoint class body=%#v, want four declarations", class.Body)
		}
		for _, bodyStatement := range class.Body {
			expression, ok := bodyStatement.(*ir.ExpressionStatement)
			if !ok {
				t.Fatalf("endpoint declaration is %T", bodyStatement)
			}
			call, ok := expression.Expression.(*ir.Call)
			if !ok || !call.DeclarationOnly {
				t.Fatalf("endpoint declaration call=%#v", expression.Expression)
			}
		}
		return
	}
	t.Fatal("endpoint contract class was not lowered")
}

func TestCompileProjectRejectsInvalidWebEndpointContracts(t *testing.T) {
	main := SourceUnit{
		Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
		Source: []byte("import trb/web\n\ndef main()\n\tWeb.serve()\n\treturn\nend\n"),
	}
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "missing handler",
			source: `import { Context, Endpoint, Response, response } from trb/web

record Payload
	value: String
end

def post(_context: Context): Response
	return Response.empty()
end

class Contract < Endpoint
	response<Payload>(status: 200)
end
`,
			want: "trb/web endpoint contract Contract must declare handles(handler)",
		},
		{
			name: "duplicate status",
			source: `import { Body, Headers } from trb/http
import { Context, Endpoint, Response, handles, response } from trb/web

record Payload
	value: String
end

def post(_context: Context): Response
	return Response.new(status: 204, headers: Headers.new(), body: Body.empty())
end

class Contract < Endpoint
	handles(post)
	response<Payload>(status: 200)
	response<Payload>(status: 200)
end
`,
			want: "trb/web endpoint contract Contract declares response status 200 more than once",
		},
		{
			name: "non route handler",
			source: `import { Body, Headers } from trb/http
import { Context, Endpoint, Response, handles, response } from trb/web

record Payload
	value: String
end

def helper(_context: Context): Response
	return Response.new(status: 204, headers: Headers.new(), body: Body.empty())
end

def post(_context: Context): Response
	return Response.new(status: 204, headers: Headers.new(), body: Body.empty())
end

class Contract < Endpoint
	handles(helper)
	response<Payload>(status: 200)
end
`,
			want: "handles helper, which is not a file-route handler in the same module",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			route := SourceUnit{
				Filename: "/project/src/routes/reports.trb", ModulePath: "routes/reports", Package: "routes", Source: []byte(test.source),
			}
			_, err := CompileProject([]SourceUnit{main, route}, Options{
				Mode: "go", GoModule: "example.com/web-contract", SourceRoot: "/project/src", ProjectRoot: "/project",
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestGoWebJSONReservesPortableHTTPImportAlias(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
			Source: []byte("import trb/web\n\ndef main()\n\tWeb.serve()\n\treturn\nend\n"),
		},
		{
			Filename: "/project/src/presentation/http/response.trb", ModulePath: "presentation/http/response", Package: "http",
			Source: []byte(`import { Response } from trb/web

def render(response: Response): Response
	return response
end
`),
		},
		{
			Filename: "/project/src/routes/index.trb", ModulePath: "routes/index", Package: "routes",
			Source: []byte(`import { Context, Response } from trb/web
import { render } from presentation/http/response

def get(_context: Context): Response
	return render(Response.json({ "status" => "ok" }))
end
`),
		},
	}
	artifacts, err := CompileProject(sources, Options{Mode: "go", GoModule: "example.com/web-routes", SourceRoot: "/project/src", ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	route := artifactForModule(artifacts, "routes/index")
	if route == nil {
		t.Fatal("route artifact was not generated")
	}
	output := string(route.Output)
	for _, expected := range []string{
		`import "example.com/web-routes/presentation/http"`,
		`import __trb_http "example.com/web-routes/trb/http"`,
		`http.Render(`,
		`__trb_http.NewHeaders`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated route is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileProjectGeneratesDefaultWebServerPort(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/src/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import trb/web

def main()
	Web.serve()
	return
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/web-server", RubyLoader: "require_relative", SourceRoot: "/project/src", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			main := artifactForModule(artifacts, "main")
			if main == nil {
				t.Fatal("main artifact was not generated")
			}
			var targets []string
			switch mode {
			case "go":
				targets = []string{`trbWebServe(web.WebServerConfig{Host: "0.0.0.0", Port: 3000, BodyLimitBytes: 1048576, ShutdownTimeoutMilliseconds: 10000})`}
			case "ruby":
				targets = []string{`trb_web_serve(Web::ServerConfig.new(host: "0.0.0.0", port: 3000, body_limit_bytes: 1048576, shutdown_timeout_milliseconds: 10000))`}
			case "typescript":
				targets = []string{`trb_web_serve({ host: "0.0.0.0", port: 3000, body_limit_bytes: 1048576, shutdown_timeout_milliseconds: 10000 });`}
			}
			for _, target := range targets {
				if !strings.Contains(string(main.Output), target) {
					t.Fatalf("%s output does not contain %q:\n%s", mode, target, main.Output)
				}
			}
		})
	}
}

func TestCompileProjectRejectsInvalidWebHandlerSignature(t *testing.T) {
	sources := []SourceUnit{
		{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte("def main()\n\treturn\nend\n")},
		{
			Filename:   "/project/src/routes/todos.trb",
			ModulePath: "routes/todos",
			Package:    "routes",
			Source: []byte(`import { Context } from trb/web

def post(context: Context)
	puts(context.request.path)
	return
end
`),
		},
	}

	_, err := CompileProject(sources, Options{Mode: "go", GoModule: "example.com/web-routes", SourceRoot: "/project/src", ProjectRoot: "/project"})
	if err == nil || !strings.Contains(err.Error(), "POST /todos handler must have signature def post(context: Context): Response") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileProjectRejectsInvalidWebMiddlewareSignature(t *testing.T) {
	sources := []SourceUnit{
		{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte("def main()\n\treturn\nend\n")},
		{
			Filename:   "/project/src/routes/_middleware.trb",
			ModulePath: "routes/_middleware",
			Package:    "routes",
			Source: []byte(`import { Body, Headers } from trb/http
import { Context, Response } from trb/web

def call(context: Context): Response
	return Response.new(status: 204, headers: Headers.new(), body: Body.empty())
end
`),
		},
	}

	_, err := CompileProject(sources, Options{Mode: "go", GoModule: "example.com/web-middleware", SourceRoot: "/project/src", ProjectRoot: "/project"})
	if err == nil || !strings.Contains(err.Error(), "middleware must have signature def call(context: Context, next_handler: Next): Response") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileProjectValidatesWebPathParameterCalls(t *testing.T) {
	main := SourceUnit{
		Filename:   "/project/src/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source:     []byte("def main()\n\treturn\nend\n"),
	}
	tests := []struct {
		name       string
		filename   string
		modulePath string
		source     string
		want       string
	}{
		{
			name:       "ordinary parameter",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response } from trb/web

def get(context: Context): Response
	return Response.text(context.path_value("id"))
end
`,
		},
		{
			name:       "typed parameters",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response } from trb/web
import { Result } from trb/std/result

record TodoParams
	id: Integer
end

def get(context: Context): Response
	case context.params<TodoParams>()
	when Result::Ok(params)
		return Response.text(params.id.to_s())
	when Result::Err(_error)
		return Response.text("invalid", 400)
	end
end
`,
		},
		{
			name:       "typed parameters missing route field",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response } from trb/web
import { Result } from trb/std/result

record TodoParams
	slug: String
end

def get(context: Context): Response
	case context.params<TodoParams>()
	when Result::Ok(params)
		return Response.text(params.slug)
	when Result::Err(_error)
		return Response.text("invalid", 400)
	end
end
`,
			want: `Context#params<TodoParams>() field "slug" is not declared by route /todos/:id`,
		},
		{
			name:       "endpoint input parameters",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response } from trb/web
import { Result } from trb/std/result

record TodoParams
	id: Integer
end

record TodoInput
	params: TodoParams
end

def get(context: Context): Response
	case context.bind<TodoInput>()
	when Result::Ok(input)
		return Response.text(input.params.id.to_s())
	when Result::Err(_error)
		return Response.text("invalid", 400)
	end
end
`,
		},
		{
			name:       "endpoint input parameters mismatch",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response } from trb/web
import { Result } from trb/std/result

record TodoParams
	slug: String
end

record TodoInput
	params: TodoParams
end

def get(context: Context): Response
	case context.bind<TodoInput>()
	when Result::Ok(input)
		return Response.text(input.params.slug)
	when Result::Err(_error)
		return Response.text("invalid", 400)
	end
end
`,
			want: `Context#bind<TodoInput>() params field "slug" is not declared by route /todos/:id`,
		},
		{
			name:       "endpoint input parameters omit route field",
			filename:   "/project/src/routes/todos/[id]/[slug].trb",
			modulePath: "routes/todos/[id]/[slug]",
			source: `import { Context, Response } from trb/web

record TodoParams
	id: Integer
end

record TodoInput
	params: TodoParams
end

def get(context: Context): Response
	input := context.bind<TodoInput>() catch |_error|
		return Response.text("invalid", 400)
	end
	return Response.text(input.params.id.to_s())
end
`,
			want: `Context#bind<TodoInput>() params is missing route parameter "slug"`,
		},
		{
			name:       "endpoint input parameters mismatch inside catch",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response } from trb/web

record TodoParams
	slug: String
end

record TodoInput
	params: TodoParams
end

def get(context: Context): Response
	input := context.bind<TodoInput>() catch |_error|
		return Response.text("invalid", 400)
	end
	return Response.text(input.params.slug)
end
`,
			want: `Context#bind<TodoInput>() params field "slug" is not declared by route /todos/:id`,
		},
		{
			name:       "endpoint input parameters mismatch inside try",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, EndpointInputError, Response } from trb/web
import { Result } from trb/std/result

record TodoParams
	slug: String
end

record TodoInput
	params: TodoParams
end

def bind_input(context: Context): Result<TodoInput, EndpointInputError>
	input := try context.bind<TodoInput>()
	return Result<TodoInput, EndpointInputError>::Ok(input)
end

def get(context: Context): Response
	input := bind_input(context) catch |_error|
		return Response.text("invalid", 400)
	end
	return Response.text(input.params.slug)
end
`,
			want: `Context#bind<TodoInput>() params field "slug" is not declared by route /todos/:id`,
		},
		{
			name:       "catch-all parameter",
			filename:   "/project/src/routes/files/[...path].trb",
			modulePath: "routes/files/[...path]",
			source: `import { Context, Response } from trb/web

def get(context: Context): Response
	return Response.text(context.path_value("path"))
end
`,
		},
		{
			name:       "helper parameter",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response } from trb/web

def todo_id(context: Context): String
	return context.path_value("id")
end

def get(context: Context): Response
	return Response.text(todo_id(context))
end
`,
		},
		{
			name:       "unrelated local function",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response } from trb/web

def path_param(context: Context, name: String): String
	puts(context.request.path)
	return name
end

def get(context: Context): Response
	name := "id"
	return Response.text(path_param(context, name))
end
`,
		},
		{
			name:       "dynamic parameter name",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response } from trb/web

def get(context: Context): Response
	name := "id"
	return Response.text(context.path_value(name))
end
`,
			want: "Context#path_value() name must be a string literal in a route file",
		},
		{
			name:       "undeclared parameter",
			filename:   "/project/src/routes/files/[...path].trb",
			modulePath: "routes/files/[...path]",
			source: `import { Context, Response } from trb/web

def get(context: Context): Response
	return Response.text(context.path_value("slug"))
end
`,
			want: `Context#path_value() references undeclared route parameter "slug"`,
		},
		{
			name:       "undeclared helper parameter",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response } from trb/web

def todo_slug(context: Context): String
	return context.path_value("slug")
end

def get(context: Context): Response
	return Response.text(todo_slug(context))
end
`,
			want: `Context#path_value() references undeclared route parameter "slug"`,
		},
	}

	for _, test := range tests {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				route := SourceUnit{Filename: test.filename, ModulePath: test.modulePath, Package: "routes", Source: []byte(test.source)}
				_, err := CompileProject([]SourceUnit{main, route}, Options{
					Mode:        mode,
					GoModule:    "example.com/web-path-parameters",
					RubyLoader:  "require_relative",
					SourceRoot:  "/project/src",
					ProjectRoot: "/project",
				})
				if test.want == "" {
					if err != nil {
						t.Fatal(err)
					}
					return
				}
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("unexpected error: %v", err)
				}
			})
		}
	}
}

func TestCompileProjectAllowsStaticRoutesAlongsideParameterRoutesAcrossModes(t *testing.T) {
	routeSource := []byte(`import { Body, Headers } from trb/http
import { Context, Response } from trb/web

def get(_context: Context): Response
	return Response.new(status: 204, headers: Headers.new(), body: Body.empty())
end
`)
	sources := []SourceUnit{
		{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte("def main()\n\treturn\nend\n")},
		{Filename: "/project/src/routes/todos/new.trb", ModulePath: "routes/todos/new", Package: "todos", Source: routeSource},
		{Filename: "/project/src/routes/todos/[id].trb", ModulePath: "routes/todos/[id]", Package: "todos", Source: routeSource},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject(sources, Options{Mode: mode, GoModule: "example.com/web-routes", RubyLoader: "require_relative", SourceRoot: "/project/src", ProjectRoot: "/project"})
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		manifest := webintegration.ManifestFrom(artifactForModule(artifacts, "main").IR.Extensions)
		if manifest == nil || len(manifest.Routes) != 2 || manifest.Routes[0].Path != "/todos/new" || manifest.Routes[1].Path != "/todos/:id" {
			t.Fatalf("unexpected %s route precedence: %#v", mode, manifest)
		}
	}
}

func TestCompileProjectDoesNotTreatRoutesAsWebRoutesWithoutPackageImport(t *testing.T) {
	sources := []SourceUnit{
		{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte("def main()\n\treturn\nend\n")},
		{Filename: "/project/src/routes/helper.trb", ModulePath: "routes/helper", Package: "routes", Source: []byte("def helper()\n\treturn\nend\n")},
	}

	artifacts, err := CompileProject(sources, Options{Mode: "go", GoModule: "example.com/non-web-routes", SourceRoot: "/project/src", ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	main := artifactForModule(artifacts, "main")
	if main == nil || webintegration.ManifestFrom(main.IR.Extensions) != nil {
		t.Fatalf("unexpected route manifest: %#v", main)
	}
}

func artifactForModule(artifacts []*Artifact, modulePath string) *Artifact {
	for _, artifact := range artifacts {
		if artifact.IR.ModulePath == modulePath {
			return artifact
		}
	}
	return nil
}

func assertWebHandlerTarget(t *testing.T, mode string, artifact *Artifact) {
	t.Helper()
	var target string
	switch mode {
	case "go":
		target = "func TrbWebRoute0("
	case "ruby":
		target = "def trb_web_route_0("
	case "typescript":
		target = "export function trb_web_route_0("
	}
	if !strings.Contains(string(artifact.Output), target) {
		t.Fatalf("%s output does not contain %q:\n%s", mode, target, artifact.Output)
	}
}

func assertWebServerTarget(t *testing.T, mode string, artifact *Artifact) {
	t.Helper()
	var targets []string
	switch mode {
	case "go":
		targets = []string{"func trbWebServe(config web.WebServerConfig)", "nethttp.MaxBytesReader(writer, request.Body, int64(config.BodyLimitBytes))", "request.URL.EscapedPath()", "request.URL.RawQuery", `net.JoinHostPort(config.Host, strconv.Itoa(config.Port))`, "signal.NotifyContext", `trbWebServe(func() web.WebServerConfig`, "web.Trb__RecordNew__WebServerConfig"}
	case "ruby":
		targets = []string{"def trb_web_serve(config)", "content_length > config.body_limit_bytes", `path, query_string = target.split("?", 2)`, "Signal.trap(signal)", `TCPServer.new(config.host, config.port)`, `trb_web_serve(-> {`, "Web::ServerConfig.__trb_record_new"}
	case "typescript":
		targets = []string{`import { createServer } from "node:http";`, "function trb_web_serve(config: TrbWebServerConfig)", "if (size > config.body_limit_bytes)", `new __trb_http.Headers(header_entries)`, `process.once("SIGTERM", shutdown)`, `server.listen(config.port, config.host)`, `trb_web_serve(__trb_web.__trbRecordNewWebServerConfig({ host: "127.0.0.1", port: 4100, body_limit_bytes: 2048, shutdown_timeout_milliseconds: 500 }));`}
	}
	targets = append(targets, "payload_too_large")
	for _, target := range targets {
		if !strings.Contains(string(artifact.Output), target) {
			t.Fatalf("%s output does not contain %q:\n%s", mode, target, artifact.Output)
		}
	}
}

func assertWebMiddlewareTarget(t *testing.T, mode string, artifact *Artifact) {
	t.Helper()
	var target string
	switch mode {
	case "go":
		target = "func TrbWebMiddleware0("
	case "ruby":
		target = "def trb_web_middleware_0("
	case "typescript":
		target = "export async function trb_web_middleware_0("
	}
	if !strings.Contains(string(artifact.Output), target) {
		t.Fatalf("%s output does not contain %q:\n%s", mode, target, artifact.Output)
	}
}
