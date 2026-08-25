package typeprovider

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
	"github.com/type-rb/type-rb/internal/types"
	webintegration "github.com/type-rb/type-rb/internal/web"
)

const webTypeProvider = "trb.web"

func init() {
	register(webTypeProvider, loadWeb, webProviderInputs)
}

func loadWeb(programs []*ast.Program, context Context) (*declaration.Catalog, error) {
	catalog := webBaseDeclarations()
	provided, err := loadWebContractDeclarations(programs, context)
	if err != nil {
		return nil, err
	}
	contracts, err := packageextensionhost.ImportDeclarationCatalog(provided)
	if err != nil {
		return nil, err
	}
	catalog.Merge(contracts)
	return catalog, nil
}

func webBaseDeclarations() *declaration.Catalog {
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
	return catalog
}

func loadWebContractDeclarations(programs []*ast.Program, context Context) (packageextension.DeclarationCatalog, error) {
	input, err := webDeclarationInput(programs, context)
	if err != nil {
		return packageextension.DeclarationCatalog{}, err
	}
	catalog, err := webintegration.Declarations(input)
	if err != nil {
		return packageextension.DeclarationCatalog{}, err
	}
	return packageextensionhost.ExportDeclarationCatalog(webintegration.PackageName, catalog)
}

func webDeclarationInput(programs []*ast.Program, context Context) (packageextension.ProjectDeclarationInput, error) {
	relevant := providerPrograms(programs, webProviderProgram)
	return packageextensionhost.ExportProjectDeclarationInput(webintegration.PackageName, relevant, packageextensionhost.ProjectDeclarationInputOptions{
		PackageAliasesByModule: context.PackageAliasesByModule,
	})
}

func webProviderInputs(programs []*ast.Program, _ Context) providerInputSnapshot {
	return providerInputSnapshot{programs: providerPrograms(programs, webProviderProgram), reusable: true}
}

func webProviderProgram(program *ast.Program) bool {
	for _, statement := range program.Statements {
		switch statement.(type) {
		case *ast.TypeAliasStatement, *ast.NewtypeStatement, *ast.RecordStatement, *ast.EnumStatement:
			return true
		case *ast.ClassStatement:
			// Include every class module so an Endpoint alias imported from a
			// declaration-only module can still be resolved canonically.
			return true
		}
	}
	return false
}
