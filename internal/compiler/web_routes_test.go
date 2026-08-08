package compiler

import (
	"strings"
	"testing"
)

func TestCompileProjectAttachesWebRouteManifestToMain(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename:   "/project/src/main.trb",
			ModulePath: "main",
			Package:    "main",
			Source:     []byte("def main()\n\treturn\nend\n"),
		},
		{
			Filename:   "/project/src/routes/todos/[id].trb",
			ModulePath: "routes/todos/[id]",
			Package:    "todos",
			Source: []byte(`import { Context, Response } from trb/web

def post(context: Context): Response
	return Response.new(status: 200, headers: {}, body: context.request.body)
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
			if len(main.IR.WebRoutes) != 1 {
				t.Fatalf("got %d routes, want 1", len(main.IR.WebRoutes))
			}
			route := main.IR.WebRoutes[0]
			if route.Method != "POST" || route.Path != "/todos/:id" || route.ModulePath != "routes/todos/[id]" || route.Handler != "post" {
				t.Fatalf("unexpected route: %#v", route)
			}
			if len(route.PathParameters) != 1 || route.PathParameters[0] != "id" {
				t.Fatalf("unexpected path parameters: %#v", route.PathParameters)
			}
			for _, artifact := range artifacts {
				if artifact.IR.ModulePath != "main" && len(artifact.IR.WebRoutes) != 0 {
					t.Fatalf("route manifest was attached to %s", artifact.IR.ModulePath)
				}
			}
		})
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
	if main == nil || len(main.IR.WebRoutes) != 0 {
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
