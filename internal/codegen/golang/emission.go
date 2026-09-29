package golang

import (
	"crypto/sha256"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

// A source module keeps its identity when the backend changes emission groups.
type goEmissionGroup struct{ directory, name string }
type goEmissionLayout map[string]goEmissionGroup

func emissionGroup(program *ir.Program) goEmissionGroup {
	if program.CompilationUnit == "$application" {
		return goEmissionGroup{"trb/application", "application"}
	}
	if program.CompilationUnit != "" {
		return goEmissionGroup{program.CompilationUnit, goIdentifier(path.Base(program.CompilationUnit), false)}
	}
	return goEmissionGroup{goPackageGroup(program.ModulePath), program.Package}
}

func emissionLayout(programs []*ir.Program) goEmissionLayout {
	result := goEmissionLayout{}
	for _, program := range programs {
		result[program.ModulePath] = emissionGroup(program)
	}
	return result
}

func (layout goEmissionLayout) directory(module string) string {
	if group, ok := layout[module]; ok {
		return group.directory
	}
	return goPackageGroup(module)
}

func ProjectOutputPath(program *ir.Program) string {
	if program.CompilationUnit == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(program.ModulePath))
	return path.Join(emissionGroup(program).directory, fmt.Sprintf("module_%x.go", sum[:16]))
}

func ProjectEntryPath(program *ir.Program) string {
	sum := sha256.Sum256([]byte(program.ModulePath))
	return fmt.Sprintf("trb/entry/%x/main.go", sum[:16])
}

func moduleInitializer(module string) string {
	sum := sha256.Sum256([]byte(module))
	return fmt.Sprintf("TrbInitialize_%x", sum[:16])
}

func ProjectEntrypoint(program *ir.Program, programs []*ir.Program) string {
	if program.CompilationUnit == "" {
		return ""
	}
	entry := false
	for _, statement := range program.Statements {
		if method, ok := statement.(*ir.Method); ok && goMethodSourceName(method) == "main" {
			entry = true
		}
	}
	if !entry {
		return ""
	}
	layout := emissionLayout(programs)
	names := analyzeGoProjectNames(programs)
	aliases := map[string]string{}
	var imports, calls strings.Builder
	qualifier := func(module string) string {
		directory := layout.directory(module)
		if alias, ok := aliases[directory]; ok {
			return alias
		}
		alias := fmt.Sprintf("unit%d", len(aliases))
		aliases[directory] = alias
		fmt.Fprintf(&imports, "\t%s %s\n", alias, strconv.Quote(path.Join(program.GoModule, directory)))
		return alias
	}
	for _, step := range program.InitializationSteps {
		name := moduleInitializer(step.Module)
		if step.Action != "" {
			name += "_" + step.Action
		}
		fmt.Fprintf(&calls, "\t%s.%s()\n", qualifier(step.Module), name)
	}
	fmt.Fprintf(&calls, "\t%s.%s()\n", qualifier(program.ModulePath), names.functions[program.ModulePath]["main"])
	return "package main\n\nimport (\n" + imports.String() + ")\n\nfunc main() {\n" + calls.String() + "}\n"
}

func (g *generator) sourceDirectory(module string) string {
	return g.projectNames.layout.directory(module)
}
