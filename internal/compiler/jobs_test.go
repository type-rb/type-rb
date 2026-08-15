package compiler

import (
	"strings"
	"testing"

	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
	jobssql "github.com/type-rb/type-rb/internal/jobs/sqladapter"
)

const jobsSQLConfigurationSource = `import { JobAdapter } from trb/jobs
import { SQLAdapter, SQLDialect } from trb/jobs/sql

def configure_jobs(): JobAdapter
	return SQLAdapter.new(dialect: SQLDialect::SQLite, source: "jobs.sqlite3")
end
`

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
			Source: []byte(`import { Job, maximum_attempts, priority, queue } from trb/jobs

class SendReceiptJob < Job
	queue("mail")
	priority(10)
	maximum_attempts(3)

	def perform(order_id: Integer, destination: String)
		return
	end
end
`),
		},
		{Filename: "/project/src/config/jobs.trb", ModulePath: "config/jobs", Package: "config", Source: []byte(jobsSQLConfigurationSource)},
	}
	artifacts, err := CompileProject(sources, Options{
		Mode: "go", GoModule: "example.com/jobs", SourceRoot: "/project/src", ProjectRoot: "/project", JobsConfiguration: "config/jobs",
	})
	if err != nil {
		t.Fatal(err)
	}
	main := artifactForModule(artifacts, "main")
	if main == nil {
		t.Fatal("main artifact was not generated")
	}
	manifest := jobsintegration.ManifestFrom(main.IR.Extensions)
	if manifest == nil || len(manifest.Jobs) != 1 || manifest.Jobs[0].Name != "SendReceiptJob" || manifest.Jobs[0].Queue != "mail" || manifest.Jobs[0].Priority != 10 || manifest.Jobs[0].MaximumAttempts != 3 {
		t.Fatalf("unexpected jobs manifest: %#v", manifest)
	}
	if !strings.Contains(string(main.Output), `jobs.SendReceiptJobPerformLater(__trbScope, 42, "ada@example.test")`) {
		t.Fatalf("main does not call the typed job enqueue wrapper:\n%s", main.Output)
	}
	job := artifactForModule(artifacts, "jobs/send_receipt_job")
	if job == nil || !strings.Contains(string(job.Output), `.TrbJobsEnqueue(__trbScope, "SendReceiptJob", string(payload), "mail", 10, waitMilliseconds, 3)`) || !strings.Contains(string(job.Output), "SendReceiptJobPerformIn") || !strings.Contains(string(job.Output), "SendReceiptJobPerformAt") {
		t.Fatalf("job module does not enqueue through the jobs runtime:\n%s", job.Output)
	}
	runtime := artifactForModule(artifacts, jobssql.ModulePath)
	if runtime == nil || !strings.Contains(string(runtime.Output), "func TrbJobsClaimNext") || !strings.Contains(string(runtime.Output), `runAtValue := runAt.Format("2006-01-02 15:04:05.000")`) || !strings.Contains(string(runtime.Output), `strftime('%Y-%m-%d %H:%M:%f', 'now')`) {
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

func TestCompileProjectAcceptsImportedTransparentScalarAliasJobArgument(t *testing.T) {
	sources := []SourceUnit{
		{Filename: "/project/src/contracts/index.trb", ModulePath: "contracts/index", Package: "contracts", Source: []byte("type OrderId = Integer\n")},
		{
			Filename: "/project/src/jobs/send_receipt_job.trb", ModulePath: "jobs/send_receipt_job", Package: "jobs",
			Source: []byte(`import { OrderId } from contracts
import { Job } from trb/jobs

class SendReceiptJob < Job
	def perform(order_id: OrderId)
		puts(order_id)
		return
	end
end
`),
		},
		{Filename: "/project/src/config/jobs.trb", ModulePath: "config/jobs", Package: "config", Source: []byte(jobsSQLConfigurationSource)},
		{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte("def main()\n\treturn\nend\n")},
	}
	artifacts, err := CompileProject(sources, Options{Mode: "go", GoModule: "example.com/jobs", SourceRoot: "/project/src", ProjectRoot: "/project", JobsConfiguration: "config/jobs"})
	if err != nil {
		t.Fatal(err)
	}
	contracts := artifactForModule(artifacts, "contracts/index")
	if contracts == nil {
		t.Fatal("contracts artifact was not generated")
	}
	if strings.Contains(string(contracts.Output), "trb/jobs/sql") {
		t.Fatalf("unrelated module imports the jobs runtime:\n%s", contracts.Output)
	}
}

func TestCompileProjectDoesNotStartAJobsWorkerFromTheTestRunner(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/src/jobs/example_job.trb", ModulePath: "jobs/example_job", Package: "jobs",
			Source: []byte(`import { Job } from trb/jobs

class ExampleJob < Job
	def perform(id: Integer)
		puts(id)
		return
	end
end
`),
		},
		{Filename: "/project/src/config/jobs.trb", ModulePath: "config/jobs", Package: "config", Source: []byte(jobsSQLConfigurationSource)},
		{Filename: "/project/src/__trb_test_main.trb", ModulePath: "trb_test_main", Package: "main", CompilerOwned: true, Source: []byte("def main()\n\treturn\nend\n")},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		options := Options{Mode: mode, GoModule: "example.com/jobs", RubyLoader: "require_relative", TypeScriptRuntime: "bun", SourceRoot: "/project/src", ProjectRoot: "/project", JobsConfiguration: "config/jobs"}
		artifacts, err := CompileProject(sources, options)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		runner := artifactForModule(artifacts, "trb_test_main")
		if runner == nil {
			t.Fatalf("%s test runner was not generated", mode)
		}
		output := string(runner.Output)
		for _, unexpected := range []string{"trbJobsDispatch", "trb_jobs_run_worker_or_command", "trbJobsRunWorkerOrCommand", "trb/jobs/sql"} {
			if strings.Contains(output, unexpected) {
				t.Fatalf("%s test runner starts or imports the jobs runtime through %q:\n%s", mode, unexpected, output)
			}
		}
	}
}

