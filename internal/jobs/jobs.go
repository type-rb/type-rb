// Package jobs models the compile-time contract shared by portable job
// adapters. It discovers typed Job subclasses without coupling the compiler
// pipeline to a particular queue implementation.
package jobs

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

const (
	PackageName     = "trb/jobs"
	TypeProvider    = "trb.jobs"
	ProjectProvider = "trb.jobs.manifest"
)

type Parameter struct {
	Name string
	Type types.Type
}

type Job struct {
	Name       string
	ModulePath string
	Parameters []Parameter
	Fails      types.Type
}

type Manifest struct {
	Jobs   []Job
	Config Config
}

type Config struct {
	Adapter                     string `json:"adapter"`
	DatabaseAdapter             string `json:"database_adapter"`
	Database                    string `json:"database"`
	PollIntervalMilliseconds    int    `json:"poll_interval_milliseconds"`
	LeaseTimeoutMilliseconds    int    `json:"lease_timeout_milliseconds"`
	ShutdownTimeoutMilliseconds int    `json:"shutdown_timeout_milliseconds"`
	DefaultMaximumAttempts      int    `json:"default_maximum_attempts"`
	WorkerConcurrency           int    `json:"worker_concurrency"`
}

func DefaultConfig() Config {
	return Config{
		Adapter: "sql", DatabaseAdapter: "sqlite", Database: "jobs.sqlite3",
		PollIntervalMilliseconds: 1000, LeaseTimeoutMilliseconds: 60_000,
		ShutdownTimeoutMilliseconds: 30_000, DefaultMaximumAttempts: 5,
		WorkerConcurrency: 1,
	}
}

