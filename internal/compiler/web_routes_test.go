package compiler

import (
	"strings"
	"testing"

	webintegration "github.com/type-rb/type-rb/internal/web"
)

func TestCompileProjectAttachesWebRouteManifestToMain(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename:   "/project/src/main.trb",
			ModulePath: "main",
			Package:    "main",
			Source: []byte(`import { configure_server, serve } from trb/web

def main()
	serve(configure_server(host: "127.0.0.1", port: 4100, body_limit_bytes: 2048, shutdown_timeout_milliseconds: 500))
	return
end
`),
		},
		{
			Filename:   "/project/src/routes/todos/[id].trb",
			ModulePath: "routes/todos/[id]",
			Package:    "todos",
			Source: []byte(`import { Context, Response, json } from trb/web
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
		return json(TodoResponse.new(id: id, title: payload.title), 201)
	when Result::Err(_error)
		return json(TodoResponse.new(id: id, title: "invalid"), 400)
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

func TestCompileProjectGeneratesDefaultWebServerPort(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/src/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { serve } from trb/web

def main()
	serve()
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
				targets = []string{`trbWebServe(web.ServerConfig{Host: "0.0.0.0", Port: 3000, BodyLimitBytes: 1048576, ShutdownTimeoutMilliseconds: 10000})`}
			case "ruby":
				targets = []string{`trb_web_serve(ServerConfig.new(host: "0.0.0.0", port: 3000, body_limit_bytes: 1048576, shutdown_timeout_milliseconds: 10000))`}
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
			source: `import { Context, Response, text } from trb/web

def get(context: Context): Response
	return text(context.path_value("id"))
end
`,
		},
		{
			name:       "typed parameters",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response, text } from trb/web
import { Result } from trb/std/result

record TodoParams
	id: Integer
end

def get(context: Context): Response
	case context.params<TodoParams>()
	when Result::Ok(params)
		return text(params.id.to_s())
	when Result::Err(_error)
		return text("invalid", 400)
	end
end
`,
		},
		{
			name:       "typed parameters missing route field",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response, text } from trb/web
import { Result } from trb/std/result

record TodoParams
	slug: String
end

def get(context: Context): Response
	case context.params<TodoParams>()
	when Result::Ok(params)
		return text(params.slug)
	when Result::Err(_error)
		return text("invalid", 400)
	end
end
`,
			want: `Context#params<TodoParams>() field "slug" is not declared by route /todos/:id`,
		},
		{
			name:       "endpoint input parameters",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response, text } from trb/web
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
		return text(input.params.id.to_s())
	when Result::Err(_error)
		return text("invalid", 400)
	end
end
`,
		},
		{
			name:       "endpoint input parameters mismatch",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response, text } from trb/web
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
		return text(input.params.slug)
	when Result::Err(_error)
		return text("invalid", 400)
	end
end
`,
			want: `Context#bind<TodoInput>() params field "slug" is not declared by route /todos/:id`,
		},
		{
			name:       "catch-all parameter",
			filename:   "/project/src/routes/files/[...path].trb",
			modulePath: "routes/files/[...path]",
			source: `import { Context, Response, text } from trb/web

def get(context: Context): Response
	return text(context.path_value("path"))
end
`,
		},
		{
			name:       "helper parameter",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response, text } from trb/web

def todo_id(context: Context): String
	return context.path_value("id")
end

def get(context: Context): Response
	return text(todo_id(context))
end
`,
		},
		{
			name:       "unrelated local function",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response, text } from trb/web

def path_param(context: Context, name: String): String
	puts(context.request.path)
	return name
end

def get(context: Context): Response
	name := "id"
	return text(path_param(context, name))
end
`,
		},
		{
			name:       "dynamic parameter name",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response, text } from trb/web

def get(context: Context): Response
	name := "id"
	return text(context.path_value(name))
end
`,
			want: "Context#path_value() name must be a string literal in a route file",
		},
		{
			name:       "undeclared parameter",
			filename:   "/project/src/routes/files/[...path].trb",
			modulePath: "routes/files/[...path]",
			source: `import { Context, Response, text } from trb/web

def get(context: Context): Response
	return text(context.path_value("slug"))
end
`,
			want: `Context#path_value() references undeclared route parameter "slug"`,
		},
		{
			name:       "undeclared helper parameter",
			filename:   "/project/src/routes/todos/[id].trb",
			modulePath: "routes/todos/[id]",
			source: `import { Context, Response, text } from trb/web

def todo_slug(context: Context): String
	return context.path_value("slug")
end

def get(context: Context): Response
	return text(todo_slug(context))
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

func TestCompileProjectRejectsAmbiguousWebRoutes(t *testing.T) {
	routeSource := []byte(`import { Body, Headers } from trb/http
import { Response } from trb/web

def get(): Response
	return Response.new(status: 204, headers: Headers.new(), body: Body.empty())
end
`)
	sources := []SourceUnit{
		{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte("def main()\n\treturn\nend\n")},
		{Filename: "/project/src/routes/todos/new.trb", ModulePath: "routes/todos/new", Package: "todos", Source: routeSource},
		{Filename: "/project/src/routes/todos/[id].trb", ModulePath: "routes/todos/[id]", Package: "todos", Source: routeSource},
	}

	_, err := CompileProject(sources, Options{Mode: "go", GoModule: "example.com/web-routes", SourceRoot: "/project/src", ProjectRoot: "/project"})
	if err == nil || !strings.Contains(err.Error(), "GET /todos/new conflicts with route /todos/:id") {
		t.Fatalf("unexpected error: %v", err)
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
		targets = []string{"func trbWebServe(config web.ServerConfig)", "nethttp.MaxBytesReader(writer, request.Body, int64(config.BodyLimitBytes))", "request.URL.EscapedPath()", "request.URL.RawQuery", `net.JoinHostPort(config.Host, strconv.Itoa(config.Port))`, "signal.NotifyContext", `trbWebServe(web.ServerConfig{Host: "127.0.0.1", Port: 4100, BodyLimitBytes: 2048, ShutdownTimeoutMilliseconds: 500})`}
	case "ruby":
		targets = []string{"def trb_web_serve(config)", "content_length > config.body_limit_bytes", `path, query_string = target.split("?", 2)`, "Signal.trap(signal)", `TCPServer.new(config.host, config.port)`, `trb_web_serve(ServerConfig.new(host: "127.0.0.1", port: 4100, body_limit_bytes: 2048, shutdown_timeout_milliseconds: 500))`}
	case "typescript":
		targets = []string{`import { createServer } from "node:http";`, "function trb_web_serve(config: TrbWebServerConfig)", "if (size > config.body_limit_bytes)", `new __trb_http.Headers(header_entries)`, `process.once("SIGTERM", shutdown)`, `server.listen(config.port, config.host)`, `trb_web_serve({ host: "127.0.0.1", port: 4100, body_limit_bytes: 2048, shutdown_timeout_milliseconds: 500 });`}
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
