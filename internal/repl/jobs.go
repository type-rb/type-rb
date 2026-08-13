//go:build !js || !wasm

package repl

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/type-rb/type-rb/internal/ir"
	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
	jobssql "github.com/type-rb/type-rb/internal/jobs/sqladapter"
	"github.com/type-rb/type-rb/internal/jobs/sqlstore"
	"github.com/type-rb/type-rb/internal/types"
)

type jobsRuntimeProvider struct {
	manifest *jobsintegration.Manifest
	SQL      *jobssql.Manifest
	database *sql.DB
}

func init() {
	registerRuntimeProvider(func() runtimeProvider { return &jobsRuntimeProvider{} })
}

func (*jobsRuntimeProvider) Name() string { return "trb/jobs" }

func (*jobsRuntimeProvider) Handles(intrinsic string) bool {
	return intrinsic == "trb.jobs.perform_later" || intrinsic == "trb.jobs.perform_in" || intrinsic == "trb.jobs.perform_at"
}

func (provider *jobsRuntimeProvider) Configure(programs []*ir.Program) error {
	var manifest *jobsintegration.Manifest
	var SQLManifest *jobssql.Manifest
	for _, program := range programs {
		if current := jobsintegration.ManifestFrom(program.Extensions); current != nil {
			manifest = current
		}
		if current := jobssql.ManifestFrom(program.Extensions); current != nil {
			SQLManifest = current
		}
	}
	if manifest == nil {
		return nil
	}
	if SQLManifest == nil {
		return fmt.Errorf("trb/jobs has no configured adapter")
	}
	if provider.manifest != nil && provider.SQL != nil && provider.SQL.Config == SQLManifest.Config {
		provider.manifest = manifest
		provider.SQL = SQLManifest
		return nil
	}
	if provider.database != nil {
		_ = provider.database.Close()
		provider.database = nil
	}
	provider.manifest = manifest
	provider.SQL = SQLManifest
	return nil
}

func (provider *jobsRuntimeProvider) Call(evaluator *Evaluator, invocation runtimeInvocation) (Value, error) {
	if provider.manifest == nil {
		return Value{}, fmt.Errorf("trb/jobs runtime is not configured")
	}
	jobName, err := jobsInvocationName(invocation.Call)
	if err != nil {
		return Value{}, err
	}
	callArguments := invocation.Arguments
	if len(callArguments) > 0 && callArguments[0].Value.Type.Name == "Class" {
		callArguments = callArguments[1:]
	}
	waitMilliseconds := int64(0)
	if invocation.Name == "trb.jobs.perform_in" {
		if len(callArguments) == 0 {
			return Value{}, fmt.Errorf("trb/jobs perform_in delay is missing")
		}
		duration, ok := callArguments[0].Value.Data.(*objectInstance)
		if !ok {
			return Value{}, fmt.Errorf("trb/jobs perform_in delay is invalid")
		}
		seconds, nanosecond := timeDurationFields(duration)
		waitMilliseconds = seconds*1000 + (nanosecond+999999)/1000000
		callArguments = callArguments[1:]
	} else if invocation.Name == "trb.jobs.perform_at" {
		if len(callArguments) == 0 {
			return Value{}, fmt.Errorf("trb/jobs perform_at scheduled_at is missing")
		}
		instant, ok := callArguments[0].Value.Data.(*objectInstance)
		if !ok {
			return Value{}, fmt.Errorf("trb/jobs perform_at scheduled_at is invalid")
		}
		seconds, nanosecond := timeInstantFields(instant)
		targetMilliseconds := seconds*1000 + (nanosecond+999999)/1000000
		waitMilliseconds = max(targetMilliseconds-time.Now().UTC().UnixMilli(), 0)
		callArguments = callArguments[1:]
	}
	arguments := make([]any, len(callArguments))
	for index, argument := range callArguments {
		arguments[index], err = jobsJSONValue(argument.Value)
		if err != nil {
			return provider.resultError(evaluator, invocation.Type, "Serialization", err)
		}
	}
	payload, err := json.Marshal(arguments)
	if err != nil {
		return provider.resultError(evaluator, invocation.Type, "Serialization", err)
	}
	database, err := provider.open()
	if err != nil {
		return provider.resultError(evaluator, invocation.Type, "Adapter", err)
	}
	id, err := jobsID()
	if err != nil {
		return provider.resultError(evaluator, invocation.Type, "Adapter", err)
	}
	config := provider.SQL.Config
	job, ok := provider.manifest.Job(jobName)
	if !ok {
		return provider.resultError(evaluator, invocation.Type, "InvalidArgument", fmt.Errorf("trb/jobs Job %s is not registered", jobName))
	}
	if waitMilliseconds < 0 {
		return provider.resultError(evaluator, invocation.Type, "InvalidArgument", fmt.Errorf("job delay must not be negative"))
	}
	maximumAttempts := job.MaximumAttempts
	if maximumAttempts <= 0 {
		maximumAttempts = config.DefaultMaximumAttempts
	}
	runAt := time.Now().UTC().Add(time.Duration(waitMilliseconds) * time.Millisecond)
	query := "INSERT INTO trb_jobs (id, queue_name, job_name, payload, payload_version, priority, run_at, state, attempts, maximum_attempts, created_at, updated_at) VALUES (" + jobsPlaceholders(config.Dialect, 7, 0) + ", 'ready', 0, " + jobsPlaceholder(config.Dialect, 8) + ", CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"
	if _, err := database.ExecContext(context.Background(), query, id, job.Queue, jobName, string(payload), 1, job.Priority, runAt, maximumAttempts); err != nil {
		return provider.resultError(evaluator, invocation.Type, "Adapter", err)
	}
	definition, ok := evaluator.definitions[symbolKey("trb/jobs/index", "JobReference")].(*recordDefinition)
	if !ok {
		return Value{}, fmt.Errorf("trb/jobs JobReference runtime is not loaded")
	}
	referenceType := types.FromName("JobReference")
	if len(invocation.Type.Args) == 2 {
		referenceType = invocation.Type.Args[0]
	}
	reference := Value{Type: referenceType, Data: &recordInstance{Definition: definition, Fields: map[string]Value{
		"id":       {Type: types.FromName("String"), Data: id},
		"job_name": {Type: types.FromName("String"), Data: jobName},
	}}}
	return evaluator.filesystemOK(invocation.Type, reference)
}