func ParseConfig(source []byte) (Config, error) {
	config := DefaultConfig()
	if len(source) != 0 {
		if err := json.Unmarshal(source, &config); err != nil {
			return Config{}, fmt.Errorf("trb/jobs options: %w", err)
		}
	}
	config.Adapter = strings.ToLower(strings.TrimSpace(config.Adapter))
	config.DatabaseAdapter = strings.ToLower(strings.TrimSpace(config.DatabaseAdapter))
	if config.Adapter != "sql" {
		return Config{}, fmt.Errorf("trb/jobs adapter must initially be sql")
	}
	switch config.DatabaseAdapter {
	case "sqlite", "postgresql", "mysql":
	default:
		return Config{}, fmt.Errorf("trb/jobs database_adapter must be sqlite, postgresql, or mysql")
	}
	if strings.TrimSpace(config.Database) == "" {
		return Config{}, fmt.Errorf("trb/jobs database must not be empty")
	}
	if config.PollIntervalMilliseconds <= 0 || config.LeaseTimeoutMilliseconds <= 0 || config.ShutdownTimeoutMilliseconds <= 0 {
		return Config{}, fmt.Errorf("trb/jobs timing options must be positive")
	}
	if config.DefaultMaximumAttempts <= 0 {
		return Config{}, fmt.Errorf("trb/jobs default_maximum_attempts must be positive")
	}
	if config.WorkerConcurrency <= 0 {
		return Config{}, fmt.Errorf("trb/jobs worker_concurrency must be positive")
	}
	if config.DatabaseAdapter == "sqlite" && config.WorkerConcurrency != 1 {
		return Config{}, fmt.Errorf("trb/jobs SQLite initially supports worker_concurrency: 1 only")
	}
	return config, nil
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

func (m *Manifest) Augment(program *ir.Program) {
	if m == nil || program == nil {
		return
	}
	for _, statement := range program.Statements {
		imported, ok := statement.(*ir.Import)
		if !ok || imported.Path != "trb/jobs/index" {
			continue
		}
		// perform_later is contributed by the type provider rather than declared
		// in the package source, so its result and error representations must be
		// made available to generated code explicitly.
		imported.RuntimeRequired = true
		for _, symbol := range []string{"EnqueueError", "JobReference"} {
			if !contains(imported.Symbols, symbol) {
				imported.Symbols = append(imported.Symbols, symbol)
			}
			imported.SymbolKinds[symbol] = "record"
		}
		sort.Strings(imported.Symbols)
	}
	for _, job := range m.Jobs {
		if job.ModulePath != program.ModulePath {
			continue
		}
		for _, statement := range program.Statements {
			class, ok := statement.(*ir.Class)
			if !ok || class.Name != job.Name {
				continue
			}
			parameters := make([]ir.Parameter, len(job.Parameters))
			for index, parameter := range job.Parameters {
				parameters[index] = ir.Parameter{Name: parameter.Name, Type: parameter.Type}
			}
			class.Body = append(class.Body, &ir.Method{
				Name: "perform_later", External: true, Class: true,
				Parameters: parameters, SuccessType: types.FromName("JobReference"),
				ReturnType: types.FromName("JobReference"), Fails: types.FromName("EnqueueError"),
			})
		}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func Declarations(programs []*ast.Program) (*declaration.Catalog, error) {
	jobs, err := Discover(programs)
	if err != nil {
		return nil, err
	}
	catalog := declaration.NewCatalog()
	for _, job := range jobs {
		declared := declaration.NewType(job.Name, "Job")
		parameters := make([]declaration.Parameter, len(job.Parameters))
		for index, parameter := range job.Parameters {
			parameters[index] = declaration.Parameter{Name: parameter.Name, Type: parameter.Type}
		}
		declared.ClassMembers["perform_later"] = declaration.Member{
			Name: "perform_later", Kind: declaration.Method, Intrinsic: "trb.jobs.perform_later",
			Parameters: parameters, Return: types.FromName("JobReference"), Fails: types.FromName("EnqueueError"),
			Class: true, Provider: PackageName,
		}
		catalog.Types[job.Name] = declared
	}
	return catalog, nil
}

func Analyze(programs []*ast.Program, options []byte) (*Manifest, error) {
	jobs, err := Discover(programs)
	if err != nil {
		return nil, err
	}
	config, err := ParseConfig(options)
	if err != nil {
		return nil, err
	}
	return &Manifest{Jobs: jobs, Config: config}, nil
}

func Discover(programs []*ast.Program) ([]Job, error) {
	seen := map[string]bool{}
	result := []Job{}
	for _, program := range programs {
		for _, statement := range program.Statements {
			class, ok := statement.(*ast.ClassStatement)
			if !ok || expressionName(class.Superclass) != "Job" {
				continue
			}
			if seen[class.Name] {
				return nil, fmt.Errorf("trb/jobs Job %s is declared more than once", class.Name)
			}
			seen[class.Name] = true
			job, err := discoverJob(program.ModulePath, class)
			if err != nil {
				return nil, err
			}
			result = append(result, job)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ModulePath != result[j].ModulePath {
			return result[i].ModulePath < result[j].ModulePath
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func discoverJob(modulePath string, class *ast.ClassStatement) (Job, error) {
	var perform *ast.MethodStatement
	for _, statement := range class.Body {
		method, ok := statement.(*ast.MethodStatement)
		if !ok || method.Name != "perform" {
			continue
		}
		if perform != nil {
			return Job{}, fmt.Errorf("trb/jobs Job %s declares perform more than once", class.Name)
		}
		perform = method
	}
	if perform == nil {
		return Job{}, fmt.Errorf("trb/jobs Job %s must declare perform", class.Name)
	}
	if perform.Class {
		return Job{}, fmt.Errorf("trb/jobs Job %s perform must be an instance method", class.Name)
	}
	if len(perform.TypeParameters) != 0 {
		return Job{}, fmt.Errorf("trb/jobs Job %s perform cannot declare type parameters", class.Name)
	}
	if !perform.ReturnType.Empty() {
		return Job{}, fmt.Errorf("trb/jobs Job %s perform must not return a value", class.Name)
	}
	job := Job{Name: class.Name, ModulePath: modulePath, Fails: typeRef(perform.Fails)}
	for _, parameter := range perform.Parameters {
		if parameter.Keyword || parameter.Rest || parameter.KeywordRest || parameter.Default != nil {
			return Job{}, fmt.Errorf("trb/jobs Job %s perform initially accepts required positional parameters only", class.Name)
		}
		typ := typeRef(parameter.Type)
		if !initialArgumentType(typ) {
			return Job{}, fmt.Errorf("trb/jobs Job %s parameter %s must initially be Boolean, Integer, Float, or String", class.Name, parameter.Name)
		}
		job.Parameters = append(job.Parameters, Parameter{Name: parameter.Name, Type: typ})
	}
	return job, nil
}

func initialArgumentType(typ types.Type) bool {
	switch typ.Kind {
	case types.Bool, types.Int, types.Float, types.String:
		return true
	default:
		return false
	}
}

func expressionName(expression ast.Expression) string {
	if identifier, ok := expression.(*ast.Identifier); ok {
		return identifier.Name
	}
	return ""
}

func typeRef(ref ast.TypeRef) types.Type {
	if ref.Empty() {
		return types.Type{}
	}
	if len(ref.Union) > 0 {
		alternatives := make([]types.Type, len(ref.Union))
		for index, alternative := range ref.Union {
			alternatives[index] = typeRef(alternative)
		}
		return types.UnionOf(alternatives...)
	}
	if ref.FunctionReturn != nil {
		parameters := make([]types.Type, len(ref.FunctionParameters))
		for index, parameter := range ref.FunctionParameters {
			parameters[index] = typeRef(parameter)
		}
		result := types.FunctionOf(parameters, typeRef(*ref.FunctionReturn))
		result.Nullable = ref.Nullable
		return result
	}
	result := types.FromName(ref.Name)
	result.Nullable = ref.Nullable
	for _, argument := range ref.Arguments {
		result.Args = append(result.Args, typeRef(argument))
	}
	if ref.Array {
		result = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{result}, Nullable: ref.Nullable}
	}
	return result
}