func TestCompileProjectRequiresTypedJobAdapterConfiguration(t *testing.T) {
	sources := []SourceUnit{{
		Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { Job } from trb/jobs

class ExampleJob < Job
	def perform()
		return
	end
end

def main()
	return
end
`),
	}}
	_, err := CompileProject(sources, Options{Mode: "go", GoModule: "example.com/jobs", SourceRoot: "/project/src", ProjectRoot: "/project"})
	if err == nil || !strings.Contains(err.Error(), "trb/jobs requires jobs.configuration") {
		t.Fatalf("expected typed jobs configuration diagnostic, got %v", err)
	}
}

func TestCompileProjectGeneratesRubyJobRuntime(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`import { Result } from trb/std/result
import { SendReceiptJob } from jobs/send_receipt_job

def main()
	case attempt SendReceiptJob.perform_later(42, "ada@example.test")
	when Result::Ok(_reference)
		return
	when Result::Err(_error)
		return
	end
end
`),
		},
		{
			Filename: "/project/src/jobs/send_receipt_job.trb", ModulePath: "jobs/send_receipt_job", Package: "jobs",
			Source: []byte(`import { Job, maximum_attempts, priority, queue } from trb/jobs

class SendReceiptJob < Job
	queue("mail")
	priority(10)
	maximum_attempts(3)

	def perform(order_id: Integer, destination: String)
		return
	end
end
`),
		},
		{Filename: "/project/src/config/jobs.trb", ModulePath: "config/jobs", Source: []byte(jobsSQLConfigurationSource)},
	}
	artifacts, err := CompileProject(sources, Options{Mode: "ruby", RubyLoader: "require_relative", SourceRoot: "/project/src", ProjectRoot: "/project", JobsConfiguration: "config/jobs"})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range artifacts {
		assertCompiledRubySyntax(t, string(artifact.Output))
	}
	main := artifactForModule(artifacts, "main")
	job := artifactForModule(artifacts, "jobs/send_receipt_job")
	runtime := artifactForModule(artifacts, jobssql.ModulePath)
	if main == nil || job == nil || runtime == nil || !strings.Contains(string(main.Output), "trb_jobs_run_worker_or_command") || !strings.Contains(string(job.Output), `TrbJobsRuntime.enqueue(__trb_scope, "SendReceiptJob", payload, "mail", 10, wait_milliseconds, 3)`) || !strings.Contains(string(job.Output), "def self.perform_in") || !strings.Contains(string(job.Output), "def self.perform_at") || !strings.Contains(string(runtime.Output), "module TrbJobsRuntime") {
		t.Fatalf("Ruby jobs runtime is incomplete:\nmain=%s\njob=%s\nruntime=%s", main.Output, job.Output, runtime.Output)
	}
}

func TestCompileProjectGeneratesTypeScriptBunJobRuntime(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`import { Result } from trb/std/result
import { SendReceiptJob } from jobs/send_receipt_job

def main()
	case attempt SendReceiptJob.perform_later(42, "ada@example.test")
	when Result::Ok(_reference)
		return
	when Result::Err(_error)
		return
	end
end
`),
		},
		{
			Filename: "/project/src/jobs/send_receipt_job.trb", ModulePath: "jobs/send_receipt_job", Package: "jobs",
			Source: []byte(`import { Job, maximum_attempts, priority, queue } from trb/jobs

class SendReceiptJob < Job
	queue("mail")
	priority(10)
	maximum_attempts(3)

	def perform(order_id: Integer, destination: String)
		return
	end
end
`),
		},
		{Filename: "/project/src/config/jobs.trb", ModulePath: "config/jobs", Source: []byte(jobsSQLConfigurationSource)},
	}
	artifacts, err := CompileProject(sources, Options{Mode: "typescript", TypeScriptRuntime: "bun", SourceRoot: "/project/src", ProjectRoot: "/project", JobsConfiguration: "config/jobs"})
	if err != nil {
		t.Fatal(err)
	}
	main := artifactForModule(artifacts, "main")
	job := artifactForModule(artifacts, "jobs/send_receipt_job")
	runtime := artifactForModule(artifacts, jobssql.ModulePath)
	if main == nil || job == nil || runtime == nil || !strings.Contains(string(main.Output), "await trbJobsRunWorkerOrCommand()") || !strings.Contains(string(job.Output), `trbJobsEnqueue(__trbScope, "SendReceiptJob", payload, "mail", 10, waitMilliseconds, 3)`) || !strings.Contains(string(job.Output), "function perform_in") || !strings.Contains(string(job.Output), "function perform_at") || !strings.Contains(string(runtime.Output), "export async function trbJobsClaim") || !strings.Contains(string(runtime.Output), "timestamp.slice(0, 23)") || !strings.Contains(string(runtime.Output), `strftime('%Y-%m-%d %H:%M:%f', 'now')`) {
		t.Fatalf("TypeScript jobs runtime is incomplete:\nmain=%s\njob=%s\nruntime=%s", main.Output, job.Output, runtime.Output)
	}
}

func TestCompileProjectRejectsTypeScriptJobsOutsideBun(t *testing.T) {
	sources := []SourceUnit{{
		Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { Job } from trb/jobs

class ExampleJob < Job
	def perform()
		return
	end
end

def main()
	return
end
`),
	}, {Filename: "/project/src/config/jobs.trb", ModulePath: "config/jobs", Source: []byte(jobsSQLConfigurationSource)}}
	_, err := CompileProject(sources, Options{Mode: "typescript", TypeScriptRuntime: "node", SourceRoot: "/project/src", ProjectRoot: "/project", JobsConfiguration: "config/jobs"})
	if err == nil || !strings.Contains(err.Error(), `trb/jobs in mode: typescript currently requires typescript.runtime: "bun"`) {
		t.Fatalf("expected Bun jobs runtime diagnostic, got %v", err)
	}
}

