package typescript

import (
	"encoding/hex"
	"strings"

	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
)

// Module members have file-level identities. Emit ordinary JavaScript values
// and functions, so Node can erase annotations without transforming namespaces.
type moduleNames struct {
	modules   map[*ir.Module]string
	owners    map[*ir.Module]string
	constants map[string]string
	methods   map[*ir.Method]string
	calls     map[identity.Dispatch]string
}

func analyzeModuleNames(statements []ir.Statement) *moduleNames {
	names := &moduleNames{
		modules: map[*ir.Module]string{}, owners: map[*ir.Module]string{},
		constants: map[string]string{}, methods: map[*ir.Method]string{},
		calls: map[identity.Dispatch]string{},
	}
	var collect func([]ir.Statement, string)
	collect = func(statements []ir.Statement, owner string) {
		for _, statement := range statements {
			switch node := statement.(type) {
			case *ir.Module:
				qualified := identity.Qualify(owner, node.Name)
				names.owners[node] = qualified
				names.modules[node] = node.Name
				if owner != "" {
					names.modules[node] = tsModuleMemberName("module", qualified)
				}
				collect(node.Body, qualified)
			case *ir.Variable:
				if node.Constant && owner != "" {
					qualified := identity.Qualify(owner, node.Name)
					names.constants[qualified] = tsModuleMemberName("constant", qualified)
				}
			case *ir.Method:
				if owner != "" {
					name := tsModuleMemberName("method", identity.Qualify(owner, node.Name))
					if node.TargetName != "" {
						name = tsCallableName(node.TargetName)
					}
					names.methods[node] = name
					if !node.Dispatch.Empty() {
						names.calls[node.Dispatch] = name
					}
				}
			}
		}
	}
	collect(statements, "")
	return names
}

func tsModuleMemberName(kind, qualified string) string {
	return "$trb$" + kind + "$" + hex.EncodeToString([]byte(qualified))
}

func (g *generator) module(module *ir.Module) {
	owner := g.moduleNames.owners[module]
	properties := []string{}
	for _, statement := range module.Body {
		switch node := statement.(type) {
		case *ir.Method:
			g.function(node)
			if !strings.HasPrefix(node.Name, "_") {
				properties = append(properties, tsMethodName(node.Name)+": "+g.moduleNames.methods[node])
			}
		case *ir.Variable:
			g.statement(node)
			properties = append(properties, node.Name+": "+g.moduleNames.constants[identity.Qualify(owner, node.Name)])
		case *ir.Module:
			g.statement(node)
			properties = append(properties, node.Name+": "+g.moduleNames.modules[node])
		case *ir.Class, *ir.Record, *ir.Enum, *ir.Interface, *ir.TypeAlias, *ir.Newtype:
			g.ownedModuleDeclarations([]ir.Statement{statement}, owner)
		default:
			g.statement(statement)
		}
	}
	g.line("export const " + g.moduleNames.modules[module] + " = { " + strings.Join(properties, ", ") + " } as const;")
}
