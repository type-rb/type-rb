package golang

import (
	pathpkg "path"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/jobs"
	"github.com/type-rb/type-rb/internal/jobs/sqlstore"
)

func (g *generator) jobsRuntimeAlias() string {
	if g.modulePath == "trb/jobs/index" {
		return ""
	}
	for _, symbol := range []string{"Job", "JobReference", "EnqueueError"} {
		if alias := g.typeAliases[symbol]; alias != "" {
			return alias
		}
	}
	alias := "__trb_jobs"
	g.requireImport(pathpkg.Join(g.goModule, "trb/jobs"), alias)
	return alias
}

func (g *generator) jobsPerformLater(call *ir.Call, arguments []string) string {
	jobName := "Job"
	qualifier := ""
	if member, ok := call.Callee.(*ir.Member); ok {
		if identifier, identifierOK := member.Receiver.(*ir.Identifier); identifierOK {
			jobName = identifier.Name
			if alias := g.typeAliases[identifier.Name]; alias != "" {
				qualifier = alias + "."
			}
		}
	}
	return qualifier + goIdentifier(jobName, true) + goMethodName("perform_later") + "(" + strings.Join(arguments, ", ") + ")"
}

func (g *generator) jobsRuntime(manifest *jobs.Manifest) {
	if manifest == nil {
		return
	}
	g.jobsClassEnqueueMethods(manifest)
	if g.topMethods["main"] {
		g.jobsWorker(manifest)
	}
	if g.modulePath != "trb/jobs/index" {
		return
	}
	config := manifest.Config
	g.requireImport("context", "")
	g.requireImport("crypto/rand", "")
	g.requireImport("database/sql", "")
	g.requireImport("encoding/hex", "")
	g.requireImport("errors", "")
	g.requireImport("os", "")
	g.requireImport("sync", "")
	if config.DatabaseAdapter == "mysql" {
		g.requireImport("net/url", "")
		g.requireImport("strings", "")
	}
	switch config.DatabaseAdapter {
	case "sqlite":
		g.requireImport("modernc.org/sqlite", "_")
	case "postgresql":
		g.requireImport("github.com/jackc/pgx/v5/stdlib", "_")
	case "mysql":
		g.requireImport("github.com/go-sql-driver/mysql", "_")
	}

	g.line("type TrbJobsClaim struct {")
	g.indent++
	g.line("Id string")
	g.line("JobName string")
	g.line("Payload string")
	g.line("PayloadVersion int")
	g.line("Attempts int")
	g.line("MaximumAttempts int")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("type TrbJobsStatus struct {")
	g.indent++
	g.line("Id string")
	g.line("QueueName string")
	g.line("JobName string")
	g.line("State string")
	g.line("Attempts int")
	g.line("MaximumAttempts int")
	g.line("LastError sql.NullString")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("var trbJobsDatabase *sql.DB")
	g.line("var trbJobsDatabaseError error")
	g.line("var trbJobsDatabaseOnce sync.Once")
	g.b.WriteByte('\n')

	g.line("func trbJobsOpenDatabase() (*sql.DB, error) {")
	g.indent++
	g.line("trbJobsDatabaseOnce.Do(func() {")
	g.indent++
	g.line("source := os.Getenv(\"TRB_JOBS_DATABASE\")")
	g.line("if source == \"\" { source = " + strconv.Quote(config.Database) + " }")
	if config.DatabaseAdapter == "mysql" {
		g.line("if strings.HasPrefix(source, \"mysql://\") { parsed, err := url.Parse(source); if err != nil { trbJobsDatabaseError = err; return }; credentials := parsed.User.Username(); if password, exists := parsed.User.Password(); exists { credentials += \":\" + password }; source = credentials + \"@tcp(\" + parsed.Host + \")\" + parsed.Path; if parsed.RawQuery != \"\" { source += \"?\" + parsed.RawQuery } }")
	}
	g.line("trbJobsDatabase, trbJobsDatabaseError = sql.Open(" + strconv.Quote(goJobsDriver(config.DatabaseAdapter)) + ", source)")
	g.line("if trbJobsDatabaseError != nil { return }")
	if config.DatabaseAdapter == "sqlite" {
		g.line("trbJobsDatabase.SetMaxOpenConns(1)")
	}
	statements, _ := sqlstore.Schema(sqlstore.Dialect(config.DatabaseAdapter))
	for _, statement := range statements {
		g.line("if _, trbJobsDatabaseError = trbJobsDatabase.Exec(" + strconv.Quote(statement) + "); trbJobsDatabaseError != nil { return }")
	}
	g.indent--
	g.line("})")
	g.line("return trbJobsDatabase, trbJobsDatabaseError")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("func TrbJobsCloseDatabase() {")
	g.indent++
	g.line("if trbJobsDatabase != nil { _ = trbJobsDatabase.Close() }")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("func trbJobsId() (string, error) {")
	g.indent++
	g.line("value := make([]byte, 16)")
	g.line("if _, err := rand.Read(value); err != nil { return \"\", err }")
	g.line("return hex.EncodeToString(value), nil")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.jobsEnqueue(config)
	g.jobsClaim(config)
	g.jobsAcknowledge(config)
	g.jobsFail(config)
	g.jobsHeartbeat(config)
	g.jobsRecoverStale(config)
	g.jobsRelease(config)
	g.jobsAdmin(config)
}

