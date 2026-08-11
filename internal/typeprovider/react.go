package typeprovider

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/types"
)

const reactTypeProvider = "trb.typescript.react"

func init() {
	register(reactTypeProvider, loadReact)
}

func loadReact(_ []*ast.Program, _ Context) (*declaration.Catalog, error) {
	typeT := types.FromName("T")
	state := declaration.NewType("ReactState", "")
	state.TypeParameters = []string{"T"}
	state.InstanceMembers["value"] = declaration.Member{
		Name:   "value",
		Kind:   declaration.Property,
		Return: typeT,
	}
	state.InstanceMembers["set"] = declaration.Member{
		Name:       "set",
		Kind:       declaration.Method,
		Parameters: []declaration.Parameter{{Name: "value", Type: typeT}},
		Return:     types.FromName("Void"),
	}
	catalog := declaration.NewCatalog()
	catalog.Types[state.Name] = state
	return catalog, nil
}
