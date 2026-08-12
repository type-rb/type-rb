// Package projectintegration runs compile-time hooks declared by TypeRB
// packages without teaching the compiler pipeline their domain-specific data.
package projectintegration

import (
	"fmt"
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/official"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/token"
)

type Source struct {
	Filename   string
	ModulePath string
	Program    *ast.Program
}

type Context struct {
	Sources           []Source
	Resolutions       map[string]resolver.Result
	SourceRoot        string
	ProjectRoot       string
	PackageOptions    map[string][]byte
	JobsConfiguration string
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
}

type Analysis struct {
	contributions []Contribution
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
	for _, source := range context.Sources {
		activeModules[source.ModulePath] = true
	}

	packageNames := official.Names()
	sort.Strings(packageNames)
	analysis := Analysis{}
	extensionOwners := map[string]string{}
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
			if augmenter, ok := contribution.Extension.(interface{ Augment(*ir.Program) }); ok {
				augmenter.Augment(program)
			}
		}
		if contribution.Extension != nil && (entrypoint || contribution.AllPrograms) {
			program.Extensions = append(program.Extensions, contribution.Extension)
		}
	}
}
