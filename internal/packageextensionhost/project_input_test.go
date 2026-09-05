package packageextensionhost

import (
	"encoding/json"
	"reflect"
	"strings"
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

func TestClosedNewtypeDoesNotExportAnInboundRepresentation(t *testing.T) {
	program := parseProjectInputTest(t, "labels", `newtype Label = String do
	private new
end
def use_label(label: Label)
	puts(label)
end
`)
	input, err := ExportProjectDeclarationInput("example", []*ast.Program{program}, ProjectDeclarationInputOptions{})
	if err != nil {
		t.Fatal(err)
	}
	module := input.Modules[0]
	if !module.Newtypes[0].PrivateNew || module.Functions[0].Parameters[0].Type.Representation != nil {
		t.Fatalf("closed construction metadata was erased: %#v", module)
	}
}

func TestExportProjectDeclarationInputCopiesDeclarationFacts(t *testing.T) {
	ids := parseProjectInputTest(t, "contracts/ids", `newtype ReceiptID = Integer

enum DeliveryState
	Pending = "pending" @json("delivery_state")
end

record Envelope<T>
	payload: T @json("data")
	trace: Boolean = false @json("trace")
end
`)
	job := parseProjectInputTest(t, "jobs/send_receipt", `import { Job, JobResult } from trb/jobs
import { DeliveryState, ReceiptID } from app/contracts/ids

alias DeliveryResult = JobResult

def endpoint<T>(value: T, *, trace: Boolean = false): T
	return value
end

class SendReceiptJob < Job
	queue("mail")
	priority(PRIORITY)
	serialize<DeliveryState>("json")
	configure(:mail, DeliveryState) do |scope|
		scope
	end

	def helper(value: ReceiptID?, *, trace: Boolean = false): ReceiptID?
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
	if input.ProtocolVersion != 7 {
		t.Fatalf("protocol version=%d, want 7", input.ProtocolVersion)
	}
	if len(input.Modules[0].Newtypes) != 1 || input.Modules[0].Newtypes[0].Name != "ReceiptID" || input.Modules[0].Newtypes[0].Identity != (packageextension.ProjectDeclarationIdentity{ModulePath: "contracts/ids", Name: "ReceiptID"}) || input.Modules[0].Newtypes[0].Target.Resolved.Kind != "int" {
		t.Fatalf("newtype declaration facts are incomplete: %#v", input.Modules[0].Newtypes)
	}
	if len(input.Modules[0].Enums) != 1 || input.Modules[0].Enums[0].Members[0].RawValue == nil || input.Modules[0].Enums[0].Members[0].RawValue.Raw != `"pending"` || len(input.Modules[0].Enums[0].Members[0].Attributes) != 1 || input.Modules[0].Enums[0].Members[0].Attributes[0].Arguments[0].Value.Raw != `"delivery_state"` {
		t.Fatalf("enum declaration facts are incomplete: %#v", input.Modules[0].Enums)
	}
	if len(input.Modules[0].Records) != 1 || len(input.Modules[0].Records[0].Fields) != 2 {
		t.Fatalf("record declaration facts are incomplete: %#v", input.Modules[0].Records)
	}
	record := input.Modules[0].Records[0]
	if len(record.TypeParameters) != 1 || record.Fields[0].Type.Authored.Name != "T" || len(record.Fields[0].Attributes) != 1 || record.Fields[0].Attributes[0].Arguments[0].Value.Raw != `"data"` || !record.Fields[1].HasDefault || record.Fields[1].Attributes[0].Arguments[0].Value.Raw != `"trace"` {
		t.Fatalf("generic record field facts are incomplete: %#v", record)
	}
	if len(module.Imports) != 2 || module.Imports[1].ModulePath != "contracts/ids" || len(module.TypeAliases) != 1 || len(module.Classes) != 1 {
		t.Fatalf("declaration facts are incomplete: %#v", module)
	}
	if len(module.Functions) != 1 || len(module.Functions[0].TypeParameters) != 1 || len(module.Functions[0].Parameters) != 2 || !module.Functions[0].Parameters[1].NamedOnly || !module.Functions[0].Parameters[1].Optional || module.Functions[0].Return == nil || module.Functions[0].Return.Authored.Name != "T" {
		t.Fatalf("top-level function signature facts are incomplete: %#v", module.Functions)
	}
	if module.Functions[0].Identity == nil || *module.Functions[0].Identity != (packageextension.ProjectDeclarationIdentity{ModulePath: "jobs/send_receipt", Name: "endpoint"}) {
		t.Fatalf("top-level function identity is incomplete: %#v", module.Functions[0])
	}
	class := module.Classes[0]
	if class.Superclass == nil || class.Superclass.Authored.Name != "Job" || class.Superclass.Authored.Definition == nil || class.Superclass.Authored.Definition.ImportPath != "trb/jobs" {
		t.Fatalf("superclass identity is missing: %#v", class.Superclass)
	}
	if len(class.Methods) != 2 || len(class.Directives) != 4 {
		t.Fatalf("class signature facts are incomplete: %#v", class)
	}
	helper := class.Methods[0]
	if len(helper.Parameters) != 2 || !helper.Parameters[1].NamedOnly || !helper.Parameters[1].Optional {
		t.Fatalf("named-only parameter facts are incomplete: %#v", helper.Parameters)
	}
	helperParameter := helper.Parameters[0].Type
	if helperParameter.Representation == nil || helperParameter.Representation.Kind != "int" || !helperParameter.Representation.Nullable {
		t.Fatalf("nullable newtype representation is missing: %#v", helperParameter)
	}
	if class.Directives[0].Arguments[0].Value.Kind != "string" || class.Directives[0].Arguments[0].Value.Raw != `"mail"` {
		t.Fatalf("literal directive was not copied: %#v", class.Directives[0])
	}
	if class.Directives[1].Arguments[0].Value.Kind != "reference" {
		t.Fatalf("reference directive argument was not copied: %#v", class.Directives[1])
	}
	serialized := class.Directives[2]
	if len(serialized.TypeArguments) != 1 || serialized.TypeArguments[0].Authored.Name != "DeliveryState" || serialized.TypeArguments[0].Authored.Definition == nil || serialized.TypeArguments[0].Authored.Definition.ModulePath != "contracts/ids" {
		t.Fatalf("generic directive type arguments are incomplete: %#v", serialized)
	}
	configured := class.Directives[3]
	if configured.Arguments[0].Value.Kind != "symbol" || configured.Arguments[0].Value.Name != "mail" || configured.Arguments[1].Value.Reference == nil || configured.Arguments[1].Value.Reference.Identity != (packageextension.ProjectDeclarationIdentity{ModulePath: "contracts/ids", Name: "DeliveryState"}) {
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

func TestExportProjectDeclarationInputResolvesAnExcludedPackageIndex(t *testing.T) {
	program := parseProjectInputTest(t, "routes/reports", `import { ReportInput } from acme/contracts

record LocalContract
	input: ReportInput
end
`)
	input, err := ExportProjectDeclarationInput("trb/web", []*ast.Program{program}, ProjectDeclarationInputOptions{
		PackageAliasesByModule: map[string]map[string]string{
			"routes/reports": {"acme/contracts": "github.com/acme/contracts"},
		},
		KnownModulePaths: []string{"github.com/acme/contracts/index"},
	})
	if err != nil {
		t.Fatal(err)
	}
	module := input.Modules[0]
	if module.Imports[0].ModulePath != "github.com/acme/contracts/index" {
		t.Fatalf("package index import resolved to %q", module.Imports[0].ModulePath)
	}
	field := module.Records[0].Fields[0].Type.Authored
	if field.Definition == nil || field.Definition.ModulePath != "github.com/acme/contracts/index" || field.Definition.ImportPath != "acme/contracts" {
		t.Fatalf("package type definition is incomplete: %#v", field.Definition)
	}
}

func TestExportProjectDeclarationInputSeparatesAuthoredAndGeneratedImportProvenance(t *testing.T) {
	authored := `import { ReportInput } from acme/contracts

record AuthoredContract
	input: ReportInput
end
`
	generated := `import { ReportInput } from github.com/acme/contracts/index

record GeneratedContract
	input: ReportInput
end
`
	program := parseProjectInputTest(t, "routes/reports", authored+generated)
	program.CompilerGeneratedStart = len(authored)
	input, err := ExportProjectDeclarationInput("trb/web", []*ast.Program{program}, ProjectDeclarationInputOptions{
		PackageAliasesByModule: map[string]map[string]string{
			"routes/reports": {"acme/contracts": "github.com/acme/contracts"},
		},
		KnownModulePaths: []string{"github.com/acme/contracts/index"},
	})
	if err != nil {
		t.Fatal(err)
	}
	definitions := map[string]*packageextension.Definition{}
	for _, record := range input.Modules[0].Records {
		definitions[record.Name] = record.Fields[0].Type.Authored.Definition
	}
	if definition := definitions["AuthoredContract"]; definition == nil || definition.ImportPath != "acme/contracts" {
		t.Fatalf("generated import replaced authored provenance: %#v", definition)
	}
	if definition := definitions["GeneratedContract"]; definition == nil || definition.ImportPath != "github.com/acme/contracts/index" {
		t.Fatalf("generated declaration did not use its generated import scope: %#v", definition)
	}
}

func TestExportProjectDeclarationInputIgnoresNestedImportsDuringResolution(t *testing.T) {
	program := parseProjectInputTest(t, "contracts/envelope", `import { Unit } from trb/std/unit

module Hidden
	import { Unit } from missing/package
end

record Envelope
	value: Unit
end
`)
	input, err := ExportProjectDeclarationInput("review/provider", []*ast.Program{program}, ProjectDeclarationInputOptions{})
	if err != nil {
		t.Fatal(err)
	}
	module := input.Modules[0]
	if len(module.Imports) != 1 || module.Imports[0].Path != "trb/std/unit" {
		t.Fatalf("PDI exported a nested import: %#v", module.Imports)
	}
	field := module.Records[0].Fields[0].Type.Authored
	if field.Definition == nil || field.Definition.ImportPath != "trb/std/unit" || field.Definition.ModulePath != "trb/std/unit" {
		t.Fatalf("nested import replaced the top-level type identity: %#v", field.Definition)
	}
}

func TestExportProjectDeclarationInputIncludesNestedModuleMetadataWithStructuredIdentity(t *testing.T) {
	program := parseProjectInputTest(t, "commands", `module CLI
	alias Count = Integer

	def build_count(): Integer
		return 1
	end

	class Helper
		def value(): Integer
			return 1
		end
	end

	record Payload
		value: String
	end

	record Options
		payload: Payload
		count: Count = 1 @schema(label: "count")
	end

	enum Command
		Run(options: Options) @schema(label: "run")
	end
end

module Admin
	record Options
		name: String = "admin"
	end
end
`)
	input, err := ExportProjectDeclarationInput("review/provider", []*ast.Program{program}, ProjectDeclarationInputOptions{})
	if err != nil {
		t.Fatal(err)
	}
	module := input.Modules[0]
	if len(module.Records) != 3 {
		t.Fatalf("nested records were not exported: %#v", module.Records)
	}
	if len(module.TypeAliases) != 0 || len(module.Classes) != 0 || len(module.Functions) != 0 {
		t.Fatalf("nested declarations outside the v7 metadata contract leaked into provider discovery: %#v", module)
	}
	byName := map[string]packageextension.ProjectRecord{}
	for _, record := range module.Records {
		byName[record.Identity.Name] = record
	}
	options, ok := byName["CLI::Options"]
	if !ok || byName["CLI::Payload"].Name != "Payload" || byName["Admin::Options"].Name != "Options" {
		t.Fatalf("nested record identities are incomplete: %#v", byName)
	}
	if options.Name != "Options" || options.Owner == nil || *options.Owner != (packageextension.ProjectDeclarationIdentity{ModulePath: "commands", Name: "CLI"}) {
		t.Fatalf("nested record display name and owner are incomplete: %#v", options)
	}
	if len(options.Fields) != 2 || !options.Fields[1].HasDefault || len(options.Fields[1].Attributes) != 1 {
		t.Fatalf("nested record default metadata is incomplete: %#v", options)
	}
	count := options.Fields[1].Type
	if count.Resolved.Name != "Integer" || len(count.ResolutionPath) != 1 || count.ResolutionPath[0].Identity.Name != "CLI::Count" {
		t.Fatalf("hidden nested alias did not participate in type resolution: %#v", count)
	}
	payload := options.Fields[0].Type
	if payload.Authored.Name != "Payload" || payload.Authored.Definition == nil || payload.Authored.Definition.ModulePath != "commands" || payload.Authored.Definition.Name != "CLI::Payload" || payload.Resolved.Name != "Payload" || len(payload.ResolutionPath) != 1 || payload.ResolutionPath[0].Identity.Name != "CLI::Payload" {
		t.Fatalf("nested record type identity is incomplete: %#v", payload)
	}
	if len(module.Enums) != 1 || module.Enums[0].Name != "Command" || module.Enums[0].Identity.Name != "CLI::Command" || module.Enums[0].Owner == nil || module.Enums[0].Owner.Name != "CLI" || len(module.Enums[0].Members[0].Attributes) != 1 {
		t.Fatalf("nested enum metadata is incomplete: %#v", module.Enums)
	}
	parameter := module.Enums[0].Members[0].Parameters[0].Type
	if parameter.Resolved.Name != "Options" || parameter.Resolved.Definition == nil || parameter.Resolved.Definition.Name != "CLI::Options" || len(parameter.ResolutionPath) != 1 || parameter.ResolutionPath[0].Identity.Name != "CLI::Options" {
		t.Fatalf("nested enum payload identity is incomplete: %#v", parameter)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || containsAny(string(encoded), "generatedName", "targetName", "backendName") {
		t.Fatalf("PDI contains a generated identifier field: %s", encoded)
	}
}

func containsProjectTypeReference(references []packageextension.ProjectDeclarationReference, name, importPath string) bool {
	for _, reference := range references {
		if reference.Identity.Name == name && reference.ImportPath == importPath {
			return true
		}
	}
	return false
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
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
