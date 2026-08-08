package web

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/parser"
)

func TestDiscoverBuildsDeterministicFileRouteManifest(t *testing.T) {
	root := t.TempDir()
	sources := []Source{
		parsedSource(t, filepath.Join(root, "routes", "todos", "[id].trb"), "routes/todos/[id]", "def post(context: Context): Response\n\treturn response\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "index.trb"), "routes/index", "def get(context: Context): Response\n\treturn response\nend\n"),
		parsedSource(t, filepath.Join(root, "main.trb"), "main", "def main()\n\treturn\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "_middleware.trb"), "routes/_middleware", "def call(context: Context): Response\n\treturn response\nend\n"),
	}

	routes, issues := Discover(sources, root)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %#v", issues)
	}
	if len(routes) != 2 {
		t.Fatalf("got %d routes, want 2", len(routes))
	}
	if routes[0].Method != "GET" || routes[0].Path != "/" || routes[0].ModulePath != "routes/index" {
		t.Fatalf("unexpected root route: %#v", routes[0])
	}
	if routes[1].Method != "POST" || routes[1].Path != "/todos/:id" || routes[1].ModulePath != "routes/todos/[id]" {
		t.Fatalf("unexpected parameter route: %#v", routes[1])
	}
	if got := strings.Join(routes[1].PathParameters, ","); got != "id" {
		t.Fatalf("path parameters = %q, want id", got)
	}
}

func TestDiscoverRejectsAmbiguousRoutes(t *testing.T) {
	root := t.TempDir()
	sources := []Source{
		parsedSource(t, filepath.Join(root, "routes", "todos", "new.trb"), "routes/todos/new", "def get()\n\treturn\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "todos", "[id].trb"), "routes/todos/[id]", "def get()\n\treturn\nend\n"),
	}

	_, issues := Discover(sources, root)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "GET /todos/new conflicts with route /todos/:id") {
		t.Fatalf("unexpected issues: %#v", issues)
	}
}

func TestDiscoverRejectsInvalidRouteFiles(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name     string
		filename string
		source   string
		message  string
	}{
		{name: "missing handler", filename: "health.trb", source: "def helper()\n\treturn\nend\n", message: "at least one HTTP handler"},
		{name: "catch all", filename: "[...path].trb", source: "def get()\n\treturn\nend\n", message: "catch-all route segments are not supported yet"},
		{name: "duplicate parameter", filename: filepath.Join("[id]", "[id].trb"), source: "def get()\n\treturn\nend\n", message: "route parameter \"id\" is duplicated"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filename := filepath.Join(root, "routes", test.filename)
			_, issues := Discover([]Source{parsedSource(t, filename, "routes/test", test.source)}, root)
			if len(issues) != 1 || !strings.Contains(issues[0].Message, test.message) {
				t.Fatalf("unexpected issues: %#v", issues)
			}
		})
	}
}

func TestDiscoverMiddlewaresOrdersRootBeforeNestedScopes(t *testing.T) {
	root := t.TempDir()
	sources := []Source{
		parsedSource(t, filepath.Join(root, "routes", "todos", "_middleware.trb"), "routes/todos/_middleware", "def call(context: Context, next: Next): Response\n\treturn next.call(context)\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "_middleware.trb"), "routes/_middleware", "def call(context: Context, next: Next): Response\n\treturn next.call(context)\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "admin", "_middleware.trb"), "routes/admin/_middleware", "def call(context: Context, next: Next): Response\n\treturn next.call(context)\nend\n"),
	}

	middlewares := discoverMiddlewares(sources, root)
	if len(middlewares) != 3 {
		t.Fatalf("got %d middlewares, want 3", len(middlewares))
	}
	if middlewares[0].Directory != "" || middlewares[0].TargetHandler != "trb_web_middleware_0" {
		t.Fatalf("unexpected root middleware: %#v", middlewares[0])
	}
	if middlewares[1].Directory != "admin" || middlewares[2].Directory != "todos" {
		t.Fatalf("unexpected nested middleware order: %#v", middlewares)
	}
	if !appliesToRoute("", "todos/items") || !appliesToRoute("todos", "todos/items") || appliesToRoute("admin", "todos/items") {
		t.Fatal("middleware scopes do not follow the route directory hierarchy")
	}
}

func parsedSource(t *testing.T, filename, modulePath, source string) Source {
	t.Helper()
	program, diagnostics := parser.Parse([]byte(source))
	if len(diagnostics) != 0 {
		t.Fatalf("parse %s: %#v", filename, diagnostics)
	}
	return Source{Filename: filename, ModulePath: modulePath, Program: program}
}
