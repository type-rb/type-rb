package typescript

import (
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/jobs"
	jobssql "github.com/type-rb/type-rb/internal/jobs/sqladapter"
	"github.com/type-rb/type-rb/internal/jobs/sqlstore"
)

func (g *generator) jobsIntegrationImports(manifest *jobs.Manifest) {
	if manifest == nil || g.jobsSQL == nil {
		return
	}
	if g.modulePath == jobssql.ModulePath {
		g.line("import { SQL, type TransactionSQL } from \"bun\";")
	}
	jobModule := false
	for _, job := range manifest.Jobs {
		if job.ModulePath == g.modulePath {
			jobModule = true
			break
		}
	}
	if jobModule {
		g.line("import * as __trbJobResult from " + strconv.Quote(tsImportPath(g.modulePath, "trb/std/result/index")) + ";")
		g.line("import * as __trbJobsRuntime from " + strconv.Quote(tsImportPath(g.modulePath, jobssql.ModulePath)) + ";")
	}
	if !g.topFunctions["main"] {
		return
	}
	if !jobModule {
		g.line("import * as __trbJobsRuntime from " + strconv.Quote(tsImportPath(g.modulePath, jobssql.ModulePath)) + ";")
	}
	modulePaths := []string{}
	seen := map[string]bool{}
	for _, job := range manifest.Jobs {
		if job.ModulePath == g.modulePath || seen[job.ModulePath] {
			continue
		}
		seen[job.ModulePath] = true
		modulePaths = append(modulePaths, job.ModulePath)
	}
	sort.Strings(modulePaths)
	for index, modulePath := range modulePaths {
		g.line("import * as __trbJob" + strconv.Itoa(index) + " from " + strconv.Quote(tsImportPath(g.modulePath, modulePath)) + ";")
	}
}

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
		arguments = append([]string{"__trbScope"}, arguments...)
	}
	return jobName + "." + tsMethodName(method) + "(" + strings.Join(arguments, ", ") + ")"
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
	if g.topFunctions["main"] {
		g.jobsWorker(manifest, g.jobsSQL.Config)
	}
}

func (g *generator) jobsClassEnqueueMethods(manifest *jobs.Manifest) {
	for _, job := range manifest.Jobs {
		if job.ModulePath != g.modulePath {
			continue
		}
		parameters := make([]string, len(job.Parameters))
		arguments := make([]string, len(job.Parameters))
		for index, parameter := range job.Parameters {
			parameters[index] = parameter.Name + ": " + g.tsType(parameter.Type)
			arguments[index] = parameter.Name
		}
		g.line("export namespace " + job.Name + " {")
		g.indent++
		emit := func(method string, methodParameters []string, waitLines ...string) {
			g.line("export async function " + tsMethodName(method) + "(__trbScope: AbortSignal | undefined" + tsJobsParameters(methodParameters) + "): Promise<__trbJobResult.Result<JobReference, EnqueueError>> {")
			g.indent++
			for _, line := range waitLines {
				g.line(line)
			}
			g.line("let payload: string;")
			g.line("try { payload = JSON.stringify([" + strings.Join(arguments, ", ") + "]); } catch (error) { return __trbJobResult.Result.Err<JobReference, EnqueueError>({ kind: EnqueueErrorKind.Serialization, message: error instanceof Error ? error.message : String(error) }); }")
			g.line("try {")
			g.indent++
			g.line("const reference = await __trbJobsRuntime.trbJobsEnqueue(__trbScope, " + strconv.Quote(job.Name) + ", payload, " + strconv.Quote(job.Queue) + ", " + strconv.Itoa(job.Priority) + ", waitMilliseconds, " + strconv.Itoa(job.MaximumAttempts) + ");")
			g.line("return __trbJobResult.Result.Ok<JobReference, EnqueueError>(reference);")
			g.indent--
			g.line("} catch (error) {")
			g.indent++
			g.line("return __trbJobResult.Result.Err<JobReference, EnqueueError>({ kind: __trbScope?.aborted ? EnqueueErrorKind.Cancelled : EnqueueErrorKind.Adapter, message: error instanceof Error ? error.message : String(error) });")
			g.indent--
			g.line("}")
			g.indent--
			g.line("}")
		}
		emit("perform_later", parameters, "const waitMilliseconds = 0;")
		emit("perform_in", append([]string{"delay: Duration"}, parameters...),
			"const waitMilliseconds = delay.whole_seconds() * 1000 + Math.ceil(delay.nanosecond() / 1000000);",
			"if (waitMilliseconds < 0) return __trbJobResult.Result.Err<JobReference, EnqueueError>({ kind: EnqueueErrorKind.InvalidArgument, message: \"job delay must not be negative\" });",
		)
		emit("perform_at", append([]string{"scheduled_at: Instant"}, parameters...),
			"const targetMilliseconds = scheduled_at.epoch_seconds() * 1000 + Math.ceil(scheduled_at.nanosecond() / 1000000);",
			"const waitMilliseconds = Math.max(targetMilliseconds - globalThis.Date.now(), 0);",
		)
		g.indent--
		g.line("}")
	}
}

