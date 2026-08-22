package compiler

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
	jobssql "github.com/type-rb/type-rb/internal/jobs/sqladapter"
)

const jobsSQLConfigurationSource = `import { JobAdapter } from trb/jobs
import { SQLAdapter, SQLDialect } from trb/jobs/sql

JOBS_ADAPTER: JobAdapter := SQLAdapter.new(dialect: SQLDialect::SQLite, source: "jobs.sqlite3")
`

func TestCompileProjectGeneratesTypedGoJobEnqueueRuntime(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`import { puts } from trb/std/io
import { SendReceiptJob } from jobs/send_receipt_job

def main()
	reference := SendReceiptJob.perform_later(42, "ada@example.test") catch |error|
		puts(error.message)
		return
	end
	puts(reference.id)
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
	if job == nil || !strings.Contains(string(job.Output), "func trbJobsSendReceiptJobRequest(") || !strings.Contains(string(job.Output), "return config.JobsAdapter.Enqueue(__trbScope, request)") || !strings.Contains(string(job.Output), "return trbJobsSendReceiptJobPerformLater(__trbScope") || strings.Contains(string(job.Output), ".TrbJobsEnqueue(") {
		t.Fatalf("job module does not enqueue through the portable generated helper:\n%s", job.Output)
	}
	configuration := artifactForModule(artifacts, "config/jobs")
	if configuration == nil {
		t.Fatal("jobs configuration artifact was not generated")
	}
	if strings.Count(string(configuration.Output), "var JobsAdapter ") != 1 || strings.Count(string(configuration.Output), "NewSQLAdapter(") != 1 {
		t.Fatalf("Go jobs adapter is not initialized once at module scope:\n%s", configuration.Output)
	}
	runtime := artifactForModule(artifacts, jobssql.ModulePath)
	if runtime == nil || !strings.Contains(string(runtime.Output), "func (self *SQLAdapter) Enqueue(") || !strings.Contains(string(runtime.Output), "requestValue.PayloadVersion") || !strings.Contains(string(runtime.Output), "func TrbJobsClaimNext") || !strings.Contains(string(runtime.Output), `runAtValue := runAt.Format("2006-01-02 15:04:05.000")`) || !strings.Contains(string(runtime.Output), `strftime('%Y-%m-%d %H:%M:%f', 'now')`) {
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

func TestCompileProjectAcceptsCanonicalJobResultPerformAcrossModes(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/src/jobs/example_job.trb", ModulePath: "jobs/example_job", Package: "jobs",
			Source: []byte(`import { Job, JobError, JobResult } from trb/jobs
import { Unit } from trb/std/unit

class ExampleJob < Job
	def perform(fail: Boolean): JobResult
		if fail
			return JobResult::Err(JobError.new(message: "stopped"))
		end
		return JobResult::Ok(Unit.new())
	end
end
`),
		},
		{Filename: "/project/src/config/jobs.trb", ModulePath: "config/jobs", Package: "config", Source: []byte(jobsSQLConfigurationSource)},
		{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte("def main()\n\treturn\nend\n")},
	}
	for _, options := range []Options{
		{Mode: "go", GoModule: "example.com/jobs"},
		{Mode: "ruby", RubyLoader: "require_relative"},
		{Mode: "typescript", TypeScriptRuntime: "bun"},
	} {
		options.SourceRoot = "/project/src"
		options.ProjectRoot = "/project"
		options.JobsConfiguration = "config/jobs"
		artifacts, err := CompileProject(sources, options)
		if err != nil {
			t.Fatalf("%s: %v", options.Mode, err)
		}
		manifest := jobsintegration.ManifestFrom(artifactForModule(artifacts, "jobs/example_job").IR.Extensions)
		if manifest == nil || len(manifest.Jobs) != 1 || manifest.Jobs[0].PerformKind != jobsintegration.PerformJobResult {
			t.Fatalf("%s produced unexpected JobResult manifest: %#v", options.Mode, manifest)
		}
	}
}

func TestCompileProjectUsesPortableGeneratedJobsDispatchAcrossModes(t *testing.T) {
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
		{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte("def main()\n\treturn\nend\n")},
	}
	for _, options := range []Options{
		{Mode: "go", GoModule: "example.com/jobs"},
		{Mode: "ruby", RubyLoader: "require_relative"},
		{Mode: "typescript", TypeScriptRuntime: "bun"},
	} {
		t.Run(options.Mode, func(t *testing.T) {
			options.SourceRoot = "/project/src"
			options.ProjectRoot = "/project"
			options.JobsConfiguration = "config/jobs"
			artifacts, err := CompileProject(sources, options)
			if err != nil {
				t.Fatal(err)
			}
			main := artifactForModule(artifacts, "main")
			if main == nil || main.CompilerGeneratedStart <= 0 {
				t.Fatalf("%s did not retain a generated source boundary", options.Mode)
			}
			foundDispatch := false
			for _, statement := range main.AST.Statements {
				method, ok := statement.(*ast.MethodStatement)
				if ok && method.Name == "__trb_jobs_dispatch" {
					foundDispatch = true
					break
				}
			}
			if !foundDispatch {
				t.Fatalf("%s did not parse and check the generated Jobs dispatch", options.Mode)
			}
			output := string(main.Output)
			for _, expected := range map[string][]string{
				"go":         {"func trbJobsDispatch(", "func trbJobsExecuteClaim("},
				"ruby":       {"def __trb_jobs_dispatch(", "def trb_jobs_execute_claim("},
				"typescript": {"function __trb_jobs_dispatch(", "function trbJobsExecuteClaim("},
			}[options.Mode] {
				if !strings.Contains(output, expected) {
					t.Fatalf("generated %s output is missing %q:\n%s", options.Mode, expected, output)
				}
			}
			for _, legacy := range []string{"json.RawMessage", "switch (claim.job_name)", "def trb_jobs_dispatch("} {
				if strings.Contains(output, legacy) {
					t.Fatalf("generated %s output retains backend-owned dispatch %q:\n%s", options.Mode, legacy, output)
				}
			}
			for _, mapping := range main.SourceMap.Mappings {
				if mapping.Source.Path == main.Filename && mapping.Source.Span.Start.Offset >= main.CompilerGeneratedStart {
					t.Fatalf("%s source map exposes generated Jobs source: %#v", options.Mode, mapping.Source)
				}
			}
		})
	}
}

func TestAnalyzerRefreshesProjectGeneratedJobsDispatchAfterJobContractChange(t *testing.T) {
	jobSource := func(parameterType string) []byte {
		return []byte("import { Job } from trb/jobs\n\nclass ExampleJob < Job\n\tdef perform(value: " + parameterType + ")\n\t\treturn\n\tend\nend\n")
	}
	sources := []SourceUnit{
		{Filename: "/project/src/jobs/example_job.trb", ModulePath: "jobs/example_job", Package: "jobs", Source: jobSource("Integer")},
		{Filename: "/project/src/config/jobs.trb", ModulePath: "config/jobs", Package: "config", Source: []byte(jobsSQLConfigurationSource)},
		{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte("def main()\n\treturn\nend\n")},
	}
	options := Options{
		Mode: "go", GoModule: "example.com/jobs", SourceRoot: "/project/src", ProjectRoot: "/project", JobsConfiguration: "config/jobs",
	}
	analyzer := NewAnalyzer()
	initial, err := analyzer.AnalyzeProject(sources, options)
	if err != nil {
		t.Fatal(err)
	}
	initialSource := jobsDispatchFragment(t, artifactForModule(initial, "main"))
	if !strings.Contains(initialSource, "as_integer(payload_values[0])") {
		t.Fatalf("initial dispatch has unexpected source:\n%s", initialSource)
	}

	sources[0].Source = jobSource("String")
	incremental, err := analyzer.AnalyzeProject(sources, options)
	if err != nil {
		t.Fatal(err)
	}
	updatedSource := jobsDispatchFragment(t, artifactForModule(incremental, "main"))
	if initialSource == updatedSource || !strings.Contains(updatedSource, "as_string(payload_values[0])") {
		t.Fatalf("incremental dispatch retained stale source:\n%s", updatedSource)
	}
	requireAnalysisMatchesFullCompilation(t, incremental, sources, options)
}

func jobsDispatchFragment(t *testing.T, artifact *Artifact) string {
	t.Helper()
	if artifact == nil {
		t.Fatal("entrypoint artifact is missing")
	}
	found := ""
	for _, generated := range artifact.sourceUnit.CompilerGeneratedSources {
		if strings.HasSuffix(generated.ID, ":worker-dispatch") {
			if found != "" {
				t.Fatal("Jobs dispatch was generated more than once")
			}
			found = string(generated.Source)
		}
	}
	if found == "" {
		t.Fatal("Jobs dispatch generated source is missing")
	}
	return found
}

func TestCompileProjectRejectsUnsupportedJobPerformContracts(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "raw standard Result",
			source: "import { Job, JobError } from trb/jobs\n" +
				"import { Result } from trb/std/result\n" +
				"import { Unit } from trb/std/unit\n\n" +
				"class InvalidJob < Job\n\tdef perform(): Result<Unit, JobError>\n\t\treturn Result<Unit, JobError>::Ok(Unit.new())\n\tend\nend\n",
			message: "must omit its return type or return JobResult",
		},
		{
			name:    "legacy effect",
			source:  "import { Job } from trb/jobs\nrecord AppError\nend\n\nclass InvalidJob < Job\n\tdef perform() fails AppError\n\t\treturn\n\tend\nend\n",
			message: "fails was removed in TypeRB 0.3",
		},
		{
			name:    "mixed result and effect",
			source:  "import { Job, JobResult } from trb/jobs\nrecord AppError\nend\n\nclass InvalidJob < Job\n\tdef perform(): JobResult fails AppError\n\t\treturn\n\tend\nend\n",
			message: "fails was removed in TypeRB 0.3",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{{
				Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte(test.source),
			}}, Options{Mode: "go", GoModule: "example.com/jobs", SourceRoot: "/project/src", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q, got %v", test.message, err)
			}
		})
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
	case SendReceiptJob.perform_later(42, "ada@example.test")
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
	configuration := artifactForModule(artifacts, "config/jobs")
	runtime := artifactForModule(artifacts, jobssql.ModulePath)
	if main == nil || job == nil || configuration == nil || runtime == nil {
		t.Fatal("Ruby Jobs artifacts were not generated")
	}
	if !strings.Contains(string(main.Output), "trb_jobs_run_worker_or_command") || !strings.Contains(string(job.Output), "return JOBS_ADAPTER.enqueue(__trb_scope, request)") || strings.Contains(string(job.Output), "TrbJobsRuntime.enqueue") || strings.Count(string(configuration.Output), "JOBS_ADAPTER = SQLAdapter.new(") != 1 || !strings.Contains(string(job.Output), "def self.perform_in") || !strings.Contains(string(job.Output), "def self.perform_at") || !strings.Contains(string(runtime.Output), "request_value.payload_version") || !strings.Contains(string(runtime.Output), "module TrbJobsRuntime") {
		t.Fatalf("Ruby jobs runtime is incomplete:\nmain=%s\njob=%s\nconfiguration=%s\nruntime=%s", main.Output, job.Output, configuration.Output, runtime.Output)
	}
	rubyEnqueue := generatedSection(string(runtime.Output), "def enqueue(scope,", "def claim(")
	if checks := strings.Count(rubyEnqueue, "scope.check!"); checks != 1 {
		t.Fatalf("Ruby enqueue must check cancellation before persistence, not report cancellation after a known commit; checks=%d\nenqueue=%s", checks, rubyEnqueue)
	}
}

func TestCompileProjectGeneratesTypeScriptBunJobRuntime(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`import { Result } from trb/std/result
import { SendReceiptJob } from jobs/send_receipt_job

def main()
	case SendReceiptJob.perform_later(42, "ada@example.test")
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
	configuration := artifactForModule(artifacts, "config/jobs")
	runtime := artifactForModule(artifacts, jobssql.ModulePath)
	if main == nil || job == nil || configuration == nil || runtime == nil {
		t.Fatal("TypeScript Jobs artifacts were not generated")
	}
	if !strings.Contains(string(main.Output), "await trbJobsRunWorkerOrCommand()") || !strings.Contains(string(job.Output), "return (await JOBS_ADAPTER.enqueue(__trbScope, request))") || strings.Contains(string(job.Output), "trbJobsEnqueue(") || strings.Count(string(configuration.Output), "export const JOBS_ADAPTER:") != 1 || strings.Count(string(configuration.Output), "new SQLAdapter(") != 1 || !strings.Contains(string(job.Output), "function perform_in") || !strings.Contains(string(job.Output), "function perform_at") || !strings.Contains(string(runtime.Output), "requestValue.payload_version") || !strings.Contains(string(runtime.Output), "export async function trbJobsClaim") || !strings.Contains(string(runtime.Output), "timestamp.slice(0, 23)") || !strings.Contains(string(runtime.Output), `strftime('%Y-%m-%d %H:%M:%f', 'now')`) {
		t.Fatalf("TypeScript jobs runtime is incomplete:\nmain=%s\njob=%s\nconfiguration=%s\nruntime=%s", main.Output, job.Output, configuration.Output, runtime.Output)
	}
	typeScriptEnqueue := generatedSection(string(runtime.Output), "export async function trbJobsEnqueue(", "export async function trbJobsClaim(")
	if checks := strings.Count(typeScriptEnqueue, "signal?.aborted"); checks != 1 {
		t.Fatalf("TypeScript enqueue must check cancellation before persistence, not report cancellation after a known commit; checks=%d\nenqueue=%s", checks, typeScriptEnqueue)
	}
}

