// Package testinspection preserves authored declaration names for test diagnostics.
package testinspection

import (
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
)

// Plan is shared by backends; target-language names never determine display names.
type Plan struct {
	Enabled bool
	Names   map[identity.Declaration]string
	Records map[identity.Declaration]*ir.Record
	Enums   map[identity.Declaration]*ir.Enum
	Classes map[identity.Declaration]*ir.Class
}

func Analyze(programs []*ir.Program) *Plan {
	p := &Plan{Names: map[identity.Declaration]string{}, Records: map[identity.Declaration]*ir.Record{}, Enums: map[identity.Declaration]*ir.Enum{}, Classes: map[identity.Declaration]*ir.Class{}}
	owners := map[string]map[identity.Declaration]bool{}
	var visit func([]ir.Statement)
	visit = func(statements []ir.Statement) {
		for _, statement := range statements {
			var declaration identity.Declaration
			switch n := statement.(type) {
			case *ir.Record:
				declaration = n.Declaration
				p.Records[declaration] = n
			case *ir.Class:
				declaration = n.Declaration
				p.Classes[declaration] = n
			case *ir.Enum:
				declaration = n.Declaration
				p.Enums[declaration] = n
			case *ir.Module:
				visit(n.Body)
			}
			if !declaration.Empty() {
				if owners[declaration.Name] == nil {
					owners[declaration.Name] = map[identity.Declaration]bool{}
				}
				owners[declaration.Name][declaration] = true
			}
		}
	}
	for _, program := range programs {
		p.Enabled = p.Enabled || program.ModulePath == "trb/std/test/index"
		visit(program.Statements)
	}
	for name, declarations := range owners {
		for declaration := range declarations {
			display := name
			if len(declarations) > 1 {
				display = declaration.Module + "::" + name
			}
			p.Names[declaration] = display
		}
	}
	return p
}

func (p *Plan) Name(declaration identity.Declaration, fallback string) string {
	if p != nil {
		if name := p.Names[declaration]; name != "" {
			return name
		}
	}
	if declaration.Name != "" {
		return declaration.Name
	}
	return fallback
}