func (provider *jobsRuntimeProvider) Close() error {
	if provider.database == nil {
		return nil
	}
	err := provider.database.Close()
	provider.database = nil
	return err
}

func (provider *jobsRuntimeProvider) open() (*sql.DB, error) {
	if provider.database != nil {
		return provider.database, nil
	}
	config := provider.SQL.Config
	source := config.Source
	if config.SourceEnvironment != "" {
		var exists bool
		source, exists = os.LookupEnv(config.SourceEnvironment)
		if !exists || strings.TrimSpace(source) == "" {
			return nil, fmt.Errorf("jobs database environment %s is not set or empty", config.SourceEnvironment)
		}
	}
	driver := config.Dialect
	switch config.Dialect {
	case "sqlite":
		driver = "sqlite"
	case "postgresql":
		driver = "pgx"
	case "mysql":
		driver = "mysql"
		if strings.HasPrefix(source, "mysql://") {
			parsed, err := url.Parse(source)
			if err != nil {
				return nil, err
			}
			credentials := parsed.User.Username()
			if password, exists := parsed.User.Password(); exists {
				credentials += ":" + password
			}
			source = credentials + "@tcp(" + parsed.Host + ")" + parsed.Path
			if parsed.RawQuery != "" {
				source += "?" + parsed.RawQuery
			}
		}
	}
	database, err := sql.Open(driver, source)
	if err != nil {
		return nil, err
	}
	if config.Dialect == "sqlite" {
		database.SetMaxOpenConns(1)
	}
	statements, err := sqlstore.Schema(sqlstore.Dialect(config.Dialect))
	if err != nil {
		database.Close()
		return nil, err
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			database.Close()
			return nil, err
		}
	}
	provider.database = database
	return database, nil
}

func (provider *jobsRuntimeProvider) resultError(evaluator *Evaluator, resultType types.Type, kind string, err error) (Value, error) {
	kindDefinition, ok := evaluator.definitions[symbolKey("trb/jobs/index", "EnqueueErrorKind")].(*enumDefinition)
	if !ok {
		return Value{}, fmt.Errorf("trb/jobs EnqueueErrorKind runtime is not loaded")
	}
	kindValue := Value{Type: types.FromName("EnqueueErrorKind"), Data: &enumValue{Definition: kindDefinition, Name: kind, Payload: map[string]Value{}}}
	return evaluator.structuredResultErrFrom(resultType, "trb/jobs/index", "EnqueueError", map[string]Value{
		"kind":    kindValue,
		"message": {Type: types.FromName("String"), Data: err.Error()},
	})
}

func jobsInvocationName(call *ir.Call) (string, error) {
	if call == nil {
		return "", fmt.Errorf("trb/jobs perform_later call metadata is missing")
	}
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return "", fmt.Errorf("trb/jobs perform_later receiver is invalid")
	}
	identifier, ok := member.Receiver.(*ir.Identifier)
	if !ok || identifier.Name == "" {
		return "", fmt.Errorf("trb/jobs perform_later Job is invalid")
	}
	return identifier.Name, nil
}

func jobsJSONValue(value Value) (any, error) {
	switch value.Type.Kind {
	case types.Bool, types.Int, types.Float, types.String:
		return value.Data, nil
	default:
		return nil, fmt.Errorf("trb/jobs cannot encode %s yet", value.Type)
	}
}

func jobsID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func jobsPlaceholder(adapter string, index int) string {
	if adapter == "postgresql" {
		return fmt.Sprintf("$%d", index)
	}
	return "?"
}

func jobsPlaceholders(adapter string, count, offset int) string {
	values := make([]string, count)
	for index := range values {
		values[index] = jobsPlaceholder(adapter, offset+index+1)
	}
	return strings.Join(values, ", ")
}