func TestCompileProjectPropagatesExecutionScopeThroughJobDispatch(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`import { EnqueueError, Job } from trb/jobs

class ChildJob < Job
	def perform(value: Integer)
		return
	end
end

class ParentJob < Job
	def perform(value: Integer) fails EnqueueError
		ChildJob.perform_later(value)
		return
	end
end

def main()
	return
end
`),
		},
		{Filename: "/project/src/config/jobs.trb", ModulePath: "config/jobs", Source: []byte(jobsSQLConfigurationSource)},
	}

	for _, test := range []struct {
		mode    string
		options Options
		want    []string
	}{
		{mode: "go", options: Options{Mode: "go", GoModule: "example.com/jobs"}, want: []string{"func (self *ParentJob) Perform(__trbScope trbcontext.Context", "ChildJobPerformLater(__trbScope, value)", "NewParentJob().Perform(__trbScope, argument0)"}},
		{mode: "ruby", options: Options{Mode: "ruby", RubyLoader: "require_relative"}, want: []string{"def perform(__trb_scope, value)", "ChildJob.perform_later(__trb_scope, value)", "ParentJob.new.perform(__trb_scope, *arguments)"}},
		{mode: "typescript", options: Options{Mode: "typescript", TypeScriptRuntime: "bun"}, want: []string{"perform(__trbScope: AbortSignal | undefined, value: number)", "ChildJob.perform_later(__trbScope, value)", "new ParentJob().perform(__trbScope, parameters[0] as number)"}},
	} {
		t.Run(test.mode, func(t *testing.T) {
			options := test.options
			options.SourceRoot = "/project/src"
			options.ProjectRoot = "/project"
			options.JobsConfiguration = "config/jobs"
			artifacts, err := CompileProject(sources, options)
			if err != nil {
				t.Fatal(err)
			}
			output := string(artifactForModule(artifacts, "main").Output)
			for _, expected := range test.want {
				if !strings.Contains(output, expected) {
					t.Fatalf("generated %s Job execution scope is missing %q:\n%s", test.mode, expected, output)
				}
			}
		})
	}
}