func (g *generator) jobsHeartbeat(config jobs.Config) {
	g.line("func TrbJobsHeartbeat(ctx context.Context, id string, workerId string) error {")
	g.indent++
	g.line("database, err := trbJobsOpenDatabase()")
	g.line("if err != nil { return err }")
	query := "UPDATE trb_jobs SET claimed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = " + goJobsPlaceholder(config.DatabaseAdapter, 1) + " AND state = 'running' AND claimed_by = " + goJobsPlaceholder(config.DatabaseAdapter, 2)
	g.line("result, err := database.ExecContext(ctx, " + strconv.Quote(query) + ", id, workerId)")
	g.line("if err != nil { return err }")
	g.line("affected, err := result.RowsAffected()")
	g.line("if err != nil { return err }")
	g.line("if affected != 1 { return errors.New(\"job claim was lost before heartbeat\") }")
	g.line("return nil")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) jobsRecoverStale(config jobs.Config) {
	g.line("func TrbJobsRecoverStale(ctx context.Context) (int64, error) {")
	g.indent++
	g.line("database, err := trbJobsOpenDatabase()")
	g.line("if err != nil { return 0, err }")
	cutoff := goJobsStaleCutoff(config)
	query := "UPDATE trb_jobs SET state = 'ready', claimed_by = NULL, claimed_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE state = 'running' AND claimed_at < " + cutoff
	g.line("result, err := database.ExecContext(ctx, " + strconv.Quote(query) + ")")
	g.line("if err != nil { return 0, err }")
	g.line("return result.RowsAffected()")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) jobsAdmin(config jobs.Config) {
	g.line("func TrbJobsList(ctx context.Context) ([]TrbJobsStatus, error) {")
	g.indent++
	g.line("database, err := trbJobsOpenDatabase()")
	g.line("if err != nil { return nil, err }")
	g.line("rows, err := database.QueryContext(ctx, \"SELECT id, queue_name, job_name, state, attempts, maximum_attempts, last_error FROM trb_jobs ORDER BY created_at ASC, id ASC\")")
	g.line("if err != nil { return nil, err }")
	g.line("defer rows.Close()")
	g.line("statuses := []TrbJobsStatus{}")
	g.line("for rows.Next() { var status TrbJobsStatus; if err := rows.Scan(&status.Id, &status.QueueName, &status.JobName, &status.State, &status.Attempts, &status.MaximumAttempts, &status.LastError); err != nil { return nil, err }; statuses = append(statuses, status) }")
	g.line("if err := rows.Err(); err != nil { return nil, err }")
	g.line("return statuses, nil")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("func TrbJobsRetry(ctx context.Context, id string) (bool, error) {")
	g.indent++
	g.line("database, err := trbJobsOpenDatabase()")
	g.line("if err != nil { return false, err }")
	query := "UPDATE trb_jobs SET state = 'ready', attempts = 0, run_at = CURRENT_TIMESTAMP, claimed_by = NULL, claimed_at = NULL, last_error = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = " + goJobsPlaceholder(config.DatabaseAdapter, 1) + " AND state = 'failed'"
	g.line("result, err := database.ExecContext(ctx, " + strconv.Quote(query) + ", id)")
	g.line("if err != nil { return false, err }")
	g.line("affected, err := result.RowsAffected()")
	g.line("return affected == 1, err")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("func TrbJobsDiscard(ctx context.Context, id string) (bool, error) {")
	g.indent++
	g.line("database, err := trbJobsOpenDatabase()")
	g.line("if err != nil { return false, err }")
	query = "DELETE FROM trb_jobs WHERE id = " + goJobsPlaceholder(config.DatabaseAdapter, 1) + " AND state != 'running'"
	g.line("result, err := database.ExecContext(ctx, " + strconv.Quote(query) + ", id)")
	g.line("if err != nil { return false, err }")
	g.line("affected, err := result.RowsAffected()")
	g.line("return affected == 1, err")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) jobsWorker(manifest *jobs.Manifest) {
	jobsAlias := g.jobsRuntimeAlias()
	resultAlias := ""
	for _, job := range manifest.Jobs {
		if job.Fails.Kind == "" || job.Fails.Kind == "Never" {
			continue
		}
		resultAlias = g.typeAliases["Result"]
		if resultAlias == "" {
			resultAlias = "__trb_result"
			g.requireImport(pathpkg.Join(g.goModule, "trb/std/result"), resultAlias)
		}
		break
	}
	g.requireImport("context", "")
	g.requireImport("encoding/json", "")
	g.requireImport("fmt", "")
	g.requireImport("os", "")
	g.requireImport("os/signal", "")
	g.requireImport("strconv", "")
	g.requireImport("syscall", "")
	g.requireImport("time", "")

	aliases := g.jobsModuleAliases(manifest)
	g.line("func trbJobsDispatch(jobName string, payload string, payloadVersion int) (result error) {")
	g.indent++
	g.line("defer func() { if recovered := recover(); recovered != nil { result = fmt.Errorf(\"job panic: %v\", recovered) } }()")
	g.line("if payloadVersion != 1 { return fmt.Errorf(\"unsupported job payload version %d\", payloadVersion) }")
	g.line("var arguments []json.RawMessage")
	g.line("if err := json.Unmarshal([]byte(payload), &arguments); err != nil { return fmt.Errorf(\"decode job payload: %w\", err) }")
	g.line("switch jobName {")
	for _, job := range manifest.Jobs {
		g.line("case " + strconv.Quote(job.Name) + ":")
		g.indent++
		g.line("if len(arguments) != " + strconv.Itoa(len(job.Parameters)) + " { return fmt.Errorf(\"job " + job.Name + " expects " + strconv.Itoa(len(job.Parameters)) + " arguments, got %d\", len(arguments)) }")
		argumentNames := make([]string, len(job.Parameters))
		for index, parameter := range job.Parameters {
			argumentName := "argument" + strconv.Itoa(index)
			argumentNames[index] = argumentName
			g.line("var " + argumentName + " " + g.goType(parameter.Type))
			g.line("if err := json.Unmarshal(arguments[" + strconv.Itoa(index) + "], &" + argumentName + "); err != nil { return fmt.Errorf(\"decode " + job.Name + "." + parameter.Name + ": %w\", err) }")
		}
		qualifier := aliases[job.ModulePath]
		if qualifier != "" {
			qualifier += "."
		}
		call := qualifier + "New" + goIdentifier(job.Name, true) + "()." + goMethodName("perform") + "(" + strings.Join(argumentNames, ", ") + ")"
		if job.Fails.Kind != "" && job.Fails.Kind != "Never" {
			g.line("execution := " + call)
			g.line("if execution.Kind == " + resultAlias + ".ResultErrTag { return fmt.Errorf(\"%v\", execution.ErrError) }")
		} else {
			g.line(call)
		}
		g.line("return nil")
		g.indent--
	}
	g.line("default:")
	g.indent++
	g.line("return fmt.Errorf(\"unknown job %s\", jobName)")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	config := manifest.Config
	g.line("func trbJobsRunWorkerIfRequested() bool {")
	g.indent++
	g.line("command := os.Getenv(\"TRB_JOBS_COMMAND\")")
	g.line("if command != \"\" {")
	g.indent++
	g.line("id := os.Getenv(\"TRB_JOBS_ID\")")
	g.line("switch command {")
	g.line("case \"list\":")
	g.indent++
	g.line("statuses, err := " + jobsAlias + ".TrbJobsList(context.Background()); if err != nil { fmt.Fprintln(os.Stderr, \"trb jobs list:\", err); return true }; for _, status := range statuses { message := \"\"; if status.LastError.Valid { message = status.LastError.String }; fmt.Printf(\"%s\\t%s\\t%s\\t%d/%d\\t%s\\n\", status.Id, status.State, status.JobName, status.Attempts, status.MaximumAttempts, message) }")
	g.indent--
	g.line("case \"retry\":")
	g.indent++
	g.line("changed, err := " + jobsAlias + ".TrbJobsRetry(context.Background(), id); if err != nil { fmt.Fprintln(os.Stderr, \"trb jobs retry:\", err) } else if !changed { fmt.Fprintln(os.Stderr, \"trb jobs retry: failed job not found\") }")
	g.indent--
	g.line("case \"discard\":")
	g.indent++
	g.line("changed, err := " + jobsAlias + ".TrbJobsDiscard(context.Background(), id); if err != nil { fmt.Fprintln(os.Stderr, \"trb jobs discard:\", err) } else if !changed { fmt.Fprintln(os.Stderr, \"trb jobs discard: job not found or currently running\") }")
	g.indent--
	g.line("}")
	g.line("return true")
	g.indent--
	g.line("}")
	g.line("if os.Getenv(\"TRB_JOBS_WORKER\") != \"1\" { return false }")
	g.line("signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)")
	g.line("defer stop()")
	g.line("workerId := \"worker-\" + strconv.Itoa(os.Getpid())")
	g.line("defer func() { _ = " + jobsAlias + ".TrbJobsReleaseWorker(context.Background(), workerId); " + jobsAlias + ".TrbJobsCloseDatabase() }()")
	g.line("pollInterval := " + strconv.Itoa(config.PollIntervalMilliseconds) + " * time.Millisecond")
	g.line("heartbeatInterval := " + strconv.Itoa(max(config.LeaseTimeoutMilliseconds/3, 100)) + " * time.Millisecond")
	g.line("shutdownTimeout := " + strconv.Itoa(config.ShutdownTimeoutMilliseconds) + " * time.Millisecond")
	g.line("runOnce := os.Getenv(\"TRB_JOBS_ONCE\") == \"1\"")
	g.line("for {")
	g.indent++
	g.line("if signalContext.Err() != nil { return true }")
	g.line("if _, err := " + jobsAlias + ".TrbJobsRecoverStale(signalContext); err != nil { fmt.Fprintln(os.Stderr, \"trb jobs recover stale:\", err) }")
	g.line("claim, err := " + jobsAlias + ".TrbJobsClaimNext(signalContext, workerId)")
	g.line("if err != nil { fmt.Fprintln(os.Stderr, \"trb jobs claim:\", err); select { case <-signalContext.Done(): return true; case <-time.After(pollInterval): continue } }")
	g.line("if claim == nil { if runOnce { return true }; select { case <-signalContext.Done(): return true; case <-time.After(pollInterval): continue } }")
	g.line("execution := make(chan error, 1)")
	g.line("heartbeatDone := make(chan struct{})")
	g.line("go func() { ticker := time.NewTicker(heartbeatInterval); defer ticker.Stop(); for { select { case <-heartbeatDone: return; case <-ticker.C: if err := " + jobsAlias + ".TrbJobsHeartbeat(context.Background(), claim.Id, workerId); err != nil { fmt.Fprintln(os.Stderr, \"trb jobs heartbeat:\", err); return } } } }()")
	g.line("go func() { execution <- trbJobsDispatch(claim.JobName, claim.Payload, claim.PayloadVersion) }()")
	g.line("var executionError error")
	g.line("select {")
	g.line("case executionError = <-execution:")
	g.line("case <-signalContext.Done():")
	g.indent++
	g.line("select { case executionError = <-execution: case <-time.After(shutdownTimeout): close(heartbeatDone); return true }")
	g.indent--
	g.line("}")
	g.line("close(heartbeatDone)")
	g.line("if executionError != nil { if err := " + jobsAlias + ".TrbJobsFail(context.Background(), claim.Id, workerId, executionError.Error()); err != nil { fmt.Fprintln(os.Stderr, \"trb jobs fail:\", err) } } else if err := " + jobsAlias + ".TrbJobsAcknowledge(context.Background(), claim.Id, workerId); err != nil { fmt.Fprintln(os.Stderr, \"trb jobs acknowledge:\", err) }")
	g.line("if runOnce { return true }")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) jobsModuleAliases(manifest *jobs.Manifest) map[string]string {
	aliases := map[string]string{}
	directories := map[string]string{}
	for _, job := range manifest.Jobs {
		directory := pathpkg.Dir(job.ModulePath)
		if directory == "." || directory == g.currentDirectory() {
			aliases[job.ModulePath] = ""
			continue
		}
		alias := directories[directory]
		if alias == "" {
			alias = g.typeAliases[job.Name]
		}
		if alias == "" {
			alias = "trb_job_" + strconv.Itoa(len(directories))
			g.requireImport(pathpkg.Join(g.goModule, directory), alias)
		}
		directories[directory] = alias
		aliases[job.ModulePath] = alias
	}
	return aliases
}