func tsJobsParameters(parameters []string) string {
	if len(parameters) == 0 {
		return ""
	}
	return ", " + strings.Join(parameters, ", ")
}

func (g *generator) jobsStorage(config jobssql.Config) {
	statements, _ := sqlstore.Schema(sqlstore.Dialect(config.Dialect))
	selection, _ := sqlstore.ClaimSelection(sqlstore.Dialect(config.Dialect))
	queueSelection, _ := sqlstore.ClaimSelectionForQueue(sqlstore.Dialect(config.Dialect), tsJobsPlaceholder(config.Dialect, 1))
	g.line("export type TrbJobsClaim = { id: string; job_name: string; payload: string; payload_version: number; attempts: number; maximum_attempts: number };")
	g.line("export type TrbJobsStatus = { id: string; queue_name: string; job_name: string; state: string; attempts: number; maximum_attempts: number; last_error: string | null };")
	g.line("const trbJobsAdapter = " + strconv.Quote(config.Dialect) + ";")
	g.line("const trbJobsConfiguredDatabase = " + strconv.Quote(config.Source) + ";")
	g.line("let trbJobsDatabase: SQL | null = null;")
	g.line("let trbJobsSchema: Promise<void> | null = null;")
	if config.SourceEnvironment == "" {
		g.line("function trbJobsDB(): SQL { if (trbJobsDatabase !== null) return trbJobsDatabase; const source = trbJobsConfiguredDatabase; trbJobsDatabase = trbJobsAdapter === \"sqlite\" ? new SQL({ adapter: \"sqlite\", filename: source }) : new SQL(source); return trbJobsDatabase; }")
	} else {
		g.line("function trbJobsDB(): SQL { if (trbJobsDatabase !== null) return trbJobsDatabase; const source = process.env[" + strconv.Quote(config.SourceEnvironment) + "]; if (source === undefined || source.trim() === \"\") throw new Error(" + strconv.Quote("jobs database environment "+config.SourceEnvironment+" is not set or empty") + "); trbJobsDatabase = trbJobsAdapter === \"sqlite\" ? new SQL({ adapter: \"sqlite\", filename: source }) : new SQL(source); return trbJobsDatabase; }")
	}
	g.line("async function trbJobsEnsureSchema(): Promise<void> { if (trbJobsSchema !== null) return trbJobsSchema; trbJobsSchema = (async () => {")
	g.indent++
	for _, statement := range statements {
		g.line("await trbJobsDB().unsafe(" + strconv.Quote(statement) + ", []);")
	}
	g.indent--
	g.line("})(); return trbJobsSchema; }")
	g.line("function trbJobsAffected(rows: any): number { return Number(rows.affectedRows ?? rows.count ?? rows.changes ?? rows.length ?? 0); }")
	g.line("export async function trbJobsClose(): Promise<void> { if (trbJobsDatabase !== null) await trbJobsDatabase.close(); trbJobsDatabase = null; trbJobsSchema = null; }")

	insert := "INSERT INTO trb_jobs (id, queue_name, job_name, payload, payload_version, priority, run_at, state, attempts, maximum_attempts, created_at, updated_at) VALUES (" + tsJobsPlaceholders(config.Dialect, 7, 0) + ", 'ready', 0, " + tsJobsPlaceholder(config.Dialect, 8) + ", CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"
	g.line("export async function trbJobsEnqueue(signal: AbortSignal | undefined, jobName: string, payload: string, queueName: string, priority: number, waitMilliseconds: number, maximumAttempts: number): Promise<JobReference> { if (signal?.aborted) throw signal.reason ?? new DOMException(\"TypeRB execution was cancelled\", \"AbortError\"); if (waitMilliseconds < 0) throw new Error(\"job delay must not be negative\"); if (maximumAttempts <= 0) maximumAttempts = " + strconv.Itoa(config.DefaultMaximumAttempts) + "; await trbJobsEnsureSchema(); const id = crypto.randomUUID().replaceAll(\"-\", \"\"); const timestamp = new Date(Date.now() + waitMilliseconds).toISOString(); const runAt = timestamp.slice(0, waitMilliseconds === 0 ? 19 : 23).replace(\"T\", \" \" ); await trbJobsDB().unsafe(" + strconv.Quote(insert) + ", [id, queueName, jobName, payload, 1, priority, runAt, maximumAttempts]); if (signal?.aborted) throw signal.reason ?? new DOMException(\"TypeRB execution was cancelled\", \"AbortError\"); return { id, job_name: jobName }; }")

	update := "UPDATE trb_jobs SET state = 'running', attempts = attempts + 1, claimed_by = " + tsJobsPlaceholder(config.Dialect, 1) + ", claimed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = " + tsJobsPlaceholder(config.Dialect, 2) + " AND state = 'ready'"
	read := "SELECT job_name, payload, payload_version, attempts, maximum_attempts FROM trb_jobs WHERE id = " + tsJobsPlaceholder(config.Dialect, 1)
	g.line("export async function trbJobsClaim(workerId: string): Promise<TrbJobsClaim | null> { await trbJobsEnsureSchema(); const run = async (transaction: SQL | TransactionSQL): Promise<TrbJobsClaim | null> => { const queueName = process.env.TRB_JOBS_QUEUE ?? \"\"; const selected = await transaction.unsafe(queueName === \"\" ? " + strconv.Quote(selection) + " : " + strconv.Quote(queueSelection) + ", queueName === \"\" ? [] : [queueName]) as any[]; if (selected.length === 0) return null; const id = String(selected[0]!.id); const updated = await transaction.unsafe(" + strconv.Quote(update) + ", [workerId, id]); if (trbJobsAffected(updated) !== 1) return null; const rows = await transaction.unsafe(" + strconv.Quote(read) + ", [id]) as any[]; const row = rows[0]!; return { id, job_name: String(row.job_name), payload: typeof row.payload === \"string\" ? row.payload : JSON.stringify(row.payload), payload_version: Number(row.payload_version), attempts: Number(row.attempts), maximum_attempts: Number(row.maximum_attempts) }; }; return trbJobsAdapter === \"sqlite\" ? await trbJobsDB().begin(\"immediate\", run) : await trbJobsDB().begin(run); }")

	ack := "DELETE FROM trb_jobs WHERE id = " + tsJobsPlaceholder(config.Dialect, 1) + " AND state = 'running' AND claimed_by = " + tsJobsPlaceholder(config.Dialect, 2)
	g.line("export async function trbJobsAcknowledge(id: string, workerId: string): Promise<void> { const result = await trbJobsDB().unsafe(" + strconv.Quote(ack) + ", [id, workerId]); if (trbJobsAffected(result) !== 1) throw new Error(\"job claim was lost before acknowledgement\"); }")
	fail := "UPDATE trb_jobs SET state = CASE WHEN attempts >= maximum_attempts THEN 'failed' ELSE 'ready' END, run_at = " + tsJobsRetryTime(config) + ", claimed_by = NULL, claimed_at = NULL, last_error = " + tsJobsPlaceholder(config.Dialect, 1) + ", updated_at = CURRENT_TIMESTAMP WHERE id = " + tsJobsPlaceholder(config.Dialect, 2) + " AND state = 'running' AND claimed_by = " + tsJobsPlaceholder(config.Dialect, 3)
	g.line("export async function trbJobsFail(id: string, workerId: string, message: string): Promise<void> { const result = await trbJobsDB().unsafe(" + strconv.Quote(fail) + ", [message, id, workerId]); if (trbJobsAffected(result) !== 1) throw new Error(\"job claim was lost before failure recording\"); }")
	heartbeat := "UPDATE trb_jobs SET claimed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = " + tsJobsPlaceholder(config.Dialect, 1) + " AND state = 'running' AND claimed_by = " + tsJobsPlaceholder(config.Dialect, 2)
	g.line("export async function trbJobsHeartbeat(id: string, workerId: string): Promise<void> { const result = await trbJobsDB().unsafe(" + strconv.Quote(heartbeat) + ", [id, workerId]); if (trbJobsAffected(result) !== 1) throw new Error(\"job claim was lost before heartbeat\"); }")
	release := "UPDATE trb_jobs SET state = 'ready', claimed_by = NULL, claimed_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE state = 'running' AND claimed_by = " + tsJobsPlaceholder(config.Dialect, 1)
	g.line("export async function trbJobsRelease(workerId: string): Promise<void> { await trbJobsDB().unsafe(" + strconv.Quote(release) + ", [workerId]); }")
	recover := "UPDATE trb_jobs SET state = 'ready', claimed_by = NULL, claimed_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE state = 'running' AND claimed_at < " + tsJobsStaleCutoff(config)
	g.line("export async function trbJobsRecoverStale(): Promise<void> { await trbJobsEnsureSchema(); await trbJobsDB().unsafe(" + strconv.Quote(recover) + ", []); }")
	g.line("export async function trbJobsList(): Promise<TrbJobsStatus[]> { await trbJobsEnsureSchema(); return await trbJobsDB().unsafe(\"SELECT id, queue_name, job_name, state, attempts, maximum_attempts, last_error FROM trb_jobs ORDER BY created_at ASC, id ASC\", []) as TrbJobsStatus[]; }")
	retry := "UPDATE trb_jobs SET state = 'ready', attempts = 0, run_at = CURRENT_TIMESTAMP, claimed_by = NULL, claimed_at = NULL, last_error = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = " + tsJobsPlaceholder(config.Dialect, 1) + " AND state = 'failed'"
	g.line("export async function trbJobsRetry(id: string): Promise<boolean> { return trbJobsAffected(await trbJobsDB().unsafe(" + strconv.Quote(retry) + ", [id])) === 1; }")
	discard := "DELETE FROM trb_jobs WHERE id = " + tsJobsPlaceholder(config.Dialect, 1) + " AND state != 'running'"
	g.line("export async function trbJobsDiscard(id: string): Promise<boolean> { return trbJobsAffected(await trbJobsDB().unsafe(" + strconv.Quote(discard) + ", [id])) === 1; }")
}

