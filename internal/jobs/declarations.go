package jobs

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/types"
)

// Declarations derives the Jobs type-provider catalog from the versioned,
// declaration-only project snapshot. It never receives compiler AST nodes.
func Declarations(input packageextension.ProjectDeclarationInput) (*declaration.Catalog, error) {
	if err := packageextension.ValidateProjectDeclarationInput(input); err != nil {
		return nil, err
	}
	if input.Provider != PackageName {
		return nil, fmt.Errorf("trb/jobs received project declaration input for provider %s", input.Provider)
	}
	jobs, err := discoverDeclarationJobs(input)
	if err != nil {
		return nil, err
	}
	catalog := declaration.NewCatalog()
	for _, job := range jobs {
		for _, function := range []string{"queue", "priority", "maximum_attempts"} {
			catalog.ClassBodyDeclarationRules = append(catalog.ClassBodyDeclarationRules, declaration.ClassBodyDeclarationRule{
				Package: PackageName, Function: function,
				Owner: declaration.DeclarationReference{ModulePath: job.ModulePath, Name: job.Name},
			})
		}
		declared := declaration.NewType(job.Name, "Job")
		parameters := make([]declaration.Parameter, len(job.Parameters))
		for index, parameter := range job.Parameters {
			parameters[index] = declaration.Parameter{Name: parameter.Name, Type: parameter.Type}
		}
		declared.ClassMembers["perform_later"] = declaration.Member{
			Name: "perform_later", Kind: declaration.Method, Intrinsic: "trb.jobs.perform_later",
			Parameters: parameters, Return: jobEnqueueResultType(),
			Class: true, Provider: PackageName,
		}
		delayedParameters := append([]declaration.Parameter{{Name: "delay", Type: types.FromName("Duration")}}, parameters...)
		declared.ClassMembers["perform_in"] = declaration.Member{
			Name: "perform_in", Kind: declaration.Method, Intrinsic: "trb.jobs.perform_in",
			Parameters: delayedParameters, Return: jobEnqueueResultType(),
			Class: true, Provider: PackageName,
		}
		scheduledParameters := append([]declaration.Parameter{{Name: "scheduled_at", Type: types.FromName("Instant")}}, parameters...)
		declared.ClassMembers["perform_at"] = declaration.Member{
			Name: "perform_at", Kind: declaration.Method, Intrinsic: "trb.jobs.perform_at",
			Parameters: scheduledParameters, Return: jobEnqueueResultType(),
			Class: true, Provider: PackageName,
		}
		catalog.Types[job.Name] = declared
	}
	return catalog, nil
}

