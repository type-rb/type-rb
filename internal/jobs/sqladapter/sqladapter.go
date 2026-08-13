// Package sqladapter owns the compile-time configuration for the official
// trb/jobs/sql adapter. The portable trb/jobs contract does not depend on it.
package sqladapter

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
)

const (
	PackageName           = "trb/jobs/sql"
	ModulePath            = "trb/jobs/sql/index"
	ProjectProvider       = "trb.jobs.sql"
	ConfigurationFunction = "configure_jobs"
)

type Config struct {
	Dialect                     string
	Source                      string
	SourceEnvironment           string
	PollIntervalMilliseconds    int
	LeaseTimeoutMilliseconds    int
	ShutdownTimeoutMilliseconds int
	DefaultMaximumAttempts      int
	RetryBaseDelayMilliseconds  int
}

func DefaultConfig() Config {
	return Config{
		Dialect: "sqlite", Source: "jobs.sqlite3",
		PollIntervalMilliseconds: 1000, LeaseTimeoutMilliseconds: 60_000,
		ShutdownTimeoutMilliseconds: 30_000, DefaultMaximumAttempts: 5,
		RetryBaseDelayMilliseconds: 1000,
	}
}

type Manifest struct {
	ConfigurationModule string
	Config              Config
}

func (*Manifest) ExtensionName() string { return ProjectProvider }

func ManifestFrom(extensions []ir.Extension) *Manifest {
	for _, extension := range extensions {
		if manifest, ok := extension.(*Manifest); ok {
			return manifest
		}
	}
	return nil
}

