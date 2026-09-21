package ruby

import (
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/codegen/naming"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
)

// Reopening a namespace in one source module preserves its identity. The same
// authored root in another source module must not reopen Ruby's existing module.
// Descendants follow the renamed root through their checked declaration names.
func analyzeRubyNamespaceNames(programs []*ir.Program) map[string]map[string]string {
	owners := map[string]map[string]bool{}
	occupied := map[string]bool{}
	namespaces := map[string]identity.Declaration{}
	for _, program := range programs {
		for _, statement := range program.Statements {
			var name string
			switch node := statement.(type) {
			case *ir.Module:
				declaration := node.Declaration
				name = node.Name
				root, _, _ := strings.Cut(declaration.Name, "::")
				declaration.Name = root
				namespaces[declaration.Key()] = declaration
			case *ir.Class:
				name = node.Name
			case *ir.Record:
				name = node.Name
			case *ir.Enum:
				name = node.Name
			case *ir.Interface:
				name = node.Name
			case *ir.Newtype:
				name = node.Name
			case *ir.TypeAlias:
				name = node.Name
			case *ir.Variable:
				if node.Constant && node.Owner == "" {
					name = node.Name
				}
			}
			if name == "" {
				continue
			}
			root, _, _ := strings.Cut(name, "::")
			occupied[root] = true
			if owners[root] == nil {
				owners[root] = map[string]bool{}
			}
			owners[root][program.ModulePath] = true
		}
	}
	keys := make([]string, 0, len(namespaces))
	for key := range namespaces {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := map[string]map[string]string{}
	for _, key := range keys {
		declaration := namespaces[key]
		if declaration.Empty() || len(owners[declaration.Name]) < 2 || strings.HasPrefix(declaration.Module, "trb/std/") {
			continue
		}
		target := "TrbNamespace_" + naming.PrivateSuffix(key)
		for occupied[target] {
			target += "_"
		}
		occupied[target] = true
		if result[declaration.Module] == nil {
			result[declaration.Module] = map[string]string{}
		}
		result[declaration.Module][declaration.Name] = target
	}
	return result
}
