package projectintegration

import (
	"github.com/type-rb/type-rb/internal/ast"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
)

func init() {
	register(ormintegration.ProjectProvider, analyzeORM)
}

func analyzeORM(context Context) (Contribution, []Issue) {
	programs := make([]*ast.Program, 0, len(context.Sources))
	for _, source := range context.Sources {
		programs = append(programs, source.Program)
	}
	manifest, err := ormintegration.Analyze(programs, context.ProjectRoot, context.PackageOptions)
	if err != nil {
		return Contribution{}, []Issue{{Message: err.Error()}}
	}
	return Contribution{Extension: manifest, AllPrograms: true}, nil
}
