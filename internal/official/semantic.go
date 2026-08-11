package official

import (
	"fmt"

	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

func semanticSymbols(provider string) map[string]stdlib.Symbol {
	switch provider {
	case "":
		return map[string]stdlib.Symbol{}
	case "trb.web":
		return webSymbols()
	case "trb.web.testing":
		return webTestingSymbols()
	case "trb.typescript.react":
		return reactSymbols()
	case "trb.typescript.browser":
		return browserSymbols()
	default:
		panic(fmt.Sprintf("unknown official package semantic provider %q", provider))
	}
}

func browserSymbols() map[string]stdlib.Symbol {
	typeT := types.FromName("T")
	jsonError := types.FromName("JsonError")
	jsonResult := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{typeT, jsonError}}
	return map[string]stdlib.Symbol{
		"get_json": {
			Name:                "get_json",
			Intrinsic:           "trb.platform.typescript.browser.get_json",
			RuntimeIndependent:  true,
			TypeParameters:      []string{"T"},
			Parameters:          []stdlib.Parameter{{Name: "url", Type: types.FromName("String")}},
			Return:              typeT,
			Fails:               types.FromName("FetchError"),
			RuntimeDependencies: []types.Type{jsonResult},
		},
		"put_json": {
			Name:                "put_json",
			Intrinsic:           "trb.platform.typescript.browser.put_json",
			RuntimeIndependent:  true,
			TypeParameters:      []string{"T"},
			Parameters:          []stdlib.Parameter{{Name: "url", Type: types.FromName("String")}, {Name: "value", Type: typeT}},
			Return:              typeT,
			Fails:               types.FromName("FetchError"),
			RuntimeDependencies: []types.Type{jsonResult},
		},
	}
}

func reactSymbols() map[string]stdlib.Symbol {
	stringType := types.FromName("String")
	integerType := types.FromName("Integer")
	booleanType := types.FromName("Boolean")
	anyType := types.FromName("Any")
	voidType := types.FromName("Void")
	return map[string]stdlib.Symbol{
		"element":         {Name: "element", Intrinsic: "trb.platform.typescript.react.element", Parameters: []stdlib.Parameter{{Name: "tag", Type: anyType}, {Name: "props", Type: types.FromName("Hash")}, {Name: "children", Type: types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{anyType}}}}, Return: types.FromName("ReactNode")},
		"mount":           {Name: "mount", Intrinsic: "trb.platform.typescript.react.mount", Parameters: []stdlib.Parameter{{Name: "component", Type: anyType}, {Name: "element_id", Type: stringType}}, Return: voidType},
		"refresh":         {Name: "refresh", Intrinsic: "trb.platform.typescript.react.refresh", Parameters: []stdlib.Parameter{{Name: "component", Type: anyType}}, Return: voidType},
		"prevent_default": {Name: "prevent_default", Intrinsic: "trb.platform.typescript.react.prevent_default", Parameters: []stdlib.Parameter{{Name: "event", Type: types.FromName("ReactEvent")}}, Return: voidType},
		"input_value":     {Name: "input_value", Intrinsic: "trb.platform.typescript.react.input_value", Parameters: []stdlib.Parameter{{Name: "event", Type: types.FromName("ReactEvent")}}, Return: stringType},
		"data_integer":    {Name: "data_integer", Intrinsic: "trb.platform.typescript.react.data_integer", Parameters: []stdlib.Parameter{{Name: "event", Type: types.FromName("ReactEvent")}, {Name: "name", Type: stringType}}, Return: integerType},
		"data_boolean":    {Name: "data_boolean", Intrinsic: "trb.platform.typescript.react.data_boolean", Parameters: []stdlib.Parameter{{Name: "event", Type: types.FromName("ReactEvent")}, {Name: "name", Type: stringType}}, Return: booleanType},
	}
}

func webTestingSymbols() map[string]stdlib.Symbol {
	return map[string]stdlib.Symbol{
		"dispatch": {
			Name:               "dispatch",
			Intrinsic:          "trb.web.testing.dispatch",
			RuntimeIndependent: true,
			Parameters:         []stdlib.Parameter{{Name: "request", Type: types.FromName("Request")}},
			Return:             types.FromName("Response"),
		},
	}
}

func webSymbols() map[string]stdlib.Symbol {
	typeT := types.FromName("T")
	jsonError := types.FromName("JsonError")
	jsonResult := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{typeT, jsonError}}
	requestError := types.FromName("RequestError")
	requestResult := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{typeT, requestError}}
	encodingResult := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{types.FromName("String"), jsonError}}
	return map[string]stdlib.Symbol{
		"configure_server": {
			Name:      "configure_server",
			Intrinsic: "trb.web.configure_server",
			Parameters: []stdlib.Parameter{
				{Name: "host", Type: types.FromName("String"), Optional: true, Keyword: true},
				{Name: "port", Type: types.FromName("Integer"), Optional: true, Keyword: true},
				{Name: "body_limit_bytes", Type: types.FromName("Integer"), Optional: true, Keyword: true},
				{Name: "shutdown_timeout_milliseconds", Type: types.FromName("Integer"), Optional: true, Keyword: true},
			},
			Return: types.FromName("ServerConfig"),
		},
		"serve": {
			Name:       "serve",
			Intrinsic:  "trb.web.serve",
			Parameters: []stdlib.Parameter{{Name: "config", Type: types.FromName("ServerConfig"), Optional: true}},
			Return:     types.FromName("Void"),
		},
		"request_json": {
			Name:                "request_json",
			Intrinsic:           "trb.web.request_json",
			TypeParameters:      []string{"T"},
			Parameters:          []stdlib.Parameter{{Name: "request", Type: types.FromName("Request")}},
			Return:              requestResult,
			RuntimeDependencies: []types.Type{jsonResult},
		},
		"json": {
			Name:                "json",
			Intrinsic:           "trb.web.json",
			TypeParameters:      []string{"T"},
			Parameters:          []stdlib.Parameter{{Name: "value", Type: typeT}, {Name: "status", Type: types.FromName("Integer"), Optional: true}},
			Return:              types.FromName("Response"),
			RuntimeDependencies: []types.Type{encodingResult},
		},
	}
}
