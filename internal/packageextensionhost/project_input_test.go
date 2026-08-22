package packageextensionhost

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/parser"
)

func TestExportProjectDeclarationInputCopiesDeclarationFacts(t *testing.T) {
	ids := parseProjectInputTest(t, "contracts/ids", `type ReceiptID = Integer
`)
	job := parseProjectInputTest(t, "jobs/send_receipt", `import { Job, JobResult } from trb/jobs
import { ReceiptID } from contracts/ids

type DeliveryResult = JobResult

class SendReceiptJob < Job
	queue("mail")
	priority(PRIORITY)

	def helper(value: String): String
		return value
	end

	def perform(receipt_id: ReceiptID): DeliveryResult
		return DeliveryResult::Ok(Unit.new())
	end
end
`)
	input, err := ExportProjectDeclarationInput("trb/jobs", []*ast.Program{job, ids})
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Modules) != 2 || input.Modules[0].ModulePath != "contracts/ids" || input.Modules[1].ModulePath != "jobs/send_receipt" {
		t.Fatalf("modules are not deterministic: %#v", input.Modules)
	}
	module := input.Modules[1]
	if len(module.Imports) != 2 || len(module.TypeAliases) != 1 || len(module.Classes) != 1 {
		t.Fatalf("declaration facts are incomplete: %#v", module)
	}
	class := module.Classes[0]
	if class.Superclass == nil || class.Superclass.Authored.Name != "Job" || class.Superclass.Authored.Definition == nil || class.Superclass.Authored.Definition.ImportPath != "trb/jobs" {
		t.Fatalf("superclass identity is missing: %#v", class.Superclass)
	}
	if len(class.Methods) != 2 || len(class.Directives) != 2 {
		t.Fatalf("class signature facts are incomplete: %#v", class)
	}
	if class.Directives[0].Arguments[0].Literal.Kind != "string" || class.Directives[0].Arguments[0].Literal.Raw != `"mail"` {
		t.Fatalf("literal directive was not copied: %#v", class.Directives[0])
	}
	if class.Directives[1].Arguments[0].Literal.Kind != "unsupported" {
		t.Fatalf("non-literal directive was not isolated: %#v", class.Directives[1])
	}
	perform := class.Methods[1]
	parameter := perform.Parameters[0].Type
	if parameter.Authored.Name != "ReceiptID" || parameter.Authored.Definition == nil || parameter.Authored.Definition.ModulePath != "contracts/ids" || parameter.Resolved.Kind != "int" {
		t.Fatalf("parameter authored and resolved types differ from the source contract: %#v", parameter)
	}
	if perform.Return == nil || perform.Return.Authored.Name != "DeliveryResult" || !containsProjectTypeReference(perform.Return.ResolutionPath, "JobResult", "trb/jobs") {
		t.Fatalf("canonical alias resolution path is missing: %#v", perform.Return)
	}
	if class.Span.Start.Line == 0 || perform.Span.Start.Line == 0 || parameter.Span.Start.Line == 0 {
		t.Fatalf("source spans are missing: class=%#v method=%#v parameter=%#v", class.Span, perform.Span, parameter.Span)
	}

	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var decoded packageextension.ProjectDeclarationInput
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := packageextension.ValidateProjectDeclarationInput(decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, input) {
		t.Fatalf("project declaration input changed across its JSON boundary:\ninput: %#v\ndecoded: %#v", input, decoded)
	}
}

func containsProjectTypeReference(references []packageextension.ProjectTypeReference, name, importPath string) bool {
	for _, reference := range references {
		if reference.Name == name && reference.ImportPath == importPath {
			return true
		}
	}
	return false
}

func parseProjectInputTest(t *testing.T, modulePath, source string) *ast.Program {
	t.Helper()
	program, diagnostics := parser.Parse([]byte(source))
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %v", diagnostics)
	}
	program.ModulePath = modulePath
	return program
}