func (g *generator) jobsWorker(manifest *jobs.Manifest, config jobssql.Config) {
	aliases := g.jobsModuleAliases(manifest)
	g.line("async function trbJobsDispatch(__trbScope: AbortSignal | undefined, claim: __trbJobsRuntime.TrbJobsClaim): Promise<void> {")
	g.indent++
	g.line("if (claim.payload_version !== 1) throw new Error(\"unsupported job payload version \" + claim.payload_version);")
	g.line("const parameters = JSON.parse(claim.payload) as unknown[];")
	g.line("switch (claim.job_name) {")
	for _, job := range manifest.Jobs {
		g.line("case " + strconv.Quote(job.Name) + ": {")
		g.indent++
		g.line("if (parameters.length !== " + strconv.Itoa(len(job.Parameters)) + ") throw new Error(\"job " + job.Name + " expects " + strconv.Itoa(len(job.Parameters)) + " arguments, got \" + parameters.length);")
		argumentValues := make([]string, len(job.Parameters))
		for index, parameter := range job.Parameters {
			argumentValues[index] = "parameters[" + strconv.Itoa(index) + "] as " + g.tsType(parameter.Type)
		}
		owner := job.Name
		if alias := aliases[job.ModulePath]; alias != "" {
			owner = alias + "." + job.Name
		}
		if g.execution.Method(job.ModulePath, job.Name, "perform") {
			argumentValues = append([]string{"__trbScope"}, argumentValues...)
		}
		call := "await new " + owner + "()." + tsMethodName("perform") + "(" + strings.Join(argumentValues, ", ") + ")"
		if job.Fails.Kind != "" && job.Fails.Kind != "Never" {
			g.line("const execution = " + call + "; if (execution.kind === \"Err\") throw new Error(String(execution.error));")
		} else {
			g.line(call + ";")
		}
		g.line("return;")
		g.indent--
		g.line("}")
	}
	g.line("default: throw new Error(\"unknown job \" + claim.job_name);")
	g.indent--
	g.line("}")
	g.line("}")

	g.line("async function trbJobsRunWorkerOrCommand(): Promise<boolean> {")
	g.indent++
	g.line("const command = process.env.TRB_JOBS_COMMAND;")
	g.line("if (command !== undefined && command !== \"\") { const id = process.env.TRB_JOBS_ID ?? \"\"; if (command === \"list\") for (const status of await __trbJobsRuntime.trbJobsList()) console.log([status.id, status.state, status.job_name, status.attempts + \"/\" + status.maximum_attempts, status.last_error ?? \"\"].join(\"\\t\")); else if (command === \"retry\" && !(await __trbJobsRuntime.trbJobsRetry(id))) { console.error(\"trb jobs retry: failed job not found\"); process.exitCode = 1; } else if (command === \"discard\" && !(await __trbJobsRuntime.trbJobsDiscard(id))) { console.error(\"trb jobs discard: job not found or currently running\"); process.exitCode = 1; } return true; }")
	g.line("if (process.env.TRB_JOBS_WORKER !== \"1\") return false;")
	g.line("const workerId = \"worker-\" + process.pid; let stopping = false; const workerController = new AbortController(); process.on(\"SIGTERM\", () => { stopping = true; workerController.abort(); }); process.on(\"SIGINT\", () => { stopping = true; workerController.abort(); });")
	g.line("try {")
	g.indent++
	g.line("while (!stopping) {")
	g.indent++
	g.line("await __trbJobsRuntime.trbJobsRecoverStale(); const claim = await __trbJobsRuntime.trbJobsClaim(workerId); if (claim === null) { if (process.env.TRB_JOBS_ONCE === \"1\") break; await Bun.sleep(" + strconv.Itoa(config.PollIntervalMilliseconds) + "); continue; }")
	g.line("const heartbeat = setInterval(() => { void __trbJobsRuntime.trbJobsHeartbeat(claim.id, workerId).catch(error => console.error(\"trb jobs heartbeat:\", error)); }, " + strconv.Itoa(max(config.LeaseTimeoutMilliseconds/3, 100)) + ");")
	g.line("try { await trbJobsDispatch(workerController.signal, claim); await __trbJobsRuntime.trbJobsAcknowledge(claim.id, workerId); } catch (error) { await __trbJobsRuntime.trbJobsFail(claim.id, workerId, error instanceof Error ? error.message : String(error)); } finally { clearInterval(heartbeat); }")
	g.line("if (process.env.TRB_JOBS_ONCE === \"1\") break;")
	g.indent--
	g.line("}")
	g.indent--
	g.line("} finally { await __trbJobsRuntime.trbJobsRelease(workerId); await __trbJobsRuntime.trbJobsClose(); }")
	g.line("return true;")
	g.indent--
	g.line("}")
}

