package typeprovider

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
	"github.com/type-rb/type-rb/internal/parser"
)

func TestJobDeclarationsCrossVersionedExtensionBoundary(t *testing.T) {
	program, diagnostics := parser.Parse([]byte(`import { Job } from trb/jobs

type ReceiptID = Integer

class SendReceiptJob < Job
	def perform(receipt_id: ReceiptID, email: String)
		return
	end
end
`))
	if len(diagnostics) > 0 {
		t.Fatalf("parse diagnostics: %#v", diagnostics)
	}
	program.ModulePath = "jobs/send_receipt_job"
	programs := []*ast.Program{program}

	provided, err := loadJobDeclarations(programs)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(provided)
	if err != nil {
		t.Fatal(err)
	}
	var decoded packageextension.DeclarationCatalog
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := packageextension.ValidateDeclarationCatalog(decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Provider != jobsintegration.PackageName {
		t.Fatalf("provider=%q, want %q", decoded.Provider, jobsintegration.PackageName)
	}

	job := declaredProtocolType(t, decoded, "SendReceiptJob")
	if job.Superclass != "Job" || len(job.ClassMembers) != 3 {
		t.Fatalf("Job declaration did not cross the protocol: %#v", job)
	}
	later := declaredProtocolMember(t, job.ClassMembers, "perform_later")
	if later.RuntimeOperation != "trb.jobs.perform_later" || len(later.Parameters) != 2 || protocolTypeString(later.Return) != "Result<JobReference, EnqueueError>" {
		t.Fatalf("perform_later declaration did not cross the protocol: %#v", later)
	}
	assertDeclaredProtocolParameter(t, later, 0, "receipt_id", "ReceiptID")
	assertDeclaredProtocolParameter(t, later, 1, "email", "String")
	delayed := declaredProtocolMember(t, job.ClassMembers, "perform_in")
	if len(delayed.Parameters) != 3 {
		t.Fatalf("perform_in declaration did not cross the protocol: %#v", delayed)
	}
	assertDeclaredProtocolParameter(t, delayed, 0, "delay", "Duration")
	scheduled := declaredProtocolMember(t, job.ClassMembers, "perform_at")
	if len(scheduled.Parameters) != 3 {
		t.Fatalf("perform_at declaration did not cross the protocol: %#v", scheduled)
	}
	assertDeclaredProtocolParameter(t, scheduled, 0, "scheduled_at", "Instant")

	imported, err := packageextensionhost.ImportDeclarationCatalog(decoded)
	if err != nil {
		t.Fatal(err)
	}
	original, err := jobsintegration.Declarations(programs)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(imported, original) {
		t.Fatal("declaration protocol changed the Jobs catalog semantics")
	}
	loaded, err := loadJobs(programs, Context{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(imported, loaded) {
		t.Fatal("Jobs provider did not load through the declaration protocol")
	}
}

func assertDeclaredProtocolParameter(t *testing.T, member packageextension.DeclaredMember, index int, name, typeName string) {
	t.Helper()
	parameter := member.Parameters[index]
	if parameter.Name != name || protocolTypeString(parameter.Type) != typeName {
		t.Fatalf("%s parameter %d=%s: %s, want %s: %s", member.Name, index, parameter.Name, protocolTypeString(parameter.Type), name, typeName)
	}
}
