package jobs

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
	"github.com/type-rb/type-rb/internal/parser"
)

func TestDeclarationsMirrorPerformParameters(t *testing.T) {
	program := parseJobsTest(t, `class SendReceiptJob < Job
	def perform(order_id: Integer, destination: String)
		return
	end
end
`)
	catalog, err := Declarations(projectDeclarationInput(t, []*ast.Program{program}))
	if err != nil {
		t.Fatal(err)
	}
	job, ok := catalog.Type("SendReceiptJob")
	if !ok {
		t.Fatal("SendReceiptJob declaration is missing")
	}
	performLater := job.ClassMembers["perform_later"]
	if performLater.Intrinsic != "trb.jobs.perform_later" || performLater.Return.String() != "Result<JobReference, EnqueueError>" {
		t.Fatalf("unexpected perform_later declaration: %#v", performLater)
	}
	if len(performLater.Parameters) != 2 || performLater.Parameters[0].Name != "order_id" || performLater.Parameters[0].Type.String() != "Integer" || performLater.Parameters[1].Type.String() != "String" {
		t.Fatalf("unexpected perform_later parameters: %#v", performLater.Parameters)
	}
	performIn := job.ClassMembers["perform_in"]
	if performIn.Intrinsic != "trb.jobs.perform_in" || len(performIn.Parameters) != 3 || performIn.Parameters[0].Type.String() != "Duration" {
		t.Fatalf("unexpected perform_in declaration: %#v", performIn)
	}
	performAt := job.ClassMembers["perform_at"]
	if performAt.Intrinsic != "trb.jobs.perform_at" || len(performAt.Parameters) != 3 || performAt.Parameters[0].Type.String() != "Instant" {
		t.Fatalf("unexpected perform_at declaration: %#v", performAt)
	}
}

func TestDeclarationsAcceptCanonicalJobResultThroughTransparentAlias(t *testing.T) {
	program := parseJobsTest(t, `import { Job, JobResult } from trb/jobs

alias DeliveryResult = JobResult

class SendReceiptJob < Job
	def perform(order_id: Integer): DeliveryResult
		return DeliveryResult::Ok(Unit.new())
	end
end
`)
	if _, err := Declarations(projectDeclarationInput(t, []*ast.Program{program})); err != nil {
		t.Fatal(err)
	}
}

func TestDeclarationsPreserveJobContractDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{name: "missing perform", source: "class EmptyJob < Job\nend\n", message: "must declare perform"},
		{name: "class perform", source: "class InvalidJob < Job\n\tdef self.perform()\n\t\treturn\n\tend\nend\n", message: "must be an instance method"},
		{name: "source-defined JobResult", source: "alias JobResult = String\nclass InvalidJob < Job\n\tdef perform(): JobResult\n\t\treturn \"invalid\"\n\tend\nend\n", message: "must omit its return type or return JobResult"},
		{name: "optional argument", source: "class InvalidJob < Job\n\tdef perform(value: Integer = 1)\n\t\treturn\n\tend\nend\n", message: "required positional parameters only"},
		{name: "dynamic queue", source: "class InvalidJob < Job\n\tqueue(QUEUE)\n\tdef perform()\n\t\treturn\n\tend\nend\n", message: "expects a literal"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program := parseJobsTest(t, test.source)
			_, err := Declarations(projectDeclarationInput(t, []*ast.Program{program}))
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q, got %v", test.message, err)
			}
		})
	}
}

