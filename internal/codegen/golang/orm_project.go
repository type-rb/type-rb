package golang

import (
	pathpkg "path"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
)

// goORMRuntimePlan selects one generated file per Go package to own the
// package-private ORM support runtime. Model-specific code remains beside its
// source module, while shared helpers must not be emitted once per model file.
type goORMRuntimePlan struct {
	owners map[string]string
	models map[string][]ormintegration.Model
}

func analyzeGoORMRuntime(programs []*ir.Program) *goORMRuntimePlan {
	plan := &goORMRuntimePlan{
		owners: map[string]string{},
		models: map[string][]ormintegration.Model{},
	}
	for _, program := range programs {
		manifest := ormintegration.ManifestFrom(program.Extensions)
		if manifest == nil {
			continue
		}
		models := manifest.ModelsForModule(program.ModulePath)
		if len(models) == 0 {
			continue
		}
		key := goORMPackageKey(program)
		owner := plan.owners[key]
		if owner == "" || program.ModulePath < owner {
			plan.owners[key] = program.ModulePath
		}
		plan.models[key] = append(plan.models[key], models...)
	}
	return plan
}

func goORMPackageKey(program *ir.Program) string {
	directory := pathpkg.Dir(program.ModulePath)
	if directory == "." {
		directory = ""
	}
	packageName := program.Package
	if packageName == "" {
		packageName = "main"
	}
	return directory + "\x00" + packageName
}
