package compiler

import (
	"strings"
	"testing"

	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
)

func TestCompileProjectGeneratesTypedGoJobEnqueueRuntime(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`import { Result } from trb/std/result
import { puts } from trb/std/io
import { SendReceiptJob } from jobs/send_receipt_job

def main()
	case attempt SendReceiptJob.perform_later(42, "ada@example.test")
	when Result::Ok(reference)
		puts(reference.id)
	when Result::Err(error)
		puts(error.message)
	end
	return
end
`),
		},
		{
			Filename: "/project/src/jobs/send_receipt_job.trb", ModulePath: "jobs/send_receipt_job", Package: "jobs",
			Source: []byte(`import { Job } from trb/jobs

class SendReceiptJob < Job
	def perform(order_id: Integer, destination: String)
		return
	end
end
`),
		},
	}
	artifacts, err := CompileProject(sources, Options{
		Mode: "go", GoModule: "example.com/jobs", SourceRoot: "/project/src", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	main := artifactForModule(artifacts, "main")
	if main == nil {
		t.Fatal("main artifact was not generated")
	}
	manifest := jobsintegration.ManifestFrom(main.IR.Extensions)
	if manifest == nil || len(manifest.Jobs) != 1 || manifest.Jobs[0].Name != "SendReceiptJob" {
		t.Fatalf("unexpected jobs manifest: %#v", manifest)
	}
	if !strings.Contains(string(main.Output), `jobs.SendReceiptJobPerformLater(42, "ada@example.test")`) {
		t.Fatalf("main does not call the typed job enqueue wrapper:\n%s", main.Output)
	}
	job := artifactForModule(artifacts, "jobs/send_receipt_job")
	if job == nil || !strings.Contains(string(job.Output), `.TrbJobsEnqueue("SendReceiptJob"`) {
		t.Fatalf("job module does not enqueue through the jobs runtime:\n%s", job.Output)
	}
	runtime := artifactForModule(artifacts, "trb/jobs/index")
	if runtime == nil || !strings.Contains(string(runtime.Output), "func TrbJobsClaimNext") || !strings.Contains(string(runtime.Output), "FOR UPDATE") && manifest.Config.DatabaseAdapter != "sqlite" {
		t.Fatalf("jobs SQL runtime was not generated:\n%s", runtime.Output)
	}
}

func TestCompileProjectRejectsUnsupportedJobArguments(t *testing.T) {
	sources := []SourceUnit{{
		Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { Job } from trb/jobs

record Payload
	id: Integer
end

class InvalidJob < Job
	def perform(payload: Payload)
		return
	end
end

def main()
	return
end
`),
	}}
	_, err := CompileProject(sources, Options{Mode: "go", GoModule: "example.com/jobs", SourceRoot: "/project/src", ProjectRoot: "/project"})
	if err == nil || !strings.Contains(err.Error(), "must initially be Boolean, Integer, Float, or String") {
		t.Fatalf("expected job argument diagnostic, got %v", err)
	}
}
