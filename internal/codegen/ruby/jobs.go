package ruby

import (
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/jobs"
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
	return identifier.Name == "queue" || identifier.Name == "priority"
}

func (g *generator) jobsRuntime(manifest *jobs.Manifest) {
	if manifest == nil {
		return
	}
	g.jobsClassEnqueueMethods(manifest)
	if g.modulePath == "trb/jobs/index" {
		g.jobsStorage(manifest.Config)
	}
	if g.topFunctions["main"] {
		g.jobsWorker(manifest)
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
		g.line("def self.perform_later("+strings.Join(parameters, ", ")+")", "")
		g.indent++
		g.line("payload = JSON.generate(["+strings.Join(parameters, ", ")+"])", "")
		g.line("reference = TrbJobsRuntime.enqueue("+strconv.Quote(job.Name)+", payload, "+strconv.Quote(job.Queue)+", "+strconv.Itoa(job.Priority)+", 0)", "")
		g.line("Result::Ok.new(reference)", "")
		g.line("rescue StandardError => error", "")
		g.line("Result::Err.new(EnqueueError.new(message: error.message))", "")
		g.indent--
		g.line("end", "")
		delayedParameters := append([]string{"delay"}, parameters...)
		g.line("def self.perform_later_in("+strings.Join(delayedParameters, ", ")+")", "")
		g.indent++
		g.line("payload = JSON.generate(["+strings.Join(parameters, ", ")+"])", "")
		g.line("wait_milliseconds = delay.whole_seconds * 1000 + (delay.nanosecond + 999999) / 1000000", "")
		g.line("reference = TrbJobsRuntime.enqueue("+strconv.Quote(job.Name)+", payload, "+strconv.Quote(job.Queue)+", "+strconv.Itoa(job.Priority)+", wait_milliseconds)", "")
		g.line("Result::Ok.new(reference)", "")
		g.line("rescue StandardError => error", "")
		g.line("Result::Err.new(EnqueueError.new(message: error.message))", "")
		g.indent--
		g.line("end", "")
		g.indent--
		g.line("end", "")
	}
}

func (g *generator) jobsStorage(config jobs.Config) {
	g.line("require \"json\"", "")
	g.line("require \"securerandom\"", "")
	g.line("require \"sequel\"", "")
	g.line("module TrbJobsRuntime", "")
	g.indent++
	g.line("module_function", "")
	g.line("def database", "")
	g.indent++
	g.line("return @database if @database", "")
	g.line("source = ENV.fetch(\"TRB_JOBS_DATABASE\", "+strconv.Quote(config.Database)+")", "")
	if config.DatabaseAdapter == "mysql" {
		g.line("source = source.sub(/\\Amysql:\\/\\//, \"mysql2://\")", "")
	}
	g.line("source = \"sqlite://#{File.expand_path(source)}\" unless source.include?(\"://\")", "")
	g.line("@database = Sequel.connect(source, max_connections: "+strconv.Itoa(max(config.WorkerConcurrency, 1))+" )", "")
	statements, _ := sqlstore.Schema(sqlstore.Dialect(config.DatabaseAdapter))
	for _, statement := range statements {
		g.line("@database.run("+strconv.Quote(statement)+")", "")
	}
	g.line("@database", "")
	g.indent--
	g.line("end", "")
	g.line("def close; @database.disconnect if @database; @database = nil; end", "")
	g.line("def enqueue(job_name, payload, queue_name, priority, wait_milliseconds)", "")
	g.indent++
	g.line("id = SecureRandom.hex(16)", "")
	g.line("now = Time.now.utc", "")
	g.line("raise \"job delay must not be negative\" if wait_milliseconds < 0", "")
	g.line("database[:trb_jobs].insert(id: id, queue_name: queue_name, job_name: job_name, payload: payload, payload_version: 1, priority: priority, run_at: now + wait_milliseconds / 1000.0, state: \"ready\", attempts: 0, maximum_attempts: "+strconv.Itoa(config.DefaultMaximumAttempts)+", created_at: now, updated_at: now)", "")
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
	if config.DatabaseAdapter != "sqlite" {
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

func (g *generator) jobsWorker(manifest *jobs.Manifest) {
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
	g.line("def trb_jobs_dispatch(row)", "")
	g.indent++
	g.line("raise \"unsupported job payload version #{row[:payload_version]}\" unless row[:payload_version] == 1", "")
	g.line("arguments = JSON.parse(row[:payload])", "")
	g.line("case row[:job_name]", "")
	for _, job := range manifest.Jobs {
		g.line("when "+strconv.Quote(job.Name), "")
		g.indent++
		g.line("raise \"job "+job.Name+" expects "+strconv.Itoa(len(job.Parameters))+" arguments, got #{arguments.length}\" unless arguments.length == "+strconv.Itoa(len(job.Parameters)), "")
		call := job.Name + ".new.perform(*arguments)"
		if job.Fails.Kind != "" && job.Fails.Kind != "Never" {
			g.line("execution = "+call, "")
			g.line("raise execution.error.inspect if execution.is_a?(Result::Err)", "")
		} else {
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
	g.line("case command; when \"list\"; TrbJobsRuntime.list.each { |row| puts [row[:id], row[:state], row[:job_name], \"#{row[:attempts]}/#{row[:maximum_attempts]}\", row[:last_error]].join(\"\\t\") }; when \"retry\"; warn \"trb jobs retry: failed job not found\" unless TrbJobsRuntime.retry_job(id); when \"discard\"; warn \"trb jobs discard: job not found or currently running\" unless TrbJobsRuntime.discard(id); end", "")
	g.line("return true", "")
	g.indent--
	g.line("end", "")
	g.line("return false unless ENV[\"TRB_JOBS_WORKER\"] == \"1\"", "")
	g.line("worker_id = \"worker-#{Process.pid}\"", "")
	g.line("stopping = false", "")
	g.line("Signal.trap(\"TERM\") { stopping = true }", "")
	g.line("Signal.trap(\"INT\") { stopping = true }", "")
	g.line("begin", "")
	g.indent++
	g.line("loop do", "")
	g.indent++
	g.line("break if stopping", "")
	g.line("TrbJobsRuntime.recover_stale", "")
	g.line("row = TrbJobsRuntime.claim(worker_id)", "")
	g.line("if !row; break if ENV[\"TRB_JOBS_ONCE\"] == \"1\"; sleep("+strconv.FormatFloat(float64(manifest.Config.PollIntervalMilliseconds)/1000.0, 'f', 3, 64)+"); next; end", "")
	g.line("begin; trb_jobs_dispatch(row); TrbJobsRuntime.acknowledge(row[:id], worker_id); rescue StandardError => error; TrbJobsRuntime.fail(row[:id], worker_id, error.message); end", "")
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
