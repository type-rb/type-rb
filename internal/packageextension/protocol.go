// Package packageextension defines the versioned, data-only boundary used by
// compiler-integrated packages. Providers receive resolved semantic facts and
// return ordinary TypeRB source plus a narrow call replacement; they never
// receive parser, checker, or typed-IR objects.
package packageextension

import (
	"fmt"
	"strings"
)

const ProtocolVersion = 1

type Definition struct {
	ModulePath string `json:"modulePath,omitempty"`
	ImportPath string `json:"importPath,omitempty"`
}

type Type struct {
	Kind       string      `json:"kind"`
	Name       string      `json:"name,omitempty"`
	Nullable   bool        `json:"nullable,omitempty"`
	Arguments  []Type      `json:"arguments,omitempty"`
	Definition *Definition `json:"definition,omitempty"`
	Record     *Record     `json:"record,omitempty"`
}

type Record struct {
	Fields []Field `json:"fields"`
}

type Field struct {
	Name string `json:"name"`
	Type Type   `json:"type"`
}

type CallSite struct {
	ID         string `json:"id"`
	ModulePath string `json:"modulePath"`
}

type SpecializeCallRequest struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Provider        string   `json:"provider"`
	CallSite        CallSite `json:"callSite"`
	TypeArguments   []Type   `json:"typeArguments"`
}

type ValueSource string

const ReceiverValue ValueSource = "receiver"

type Replacement struct {
	Callee    string        `json:"callee"`
	Arguments []ValueSource `json:"arguments"`
}

type GeneratedSource struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

type RequiredImport struct {
	Path    string   `json:"path"`
	Symbols []string `json:"symbols"`
}

type Issue struct {
	Message string `json:"message"`
}

type SpecializeCallResponse struct {
	ProtocolVersion int              `json:"protocolVersion"`
	GeneratedSource *GeneratedSource `json:"generatedSource,omitempty"`
	Replacement     *Replacement     `json:"replacement,omitempty"`
	RequiredImports []RequiredImport `json:"requiredImports,omitempty"`
	Issues          []Issue          `json:"issues,omitempty"`
}

type CallProvider func(SpecializeCallRequest) SpecializeCallResponse

var callProviders = map[string]CallProvider{}

func RegisterCallProvider(name string, provider CallProvider) {
	if name == "" || provider == nil {
		panic("package extension call provider requires a name and implementation")
	}
	if _, exists := callProviders[name]; exists {
		panic("package extension call provider " + name + " is already registered")
	}
	callProviders[name] = provider
}

func SpecializeCall(request SpecializeCallRequest) (SpecializeCallResponse, error) {
	if request.ProtocolVersion != ProtocolVersion {
		return SpecializeCallResponse{}, fmt.Errorf("unsupported package extension protocol version %d", request.ProtocolVersion)
	}
	provider := callProviders[request.Provider]
	if provider == nil {
		return SpecializeCallResponse{}, fmt.Errorf("package call specializer %q is not registered", request.Provider)
	}
	response := provider(request)
	if response.ProtocolVersion != ProtocolVersion {
		return SpecializeCallResponse{}, fmt.Errorf("package call specializer %q returned protocol version %d", request.Provider, response.ProtocolVersion)
	}
	if len(response.Issues) > 0 {
		return response, nil
	}
	if response.GeneratedSource == nil || response.GeneratedSource.ID == "" || response.GeneratedSource.Source == "" {
		return SpecializeCallResponse{}, fmt.Errorf("package call specializer %q did not return generated TypeRB source", request.Provider)
	}
	if response.Replacement == nil || !strings.HasPrefix(response.Replacement.Callee, "__trb") {
		return SpecializeCallResponse{}, fmt.Errorf("package call specializer %q did not return a compiler-reserved call replacement", request.Provider)
	}
	for _, imported := range response.RequiredImports {
		if imported.Path == "" || len(imported.Symbols) == 0 {
			return SpecializeCallResponse{}, fmt.Errorf("package call specializer %q returned an invalid required import", request.Provider)
		}
		for _, symbol := range imported.Symbols {
			if symbol == "" {
				return SpecializeCallResponse{}, fmt.Errorf("package call specializer %q returned an invalid required import", request.Provider)
			}
		}
	}
	return response, nil
}
