package languageservice

import "testing"

func TestDocumentSymbolsDescribeNestedDeclarations(t *testing.T) {
	source := `API_NAME := "TypeRB"

module Accounts
	record Profile
		name: String
	end

	class User
		readonly @id: Integer

		def display_name(prefix: String): String
			return prefix + "Ada"
		end
	end
end

def main()
	return
end
`
	symbols := DocumentSymbols(source)
	if len(symbols) != 3 {
		t.Fatalf("symbols=%#v", symbols)
	}
	if symbols[0].Name != "API_NAME" || symbols[0].Kind != DocumentSymbolConstant {
		t.Fatalf("constant=%#v", symbols[0])
	}
	module := symbols[1]
	if module.Name != "Accounts" || module.Kind != DocumentSymbolModule || len(module.Children) != 2 {
		t.Fatalf("module=%#v", module)
	}
	if module.Children[0].Name != "Profile" || module.Children[0].Kind != DocumentSymbolRecord || len(module.Children[0].Children) != 1 {
		t.Fatalf("record=%#v", module.Children[0])
	}
	class := module.Children[1]
	if class.Name != "User" || class.Kind != DocumentSymbolClass || len(class.Children) != 2 {
		t.Fatalf("class=%#v", class)
	}
	if class.Children[1].Name != "display_name" || class.Children[1].Kind != DocumentSymbolMethod || class.Children[1].Detail != "(prefix: String): String" {
		t.Fatalf("method=%#v", class.Children[1])
	}
	if symbols[2].Name != "main" || symbols[2].Kind != DocumentSymbolFunction {
		t.Fatalf("main=%#v", symbols[2])
	}
	selection := source[symbols[1].SelectionRange.Start:symbols[1].SelectionRange.End]
	if selection != "Accounts" {
		t.Fatalf("selection=%q", selection)
	}
}

func TestDocumentSymbolsRemainAvailableWithTypeErrors(t *testing.T) {
	symbols := DocumentSymbols("record User\n\tid: Missing\nend\n")
	if len(symbols) != 1 || symbols[0].Name != "User" || len(symbols[0].Children) != 1 {
		t.Fatalf("symbols=%#v", symbols)
	}
}