func (g *generator) jobsClassEnqueueMethods(manifest *jobs.Manifest) {
	for _, job := range manifest.Jobs {
		if job.ModulePath != g.modulePath {
			continue
		}
		g.requireImport("encoding/json", "")
		resultAlias := g.typeAliases["Result"]
		if resultAlias == "" {
			resultAlias = "__trb_result"
			g.requireImport(pathpkg.Join(g.goModule, "trb/std/result"), resultAlias)
		}
		jobsAlias := g.jobsRuntimeAlias()
		successType := jobsAlias + ".JobReference"
		errorType := jobsAlias + ".EnqueueError"
		parameters := make([]string, len(job.Parameters))
		arguments := make([]string, len(job.Parameters))
		for index, parameter := range job.Parameters {
			name := goBindingIdentifier(parameter.Name)
			parameters[index] = name + " " + g.goType(parameter.Type)
			arguments[index] = name
		}
		resultType := resultAlias + ".Result[" + successType + ", " + errorType + "]"
		functionName := goIdentifier(job.Name, true) + goMethodName("perform_later")
		g.line("func " + functionName + "(" + strings.Join(parameters, ", ") + ") " + resultType + " {")
		g.indent++
		g.line("payload, err := json.Marshal([]any{" + strings.Join(arguments, ", ") + "})")
		g.line("if err != nil { return " + resultAlias + ".NewResultErr[" + successType + ", " + errorType + "](" + errorType + "{Message: err.Error()}) }")
		g.line("reference, err := " + jobsAlias + ".TrbJobsEnqueue(" + strconv.Quote(job.Name) + ", string(payload))")
		g.line("if err != nil { return " + resultAlias + ".NewResultErr[" + successType + ", " + errorType + "](" + errorType + "{Message: err.Error()}) }")
		g.line("return " + resultAlias + ".NewResultOk[" + successType + ", " + errorType + "](reference)")
		g.indent--
		g.line("}")
		g.b.WriteByte('\n')
	}
}