func generatedSection(output, start, end string) string {
	startIndex := strings.Index(output, start)
	if startIndex < 0 {
		return ""
	}
	endIndex := strings.Index(output[startIndex+len(start):], end)
	if endIndex < 0 {
		return output[startIndex:]
	}
	return output[startIndex : startIndex+len(start)+endIndex]
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
			Source: []byte(`import { Job, JobError, JobResult } from trb/jobs
import { Unit } from trb/std/unit

class ChildJob < Job
	def perform(value: Integer)
		return
	end
end

class ParentJob < Job
	def perform(value: Integer): JobResult
		ChildJob.perform_later(value) catch |error|
			return JobResult::Err(JobError.new(message: error.message))
		end
		return JobResult::Ok(Unit.new())
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
		{mode: "ruby", options: Options{Mode: "ruby", RubyLoader: "require_relative"}, want: []string{"def perform(__trb_scope, value)", "ChildJob.perform_later(__trb_scope, value)", "return ParentJob.new().perform(__trb_scope, argument0)"}},
		{mode: "typescript", options: Options{Mode: "typescript", TypeScriptRuntime: "bun"}, want: []string{"perform(__trbScope: AbortSignal | undefined, value: number)", "ChildJob.perform_later(__trbScope, value)", "return (await new ParentJob().perform(__trbScope, argument0))"}},
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
