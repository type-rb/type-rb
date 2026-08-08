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
	case "trb.web.middleware.logger":
		return webLoggerSymbols()
	default:
		panic(fmt.Sprintf("unknown official package semantic provider %q", provider))
	}
}

func webLoggerSymbols() map[string]stdlib.Symbol {
	return map[string]stdlib.Symbol{
		"call": {
			Name:      "call",
			Intrinsic: "trb.web.middleware.logger.call",
			Parameters: []stdlib.Parameter{
				{Name: "context", Type: types.FromName("Context")},
				{Name: "next_handler", Type: types.FromName("Next")},
				{Name: "options", Type: types.FromName("Options"), Optional: true},
			},
			Return: types.FromName("Response"),
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
	jsonError := types.FromName("JsonError")
	jsonResult := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{typeT, jsonError}}
	encodingResult := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{types.FromName("String"), jsonError}}
	return map[string]stdlib.Symbol{
		"serve": {
			Name:       "serve",
			Intrinsic:  "trb.web.serve",
			Parameters: []stdlib.Parameter{{Name: "port", Type: types.FromName("Integer"), Optional: true}},
			Return:     types.FromName("Void"),
		},
		"request_json": {
			Name:           "request_json",
			Intrinsic:      "trb.web.request_json",
			TypeParameters: []string{"T"},
			Parameters:     []stdlib.Parameter{{Name: "request", Type: types.FromName("Request")}},
			Return:         jsonResult,
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
