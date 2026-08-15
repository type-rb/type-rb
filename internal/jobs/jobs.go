// Package jobs models the compile-time contract shared by portable job
// adapters. It discovers typed Job subclasses without coupling the compiler
// pipeline to a particular queue implementation.
package jobs

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/resolver"
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
	Name            string
	ModulePath      string
	Parameters      []Parameter
	Fails           types.Type
	Queue           string
	Priority        int
	MaximumAttempts int
}

type Manifest struct {
	Jobs []Job
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

func (m *Manifest) Job(name string) (Job, bool) {
	if m == nil {
		return Job{}, false
	}
	for _, job := range m.Jobs {
		if job.Name == name {
			return job, true
		}
	}
	return Job{}, false
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
		for _, symbol := range []string{"EnqueueError", "EnqueueErrorKind", "JobReference"} {
			if !contains(imported.Symbols, symbol) {
				imported.Symbols = append(imported.Symbols, symbol)
			}
			if symbol == "EnqueueErrorKind" {
				imported.SymbolKinds[symbol] = "enum"
			} else {
				imported.SymbolKinds[symbol] = "record"
			}
		}
		sort.Strings(imported.Symbols)
	}
	for jobIndex := range m.Jobs {
		job := &m.Jobs[jobIndex]
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
			delayedParameters := append([]ir.Parameter{{Name: "delay", Type: types.FromName("Duration")}}, parameters...)
			class.Body = append(class.Body, &ir.Method{
				Name: "perform_in", External: true, Class: true,
				Parameters: delayedParameters, SuccessType: types.FromName("JobReference"),
				ReturnType: types.FromName("JobReference"), Fails: types.FromName("EnqueueError"),
			})
			scheduledParameters := append([]ir.Parameter{{Name: "scheduled_at", Type: types.FromName("Instant")}}, parameters...)
			class.Body = append(class.Body, &ir.Method{
				Name: "perform_at", External: true, Class: true,
				Parameters: scheduledParameters, SuccessType: types.FromName("JobReference"),
				ReturnType: types.FromName("JobReference"), Fails: types.FromName("EnqueueError"),
			})
			ensureJobRuntimeImport(program, "trb/std/time/index", "Duration", "class")
			ensureJobRuntimeImport(program, "trb/std/time/index", "Instant", "class")
		}
	}
}

