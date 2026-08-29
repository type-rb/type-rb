// Package projectintegration runs compile-time hooks declared by TypeRB
// packages without teaching the compiler pipeline their domain-specific data.
package projectintegration

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/official"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/token"
)

type Source struct {
	Filename        string
	ModulePath      string
	Program         *ast.Program
	CompilerOwned   bool
	Official        bool
	ExternalPackage bool
}

type Context struct {
	Sources                []Source
	Resolutions            map[string]resolver.Result
	EntrypointModule       string
	SourceRoot             string
	ProjectRoot            string
	PackageOptions         map[string][]byte
	PackageAliasesByModule map[string]map[string]string
	JobsConfiguration      string
}

type Issue struct {
	Filename string
	Message  string
	Span     token.Span
}

type Contribution struct {
	Extension     ir.Extension
	MethodTargets map[string]map[string]string
	AllPrograms   bool
	Generation    *packageextension.ProjectGenerationResponse
}

type Analysis struct {
	contributions []Contribution
}

type GeneratedSource struct {
	Provider string
	Source   packageextension.ProjectGeneratedSource
}

type incrementalLoweringExtension interface {
	EquivalentForIncrementalLowering(ir.Extension) bool
	RequiresIncrementalRelowering(string) bool
}

type provider func(Context) (Contribution, []Issue)

var providers = map[string]provider{}

func register(name string, implementation provider) {
	if name == "" || implementation == nil {
		panic("project integration provider requires a name and implementation")
	}
	if _, exists := providers[name]; exists {
		panic("project integration provider is already registered: " + name)
	}
	providers[name] = implementation
}

func Analyze(context Context) (Analysis, []Issue, error) {
	activeModules := map[string]bool{}
	for _, resolution := range context.Resolutions {
		for _, imported := range resolution.Capabilities {
			if imported != nil {
				activeModules[imported.RuntimePath()] = true
			}
		}
	}

	packageNames := official.Names()
	sort.Strings(packageNames)
	analysis := Analysis{}
	extensionOwners := map[string]string{}
	generationOwners := map[string]string{}
	sourcesByModule := map[string]Source{}
	for _, source := range context.Sources {
		sourcesByModule[source.ModulePath] = source
	}
	var issues []Issue
	for _, packageName := range packageNames {
		definition, _ := official.Lookup(packageName)
		if definition.ProjectProvider == "" || !activeModules[definition.Definition.ModulePath] {
			continue
		}
		implementation := providers[definition.ProjectProvider]
		if implementation == nil {
			return Analysis{}, nil, fmt.Errorf("official package %s declares unknown project provider %s", packageName, definition.ProjectProvider)
		}
		contribution, providerIssues := implementation(context)
		if contribution.Generation != nil {
			generation := *contribution.Generation
			if err := packageextension.ValidateProjectGenerationResponse(generation); err != nil {
				return Analysis{}, nil, fmt.Errorf("project provider %s returned invalid generated source response: %w", definition.ProjectProvider, err)
			}
			if generation.Provider != definition.ProjectProvider {
				return Analysis{}, nil, fmt.Errorf("project provider %s returned generated source response for %s", definition.ProjectProvider, generation.Provider)
			}
			if owner := generationOwners[generation.Provider]; owner != "" {
				return Analysis{}, nil, fmt.Errorf("official packages %s and %s returned generated sources for the same project provider %s", owner, packageName, generation.Provider)
			}
			generationOwners[generation.Provider] = packageName
			for _, generatedIssue := range generation.Issues {
				source, exists := sourcesByModule[generatedIssue.ModulePath]
				if !exists {
					return Analysis{}, nil, fmt.Errorf("project provider %s returned an issue for unknown module %s", generation.Provider, generatedIssue.ModulePath)
				}
				providerIssues = append(providerIssues, Issue{
					Filename: source.Filename, Message: generatedIssue.Message,
					Span: packageextensionhost.ImportSourceSpan(generatedIssue.Span),
				})
			}
		}
		if contribution.Extension != nil {
			extensionName := contribution.Extension.ExtensionName()
			if extensionName != definition.ProjectProvider {
				return Analysis{}, nil, fmt.Errorf("project provider %s returned extension %s", definition.ProjectProvider, extensionName)
			}
			if owner := extensionOwners[extensionName]; owner != "" {
				return Analysis{}, nil, fmt.Errorf("official packages %s and %s returned the same project extension %s", owner, packageName, extensionName)
			}
			extensionOwners[extensionName] = packageName
		}
		analysis.contributions = append(analysis.contributions, contribution)
		issues = append(issues, providerIssues...)
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Filename != issues[j].Filename {
			return issues[i].Filename < issues[j].Filename
		}
		if issues[i].Span.Start.Offset != issues[j].Span.Start.Offset {
			return issues[i].Span.Start.Offset < issues[j].Span.Start.Offset
		}
		return issues[i].Message < issues[j].Message
	})
	return analysis, issues, nil
}

func (a Analysis) GeneratedSources() []GeneratedSource {
	var result []GeneratedSource
	for _, contribution := range a.contributions {
		if contribution.Generation == nil {
			continue
		}
		for _, source := range contribution.Generation.Sources {
			result = append(result, GeneratedSource{Provider: contribution.Generation.Provider, Source: source})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Provider != result[j].Provider {
			return result[i].Provider < result[j].Provider
		}
		if result[i].Source.ModulePath != result[j].Source.ModulePath {
			return result[i].Source.ModulePath < result[j].Source.ModulePath
		}
		return result[i].Source.ID < result[j].Source.ID
	})
	return result
}

func (a Analysis) Apply(program *ir.Program, entrypoint bool) {
	for _, contribution := range a.contributions {
		targets := contribution.MethodTargets[program.ModulePath]
		for _, statement := range program.Statements {
			if method, ok := statement.(*ir.Method); ok {
				if target, exists := targets[method.Name]; exists {
					method.TargetName = target
				}
			}
		}
		if contribution.Extension != nil {
			if augmenter, ok := contribution.Extension.(interface {
				AugmentProgram(*ir.Program, bool)
			}); ok {
				augmenter.AugmentProgram(program, entrypoint)
			} else if augmenter, ok := contribution.Extension.(interface{ Augment(*ir.Program) }); ok {
				augmenter.Augment(program)
			}
		}
		if contribution.Extension != nil && (entrypoint || contribution.AllPrograms) {
			program.Extensions = append(program.Extensions, contribution.Extension)
		}
	}
}

// CanReuseLoweredPrograms reports whether unchanged IR augmented by previous
// has the same project-integration behavior under the receiver.
func (a Analysis) CanReuseLoweredPrograms(previous Analysis, affected map[string]bool) bool {
	if len(a.contributions) != len(previous.contributions) {
		return false
	}
	for index, current := range a.contributions {
		cached := previous.contributions[index]
		if current.AllPrograms != cached.AllPrograms || !reflect.DeepEqual(current.MethodTargets, cached.MethodTargets) {
			return false
		}
		if current.Extension == nil || cached.Extension == nil {
			if current.Extension != nil || cached.Extension != nil {
				return false
			}
			continue
		}
		if comparable, ok := current.Extension.(incrementalLoweringExtension); ok {
			if !comparable.EquivalentForIncrementalLowering(cached.Extension) {
				return false
			}
			for modulePath := range affected {
				if comparable.RequiresIncrementalRelowering(modulePath) {
					return false
				}
			}
			continue
		}
		if !reflect.DeepEqual(current.Extension, cached.Extension) {
			return false
		}
	}
	return true
}
