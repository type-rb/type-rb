package cli

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestRunOfficialWebResponseCompressionAcrossAvailableBackends(t *testing.T) {
	largeBody := strings.Repeat("TypeRB portable response compression. ", 80)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireWebServerRuntime(t, mode)
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/web-compression-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}

			mainSource := `import { Header, Headers, HttpMethod, Body } from trb/http
import { Request, Response } from trb/web
import { dispatch } from trb/web/testing
import { encode } from trb/std/encoding/base64

def request(path: String, accept_encoding: String): Request
	return Request.new(
		method: HttpMethod.get(),
		path: path,
		query_string: "",
		headers: Headers.new([Header.new(name: "accept-encoding", value: accept_encoding)]),
		body: Body.empty(),
	)
end

def header(response: Response, name: String): String
	return response.header_values(name).join(",")
end

def main()
	compressed := dispatch(request("/large", "br, gzip; q=1"))
	puts("gzip-status=" + compressed.status.to_s())
	puts("gzip-encoding=" + header(compressed, "content-encoding"))
	puts("gzip-vary=" + header(compressed, "vary"))
	puts("gzip-length=" + header(compressed, "content-length"))
	puts("gzip-etag=" + header(compressed, "etag"))
	puts("gzip-body=" + encode(compressed.body.bytes()))

	rejected := dispatch(request("/large", "gzip; q=0, identity"))
	puts("rejected-encoding=" + header(rejected, "content-encoding"))
	puts("rejected-vary=" + header(rejected, "vary"))
	puts("rejected-etag=" + header(rejected, "etag"))
	puts("rejected-body=" + rejected.body.to_s())

	small := dispatch(request("/small", "gzip"))
	puts("small-encoding=" + header(small, "content-encoding"))
	puts("small-vary=" + header(small, "vary"))
	puts("small-body=" + small.body.to_s())

	no_transform := dispatch(request("/no_transform", "*"))
	puts("no-transform-encoding=" + header(no_transform, "content-encoding"))
	puts("no-transform-vary=" + header(no_transform, "vary"))
	puts("no-transform-body=" + no_transform.body.to_s())
	return
end
`
			middlewareSource := `import { Context, Next, Response } from trb/web
import trb/web/middleware/compression
import { CompressionOptions } from trb/web/middleware/compression

OPTIONS := CompressionOptions.new(minimum_size_bytes: 10)

def call(context: Context, next_handler: Next): Response
	return compression.call(context, next_handler, OPTIONS)
end
`
			routes := map[string]string{
				"large.trb": fmt.Sprintf(`import { Context, Response, text } from trb/web

def get(_context: Context): Response
	return text(%q).with_header("content-length", %q).with_header("etag", %q)
end
`, largeBody, fmt.Sprintf("%d", len(largeBody)), `"strong"`),
				"small.trb": `import { Context, Response, text } from trb/web

def get(_context: Context): Response
	return text("small")
end
`,
				"no_transform.trb": fmt.Sprintf(`import { Context, Response, text } from trb/web

def get(_context: Context): Response
	return text(%q).with_header("cache-control", "private, no-transform")
end
`, largeBody),
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
			values := parseCompressionOutput(t, mode, stdout.String())
			if values["gzip-status"] != "200" || values["gzip-encoding"] != "gzip" || values["gzip-vary"] != "Accept-Encoding" {
				t.Fatalf("unexpected %s compressed metadata: %#v", mode, values)
			}
			if values["gzip-length"] != "" || values["gzip-etag"] != "" {
				t.Fatalf("%s retained representation metadata after compression: %#v", mode, values)
			}
			compressed, err := base64.StdEncoding.DecodeString(values["gzip-body"])
			if err != nil {
				t.Fatalf("%s emitted invalid compressed Base64: %v", mode, err)
			}
			reader, err := gzip.NewReader(bytes.NewReader(compressed))
			if err != nil {
				t.Fatalf("%s emitted an invalid gzip body: %v", mode, err)
			}
			decoded, readErr := io.ReadAll(reader)
			closeErr := reader.Close()
			if readErr != nil || closeErr != nil || string(decoded) != largeBody {
				t.Fatalf("unexpected %s decompressed body: read=%v close=%v body=%q", mode, readErr, closeErr, decoded)
			}
			if values["rejected-encoding"] != "" || values["rejected-vary"] != "Accept-Encoding" || values["rejected-etag"] != `"strong"` || values["rejected-body"] != largeBody {
				t.Fatalf("unexpected %s q=0 response: %#v", mode, values)
			}
			if values["small-encoding"] != "" || values["small-vary"] != "" || values["small-body"] != "small" {
				t.Fatalf("unexpected %s small response: %#v", mode, values)
			}
			if values["no-transform-encoding"] != "" || values["no-transform-vary"] != "" || values["no-transform-body"] != largeBody {
				t.Fatalf("unexpected %s no-transform response: %#v", mode, values)
			}
		})
	}
}

func parseCompressionOutput(t *testing.T, mode, output string) map[string]string {
	t.Helper()
	values := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			t.Fatalf("unexpected %s compression output line %q in %q", mode, line, output)
		}
		values[parts[0]] = parts[1]
	}
	return values
}