func ensureJobRuntimeImport(program *ir.Program, modulePath, symbol, kind string) {
	for _, statement := range program.Statements {
		imported, ok := statement.(*ir.Import)
		if !ok || imported.Path != modulePath {
			continue
		}
		if !contains(imported.Symbols, symbol) {
			imported.Symbols = append(imported.Symbols, symbol)
		}
		if imported.SymbolKinds == nil {
			imported.SymbolKinds = map[string]string{}
		}
		imported.SymbolKinds[symbol] = kind
		imported.RuntimeRequired = true
		return
	}
	program.Statements = append([]ir.Statement{&ir.Import{
		Path: modulePath, Symbols: []string{symbol}, SymbolKinds: map[string]string{symbol: kind}, Implicit: true, RuntimeRequired: true,
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

func Declarations(programs []*ast.Program) (*declaration.Catalog, error) {
	jobs, err := discoverJobs(programs, func(_ *ast.Program, typ types.Type, _ aliasResolver) bool {
		return initialArgumentType(typ) || potentialAliasType(typ)
	})
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
		delayedParameters := append([]declaration.Parameter{{Name: "delay", Type: types.FromName("Duration")}}, parameters...)
		declared.ClassMembers["perform_in"] = declaration.Member{
			Name: "perform_in", Kind: declaration.Method, Intrinsic: "trb.jobs.perform_in",
			Parameters: delayedParameters, Return: types.FromName("JobReference"), Fails: types.FromName("EnqueueError"),
			Class: true, Provider: PackageName,
		}
		scheduledParameters := append([]declaration.Parameter{{Name: "scheduled_at", Type: types.FromName("Instant")}}, parameters...)
		declared.ClassMembers["perform_at"] = declaration.Member{
			Name: "perform_at", Kind: declaration.Method, Intrinsic: "trb.jobs.perform_at",
			Parameters: scheduledParameters, Return: types.FromName("JobReference"), Fails: types.FromName("EnqueueError"),
			Class: true, Provider: PackageName,
		}
		catalog.Types[job.Name] = declared
	}
	return catalog, nil
}

func Analyze(programs []*ast.Program, resolutions map[string]resolver.Result) (*Manifest, error) {
	jobs, err := discoverJobs(programs, func(program *ast.Program, typ types.Type, aliases aliasResolver) bool {
		expanded := aliases.expand(program, typ, map[string]bool{})
		if initialArgumentType(expanded) {
			return true
		}
		resolution := resolutions[program.ModulePath]
		binding, exists := resolution.ImportedType(typ.Name)
		return exists && binding.Export != nil && binding.Export.Kind == resolver.TypeAliasExport && initialArgumentType(binding.Export.AliasTarget)
	})
	if err != nil {
		return nil, err
	}
	return &Manifest{Jobs: jobs}, nil
}

func Discover(programs []*ast.Program) ([]Job, error) {
	return discoverJobs(programs, func(program *ast.Program, typ types.Type, aliases aliasResolver) bool {
		return initialArgumentType(aliases.expand(program, typ, map[string]bool{}))
	})
}

func discoverJobs(programs []*ast.Program, argumentType func(*ast.Program, types.Type, aliasResolver) bool) ([]Job, error) {
	aliases := newAliasResolver(programs)
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
			job, err := discoverJob(program, class, aliases, argumentType)
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

func discoverJob(program *ast.Program, class *ast.ClassStatement, aliases aliasResolver, argumentType func(*ast.Program, types.Type, aliasResolver) bool) (Job, error) {
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
	job := Job{Name: class.Name, ModulePath: program.ModulePath, Fails: typeRef(perform.Fails), Queue: "default"}
	if err := discoverJobDefaults(&job, class); err != nil {
		return Job{}, err
	}
	for _, parameter := range perform.Parameters {
		if parameter.Keyword || parameter.Rest || parameter.KeywordRest || parameter.Default != nil {
			return Job{}, fmt.Errorf("trb/jobs Job %s perform initially accepts required positional parameters only", class.Name)
		}
		typ := typeRef(parameter.Type)
		if !argumentType(program, typ, aliases) {
			return Job{}, fmt.Errorf("trb/jobs Job %s parameter %s must initially be Boolean, Integer, Float, or String", class.Name, parameter.Name)
		}
		job.Parameters = append(job.Parameters, Parameter{Name: parameter.Name, Type: typ})
	}
	return job, nil
}

func potentialAliasType(typ types.Type) bool {
	return typ.Kind == types.Named && !typ.Nullable && len(typ.Args) == 0
}

type aliasResolver struct {
	programs map[string]*ast.Program
	aliases  map[string]map[string]*ast.TypeAliasStatement
	imports  map[string]map[string]string
}

func newAliasResolver(programs []*ast.Program) aliasResolver {
	result := aliasResolver{
		programs: map[string]*ast.Program{},
		aliases:  map[string]map[string]*ast.TypeAliasStatement{},
		imports:  map[string]map[string]string{},
	}
	for _, program := range programs {
		result.programs[program.ModulePath] = program
		result.aliases[program.ModulePath] = map[string]*ast.TypeAliasStatement{}
		result.imports[program.ModulePath] = map[string]string{}
		for _, statement := range program.Statements {
			switch node := statement.(type) {
			case *ast.TypeAliasStatement:
				result.aliases[program.ModulePath][node.Name] = node
			case *ast.ImportStatement:
				for _, symbol := range node.Symbols {
					result.imports[program.ModulePath][symbol] = node.Path
				}
			}
		}
	}
	return result
}

func (r aliasResolver) expand(program *ast.Program, typ types.Type, visiting map[string]bool) types.Type {
	if typ.Kind != types.Named || typ.Nullable || len(typ.Args) != 0 {
		return typ
	}
	modulePath := program.ModulePath
	alias := r.aliases[modulePath][typ.Name]
	if alias == nil {
		importPath := r.imports[modulePath][typ.Name]
		modulePath = r.modulePath(importPath)
		alias = r.aliases[modulePath][typ.Name]
	}
	if alias == nil || len(alias.TypeParameters) != 0 {
		return typ
	}
	key := modulePath + "\x00" + typ.Name
	if visiting[key] {
		return typ
	}
	visiting[key] = true
	targetProgram := r.programs[modulePath]
	if targetProgram == nil {
		return typ
	}
	return r.expand(targetProgram, typeRef(alias.Target), visiting)
}

func (r aliasResolver) modulePath(importPath string) string {
	if _, exists := r.programs[importPath]; exists {
		return importPath
	}
	if _, exists := r.programs[importPath+"/index"]; exists {
		return importPath + "/index"
	}
	return importPath
}

func discoverJobDefaults(job *Job, class *ast.ClassStatement) error {
	seen := map[string]bool{}
	for _, statement := range class.Body {
		expression, ok := statement.(*ast.ExpressionStatement)
		if !ok {
			continue
		}
		call, ok := expression.Expression.(*ast.CallExpression)
		if !ok || call.Block != nil {
			continue
		}
		callee, ok := call.Callee.(*ast.Identifier)
		if !ok || callee.Name != "queue" && callee.Name != "priority" && callee.Name != "maximum_attempts" {
			continue
		}
		if seen[callee.Name] {
			return fmt.Errorf("trb/jobs Job %s declares %s more than once", job.Name, callee.Name)
		}
		seen[callee.Name] = true
		if len(call.Arguments) != 1 || call.Arguments[0].Name != "" {
			return fmt.Errorf("trb/jobs Job %s.%s expects one positional literal", job.Name, callee.Name)
		}
		literal, ok := call.Arguments[0].Value.(*ast.Literal)
		if !ok {
			return fmt.Errorf("trb/jobs Job %s.%s expects a literal", job.Name, callee.Name)
		}
		switch callee.Name {
		case "queue":
			if literal.Kind != ast.StringLiteral {
				return fmt.Errorf("trb/jobs Job %s.queue expects a String literal", job.Name)
			}
			value, err := strconv.Unquote(literal.Raw)
			if err != nil || strings.TrimSpace(value) == "" || len(value) > 255 {
				return fmt.Errorf("trb/jobs Job %s.queue must be a non-empty String of at most 255 bytes", job.Name)
			}
			job.Queue = value
		case "priority":
			if literal.Kind != ast.IntegerLiteral {
				return fmt.Errorf("trb/jobs Job %s.priority expects an Integer literal", job.Name)
			}
			value, err := strconv.ParseInt(strings.ReplaceAll(literal.Raw, "_", ""), 10, 32)
			if err != nil || value < 0 {
				return fmt.Errorf("trb/jobs Job %s.priority must be a non-negative Integer", job.Name)
			}
			job.Priority = int(value)
		case "maximum_attempts":
			if literal.Kind != ast.IntegerLiteral {
				return fmt.Errorf("trb/jobs Job %s.maximum_attempts expects an Integer literal", job.Name)
			}
			value, err := strconv.ParseInt(strings.ReplaceAll(literal.Raw, "_", ""), 10, 32)
			if err != nil || value <= 0 {
				return fmt.Errorf("trb/jobs Job %s.maximum_attempts must be a positive Integer", job.Name)
			}
			job.MaximumAttempts = int(value)
		}
	}
	return nil
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
