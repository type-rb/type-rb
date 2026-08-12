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
	stringType := types.FromName("String")
	booleanType := types.FromName("Boolean")
	integerType := types.FromName("Integer")
	voidType := types.FromName("Void")

	element := declaration.NewType("HTMLElement", "")
	element.InstanceMembers["id"] = declaration.Member{Name: "id", Kind: declaration.Property, Return: stringType, Provider: reactTypeProvider}

	input := declaration.NewType("HTMLInputElement", "HTMLElement")
	input.InstanceMembers["value"] = declaration.Member{Name: "value", Kind: declaration.Property, Return: stringType, Provider: reactTypeProvider}
	input.InstanceMembers["checked"] = declaration.Member{Name: "checked", Kind: declaration.Property, Return: booleanType, Provider: reactTypeProvider}

	form := declaration.NewType("HTMLFormElement", "HTMLElement")

	synthetic := declaration.NewType("SyntheticEvent", "")
	synthetic.InstanceMembers["defaultPrevented"] = declaration.Member{Name: "defaultPrevented", Kind: declaration.Property, Return: booleanType, Provider: reactTypeProvider}
	synthetic.InstanceMembers["preventDefault"] = declaration.Member{Name: "preventDefault", Kind: declaration.Method, Return: voidType, Provider: reactTypeProvider}
	synthetic.InstanceMembers["stopPropagation"] = declaration.Member{Name: "stopPropagation", Kind: declaration.Method, Return: voidType, Provider: reactTypeProvider}

	mouse := declaration.NewType("MouseEvent", "SyntheticEvent")
	mouse.InstanceMembers["currentTarget"] = declaration.Member{Name: "currentTarget", Kind: declaration.Property, Return: types.FromName("HTMLElement"), Provider: reactTypeProvider}
	mouse.InstanceMembers["button"] = declaration.Member{Name: "button", Kind: declaration.Property, Return: integerType, Provider: reactTypeProvider}

	change := declaration.NewType("ChangeEvent", "SyntheticEvent")
	change.InstanceMembers["currentTarget"] = declaration.Member{Name: "currentTarget", Kind: declaration.Property, Return: types.FromName("HTMLInputElement"), Provider: reactTypeProvider}

	formEvent := declaration.NewType("FormEvent", "SyntheticEvent")
	formEvent.InstanceMembers["currentTarget"] = declaration.Member{Name: "currentTarget", Kind: declaration.Property, Return: types.FromName("HTMLFormElement"), Provider: reactTypeProvider}

	keyboard := declaration.NewType("KeyboardEvent", "SyntheticEvent")
	keyboard.InstanceMembers["currentTarget"] = declaration.Member{Name: "currentTarget", Kind: declaration.Property, Return: types.FromName("HTMLElement"), Provider: reactTypeProvider}
	keyboard.InstanceMembers["key"] = declaration.Member{Name: "key", Kind: declaration.Property, Return: stringType, Provider: reactTypeProvider}
	keyboard.InstanceMembers["code"] = declaration.Member{Name: "code", Kind: declaration.Property, Return: stringType, Provider: reactTypeProvider}

	catalog := declaration.NewCatalog()
	for _, declarationType := range []*declaration.Type{element, input, form, synthetic, mouse, change, formEvent, keyboard} {
		catalog.Types[declarationType.Name] = declarationType
	}
	return catalog, nil
}