func (g *generator) jobsEnqueue(config jobs.Config) {
	g.line("func TrbJobsEnqueue(jobName string, payload string) (JobReference, error) {")
	g.indent++
	g.line("database, err := trbJobsOpenDatabase()")
	g.line("if err != nil { return JobReference{}, err }")
	g.line("id, err := trbJobsId()")
	g.line("if err != nil { return JobReference{}, err }")
	query := "INSERT INTO trb_jobs (id, queue_name, job_name, payload, payload_version, priority, run_at, state, attempts, maximum_attempts, created_at, updated_at) VALUES (" + goJobsPlaceholders(config.DatabaseAdapter, 6, 0) + ", CURRENT_TIMESTAMP, 'ready', 0, " + goJobsPlaceholder(config.DatabaseAdapter, 7) + ", CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"
	g.line("_, err = database.Exec(" + strconv.Quote(query) + ", id, \"default\", jobName, payload, 1, 0, " + strconv.Itoa(config.DefaultMaximumAttempts) + ")")
	g.line("if err != nil { return JobReference{}, err }")
	g.line("return JobReference{Id: id, JobName: jobName}, nil")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) jobsClaim(config jobs.Config) {
	selection, _ := sqlstore.ClaimSelection(sqlstore.Dialect(config.DatabaseAdapter))
	g.line("func TrbJobsClaimNext(ctx context.Context, workerId string) (*TrbJobsClaim, error) {")
	g.indent++
	g.line("database, err := trbJobsOpenDatabase()")
	g.line("if err != nil { return nil, err }")
	g.line("transaction, err := database.BeginTx(ctx, nil)")
	g.line("if err != nil { return nil, err }")
	g.line("defer func() { _ = transaction.Rollback() }()")
	g.line("var id string")
	g.line("if err = transaction.QueryRowContext(ctx, " + strconv.Quote(selection) + ").Scan(&id); errors.Is(err, sql.ErrNoRows) { return nil, nil } else if err != nil { return nil, err }")
	update := "UPDATE trb_jobs SET state = 'running', attempts = attempts + 1, claimed_by = " + goJobsPlaceholder(config.DatabaseAdapter, 1) + ", claimed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = " + goJobsPlaceholder(config.DatabaseAdapter, 2) + " AND state = 'ready'"
	g.line("result, err := transaction.ExecContext(ctx, " + strconv.Quote(update) + ", workerId, id)")
	g.line("if err != nil { return nil, err }")
	g.line("affected, err := result.RowsAffected()")
	g.line("if err != nil { return nil, err }")
	g.line("if affected != 1 { return nil, nil }")
	g.line("claim := &TrbJobsClaim{Id: id}")
	read := "SELECT job_name, payload, payload_version, attempts, maximum_attempts FROM trb_jobs WHERE id = " + goJobsPlaceholder(config.DatabaseAdapter, 1)
	g.line("if err = transaction.QueryRowContext(ctx, " + strconv.Quote(read) + ", id).Scan(&claim.JobName, &claim.Payload, &claim.PayloadVersion, &claim.Attempts, &claim.MaximumAttempts); err != nil { return nil, err }")
	g.line("if err = transaction.Commit(); err != nil { return nil, err }")
	g.line("return claim, nil")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) jobsAcknowledge(config jobs.Config) {
	g.line("func TrbJobsAcknowledge(ctx context.Context, id string, workerId string) error {")
	g.indent++
	g.line("database, err := trbJobsOpenDatabase()")
	g.line("if err != nil { return err }")
	query := "DELETE FROM trb_jobs WHERE id = " + goJobsPlaceholder(config.DatabaseAdapter, 1) + " AND state = 'running' AND claimed_by = " + goJobsPlaceholder(config.DatabaseAdapter, 2)
	g.line("result, err := database.ExecContext(ctx, " + strconv.Quote(query) + ", id, workerId)")
	g.line("if err != nil { return err }")
	g.line("affected, err := result.RowsAffected()")
	g.line("if err != nil { return err }")
	g.line("if affected != 1 { return errors.New(\"job claim was lost before acknowledgement\") }")
	g.line("return nil")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) jobsFail(config jobs.Config) {
	g.line("func TrbJobsFail(ctx context.Context, id string, workerId string, message string) error {")
	g.indent++
	g.line("database, err := trbJobsOpenDatabase()")
	g.line("if err != nil { return err }")
	query := "UPDATE trb_jobs SET state = CASE WHEN attempts >= maximum_attempts THEN 'failed' ELSE 'ready' END, run_at = " + goJobsRetryTime(config) + ", claimed_by = NULL, claimed_at = NULL, last_error = " + goJobsPlaceholder(config.DatabaseAdapter, 1) + ", updated_at = CURRENT_TIMESTAMP WHERE id = " + goJobsPlaceholder(config.DatabaseAdapter, 2) + " AND state = 'running' AND claimed_by = " + goJobsPlaceholder(config.DatabaseAdapter, 3)
	g.line("result, err := database.ExecContext(ctx, " + strconv.Quote(query) + ", message, id, workerId)")
	g.line("if err != nil { return err }")
	g.line("affected, err := result.RowsAffected()")
	g.line("if err != nil { return err }")
	g.line("if affected != 1 { return errors.New(\"job claim was lost before failure recording\") }")
	g.line("return nil")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) jobsRelease(config jobs.Config) {
	g.line("func TrbJobsReleaseWorker(ctx context.Context, workerId string) error {")
	g.indent++
	g.line("database, err := trbJobsOpenDatabase()")
	g.line("if err != nil { return err }")
	query := "UPDATE trb_jobs SET state = 'ready', claimed_by = NULL, claimed_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE state = 'running' AND claimed_by = " + goJobsPlaceholder(config.DatabaseAdapter, 1)
	g.line("_, err = database.ExecContext(ctx, " + strconv.Quote(query) + ", workerId)")
	g.line("return err")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func goJobsDriver(adapter string) string {
	switch adapter {
	case "postgresql":
		return "pgx"
	case "mysql":
		return "mysql"
	default:
		return "sqlite"
	}
}

func goJobsStaleCutoff(config jobs.Config) string {
	milliseconds := config.LeaseTimeoutMilliseconds
	switch config.DatabaseAdapter {
	case "postgresql":
		return "CURRENT_TIMESTAMP - INTERVAL '" + strconv.Itoa(milliseconds) + " milliseconds'"
	case "mysql":
		return "CURRENT_TIMESTAMP(6) - INTERVAL " + strconv.Itoa(milliseconds*1000) + " MICROSECOND"
	default:
		seconds := max((milliseconds+999)/1000, 1)
		return "datetime(CURRENT_TIMESTAMP, '-" + strconv.Itoa(seconds) + " seconds')"
	}
}

func goJobsRetryTime(config jobs.Config) string {
	switch config.DatabaseAdapter {
	case "postgresql":
		return "CURRENT_TIMESTAMP + (attempts * INTERVAL '" + strconv.Itoa(config.RetryBaseDelayMilliseconds) + " milliseconds')"
	case "mysql":
		return "TIMESTAMPADD(MICROSECOND, attempts * " + strconv.Itoa(config.RetryBaseDelayMilliseconds*1000) + ", CURRENT_TIMESTAMP(6))"
	default:
		seconds := max((config.RetryBaseDelayMilliseconds+999)/1000, 1)
		return "datetime(CURRENT_TIMESTAMP, '+' || (attempts * " + strconv.Itoa(seconds) + ") || ' seconds')"
	}
}

func goJobsPlaceholder(adapter string, index int) string {
	if adapter == "postgresql" {
		return "$" + strconv.Itoa(index)
	}
	return "?"
}

func goJobsPlaceholders(adapter string, count, offset int) string {
	values := make([]string, count)
	for index := range values {
		values[index] = goJobsPlaceholder(adapter, offset+index+1)
	}
	return strings.Join(values, ", ")
}
