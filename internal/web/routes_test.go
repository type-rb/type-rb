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
		parsedSource(t, filepath.Join(root, "routes", "assets", "[...path].trb"), "routes/assets/[...path]", "def get(context: Context): Response\n\treturn response\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "todos", "[id].trb"), "routes/todos/[id]", "def post(context: Context): Response\n\treturn response\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "index.trb"), "routes/index", "def get(context: Context): Response\n\treturn response\nend\n"),
		parsedSource(t, filepath.Join(root, "main.trb"), "main", "def main()\n\treturn\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "_middleware.trb"), "routes/_middleware", "def call(context: Context): Response\n\treturn response\nend\n"),
	}

	routes, issues := Discover(sources, root)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %#v", issues)
	}
	if len(routes) != 3 {
		t.Fatalf("got %d routes, want 3", len(routes))
	}
	if routes[0].Method != "GET" || routes[0].Path != "/" || routes[0].ModulePath != "routes/index" {
		t.Fatalf("unexpected root route: %#v", routes[0])
	}
	if routes[1].Method != "GET" || routes[1].Path != "/assets/*path" || routes[1].ModulePath != "routes/assets/[...path]" {
		t.Fatalf("unexpected catch-all route: %#v", routes[1])
	}
	if got := strings.Join(routes[1].PathParameters, ","); got != "path" {
		t.Fatalf("catch-all path parameters = %q, want path", got)
	}
	if routes[2].Method != "POST" || routes[2].Path != "/todos/:id" || routes[2].ModulePath != "routes/todos/[id]" {
		t.Fatalf("unexpected parameter route: %#v", routes[2])
	}
	if got := strings.Join(routes[2].PathParameters, ","); got != "id" {
		t.Fatalf("path parameters = %q, want id", got)
	}
}

func TestDiscoverIgnoresColocatedTestFiles(t *testing.T) {
	root := t.TempDir()
	sources := []Source{
		parsedSource(t, filepath.Join(root, "routes", "health.trb"), "routes/health", "def get(context: Context): Response\n\treturn response\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "health_test.trb"), "routes/health_test", "def helper()\n\treturn\nend\n"),
	}

	routes, issues := Discover(sources, root)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %#v", issues)
	}
	if len(routes) != 1 || routes[0].ModulePath != "routes/health" {
		t.Fatalf("unexpected routes: %#v", routes)
	}
}

func TestUniquePathRoutesKeepsOneRepresentativeInManifestOrder(t *testing.T) {
	routes := []Route{
		{Method: "GET", Path: "/todos", ModulePath: "routes/todos"},
		{Method: "POST", Path: "/todos", ModulePath: "routes/todos"},
		{Method: "GET", Path: "/users/:id", ModulePath: "routes/users/[id]"},
	}

	unique := UniquePathRoutes(routes)
	if len(unique) != 2 || unique[0].Path != "/todos" || unique[1].Path != "/users/:id" {
		t.Fatalf("unexpected unique routes: %#v", unique)
	}
}

func TestDiscoverOrdersStaticRoutesBeforeParameterRoutes(t *testing.T) {
	root := t.TempDir()
	sources := []Source{
		parsedSource(t, filepath.Join(root, "routes", "todos", "[id].trb"), "routes/todos/[id]", "def get()\n\treturn\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "todos", "new.trb"), "routes/todos/new", "def get()\n\treturn\nend\n"),
	}

	routes, issues := Discover(sources, root)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %#v", issues)
	}
	if len(routes) != 2 || routes[0].Path != "/todos/new" || routes[1].Path != "/todos/:id" {
		t.Fatalf("unexpected route precedence: %#v", routes)
	}
}