func TestManifestAugmentsJobRuntimeContract(t *testing.T) {
	manifest := &Manifest{Jobs: []Job{{
		Name: "SendReceiptJob", ModulePath: "jobs/send_receipt_job",
		Parameters: []Parameter{{Name: "order_id", Type: typeRef(ast.TypeRef{Name: "Integer"})}},
	}}}
	imported := &ir.Import{Path: "trb/jobs/index", Symbols: []string{"Job"}, SymbolKinds: map[string]string{"Job": "class"}}
	class := &ir.Class{Name: "SendReceiptJob"}
	program := &ir.Program{ModulePath: "jobs/send_receipt_job", Statements: []ir.Statement{imported, class}}

	manifest.Augment(program)

	if !imported.RuntimeRequired || !contains(imported.Symbols, "EnqueueError") || !contains(imported.Symbols, "EnqueueErrorKind") || !contains(imported.Symbols, "JobReference") {
		t.Fatalf("job runtime types were not attached: %#v", imported)
	}
	method, ok := class.Body[len(class.Body)-3].(*ir.Method)
	if !ok || method.Name != "perform_later" || !method.External || !method.Class || method.ReturnType.String() != "Result<JobReference, EnqueueError>" {
		t.Fatalf("job class was not augmented: %#v", class.Body)
	}
	delayed, ok := class.Body[len(class.Body)-2].(*ir.Method)
	if !ok || delayed.Name != "perform_in" || len(delayed.Parameters) != 2 || delayed.Parameters[0].Type.String() != "Duration" {
		t.Fatalf("delayed job method was not augmented: %#v", class.Body)
	}
	scheduled, ok := class.Body[len(class.Body)-1].(*ir.Method)
	if !ok || scheduled.Name != "perform_at" || len(scheduled.Parameters) != 2 || scheduled.Parameters[0].Type.String() != "Instant" {
		t.Fatalf("scheduled job method was not augmented: %#v", class.Body)
	}
}

func TestDiscoverRejectsInvalidInitialJobContracts(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{name: "missing perform", source: "class EmptyJob < Job\nend\n", message: "must declare perform"},
		{name: "class perform", source: "class InvalidJob < Job\n\tdef self.perform()\n\t\treturn\n\tend\nend\n", message: "must be an instance method"},
		{name: "return value", source: "class InvalidJob < Job\n\tdef perform(): String\n\t\treturn \"x\"\n\tend\nend\n", message: "must omit its return type or return JobResult"},
		{name: "raw result", source: "import { Job, JobError } from trb/jobs\nimport { Result } from trb/std/result\nimport { Unit } from trb/std/unit\nclass InvalidJob < Job\n\tdef perform(): Result<Unit, JobError>\n\t\treturn Result<Unit, JobError>::Ok(Unit.new())\n\tend\nend\n", message: "must omit its return type or return JobResult"},
		{name: "source-defined JobResult", source: "import { Job } from trb/jobs\nimport { Result } from trb/std/result\nimport { Unit } from trb/std/unit\nrecord LocalJobError\n\tmessage: String\nend\nalias JobResult = Result<Unit, LocalJobError>\nclass InvalidJob < Job\n\tdef perform(): JobResult\n\t\treturn JobResult::Ok(Unit.new())\n\tend\nend\n", message: "must omit its return type or return JobResult"},
		{name: "record argument", source: "record Payload\n\tid: Integer\nend\nclass InvalidJob < Job\n\tdef perform(payload: Payload)\n\t\treturn\n\tend\nend\n", message: "must initially be Boolean, Integer, Float, or String"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Discover([]*ast.Program{parseJobsTest(t, test.source)})
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q, got %v", test.message, err)
			}
		})
	}
}

func TestLegacyPerformEffectsAreRejectedByParserMigration(t *testing.T) {
	for _, source := range []string{
		"import { Job } from trb/jobs\nrecord AppError\nend\nclass InvalidJob < Job\n\tdef perform() fails AppError\n\t\treturn\n\tend\nend\n",
		"import { Job, JobResult } from trb/jobs\nrecord AppError\nend\nclass InvalidJob < Job\n\tdef perform(): JobResult fails AppError\n\t\treturn\n\tend\nend\n",
	} {
		_, diagnostics := parser.Parse([]byte(source))
		if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "fails was removed in TypeRB 0.3") {
			t.Fatalf("expected the parser migration diagnostic, got %v", diagnostics)
		}
	}
}

