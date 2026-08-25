package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
	webintegration "github.com/type-rb/type-rb/internal/web"
)

func TestWebOpenAPIGeneratesTheSameDocumentAcrossModes(t *testing.T) {
	var baseline webintegration.OpenAPIDocument
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, mode)
			config.Name = "reports-api"
			config.Version = "2.4.0"
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/openapi-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			writeOpenAPIProject(t, root)

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"web", "openapi", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			var document webintegration.OpenAPIDocument
			if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
				t.Fatalf("invalid OpenAPI JSON: %v\n%s", err, stdout.String())
			}
			if document.Info.Title != "reports-api" || document.Info.Version != "2.4.0" || document.Paths["/reports/{id}"]["post"].OperationID != "CreateReportEndpoint" {
				t.Fatalf("unexpected %s OpenAPI document: %#v", mode, document)
			}
			if mode == "go" {
				baseline = document
			} else if !reflect.DeepEqual(document, baseline) {
				t.Fatalf("%s OpenAPI differs from Go:\n%#v\n%#v", mode, baseline, document)
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected stderr: %s", stderr.String())
			}
		})
	}
}

func TestWebOpenAPIWritesBelowTheProjectRoot(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/openapi-output-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	writeOpenAPIProject(t, root)

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"web", "openapi", "--config", config.Path, "--output", "api/openapi.json", "--title", "Public API", "--api-version", "2026-08"}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	path := filepath.Join(root, "api", "openapi.json")
	if stdout.String() != path+"\n" {
		t.Fatalf("stdout=%q, want output path", stdout.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document webintegration.OpenAPIDocument
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.Info.Title != "Public API" || document.Info.Version != "2026-08" {
		t.Fatalf("metadata overrides were not applied: %#v", document.Info)
	}
	if status := command.Run([]string{"web", "openapi", "--config", config.Path, "--output", "../outside.json"}); status == 0 || !strings.Contains(stderr.String(), "path cannot escape") {
		t.Fatalf("escaping output path status=%d stderr=%s", status, stderr.String())
	}
}

func writeOpenAPIProject(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"src/main.trb": `import { serve } from trb/web

def main()
	serve()
	return
end
`,
		"src/contracts/reports.trb": `newtype ReportID = Integer

record ReportParams
	id: ReportID
end

record CreateReportBody
	title: String @json("report_title")
end

record CreateReportInput
	params: ReportParams
	body: CreateReportBody
end

record CreateReportResponse
	id: ReportID
end

record ErrorResponse
	message: String
end
`,
		"src/routes/reports/[id].trb": `import { Context, Endpoint, Response, handles, input, json, response } from trb/web
import { CreateReportInput, CreateReportResponse, ErrorResponse, ReportID } from contracts/reports

def post(_context: Context): Response
	return json(CreateReportResponse.new(id: ReportID.new(42)), 202)
end

class CreateReportEndpoint < Endpoint
	handles(post)
	input<CreateReportInput>()
	response<CreateReportResponse>(status: 202)
	response<ErrorResponse>(status: 400)
end
`,
	}
	for relative, source := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
