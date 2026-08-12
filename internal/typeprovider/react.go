package typeprovider

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/types"
)

const reactTypeProvider = "trb.typescript.react"
const reactFormTypeProvider = "trb.typescript.react.form"
const reactOidcTypeProvider = "trb.typescript.react.oidc"

func init() {
	register(reactTypeProvider, loadReact)
	register(reactFormTypeProvider, loadReactForm)
	register(reactOidcTypeProvider, loadReactOidc)
}

func loadReactOidc(_ []*ast.Program, _ Context) (*declaration.Catalog, error) {
	stringType := types.FromName("String")
	optionalString := stringType
	optionalString.Nullable = true
	principalType := types.FromName("OidcPrincipal")
	principalType.Nullable = true
	eventHandler := types.FunctionOf([]types.Type{types.FromName("ReactEvent")}, types.FromName("Void"))
	state := declaration.NewType("ReactOidcState", "")
	state.InstanceMembers["loading"] = declaration.Member{Name: "loading", Kind: declaration.Property, Return: types.FromName("Boolean")}
	state.InstanceMembers["authenticated"] = declaration.Member{Name: "authenticated", Kind: declaration.Property, Return: types.FromName("Boolean")}
	state.InstanceMembers["principal"] = declaration.Member{Name: "principal", Kind: declaration.Property, Return: principalType}
	state.InstanceMembers["access_token"] = declaration.Member{Name: "access_token", Kind: declaration.Property, Return: optionalString}
	state.InstanceMembers["sign_in"] = declaration.Member{Name: "sign_in", Kind: declaration.Property, Return: eventHandler}
	state.InstanceMembers["sign_out"] = declaration.Member{Name: "sign_out", Kind: declaration.Property, Return: eventHandler}
	catalog := declaration.NewCatalog()
	catalog.Types[state.Name] = state
	return catalog, nil
}

func loadReactForm(_ []*ast.Program, _ Context) (*declaration.Catalog, error) {
	typeT := types.FromName("T")
	typeE := types.FromName("E")
	form := declaration.NewType("ReactForm", "")
	form.TypeParameters = []string{"T", "E"}
	form.InstanceMembers["value"] = declaration.Member{Name: "value", Kind: declaration.Property, Return: typeT}
	form.InstanceMembers["errors"] = declaration.Member{Name: "errors", Kind: declaration.Property, Return: typeE}
	form.InstanceMembers["dirty"] = declaration.Member{Name: "dirty", Kind: declaration.Property, Return: types.FromName("Boolean")}
	form.InstanceMembers["submitting"] = declaration.Member{Name: "submitting", Kind: declaration.Property, Return: types.FromName("Boolean")}
	form.InstanceMembers["set_value"] = declaration.Member{Name: "set_value", Kind: declaration.Method, Parameters: []declaration.Parameter{{Name: "value", Type: typeT}}, Return: types.FromName("Void")}
	form.InstanceMembers["set_errors"] = declaration.Member{Name: "set_errors", Kind: declaration.Method, Parameters: []declaration.Parameter{{Name: "errors", Type: typeE}}, Return: types.FromName("Void")}
	form.InstanceMembers["set_submitting"] = declaration.Member{Name: "set_submitting", Kind: declaration.Method, Parameters: []declaration.Parameter{{Name: "submitting", Type: types.FromName("Boolean")}}, Return: types.FromName("Void")}
	form.InstanceMembers["clear_errors"] = declaration.Member{Name: "clear_errors", Kind: declaration.Method, Return: types.FromName("Void")}
	form.InstanceMembers["reset"] = declaration.Member{Name: "reset", Kind: declaration.Method, Return: types.FromName("Void")}
	catalog := declaration.NewCatalog()
	catalog.Types[form.Name] = form
	return catalog, nil
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
