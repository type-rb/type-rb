package ruby

import (
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/jobs"
	jobssql "github.com/type-rb/type-rb/internal/jobs/sqladapter"
	"github.com/type-rb/type-rb/internal/jobs/sqlstore"
)

func (g *generator) jobsPerformLater(call *ir.Call, arguments []string) string {
	jobName := "Job"
	method := "perform_later"
	if member, ok := call.Callee.(*ir.Member); ok {
		method = member.Name
		if identifier, identifierOK := member.Receiver.(*ir.Identifier); identifierOK {
			jobName = identifier.Name
		}
	}
	if g.execution != nil && g.execution.Calls[call] {
		arguments = append([]string{"__trb_scope"}, arguments...)
	}
	return g.rubyClassName(jobName, nil) + "." + method + "(" + strings.Join(arguments, ", ") + ")"
}

func (g *generator) jobsDeclaration(call *ir.Call) bool {
	if g.jobs == nil || call == nil {
		return false
	}
	identifier, ok := call.Callee.(*ir.Identifier)
	if !ok || identifier.Reference == nil || identifier.Reference.Package != "trb/jobs/index" {
		return false
	}
	return identifier.Name == "queue" || identifier.Name == "priority" || identifier.Name == "maximum_attempts"
}

func (g *generator) jobsRuntime(manifest *jobs.Manifest) {
	if manifest == nil || g.jobsSQL == nil {
		return
	}
	g.jobsClassEnqueueMethods(manifest)
	if g.modulePath == jobssql.ModulePath {
		g.jobsStorage(g.jobsSQL.Config)
	}
	if g.topFunctions["main"] && g.modulePath != "trb_test_main" {
		g.jobsWorker(manifest, g.jobsSQL.Config)
	}
}

func (g *generator) jobsClassEnqueueMethods(manifest *jobs.Manifest) {
	for _, job := range manifest.Jobs {
		if job.ModulePath != g.modulePath {
			continue
		}
		parameters := make([]string, len(job.Parameters))
		for index, parameter := range job.Parameters {
			parameters[index] = parameter.Name
		}
		g.line("class "+g.rubyClassName(job.Name, nil), "")
		g.indent++
		emit := func(method string, methodParameters []string, waitLines ...string) {
			g.line("def self."+method+"(__trb_scope"+rubyJobsParameters(methodParameters)+")", "")
			g.indent++
			for _, line := range waitLines {
				g.line(line, "")
			}
			g.line("begin", "")
			g.indent++
			g.line("payload = JSON.generate(["+strings.Join(parameters, ", ")+"])", "")
			g.indent--
			g.line("rescue StandardError => error", "")
			g.indent++
			g.line("return Result::Err.new(EnqueueError.new(kind: EnqueueErrorKind::Serialization, message: error.message))", "")
			g.indent--
			g.line("end", "")
			g.line("begin", "")
			g.indent++
			g.line("reference = TrbJobsRuntime.enqueue(__trb_scope, "+strconv.Quote(job.Name)+", payload, "+strconv.Quote(job.Queue)+", "+strconv.Itoa(job.Priority)+", wait_milliseconds, "+strconv.Itoa(job.MaximumAttempts)+")", "")
			g.line("Result::Ok.new(reference)", "")
			g.indent--
			g.line("rescue TrbExecutionCancelled => error", "")
			g.indent++
			g.line("Result::Err.new(EnqueueError.new(kind: EnqueueErrorKind::Cancelled, message: error.message))", "")
			g.indent--
			g.line("rescue StandardError => error", "")
			g.indent++
			g.line("Result::Err.new(EnqueueError.new(kind: EnqueueErrorKind::Adapter, message: error.message))", "")
			g.indent--
			g.line("end", "")
			g.indent--
			g.line("end", "")
		}
		emit("perform_later", parameters, "wait_milliseconds = 0")
		emit("perform_in", append([]string{"delay"}, parameters...),
			"wait_milliseconds = delay.whole_seconds * 1000 + (delay.nanosecond + 999999) / 1000000",
			"return Result::Err.new(EnqueueError.new(kind: EnqueueErrorKind::InvalidArgument, message: \"job delay must not be negative\")) if wait_milliseconds < 0",
		)
		emit("perform_at", append([]string{"scheduled_at"}, parameters...),
			"target_milliseconds = scheduled_at.epoch_seconds * 1000 + (scheduled_at.nanosecond + 999999) / 1000000",
			"wait_milliseconds = [target_milliseconds - (Time.now.utc.to_r * 1000).ceil, 0].max",
		)
		g.indent--
		g.line("end", "")
	}
}

func rubyJobsParameters(parameters []string) string {
	if len(parameters) == 0 {
		return ""
	}
	return ", " + strings.Join(parameters, ", ")
}

