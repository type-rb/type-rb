package typeprovider

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/types"
)

const webTypeProvider = "trb.web"

func init() {
	register(webTypeProvider, loadWeb, staticProviderInputs)
}

func loadWeb(_ []*ast.Program, _ Context) (*declaration.Catalog, error) {
	typeT := types.FromName("T")
	parameterResult := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{typeT, types.FromName("ParameterError")}}
	request := declaration.NewType("Request", "")
	request.InstanceMembers["json"] = declaration.Member{
		Name:           "json",
		Kind:           declaration.Method,
		Intrinsic:      "trb.web.request_json",
		Return:         types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{typeT, types.FromName("RequestError")}},
		TypeParameters: []string{"T"},
		Provider:       webTypeProvider,
	}
	request.InstanceMembers["query"] = declaration.Member{
		Name:           "query",
		Kind:           declaration.Method,
		Intrinsic:      "trb.web.request_query",
		Return:         parameterResult,
		TypeParameters: []string{"T"},
		Provider:       webTypeProvider,
	}
	context := declaration.NewType("Context", "")
	context.InstanceMembers["params"] = declaration.Member{
		Name:           "params",
		Kind:           declaration.Method,
		Intrinsic:      "trb.web.context_params",
		Return:         parameterResult,
		TypeParameters: []string{"T"},
		Provider:       webTypeProvider,
	}
	context.InstanceMembers["bind"] = declaration.Member{
		Name:           "bind",
		Kind:           declaration.Method,
		Specializer:    "trb.web.bind",
		Return:         types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{typeT, types.FromName("EndpointInputError")}},
		TypeParameters: []string{"T"},
		Provider:       webTypeProvider,
	}

	catalog := declaration.NewCatalog()
	catalog.Types[request.Name] = request
	catalog.Types[context.Name] = context
	return catalog, nil
}
