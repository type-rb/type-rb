package packageextensionhost

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/token"
)

func TestProjectSourceSpanBoundaryRoundTrips(t *testing.T) {
	span := token.Span{
		Start: token.Position{Offset: 4, Line: 2, Column: 3},
		End:   token.Position{Offset: 11, Line: 2, Column: 10},
	}
	if roundTrip := ImportSourceSpan(ExportSourceSpan(span)); roundTrip != span {
		t.Fatalf("source span round trip=%#v, want %#v", roundTrip, span)
	}
}

func TestExportProjectDeclarationInputCopiesDeclarationFacts(t *testing.T) {
	ids := parseProjectInputTest(t, "contracts/ids", `newtype ReceiptID = Integer

enum DeliveryState
	Pending = "pending"
end
`)
	job := parseProjectInputTest(t, "jobs/send_receipt", `import { Job, JobResult } from trb/jobs
import { DeliveryState, ReceiptID } from app/contracts/ids

alias DeliveryResult = JobResult

class SendReceiptJob < Job
	queue("mail")
	priority(PRIORITY)
	configure(:mail, DeliveryState) do |scope|
		scope
	end

	def helper(value: ReceiptID?): ReceiptID?
		return value
	end

	def perform(receipt_id: ReceiptID): DeliveryResult
		return DeliveryResult::Ok(Unit.new())
	end
end
`)
	input, err := ExportProjectDeclarationInput("trb/jobs", []*ast.Program{job, ids}, ProjectDeclarationInputOptions{
		PackageAliasesByModule: map[string]map[string]string{"jobs/send_receipt": {"app/contracts": "contracts"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Modules) != 2 || input.Modules[0].ModulePath != "contracts/ids" || input.Modules[1].ModulePath != "jobs/send_receipt" {
		t.Fatalf("modules are not deterministic: %#v", input.Modules)
	}
	module := input.Modules[1]
	if len(input.Modules[0].Newtypes) != 1 || input.Modules[0].Newtypes[0].Name != "ReceiptID" || input.Modules[0].Newtypes[0].Target.Resolved.Kind != "int" {
		t.Fatalf("newtype declaration facts are incomplete: %#v", input.Modules[0].Newtypes)
	}
	if len(input.Modules[0].Enums) != 1 || input.Modules[0].Enums[0].Members[0].RawValue == nil || input.Modules[0].Enums[0].Members[0].RawValue.Raw != `"pending"` {
		t.Fatalf("enum declaration facts are incomplete: %#v", input.Modules[0].Enums)
	}
	if len(module.Imports) != 2 || module.Imports[1].ModulePath != "contracts/ids" || len(module.TypeAliases) != 1 || len(module.Classes) != 1 {
		t.Fatalf("declaration facts are incomplete: %#v", module)
	}
	class := module.Classes[0]
	if class.Superclass == nil || class.Superclass.Authored.Name != "Job" || class.Superclass.Authored.Definition == nil || class.Superclass.Authored.Definition.ImportPath != "trb/jobs" {
		t.Fatalf("superclass identity is missing: %#v", class.Superclass)
	}
	if len(class.Methods) != 2 || len(class.Directives) != 3 {
		t.Fatalf("class signature facts are incomplete: %#v", class)
	}
	helperParameter := class.Methods[0].Parameters[0].Type
	if helperParameter.Representation == nil || helperParameter.Representation.Kind != "int" || !helperParameter.Representation.Nullable {
		t.Fatalf("nullable newtype representation is missing: %#v", helperParameter)
	}
	if class.Directives[0].Arguments[0].Value.Kind != "string" || class.Directives[0].Arguments[0].Value.Raw != `"mail"` {
		t.Fatalf("literal directive was not copied: %#v", class.Directives[0])
	}
	if class.Directives[1].Arguments[0].Value.Kind != "reference" {
		t.Fatalf("reference directive argument was not copied: %#v", class.Directives[1])
	}
	configured := class.Directives[2]
	if configured.Arguments[0].Value.Kind != "symbol" || configured.Arguments[0].Value.Name != "mail" || configured.Arguments[1].Value.Reference == nil || configured.Arguments[1].Value.Reference.ModulePath != "contracts/ids" {
		t.Fatalf("declarative directive values are incomplete: %#v", configured)
	}
	if configured.Block == nil || len(configured.Block.Parameters) != 1 || configured.Block.StatementCount != 1 || !configured.Block.ResultExpression {
		t.Fatalf("directive block summary is incomplete: %#v", configured.Block)
	}
	perform := class.Methods[1]
	parameter := perform.Parameters[0].Type
	if parameter.Authored.Name != "ReceiptID" || parameter.Authored.Definition == nil || parameter.Authored.Definition.ModulePath != "contracts/ids" || parameter.Resolved.Name != "ReceiptID" || parameter.Representation == nil || parameter.Representation.Kind != "int" {
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