// Augment makes the selected runtime available wherever generated enqueue or
// worker code may need it. It deliberately avoids importing the adapter into
// the portable contract package, which would create a dependency cycle.
func (m *Manifest) Augment(program *ir.Program) {
	if m == nil || program == nil || program.ModulePath == "trb/jobs/index" {
		return
	}
	if program.ModulePath == ModulePath {
		for _, statement := range program.Statements {
			imported, ok := statement.(*ir.Import)
			if !ok || imported.Path != "trb/jobs/index" {
				continue
			}
			if !contains(imported.Symbols, "JobReference") {
				imported.Symbols = append(imported.Symbols, "JobReference")
			}
			if imported.SymbolKinds == nil {
				imported.SymbolKinds = map[string]string{}
			}
			imported.SymbolKinds["JobReference"] = "record"
			imported.RuntimeRequired = true
			return
		}
		return
	}
	if strings.HasPrefix(program.ModulePath, "trb/") {
		return
	}
	for _, statement := range program.Statements {
		if imported, ok := statement.(*ir.Import); ok && imported.Path == ModulePath {
			imported.RuntimeRequired = true
			return
		}
	}
	program.Statements = append([]ir.Statement{&ir.Import{
		Path: ModulePath, Implicit: true, Runtime: true, RuntimeRequired: true, Official: true,
	}}, program.Statements...)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func Analyze(programs []*ast.Program, configurationModule string) (*Manifest, error) {
	configurationModule = strings.TrimSpace(configurationModule)
	if configurationModule == "" {
		return nil, fmt.Errorf("trb/jobs requires jobs.configuration in trbconfig.jsonc")
	}
	var configuration *ast.Program
	for _, program := range programs {
		if program.ModulePath == configurationModule {
			configuration = program
			break
		}
	}
	if configuration == nil {
		return nil, fmt.Errorf("jobs.configuration module %q was not found", configurationModule)
	}
	config, err := ParseConfiguration(configuration)
	if err != nil {
		return nil, err
	}
	return &Manifest{ConfigurationModule: configurationModule, Config: config}, nil
}

func ParseConfiguration(program *ast.Program) (Config, error) {
	if program == nil {
		return Config{}, fmt.Errorf("jobs configuration is missing")
	}
	var method *ast.MethodStatement
	for _, statement := range program.Statements {
		candidate, ok := statement.(*ast.MethodStatement)
		if !ok || candidate.Name != ConfigurationFunction {
			continue
		}
		if method != nil {
			return Config{}, fmt.Errorf("jobs configuration declares %s more than once", ConfigurationFunction)
		}
		method = candidate
	}
	if method == nil {
		return Config{}, fmt.Errorf("jobs.configuration must define def %s(): JobAdapter", ConfigurationFunction)
	}
	if method.Class || len(method.Parameters) != 0 || method.ReturnType.String() != "JobAdapter" {
		return Config{}, fmt.Errorf("%s must have signature def %s(): JobAdapter", ConfigurationFunction, ConfigurationFunction)
	}
	if len(method.Body) != 1 {
		return Config{}, fmt.Errorf("%s must return one SQLAdapter.new(...) expression", ConfigurationFunction)
	}
	returned, ok := method.Body[0].(*ast.ReturnStatement)
	if !ok || returned.Value == nil {
		return Config{}, fmt.Errorf("%s must return SQLAdapter.new(...)", ConfigurationFunction)
	}
	call, ok := returned.Value.(*ast.CallExpression)
	if !ok || !sqlAdapterConstructor(call.Callee) {
		return Config{}, fmt.Errorf("%s must return SQLAdapter.new(...)", ConfigurationFunction)
	}
	config := DefaultConfig()
	seen := map[string]bool{}
	for _, argument := range call.Arguments {
		if argument.Name == "" {
			return Config{}, fmt.Errorf("SQLAdapter.new accepts keyword arguments only")
		}
		if seen[argument.Name] {
			return Config{}, fmt.Errorf("SQLAdapter.new receives %s more than once", argument.Name)
		}
		seen[argument.Name] = true
		var err error
		switch argument.Name {
		case "dialect":
			config.Dialect, err = sqlDialect(argument.Value)
		case "source":
			config.Source, err = stringLiteral(argument.Value)
		case "source_environment":
			config.SourceEnvironment, err = stringLiteral(argument.Value)
		case "poll_interval":
			config.PollIntervalMilliseconds, err = durationMilliseconds(argument.Value)
		case "lease_timeout":
			config.LeaseTimeoutMilliseconds, err = durationMilliseconds(argument.Value)
		case "default_maximum_attempts":
			config.DefaultMaximumAttempts, err = integerLiteral(argument.Value)
		case "retry_base_delay":
			config.RetryBaseDelayMilliseconds, err = durationMilliseconds(argument.Value)
		default:
			return Config{}, fmt.Errorf("SQLAdapter.new has no option %s", argument.Name)
		}
		if err != nil {
			return Config{}, fmt.Errorf("SQLAdapter.new %s: %w", argument.Name, err)
		}
	}
	if strings.TrimSpace(config.Source) == "" {
		return Config{}, fmt.Errorf("SQLAdapter.new source must not be empty")
	}
	if seen["source_environment"] && strings.TrimSpace(config.SourceEnvironment) == "" {
		return Config{}, fmt.Errorf("SQLAdapter.new source_environment must not be empty")
	}
	if config.PollIntervalMilliseconds <= 0 || config.LeaseTimeoutMilliseconds <= 0 || config.ShutdownTimeoutMilliseconds <= 0 || config.RetryBaseDelayMilliseconds <= 0 {
		return Config{}, fmt.Errorf("SQLAdapter.new durations must be positive")
	}
	if config.DefaultMaximumAttempts <= 0 {
		return Config{}, fmt.Errorf("SQLAdapter.new default_maximum_attempts must be positive")
	}
	return config, nil
}

func sqlAdapterConstructor(expression ast.Expression) bool {
	member, ok := expression.(*ast.MemberExpression)
	if !ok || member.Name != "new" {
		return false
	}
	receiver, ok := member.Receiver.(*ast.Identifier)
	return ok && receiver.Name == "SQLAdapter"
}

func sqlDialect(expression ast.Expression) (string, error) {
	member, ok := expression.(*ast.MemberExpression)
	if !ok || !member.Namespace {
		return "", fmt.Errorf("must be a SQLDialect member")
	}
	receiver, ok := member.Receiver.(*ast.Identifier)
	if !ok || receiver.Name != "SQLDialect" {
		return "", fmt.Errorf("must be a SQLDialect member")
	}
	switch member.Name {
	case "SQLite":
		return "sqlite", nil
	case "PostgreSQL":
		return "postgresql", nil
	case "MySQL":
		return "mysql", nil
	default:
		return "", fmt.Errorf("has unsupported value SQLDialect::%s", member.Name)
	}
}

func stringLiteral(expression ast.Expression) (string, error) {
	literal, ok := expression.(*ast.Literal)
	if !ok || literal.Kind != ast.StringLiteral {
		return "", fmt.Errorf("must be a String literal")
	}
	value, err := strconv.Unquote(literal.Raw)
	if err != nil {
		return "", fmt.Errorf("contains an invalid String literal")
	}
	return value, nil
}

func integerLiteral(expression ast.Expression) (int, error) {
	literal, ok := expression.(*ast.Literal)
	if !ok || literal.Kind != ast.IntegerLiteral {
		return 0, fmt.Errorf("must be an Integer literal")
	}
	value, err := strconv.ParseInt(strings.ReplaceAll(literal.Raw, "_", ""), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("contains an invalid Integer literal")
	}
	return int(value), nil
}

func durationMilliseconds(expression ast.Expression) (int, error) {
	call, ok := expression.(*ast.CallExpression)
	if !ok || len(call.Arguments) != 1 || call.Arguments[0].Name != "" {
		return 0, fmt.Errorf("must use Duration.milliseconds(...), seconds(...), or minutes(...) with one Integer literal")
	}
	member, ok := call.Callee.(*ast.MemberExpression)
	if !ok {
		return 0, fmt.Errorf("must be a Duration constructor")
	}
	receiver, ok := member.Receiver.(*ast.Identifier)
	if !ok || receiver.Name != "Duration" {
		return 0, fmt.Errorf("must be a Duration constructor")
	}
	value, err := integerLiteral(call.Arguments[0].Value)
	if err != nil {
		return 0, err
	}
	switch member.Name {
	case "milliseconds":
		return value, nil
	case "seconds":
		return value * 1000, nil
	case "minutes":
		return value * 60_000, nil
	default:
		return 0, fmt.Errorf("must use Duration.milliseconds(...), seconds(...), or minutes(...)")
	}
}
