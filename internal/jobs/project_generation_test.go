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

func TestGenerateProjectReturnsDeterministicPortableSources(t *testing.T) {
	contracts := parseProjectGenerationProgram(t, "contracts/index", "alias OrderId = Integer\n")
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
	first, err := GenerateProject(input, "main", "config/jobs", origin)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateProject(input, "main", "config/jobs", origin)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("project generation is not deterministic:\n%s\n%s", firstJSON, secondJSON)
	}
	if len(first.Sources) != 2 {
		t.Fatalf("unexpected generated sources: %#v", first.Sources)
	}
	source := projectGenerationSource(t, first, projectDispatchSourceID)
	for _, expected := range []string{
		"def __trb_jobs_dispatch(job_name: String, payload: String, payload_version: Integer): JobResult",
		"argument0 := JSON.as_integer(payload_values[0]) catch |error|",
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
	enqueue := projectGenerationSource(t, first, projectEnqueueSourceID+"jobs/send_receipt_job:SendReceiptJob")
	for _, expected := range []string{
		"def __trb_jobs_SendReceiptJob_request(order_id: OrderId, ratio: Float): Result<EnqueueRequest, EnqueueError>",
		"payload_values: Array<JSON::Value> := [JSON::Value::Integer(order_id), JSON::Value::Float(ratio)]",
		"JSON.stringify(JSON::Value::Array(payload_values))",
		"maximum_attempts: nil",
		"return JOBS_ADAPTER.enqueue(request)",
		"if delay.before?(Duration.seconds(0))",
		"return JOBS_ADAPTER.enqueue_at(request, Instant.now().add(delay))",
		"return JOBS_ADAPTER.enqueue_at(request, scheduled_at)",
	} {
		if !strings.Contains(enqueue.Source, expected) {
			t.Fatalf("generated enqueue source is missing %q:\n%s", expected, enqueue.Source)
		}
	}
	if !hasProjectGenerationImport(enqueue.RequiredImports, "config/jobs", "JOBS_ADAPTER") {
		t.Fatalf("generated enqueue source did not request the configuration import: %#v", enqueue.RequiredImports)
	}
	if !hasProjectGenerationImport(enqueue.RequiredImports, "contracts/index", "OrderId") {
		t.Fatalf("generated enqueue source did not request the imported alias type: %#v", enqueue.RequiredImports)
	}
}

func TestGenerateProjectKeepsEachJobEnqueueFragmentAtItsAuthoredOrigin(t *testing.T) {
	jobs := parseProjectGenerationProgram(t, "jobs/examples", `import { Job } from trb/jobs

class FirstJob < Job
	def perform(value: Integer)
		return
	end
end

class SecondJob < Job
	def perform(value: String)
		return
	end
end
`)
	input, err := packageextensionhost.ExportProjectDeclarationInput(PackageName, []*ast.Program{jobs}, packageextensionhost.ProjectDeclarationInputOptions{})
	if err != nil {
		t.Fatal(err)
	}
	response, err := GenerateProject(input, "", "config/jobs", packageextension.SourceSpan{})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Sources) != 2 {
		t.Fatalf("unexpected generated sources: %#v", response.Sources)
	}
	first := projectGenerationSource(t, response, projectEnqueueSourceID+"jobs/examples:FirstJob")
	second := projectGenerationSource(t, response, projectEnqueueSourceID+"jobs/examples:SecondJob")
	if first.Origin == second.Origin {
		t.Fatalf("Job enqueue fragments share an authored origin: %#v", first.Origin)
	}
	if !strings.Contains(first.Source, "__trb_jobs_FirstJob_perform_later") || !strings.Contains(second.Source, "__trb_jobs_SecondJob_perform_later") {
		t.Fatalf("unexpected generated enqueue fragments:\nfirst=%s\nsecond=%s", first.Source, second.Source)
	}
}

func TestGenerateProjectSkipsDispatchWithoutJobsOrEntrypoint(t *testing.T) {
	input, err := packageextensionhost.ExportProjectDeclarationInput(PackageName, []*ast.Program{
		parseProjectGenerationProgram(t, "main", "def main()\n\treturn\nend\n"),
	}, packageextensionhost.ProjectDeclarationInputOptions{})
	if err != nil {
		t.Fatal(err)
	}
	response, err := GenerateProject(input, "main", "config/jobs", packageextension.SourceSpan{})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Sources) != 0 {
		t.Fatalf("unexpected generated source: %#v", response.Sources)
	}
}

func projectGenerationSource(t *testing.T, response packageextension.ProjectGenerationResponse, id string) packageextension.ProjectGeneratedSource {
	t.Helper()
	for _, source := range response.Sources {
		if source.ID == id {
			return source
		}
	}
	t.Fatalf("generated source %s is missing: %#v", id, response.Sources)
	return packageextension.ProjectGeneratedSource{}
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
