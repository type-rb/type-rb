package jobs

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/parser"
)

func TestDeclarationsMirrorPerformParameters(t *testing.T) {
	program := parseJobsTest(t, `class SendReceiptJob < Job
	def perform(order_id: Integer, destination: String)
		return
	end
end
`)
	catalog, err := Declarations([]*ast.Program{program})
	if err != nil {
		t.Fatal(err)
	}
	job, ok := catalog.Type("SendReceiptJob")
	if !ok {
		t.Fatal("SendReceiptJob declaration is missing")
	}
	performLater := job.ClassMembers["perform_later"]
	if performLater.Intrinsic != "trb.jobs.perform_later" || performLater.Fails.String() != "EnqueueError" || performLater.Return.String() != "JobReference" {
		t.Fatalf("unexpected perform_later declaration: %#v", performLater)
	}
	if len(performLater.Parameters) != 2 || performLater.Parameters[0].Name != "order_id" || performLater.Parameters[0].Type.String() != "Integer" || performLater.Parameters[1].Type.String() != "String" {
		t.Fatalf("unexpected perform_later parameters: %#v", performLater.Parameters)
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

	if !imported.RuntimeRequired || !contains(imported.Symbols, "EnqueueError") || !contains(imported.Symbols, "JobReference") {
		t.Fatalf("job runtime types were not attached: %#v", imported)
	}
	method, ok := class.Body[len(class.Body)-1].(*ir.Method)
	if !ok || method.Name != "perform_later" || !method.External || !method.Class || method.Fails.String() != "EnqueueError" {
		t.Fatalf("job class was not augmented: %#v", class.Body)
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
		{name: "return value", source: "class InvalidJob < Job\n\tdef perform(): String\n\t\treturn \"x\"\n\tend\nend\n", message: "must not return a value"},
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

func TestParseConfigDefaultsToSingleWorkerSQLite(t *testing.T) {
	config, err := ParseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if config.Adapter != "sql" || config.DatabaseAdapter != "sqlite" || config.Database != "jobs.sqlite3" || config.WorkerConcurrency != 1 {
		t.Fatalf("unexpected default config: %#v", config)
	}
}

func TestParseConfigRejectsMultipleSQLiteWorkers(t *testing.T) {
	_, err := ParseConfig([]byte(`{"worker_concurrency":2}`))
	if err == nil || !strings.Contains(err.Error(), "worker_concurrency: 1") {
		t.Fatalf("expected SQLite concurrency diagnostic, got %v", err)
	}
}

func TestParseConfigAcceptsMultiWorkerServerDatabases(t *testing.T) {
	for _, adapter := range []string{"postgresql", "mysql"} {
		config, err := ParseConfig([]byte(`{"database_adapter":"` + adapter + `","database":"jobs","worker_concurrency":4}`))
		if err != nil {
			t.Fatalf("%s: %v", adapter, err)
		}
		if config.WorkerConcurrency != 4 {
			t.Fatalf("%s concurrency: %d", adapter, config.WorkerConcurrency)
		}
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