func (g *generator) jobsModuleAliases(manifest *jobs.Manifest) map[string]string {
	aliases := map[string]string{}
	modulePaths := []string{}
	seen := map[string]bool{}
	for _, job := range manifest.Jobs {
		if job.ModulePath == g.modulePath {
			aliases[job.ModulePath] = ""
			continue
		}
		if !seen[job.ModulePath] {
			seen[job.ModulePath] = true
			modulePaths = append(modulePaths, job.ModulePath)
		}
	}
	sort.Strings(modulePaths)
	for index, modulePath := range modulePaths {
		aliases[modulePath] = "__trbJob" + strconv.Itoa(index)
	}
	return aliases
}

func tsJobsPlaceholder(adapter string, index int) string {
	if adapter == "postgresql" {
		return "$" + strconv.Itoa(index)
	}
	return "?"
}

func tsJobsPlaceholders(adapter string, count, offset int) string {
	values := make([]string, count)
	for index := range values {
		values[index] = tsJobsPlaceholder(adapter, offset+index+1)
	}
	return strings.Join(values, ", ")
}

func tsJobsStaleCutoff(config jobssql.Config) string {
	switch config.Dialect {
	case "postgresql":
		return "CURRENT_TIMESTAMP - INTERVAL '" + strconv.Itoa(config.LeaseTimeoutMilliseconds) + " milliseconds'"
	case "mysql":
		return "CURRENT_TIMESTAMP(6) - INTERVAL " + strconv.Itoa(config.LeaseTimeoutMilliseconds*1000) + " MICROSECOND"
	default:
		return "datetime(CURRENT_TIMESTAMP, '-" + strconv.Itoa(max((config.LeaseTimeoutMilliseconds+999)/1000, 1)) + " seconds')"
	}
}

func tsJobsRetryTime(config jobssql.Config) string {
	switch config.Dialect {
	case "postgresql":
		return "CURRENT_TIMESTAMP + (attempts * INTERVAL '" + strconv.Itoa(config.RetryBaseDelayMilliseconds) + " milliseconds')"
	case "mysql":
		return "TIMESTAMPADD(MICROSECOND, attempts * " + strconv.Itoa(config.RetryBaseDelayMilliseconds*1000) + ", CURRENT_TIMESTAMP(6))"
	default:
		seconds := max((config.RetryBaseDelayMilliseconds+999)/1000, 1)
		return "datetime(CURRENT_TIMESTAMP, '+' || (attempts * " + strconv.Itoa(seconds) + ") || ' seconds')"
	}
}
