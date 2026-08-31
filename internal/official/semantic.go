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
	case "trb.typescript.browser":
		return typescriptBrowserSymbols()
	case "trb.typescript.react":
		return reactSymbols()
	case "trb.cli":
		return cliSymbols()
	default:
		panic(fmt.Sprintf("unknown official package semantic provider %q", provider))
	}
}

func cliSymbols() map[string]stdlib.Symbol {
	typeT := types.FromName("T")
	optionalString := types.FromName("String")
	optionalString.Nullable = true
	return map[string]stdlib.Symbol{
		"fail": {
			Name:               "fail",
			Intrinsic:          "trb.cli.fail",
			RuntimeIndependent: true,
			Parameters:         []stdlib.Parameter{{Name: "message", Type: types.FromName("String")}},
			Return:             types.Type{Kind: types.Never, Name: "Never"},
		},
		"run": {
			Name:               "run",
			Intrinsic:          "trb.cli.run",
			RuntimeIndependent: true,
			TypeParameters:     []string{"T"},
			Parameters: []stdlib.Parameter{
				{Name: "name", Type: types.FromName("String"), Keyword: true},
				{Name: "version", Type: optionalString, Optional: true, Keyword: true},
				{Name: "about", Type: optionalString, Optional: true, Keyword: true},
			},
			Return: typeT,
		},
	}
}

func semanticJSX(provider string) *stdlib.JSXProvider {
	if provider != "trb.typescript.react" {
		return nil
	}
	callback := func(event string) types.Type {
		return types.FunctionOf([]types.Type{types.FromName(event)}, types.FromName("Void"))
	}
	return &stdlib.JSXProvider{
		Node: types.FromName("ReactNode"),
		IntrinsicAttributes: map[string]types.Type{
			"className": types.FromName("String"),
			"id":        types.FromName("String"),
			"name":      types.FromName("String"),
			"type":      types.FromName("String"),
			"value":     types.FromName("String"),
			"checked":   types.FromName("Boolean"),
			"disabled":  types.FromName("Boolean"),
			"onClick":   callback("MouseEvent"),
			"onChange":  callback("ChangeEvent"),
			"onSubmit":  callback("FormEvent"),
			"onKeyDown": callback("KeyboardEvent"),
			"onKeyUp":   callback("KeyboardEvent"),
		},
	}
}

func typescriptBrowserSymbols() map[string]stdlib.Symbol {
	typeT := types.FromName("T")
	nullable := func(typ types.Type) types.Type {
		typ.Nullable = true
		return typ
	}
	responseOf := func(body types.Type) types.Type {
		return types.Type{Kind: types.Named, Name: "Response", Args: []types.Type{body}}
	}
	resultOf := func(success, failure types.Type) types.Type {
		return types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{success, failure}}
	}
	body := types.FromName("Body")
	requestError := types.FromName("RequestError")
	fileReadError := types.FromName("FileReadError")
	jsonRuntime := []types.Type{types.FromName("JSON::Value")}
	return map[string]stdlib.Symbol{
		"read": {
			Name:      "read",
			Intrinsic: "trb.platform.typescript.browser.file_read",
			Receiver:  types.FromName("File"),
			Return:    resultOf(types.FromName("Bytes"), fileReadError),
		},
		"read_text": {
			Name:      "read_text",
			Intrinsic: "trb.platform.typescript.browser.file_read_text",
			Receiver:  types.FromName("File"),
			Return:    resultOf(types.FromName("String"), fileReadError),
		},
		"request": {
			Name:      "request",
			Intrinsic: "trb.platform.typescript.browser.request",
			Receiver:  types.FromName("HttpClient"),
			Parameters: []stdlib.Parameter{
				{Name: "path", Type: types.FromName("String")},
				{Name: "method", Type: types.FromName("HttpMethod"), Optional: true, Keyword: true},
				{Name: "query", Type: types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("URL::QueryParameter")}}, Optional: true, Keyword: true},
				{Name: "headers", Type: types.FromName("Headers"), Optional: true, Keyword: true},
				{Name: "body", Type: nullable(types.FromName("RequestBody")), Optional: true, Keyword: true},
				{Name: "timeout_milliseconds", Type: nullable(types.FromName("Integer")), Optional: true, Keyword: true},
			},
			Return: resultOf(responseOf(body), requestError),
		},
		"json": {
			Name:                "json",
			Intrinsic:           "trb.platform.typescript.browser.response_json",
			Receiver:            responseOf(body),
			TypeParameters:      []string{"T"},
			Return:              resultOf(responseOf(typeT), requestError),
			RuntimeDependencies: jsonRuntime,
		},
		"text": {
			Name:      "text",
			Intrinsic: "trb.platform.typescript.browser.response_text",
			Receiver:  responseOf(body),
			Return:    responseOf(types.FromName("String")),
		},
		"bytes": {
			Name:      "bytes",
			Intrinsic: "trb.platform.typescript.browser.response_bytes",
			Receiver:  responseOf(body),
			Return:    responseOf(types.FromName("Bytes")),
		},
		"no_body": {
			Name:      "no_body",
			Intrinsic: "trb.platform.typescript.browser.response_no_body",
			Receiver:  responseOf(body),
			Return:    resultOf(responseOf(types.FromName("NoBody")), requestError),
		},
		"json_body": {
			Name:                "json_body",
			Intrinsic:           "trb.platform.typescript.browser.json_body",
			TypeParameters:      []string{"T"},
			Parameters:          []stdlib.Parameter{{Name: "value", Type: typeT}},
			Return:              resultOf(types.FromName("RequestBody"), requestError),
			RuntimeDependencies: jsonRuntime,
		},
	}
}

