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
			Source: []byte(`import { serve } from trb/web

def main()
	serve(4100)
	return
end
`),
		},
		{
			Filename:   "/project/src/routes/todos/[id].trb",
			ModulePath: "routes/todos/[id]",
			Package:    "todos",
			Source: []byte(`import { Context, Response, json, path_param, request_json } from trb/web
import { Result } from trb/std/result

record TodoRequest
	title: String
end

record TodoResponse
	id: String
	title: String
end

def post(context: Context): Response
	id := path_param(context, "id")
	case request_json<TodoRequest>(context.request)
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
			var target string
			switch mode {
			case "go":
				target = "trbWebServe(3000)"
			case "ruby":
				target = "trb_web_serve(3000)"
			case "typescript":
				target = "trb_web_serve(3000);"
			}
			if !strings.Contains(string(main.Output), target) {
				t.Fatalf("%s output does not contain %q:\n%s", mode, target, main.Output)
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
			Source: []byte(`import { Context, Response } from trb/web

def call(context: Context): Response
	return Response.new(status: 204, headers: {}, body: "".to_bytes())
end
`),
		},
	}

	_, err := CompileProject(sources, Options{Mode: "go", GoModule: "example.com/web-middleware", SourceRoot: "/project/src", ProjectRoot: "/project"})
	if err == nil || !strings.Contains(err.Error(), "middleware must have signature def call(context: Context, next_handler: Next): Response") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileProjectRejectsAmbiguousWebRoutes(t *testing.T) {
	routeSource := []byte(`import { Response } from trb/web

def get(): Response
	return Response.new(status: 204, headers: {}, body: "".to_bytes())
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
		targets = []string{"func trbWebServe(port int64)", "trbWebServe(4100)"}
	case "ruby":
		targets = []string{"def trb_web_serve(port)", "trb_web_serve(4100)"}
	case "typescript":
		targets = []string{`import { createServer } from "node:http";`, "function trb_web_serve(port: number)", "trb_web_serve(4100);"}
	}
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
		target = "export function trb_web_middleware_0("
	}
	if !strings.Contains(string(artifact.Output), target) {
		t.Fatalf("%s output does not contain %q:\n%s", mode, target, artifact.Output)
	}
}
