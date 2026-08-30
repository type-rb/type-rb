package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestRunOfficialWebPrefersStaticRoutesAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireWebServerRuntime(t, mode)
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/web-route-precedence-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}

			mainSource := `import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def show(method: String, path: String)
	response := dispatch(Request.new(method: HttpMethod.new(method), path: path, query_string: "", headers: Headers.new(), body: Body.empty()))
	puts(response.status)
	puts(response.body.to_s())
	allow := response.header_values("allow")
	if allow.empty?()
		puts("-")
	else
		puts(allow[0])
	end
	return
end

def main()
	show("GET", "/digital-gift-runs/candidates")
	show("GET", "/digital-gift-runs/42")
	show("GET", "/digital-gift-runs/reserved")
	show("POST", "/digital-gift-runs/reserved")
	show("OPTIONS", "/digital-gift-runs/candidates")
	return
end
`
			dynamicRouteSource := `import { Context, Response } from trb/web

def get(context: Context): Response
	return Response.text("dynamic:" + context.path_value("id"))
end
`
			candidatesRouteSource := `import { Context, Response } from trb/web

def get(_context: Context): Response
	return Response.text("static:candidates")
end
`
			reservedRouteSource := `import { Context, Response } from trb/web

def post(_context: Context): Response
	return Response.text("static:reserved")
end
`
			routeRoot := filepath.Join(root, "src", "routes", "digital-gift-runs")
			if err := os.MkdirAll(filepath.Join(routeRoot, "candidates"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(routeRoot, "reserved"), 0o755); err != nil {
				t.Fatal(err)
			}
			files := map[string]string{
				filepath.Join(root, "src", "main.trb"):              mainSource,
				filepath.Join(routeRoot, "[id].trb"):                dynamicRouteSource,
				filepath.Join(routeRoot, "candidates", "index.trb"): candidatesRouteSource,
				filepath.Join(routeRoot, "reserved", "index.trb"):   reservedRouteSource,
			}
			for filename, source := range files {
				if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
			}
			want := "200\nstatic:candidates\n-\n200\ndynamic:42\n-\n405\n{\"error\":\"method_not_allowed\"}\nOPTIONS, POST\n200\nstatic:reserved\n-\n204\n\nGET, HEAD, OPTIONS\n"
			if stdout.String() != want {
				t.Fatalf("unexpected %s route precedence output: want %q, got %q", mode, want, stdout.String())
			}
		})
	}
}
