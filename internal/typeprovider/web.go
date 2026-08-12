package typeprovider

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/types"
)

const webTypeProvider = "trb.web"

func init() {
	register(webTypeProvider, loadWeb)
}

func loadWeb(_ []*ast.Program, _ Context) (*declaration.Catalog, error) {
	typeT := types.FromName("T")
	request := declaration.NewType("Request", "")
	request.InstanceMembers["json"] = declaration.Member{
		Name:           "json",
		Kind:           declaration.Method,
		Intrinsic:      "trb.web.request_json",
		Return:         types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{typeT, types.FromName("RequestError")}},
		TypeParameters: []string{"T"},
		Provider:       webTypeProvider,
	}

	catalog := declaration.NewCatalog()
	catalog.Types[request.Name] = request
	return catalog, nil
}
