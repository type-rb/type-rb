package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestRunOfficialWebTimeoutAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireWebServerRuntime(t, mode)
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/web-timeout-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}

			mainSource := `import { Headers, HttpMethod, Body } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def request(path: String): Request
	return Request.new(
		method: HttpMethod.get(),
		path: path,
		query_string: "",
		headers: Headers.new([]),
		body: Body.empty(),
	)
end

def main()
	fast := dispatch(request("/fast"))
	puts("fast-status=" + fast.status.to_s())
	slow := dispatch(request("/slow"))
	puts("slow-status=" + slow.status.to_s())
	puts("slow-body=" + slow.body.to_s())
	return
end
`
			middlewareSource := `import { Context, Next, Response } from trb/web
import trb/web/middleware/timeout
import { TimeoutOptions } from trb/web/middleware/timeout

OPTIONS := TimeoutOptions.new(milliseconds: 5)

def call(context: Context, next_handler: Next): Response
	return Timeout.call(context, next_handler, OPTIONS)
end
`
			routes := map[string]string{
				"fast.trb": `import { Context, Response } from trb/web

def get(_context: Context): Response
	return Response.text("ok")
end
`,
				"slow.trb": `import { Context, Response } from trb/web

def get(_context: Context): Response
	mut index := 0
	while index < 1000000000
		index = index + 1
	end
	return Response.text("late")
end
`,
			}
			if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(middlewareSource), 0o644); err != nil {
				t.Fatal(err)
			}
			for name, source := range routes {
				if err := os.WriteFile(filepath.Join(root, "src", "routes", name), []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
			}
			output := stdout.String()
			for _, expected := range []string{"fast-status=200", "slow-status=504", `slow-body={"error":"gateway_timeout"}`} {
				if !strings.Contains(output, expected) {
					t.Fatalf("%s timeout output is missing %q: %s", mode, expected, output)
				}
			}
		})
	}
}