func TestDiscoverClassifiesPerformContracts(t *testing.T) {
	tests := []struct {
		name   string
		source string
		kind   PerformKind
	}{
		{
			name:   "infallible void",
			source: "import { Job } from trb/jobs\nclass ExampleJob < Job\n\tdef perform()\n\t\treturn\n\tend\nend\n",
			kind:   PerformVoid,
		},
		{
			name:   "canonical result",
			source: "import { Job, JobResult } from trb/jobs\nimport { Unit } from trb/std/unit\nclass ExampleJob < Job\n\tdef perform(): JobResult\n\t\treturn JobResult::Ok(Unit.new())\n\tend\nend\n",
			kind:   PerformJobResult,
		},
		{
			name:   "transparent alias of canonical result",
			source: "import { Job, JobResult } from trb/jobs\nimport { Unit } from trb/std/unit\nalias ExampleJobResult = JobResult\nclass ExampleJob < Job\n\tdef perform(): ExampleJobResult\n\t\treturn ExampleJobResult::Ok(Unit.new())\n\tend\nend\n",
			kind:   PerformJobResult,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			jobs, err := Discover([]*ast.Program{parseJobsTest(t, test.source)})
			if err != nil {
				t.Fatal(err)
			}
			if len(jobs) != 1 || jobs[0].PerformKind != test.kind {
				t.Fatalf("unexpected Job contract: %#v", jobs)
			}
		})
	}
}

func TestDiscoverCapturesJobDefaults(t *testing.T) {
	program := parseJobsTest(t, `class SendReceiptJob < Job
	queue("mail")
	priority(10)
	maximum_attempts(3)

	def perform(order_id: Integer)
		return
	end
end
`)
	jobs, err := Discover([]*ast.Program{program})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Queue != "mail" || jobs[0].Priority != 10 || jobs[0].MaximumAttempts != 3 {
		t.Fatalf("unexpected Job defaults: %#v", jobs)
	}
}

func TestDiscoverAcceptsScalarNewtypePayload(t *testing.T) {
	program := parseJobsTest(t, `newtype OrderId = Integer
class SendReceiptJob < Job
	def perform(order_id: OrderId)
		return
	end
end
`)
	jobs, err := Discover([]*ast.Program{program})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || len(jobs[0].Parameters) != 1 || jobs[0].Parameters[0].Type.String() != "OrderId" {
		t.Fatalf("unexpected newtype Job payload: %#v", jobs)
	}
}

func TestDiscoverRejectsInvalidQueueAndPriorityDefaults(t *testing.T) {
	tests := []struct {
		name    string
		setting string
		message string
	}{
		{name: "empty queue", setting: `queue("")`, message: "non-empty String"},
		{name: "dynamic queue", setting: `queue(QUEUE)`, message: "expects a literal"},
		{name: "negative priority", setting: `priority(-1)`, message: "expects a literal"},
		{name: "string priority", setting: `priority("first")`, message: "expects an Integer literal"},
		{name: "zero attempts", setting: `maximum_attempts(0)`, message: "must be a positive Integer"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program := parseJobsTest(t, "class InvalidJob < Job\n\t"+test.setting+"\n\tdef perform()\n\t\treturn\n\tend\nend\n")
			_, err := Discover([]*ast.Program{program})
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q, got %v", test.message, err)
			}
		})
	}
}

func parseJobsTest(t *testing.T, source string) *ast.Program {
	t.Helper()
	program, diagnostics := parser.Parse([]byte(source))
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %v", diagnostics)
	}
	program.ModulePath = "jobs/send_receipt_job"
	return program
}

func projectDeclarationInput(t *testing.T, programs []*ast.Program) packageextension.ProjectDeclarationInput {
	t.Helper()
	input, err := packageextensionhost.ExportProjectDeclarationInput(PackageName, programs, packageextensionhost.ProjectDeclarationInputOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return input
}
