package jobs

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
	"github.com/type-rb/type-rb/internal/parser"
)

func TestGenerateProjectReturnsDeterministicPortableDispatch(t *testing.T) {
	contracts := parseProjectGenerationProgram(t, "contracts/index", "type OrderId = Integer\n")
	job := parseProjectGenerationProgram(t, "jobs/send_receipt_job", `import { OrderId } from contracts
import { Job, JobError, JobResult } from trb/jobs
import { Unit } from trb/std/unit

class SendReceiptJob < Job
	def perform(order_id: OrderId, ratio: Float): JobResult
		return JobResult::Ok(Unit.new())
	end
end
`)
	main := parseProjectGenerationProgram(t, "main", "def main()\n\treturn\nend\n")
	input, err := packageextensionhost.ExportProjectDeclarationInput(PackageName, []*ast.Program{main, job, contracts}, packageextensionhost.ProjectDeclarationInputOptions{})
	if err != nil {
		t.Fatal(err)
	}
	origin := packageextension.SourceSpan{Start: packageextension.SourcePosition{Offset: 0, Line: 1, Column: 1}, End: packageextension.SourcePosition{Offset: 23, Line: 3, Column: 4}}
	first, err := GenerateProject(input, "main", origin)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateProject(input, "main", origin)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("project generation is not deterministic:\n%s\n%s", firstJSON, secondJSON)
	}
	if len(first.Sources) != 1 {
		t.Fatalf("unexpected generated sources: %#v", first.Sources)
	}
	source := first.Sources[0]
	for _, expected := range []string{
		"def __trb_jobs_dispatch(job_name: String, payload: String, payload_version: Integer): JobResult",
		"argument0 := as_integer(payload_values[0]) catch |error|",
		"argument1 := __trb_jobs_as_float(payload_values[1]) catch |error|",
		"return SendReceiptJob.new().perform(argument0, argument1)",
	} {
		if !strings.Contains(source.Source, expected) {
			t.Fatalf("generated dispatch is missing %q:\n%s", expected, source.Source)
		}
	}
	if !hasProjectGenerationImport(source.RequiredImports, "jobs/send_receipt_job", "SendReceiptJob") {
		t.Fatalf("generated dispatch did not request the Job import: %#v", source.RequiredImports)
	}
}

func TestGenerateProjectSkipsDispatchWithoutJobsOrEntrypoint(t *testing.T) {
	input, err := packageextensionhost.ExportProjectDeclarationInput(PackageName, []*ast.Program{
		parseProjectGenerationProgram(t, "main", "def main()\n\treturn\nend\n"),
	}, packageextensionhost.ProjectDeclarationInputOptions{})
	if err != nil {
		t.Fatal(err)
	}
	response, err := GenerateProject(input, "main", packageextension.SourceSpan{})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Sources) != 0 {
		t.Fatalf("unexpected generated source: %#v", response.Sources)
	}
}

func parseProjectGenerationProgram(t *testing.T, modulePath, source string) *ast.Program {
	t.Helper()
	program, diagnostics := parser.Parse([]byte(source))
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %v", diagnostics)
	}
	program.ModulePath = modulePath
	return program
}

func hasProjectGenerationImport(imports []packageextension.RequiredImport, path, symbol string) bool {
	for _, imported := range imports {
		if imported.Path != path {
			continue
		}
		for _, candidate := range imported.Symbols {
			if candidate == symbol {
				return true
			}
		}
	}
	return false
}