func reactSymbols() map[string]stdlib.Symbol {
	typeT := types.FromName("T")
	node := types.FromName("ReactNode")
	return map[string]stdlib.Symbol{
		"mount": {
			Name:      "mount",
			Intrinsic: "trb.platform.typescript.react.mount",
			Parameters: []stdlib.Parameter{
				{Name: "node", Type: node},
				{Name: "element_id", Type: types.FromName("String")},
			},
			Return: types.FromName("Void"),
		},
		"use_state": {
			Name:               "use_state",
			Intrinsic:          "trb.platform.typescript.react.use_state",
			RuntimeIndependent: true,
			TypeParameters:     []string{"T"},
			Parameters:         []stdlib.Parameter{{Name: "initial", Type: typeT}},
			Return:             types.Type{Kind: types.Named, Name: "ReactState", Args: []types.Type{typeT}},
		},
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
	jsonError := types.FromName("JSON::Error")
	encodingResult := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{types.FromName("String"), jsonError}}
	contextKey := types.Type{Kind: types.Named, Name: "ContextKey", Args: []types.Type{typeT}}
	contextValueError := types.FromName("ContextValueError")
	contextValueResult := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{typeT, contextValueError}}
	return map[string]stdlib.Symbol{
		"serve": {
			Name:        "serve",
			Intrinsic:   "trb.web.serve",
			StaticOwner: "Web",
			Parameters:  []stdlib.Parameter{{Name: "config", Type: types.FromName("Web::ServerConfig"), Optional: true}},
			Return:      types.FromName("Void"),
		},
		"json": {
			Name:                "json",
			Intrinsic:           "trb.web.json",
			StaticOwner:         "Response",
			TypeParameters:      []string{"T"},
			Parameters:          []stdlib.Parameter{{Name: "value", Type: typeT}, {Name: "status", Type: types.FromName("Integer"), Optional: true}},
			Return:              types.FromName("Response"),
			RuntimeDependencies: []types.Type{encodingResult},
		},
		"with": {
			Name:           "with",
			Intrinsic:      "trb.web.context_with",
			Receiver:       types.FromName("Context"),
			TypeParameters: []string{"T"},
			Parameters: []stdlib.Parameter{
				{Name: "key", Type: contextKey},
				{Name: "value", Type: typeT},
			},
			Return: types.FromName("Context"),
		},
		"with_request": {
			Name:       "with_request",
			Intrinsic:  "trb.web.context_with_request",
			Receiver:   types.FromName("Context"),
			Parameters: []stdlib.Parameter{{Name: "request", Type: types.FromName("Request")}},
			Return:     types.FromName("Context"),
		},
		"fetch": {
			Name:                "fetch",
			Intrinsic:           "trb.web.context_fetch",
			Receiver:            types.FromName("Context"),
			TypeParameters:      []string{"T"},
			Parameters:          []stdlib.Parameter{{Name: "key", Type: contextKey}},
			Return:              contextValueResult,
			RuntimeDependencies: []types.Type{contextValueResult},
		},
	}
}