func (g *generator) jobsStorage(config jobssql.Config) {
	g.line("require \"json\"", "")
	g.line("require \"securerandom\"", "")
	g.line("require \"sequel\"", "")
	g.line("module TrbJobsRuntime", "")
	g.indent++
	g.line("module_function", "")
	g.line("def database", "")
	g.indent++
	g.line("return @database if @database", "")
	if config.SourceEnvironment == "" {
		g.line("source = "+strconv.Quote(config.Source), "")
	} else {
		g.line("source = ENV.fetch("+strconv.Quote(config.SourceEnvironment)+")", "")
		g.line("raise "+strconv.Quote("jobs database environment "+config.SourceEnvironment+" is not set or empty")+" if source.strip.empty?", "")
	}
	if config.Dialect == "mysql" {
		g.line("source = source.sub(/\\Amysql:\\/\\//, \"mysql2://\")", "")
	}
	g.line("source = \"sqlite://#{File.expand_path(source)}\" unless source.include?(\"://\")", "")
	g.line("@database = Sequel.connect(source, max_connections: 1)", "")
	statements, _ := sqlstore.Schema(sqlstore.Dialect(config.Dialect))
	for _, statement := range statements {
		g.line("@database.run("+strconv.Quote(statement)+")", "")
	}
	g.line("@database", "")
	g.indent--
	g.line("end", "")
	g.line("def close; @database.disconnect if @database; @database = nil; end", "")
	g.line("def enqueue(scope, job_name, payload, queue_name, priority, wait_milliseconds, maximum_attempts)", "")
	g.indent++
	g.line("scope.check!", "")
	g.line("id = SecureRandom.hex(16)", "")
	g.line("now = Time.now.utc", "")
	g.line("raise \"job delay must not be negative\" if wait_milliseconds < 0", "")
	g.line("maximum_attempts = "+strconv.Itoa(config.DefaultMaximumAttempts)+" if maximum_attempts <= 0", "")
	g.line("database[:trb_jobs].insert(id: id, queue_name: queue_name, job_name: job_name, payload: payload, payload_version: 1, priority: priority, run_at: now + wait_milliseconds / 1000.0, state: \"ready\", attempts: 0, maximum_attempts: maximum_attempts, created_at: now, updated_at: now)", "")
	g.line("scope.check!", "")
	g.line("JobReference.new(id: id, job_name: job_name)", "")
	g.indent--
	g.line("end", "")
	g.line("def claim(worker_id)", "")
	g.indent++
	g.line("claimed = nil", "")
	g.line("database.transaction do", "")
	g.indent++
	g.line("dataset = database[:trb_jobs].where(state: \"ready\").where { run_at <= Time.now.utc }.order(:priority, :run_at, :id).limit(1)", "")
	g.line("dataset = dataset.where(queue_name: ENV[\"TRB_JOBS_QUEUE\"]) if ENV[\"TRB_JOBS_QUEUE\"] && !ENV[\"TRB_JOBS_QUEUE\"].empty?", "")
	if config.Dialect != "sqlite" {
		g.line("dataset = dataset.for_update.skip_locked", "")
	}
	g.line("row = dataset.first", "")
	g.line("if row", "")
	g.indent++
	g.line("now = Time.now.utc", "")
	g.line("updated = database[:trb_jobs].where(id: row[:id], state: \"ready\").update(state: \"running\", attempts: row[:attempts] + 1, claimed_by: worker_id, claimed_at: now, updated_at: now)", "")
	g.line("claimed = database[:trb_jobs].where(id: row[:id]).first if updated == 1", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.line("claimed", "")
	g.indent--
	g.line("end", "")
	g.line("def acknowledge(id, worker_id); database[:trb_jobs].where(id: id, state: \"running\", claimed_by: worker_id).delete == 1; end", "")
	g.line("def fail(id, worker_id, message)", "")
	g.indent++
	g.line("row = database[:trb_jobs].where(id: id, state: \"running\", claimed_by: worker_id).first", "")
	g.line("return false unless row", "")
	g.line("state = row[:attempts] >= row[:maximum_attempts] ? \"failed\" : \"ready\"", "")
	g.line("database[:trb_jobs].where(id: id, state: \"running\", claimed_by: worker_id).update(state: state, run_at: Time.now.utc + row[:attempts] * "+strconv.FormatFloat(float64(config.RetryBaseDelayMilliseconds)/1000.0, 'f', 3, 64)+", claimed_by: nil, claimed_at: nil, last_error: message, updated_at: Time.now.utc) == 1", "")
	g.indent--
	g.line("end", "")
	g.line("def heartbeat(id, worker_id); database[:trb_jobs].where(id: id, state: \"running\", claimed_by: worker_id).update(claimed_at: Time.now.utc, updated_at: Time.now.utc) == 1; end", "")
	g.line("def release(worker_id); database[:trb_jobs].where(state: \"running\", claimed_by: worker_id).update(state: \"ready\", claimed_by: nil, claimed_at: nil, updated_at: Time.now.utc); end", "")
	g.line("def recover_stale; cutoff = Time.now.utc - "+strconv.FormatFloat(float64(config.LeaseTimeoutMilliseconds)/1000.0, 'f', 3, 64)+"; database[:trb_jobs].where(state: \"running\").where { claimed_at < cutoff }.update(state: \"ready\", claimed_by: nil, claimed_at: nil, updated_at: Time.now.utc); end", "")
	g.line("def list; database[:trb_jobs].order(:created_at, :id).all; end", "")
	g.line("def retry_job(id); database[:trb_jobs].where(id: id, state: \"failed\").update(state: \"ready\", attempts: 0, run_at: Time.now.utc, claimed_by: nil, claimed_at: nil, last_error: nil, updated_at: Time.now.utc) == 1; end", "")
	g.line("def discard(id); database[:trb_jobs].where(id: id).exclude(state: \"running\").delete == 1; end", "")
	g.indent--
	g.line("end", "")
}

func (g *generator) jobsWorker(manifest *jobs.Manifest, config jobssql.Config) {
	modules := map[string]bool{}
	for _, job := range manifest.Jobs {
		if job.ModulePath == g.modulePath {
			continue
		}
		modules[job.ModulePath] = true
	}
	modulePaths := make([]string, 0, len(modules))
	for modulePath := range modules {
		modulePaths = append(modulePaths, modulePath)
	}
	sort.Strings(modulePaths)
	for _, modulePath := range modulePaths {
		g.line("require_relative "+strconv.Quote(rubyImportPath(g.modulePath, modulePath)), "")
	}
	g.line("def trb_jobs_dispatch(__trb_scope, row)", "")
	g.indent++
	g.line("raise \"unsupported job payload version #{row[:payload_version]}\" unless row[:payload_version] == 1", "")
	g.line("arguments = JSON.parse(row[:payload])", "")
	g.line("case row[:job_name]", "")
	for _, job := range manifest.Jobs {
		g.line("when "+strconv.Quote(job.Name), "")
		g.indent++
		g.line("raise \"job "+job.Name+" expects "+strconv.Itoa(len(job.Parameters))+" arguments, got #{arguments.length}\" unless arguments.length == "+strconv.Itoa(len(job.Parameters)), "")
		call := job.Name + ".new.perform(*arguments)"
		if g.execution.Method(job.ModulePath, job.Name, "perform") {
			call = job.Name + ".new.perform(__trb_scope, *arguments)"
		}
		switch job.PerformKind {
		case jobs.PerformJobResult:
			g.line("execution = "+call, "")
			g.line("raise execution.error.message if execution.is_a?(Result::Err)", "")
		default:
			g.line(call, "")
		}
	}
	g.line("else", "")
	g.indent++
	g.line("raise \"unknown job #{row[:job_name]}\"", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.line("def trb_jobs_run_worker_or_command", "")
	g.indent++
	g.line("command = ENV[\"TRB_JOBS_COMMAND\"]", "")
	g.line("if command", "")
	g.indent++
	g.line("id = ENV[\"TRB_JOBS_ID\"]", "")
	g.line("case command; when \"list\"; TrbJobsRuntime.list.each { |row| puts [row[:id], row[:state], row[:job_name], \"#{row[:attempts]}/#{row[:maximum_attempts]}\", row[:last_error]].join(\"\\t\") }; when \"retry\"; unless TrbJobsRuntime.retry_job(id); warn \"trb jobs retry: failed job not found\"; exit(1); end; when \"discard\"; unless TrbJobsRuntime.discard(id); warn \"trb jobs discard: job not found or currently running\"; exit(1); end; end", "")
	g.line("return true", "")
	g.indent--
	g.line("end", "")
	g.line("return false unless ENV[\"TRB_JOBS_WORKER\"] == \"1\"", "")
	g.line("worker_id = \"worker-#{Process.pid}\"", "")
	g.line("stopping = false", "")
	g.line("worker_scope = TrbExecutionScope.root", "")
	g.line("Signal.trap(\"TERM\") { stopping = true; worker_scope.cancel }", "")
	g.line("Signal.trap(\"INT\") { stopping = true; worker_scope.cancel }", "")
	g.line("begin", "")
	g.indent++
	g.line("loop do", "")
	g.indent++
	g.line("break if stopping", "")
	g.line("TrbJobsRuntime.recover_stale", "")
	g.line("row = TrbJobsRuntime.claim(worker_id)", "")
	g.line("if !row; break if ENV[\"TRB_JOBS_ONCE\"] == \"1\"; sleep("+strconv.FormatFloat(float64(config.PollIntervalMilliseconds)/1000.0, 'f', 3, 64)+"); next; end", "")
	g.line("begin; trb_jobs_dispatch(worker_scope, row); TrbJobsRuntime.acknowledge(row[:id], worker_id); rescue StandardError => error; TrbJobsRuntime.fail(row[:id], worker_id, error.message); end", "")
	g.line("break if ENV[\"TRB_JOBS_ONCE\"] == \"1\"", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("ensure", "")
	g.indent++
	g.line("TrbJobsRuntime.release(worker_id)", "")
	g.line("TrbJobsRuntime.close", "")
	g.indent--
	g.line("end", "")
	g.line("true", "")
	g.indent--
	g.line("end", "")
}