func TestDiscoverRejectsIrreduciblyAmbiguousParameterRoutes(t *testing.T) {
	root := t.TempDir()
	sources := []Source{
		parsedSource(t, filepath.Join(root, "routes", "todos", "[id].trb"), "routes/todos/[id]", "def get()\n\treturn\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "todos", "[slug].trb"), "routes/todos/[slug]", "def get()\n\treturn\nend\n"),
	}

	_, issues := Discover(sources, root)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "conflicts with route") {
		t.Fatalf("unexpected issues: %#v", issues)
	}
}

func TestDiscoverRejectsPatternsWithReversedSpecificity(t *testing.T) {
	root := t.TempDir()
	sources := []Source{
		parsedSource(t, filepath.Join(root, "routes", "todos", "new", "[id].trb"), "routes/todos/new/[id]", "def get()\n\treturn\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "todos", "[id]", "edit.trb"), "routes/todos/[id]/edit", "def get()\n\treturn\nend\n"),
	}

	_, issues := Discover(sources, root)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "conflicts with route") {
		t.Fatalf("unexpected issues: %#v", issues)
	}
}

func TestDiscoverRejectsRoutesOverlappingCatchAll(t *testing.T) {
	root := t.TempDir()
	sources := []Source{
		parsedSource(t, filepath.Join(root, "routes", "assets", "[...path].trb"), "routes/assets/[...path]", "def get()\n\treturn\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "assets", "icons", "[name].trb"), "routes/assets/icons/[name]", "def get()\n\treturn\nend\n"),
	}

	_, issues := Discover(sources, root)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "conflicts with route") {
		t.Fatalf("unexpected issues: %#v", issues)
	}
}

func TestPathsOverlapUsesOneOrMoreSegmentsForParametersAndCatchAlls(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  bool
	}{
		{left: "/", right: "/:id", want: false},
		{left: "/", right: "/*path", want: false},
		{left: "/*path", right: "/readme", want: true},
		{left: "/files", right: "/files/*path", want: false},
		{left: "/files/:id", right: "/files/*path", want: true},
		{left: "/files/*left", right: "/files/*right", want: true},
		{left: "/files/*path", right: "/images/icon", want: false},
	}
	for _, test := range tests {
		if got := pathsOverlap(test.left, test.right); got != test.want {
			t.Errorf("pathsOverlap(%q, %q) = %t, want %t", test.left, test.right, got, test.want)
		}
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
		{name: "empty catch all", filename: "[...].trb", source: "def get()\n\treturn\nend\n", message: "invalid catch-all route parameter"},
		{name: "non-terminal catch all", filename: filepath.Join("[...path]", "edit.trb"), source: "def get()\n\treturn\nend\n", message: "must be the final segment"},
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
		parsedSource(t, filepath.Join(root, "routes", "todos", "_middleware.trb"), "routes/todos/_middleware", "def call(context: Context, next_handler: Next): Response\n\treturn next_handler.call(context)\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "_middleware.trb"), "routes/_middleware", "def call(context: Context, next_handler: Next): Response\n\treturn next_handler.call(context)\nend\n"),
		parsedSource(t, filepath.Join(root, "routes", "admin", "_middleware.trb"), "routes/admin/_middleware", "def call(context: Context, next_handler: Next): Response\n\treturn next_handler.call(context)\nend\n"),
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

func TestRootMiddlewaresSelectOnlyApplicationScope(t *testing.T) {
	middlewares := []Middleware{
		{Directory: "", ModulePath: "routes/_middleware"},
		{Directory: "admin", ModulePath: "routes/admin/_middleware"},
		{Directory: "", ModulePath: "routes/other/_middleware"},
	}

	root := RootMiddlewares(middlewares)
	if len(root) != 2 || root[0].ModulePath != "routes/_middleware" || root[1].ModulePath != "routes/other/_middleware" {
		t.Fatalf("unexpected root middleware selection: %#v", root)
	}
	nested := NestedMiddlewares(middlewares)
	if len(nested) != 1 || nested[0].ModulePath != "routes/admin/_middleware" {
		t.Fatalf("unexpected nested middleware selection: %#v", nested)
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