func discoverDeclarationJobs(input packageextension.ProjectDeclarationInput) ([]Job, error) {
	seen := map[string]bool{}
	result := []Job{}
	for _, module := range input.Modules {
		for _, class := range module.Classes {
			if class.Superclass == nil || class.Superclass.Authored.Kind != "named" || class.Superclass.Authored.Name != "Job" {
				continue
			}
			if seen[class.Name] {
				return nil, fmt.Errorf("trb/jobs Job %s is declared more than once", class.Name)
			}
			seen[class.Name] = true
			job, err := discoverDeclarationJob(module.ModulePath, class)
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

func discoverDeclarationJob(modulePath string, class packageextension.ProjectClass) (Job, error) {
	var perform *packageextension.ProjectMethod
	for index := range class.Methods {
		method := &class.Methods[index]
		if method.Name != "perform" {
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
	performKind := PerformVoid
	if perform.Return != nil {
		if !canonicalProjectJobResult(*perform.Return) {
			return Job{}, fmt.Errorf("trb/jobs Job %s perform must omit its return type or return JobResult", class.Name)
		}
		performKind = PerformJobResult
	}
	job := Job{Name: class.Name, ModulePath: modulePath, PerformKind: performKind, Queue: "default"}
	if err := discoverDeclarationJobDefaults(&job, class.Directives); err != nil {
		return Job{}, err
	}
	for _, parameter := range perform.Parameters {
		if parameter.NamedOnly || parameter.Keyword || parameter.Rest || parameter.KeywordRest || parameter.Optional {
			return Job{}, fmt.Errorf("trb/jobs Job %s perform initially accepts required positional parameters only", class.Name)
		}
		typ := importProjectType(parameter.Type.Authored)
		if !initialArgumentType(typ) && !potentialAliasType(typ) {
			return Job{}, fmt.Errorf("trb/jobs Job %s parameter %s must initially be Boolean, Integer, Float, or String", class.Name, parameter.Name)
		}
		wireType := importProjectType(parameter.Type.Resolved)
		newtype := parameter.Type.Representation != nil
		if parameter.Type.Representation != nil {
			wireType = importProjectType(*parameter.Type.Representation)
		}
		if !initialArgumentType(wireType) {
			wireType = typ
		}
		newtypeModule := ""
		if newtype && parameter.Type.Authored.Definition != nil {
			newtypeModule = parameter.Type.Authored.Definition.ModulePath
		}
		job.Parameters = append(job.Parameters, Parameter{Name: parameter.Name, Type: typ, WireType: wireType, Newtype: newtype, NewtypeModule: newtypeModule})
	}
	return job, nil
}

func canonicalProjectJobResult(use packageextension.ProjectTypeUse) bool {
	if use.Authored.Kind != "named" || use.Authored.Nullable || len(use.Authored.Arguments) != 0 {
		return false
	}
	for _, reference := range use.ResolutionPath {
		if reference.Name == "JobResult" && (canonicalJobsPath(reference.ModulePath) || canonicalJobsPath(reference.ImportPath)) {
			return true
		}
	}
	return false
}

func discoverDeclarationJobDefaults(job *Job, directives []packageextension.ProjectDirective) error {
	seen := map[string]bool{}
	for _, directive := range directives {
		if directive.Block != nil || directive.Name != "queue" && directive.Name != "priority" && directive.Name != "maximum_attempts" {
			continue
		}
		if seen[directive.Name] {
			return fmt.Errorf("trb/jobs Job %s declares %s more than once", job.Name, directive.Name)
		}
		seen[directive.Name] = true
		if len(directive.Arguments) != 1 || directive.Arguments[0].Name != "" {
			return fmt.Errorf("trb/jobs Job %s.%s expects one positional literal", job.Name, directive.Name)
		}
		literal := directive.Arguments[0].Value
		switch literal.Kind {
		case "string", "integer", "float", "boolean", "nil":
		default:
			return fmt.Errorf("trb/jobs Job %s.%s expects a literal", job.Name, directive.Name)
		}
		switch directive.Name {
		case "queue":
			if literal.Kind != "string" {
				return fmt.Errorf("trb/jobs Job %s.queue expects a String literal", job.Name)
			}
			value, err := strconv.Unquote(literal.Raw)
			if err != nil || strings.TrimSpace(value) == "" || len(value) > 255 {
				return fmt.Errorf("trb/jobs Job %s.queue must be a non-empty String of at most 255 bytes", job.Name)
			}
			job.Queue = value
		case "priority":
			if literal.Kind != "integer" {
				return fmt.Errorf("trb/jobs Job %s.priority expects an Integer literal", job.Name)
			}
			value, err := strconv.ParseInt(strings.ReplaceAll(literal.Raw, "_", ""), 10, 32)
			if err != nil || value < 0 {
				return fmt.Errorf("trb/jobs Job %s.priority must be a non-negative Integer", job.Name)
			}
			job.Priority = int(value)
		case "maximum_attempts":
			if literal.Kind != "integer" {
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

func importProjectType(source packageextension.Type) types.Type {
	result := types.Type{Kind: types.Kind(source.Kind), Name: source.Name, Nullable: source.Nullable}
	for _, argument := range source.Arguments {
		result.Args = append(result.Args, importProjectType(argument))
	}
	return result
}
