package web

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/packageextension"
)

const EndpointCatalogProtocolVersion = 1

// EndpointCatalog is the versioned, target-independent contract exposed by
// trb/web. It binds authored endpoint declarations to file-based routes while
// retaining portable TypeRB type uses for downstream tools such as OpenAPI
// generators. It never contains parser nodes, checked method bodies, or
// backend-specific data.
type EndpointCatalog struct {
	ProtocolVersion int                `json:"protocolVersion"`
	Package         string             `json:"package"`
	Endpoints       []EndpointContract `json:"endpoints,omitempty"`
}

type EndpointContract struct {
	Name       string                           `json:"name"`
	ModulePath string                           `json:"modulePath"`
	Handler    string                           `json:"handler"`
	Method     string                           `json:"method"`
	Path       string                           `json:"path"`
	Input      *packageextension.ProjectTypeUse `json:"input,omitempty"`
	Responses  []EndpointResponse               `json:"responses"`
	Span       packageextension.SourceSpan      `json:"span"`
}

type EndpointResponse struct {
	Status int                             `json:"status"`
	Type   packageextension.ProjectTypeUse `json:"type"`
	Span   packageextension.SourceSpan     `json:"span"`
}

type EndpointContractIssue struct {
	ModulePath string
	Message    string
	Span       packageextension.SourceSpan
}

type endpointDeclaration struct {
	Name       string
	ModulePath string
	Handler    string
	Input      *packageextension.ProjectTypeUse
	Responses  []EndpointResponse
	Span       packageextension.SourceSpan
}

// Declarations returns the generic declaration capabilities required by
// endpoint contract classes. Calls remain ordinary, type-checked trb/web
// functions; only exact Endpoint subclasses discovered from this data-only
// project snapshot may treat them as non-runtime declarations.
func Declarations(input packageextension.ProjectDeclarationInput) (*declaration.Catalog, error) {
	if err := validateEndpointInput(input); err != nil {
		return nil, err
	}
	catalog := declaration.NewCatalog()
	for _, module := range input.Modules {
		for _, class := range module.Classes {
			if !isEndpointClass(class) {
				continue
			}
			for _, function := range []string{"handles", "input", "response"} {
				catalog.ClassBodyDeclarationRules = append(catalog.ClassBodyDeclarationRules, declaration.ClassBodyDeclarationRule{
					Package:  PackageName,
					Function: function,
					Owner:    declaration.DeclarationReference{ModulePath: module.ModulePath, Name: class.Name},
				})
			}
		}
	}
	return catalog, nil
}

// BuildEndpointCatalog validates authored endpoint declarations and binds them
// to the routes discovered from the same project. Routes without a contract
// remain valid; a contract must map to exactly one local file-route handler.
func BuildEndpointCatalog(input packageextension.ProjectDeclarationInput, routes []Route) (EndpointCatalog, []EndpointContractIssue, error) {
	result := EndpointCatalog{ProtocolVersion: EndpointCatalogProtocolVersion, Package: PackageName}
	if err := validateEndpointInput(input); err != nil {
		return result, nil, err
	}
	declarations, issues := discoverEndpointDeclarations(input)
	if len(issues) != 0 {
		return result, issues, nil
	}
	routesByHandler := map[string]Route{}
	for _, route := range routes {
		routesByHandler[route.ModulePath+"\x00"+route.Handler] = route
	}
	contractByRoute := map[string]string{}
	for _, declared := range declarations {
		key := declared.ModulePath + "\x00" + declared.Handler
		route, exists := routesByHandler[key]
		if !exists {
			issues = append(issues, endpointIssue(declared.ModulePath, declared.Span,
				fmt.Sprintf("trb/web endpoint contract %s handles %s, which is not a file-route handler in the same module", declared.Name, declared.Handler)))
			continue
		}
		if previous := contractByRoute[key]; previous != "" {
			issues = append(issues, endpointIssue(declared.ModulePath, declared.Span,
				fmt.Sprintf("trb/web route %s %s is declared by both %s and %s", route.Method, route.Path, previous, declared.Name)))
			continue
		}
		contractByRoute[key] = declared.Name
		result.Endpoints = append(result.Endpoints, EndpointContract{
			Name: declared.Name, ModulePath: declared.ModulePath, Handler: declared.Handler,
			Method: route.Method, Path: route.Path, Input: declared.Input,
			Responses: append([]EndpointResponse(nil), declared.Responses...), Span: declared.Span,
		})
	}
	if len(issues) != 0 {
		return result, issues, nil
	}
	sort.Slice(result.Endpoints, func(i, j int) bool {
		if result.Endpoints[i].Path != result.Endpoints[j].Path {
			return result.Endpoints[i].Path < result.Endpoints[j].Path
		}
		if result.Endpoints[i].Method != result.Endpoints[j].Method {
			return result.Endpoints[i].Method < result.Endpoints[j].Method
		}
		return result.Endpoints[i].Name < result.Endpoints[j].Name
	})
	if err := ValidateEndpointCatalog(result); err != nil {
		return EndpointCatalog{}, nil, err
	}
	return result, nil, nil
}

func ValidateEndpointCatalog(catalog EndpointCatalog) error {
	if catalog.ProtocolVersion != EndpointCatalogProtocolVersion {
		return fmt.Errorf("unsupported trb/web endpoint catalog protocol version %d", catalog.ProtocolVersion)
	}
	if catalog.Package != PackageName {
		return fmt.Errorf("trb/web endpoint catalog belongs to package %q", catalog.Package)
	}
	seenNames := map[string]bool{}
	seenRoutes := map[string]bool{}
	for _, endpoint := range catalog.Endpoints {
		identity := endpoint.ModulePath + "\x00" + endpoint.Name
		if strings.TrimSpace(endpoint.ModulePath) == "" || strings.TrimSpace(endpoint.Name) == "" || seenNames[identity] {
			return fmt.Errorf("trb/web endpoint catalog contains an empty or duplicate endpoint %s.%s", endpoint.ModulePath, endpoint.Name)
		}
		seenNames[identity] = true
		if strings.TrimSpace(endpoint.Handler) == "" || strings.TrimSpace(endpoint.Method) == "" || !strings.HasPrefix(endpoint.Path, "/") {
			return fmt.Errorf("trb/web endpoint catalog endpoint %s.%s has an invalid route binding", endpoint.ModulePath, endpoint.Name)
		}
		routeIdentity := endpoint.ModulePath + "\x00" + endpoint.Handler
		if seenRoutes[routeIdentity] {
			return fmt.Errorf("trb/web endpoint catalog contains duplicate route binding %s.%s", endpoint.ModulePath, endpoint.Handler)
		}
		seenRoutes[routeIdentity] = true
		if len(endpoint.Responses) == 0 {
			return fmt.Errorf("trb/web endpoint catalog endpoint %s.%s has no responses", endpoint.ModulePath, endpoint.Name)
		}
		if endpoint.Input != nil && endpoint.Input.Authored.Kind == "" {
			return fmt.Errorf("trb/web endpoint catalog endpoint %s.%s has an invalid input type", endpoint.ModulePath, endpoint.Name)
		}
		statuses := map[int]bool{}
		for _, response := range endpoint.Responses {
			if response.Status < 100 || response.Status > 599 || statuses[response.Status] {
				return fmt.Errorf("trb/web endpoint catalog endpoint %s.%s has invalid or duplicate response status %d", endpoint.ModulePath, endpoint.Name, response.Status)
			}
			if response.Type.Authored.Kind == "" {
				return fmt.Errorf("trb/web endpoint catalog endpoint %s.%s response %d has an invalid type", endpoint.ModulePath, endpoint.Name, response.Status)
			}
			statuses[response.Status] = true
		}
	}
	return nil
}

func discoverEndpointDeclarations(input packageextension.ProjectDeclarationInput) ([]endpointDeclaration, []EndpointContractIssue) {
	var result []endpointDeclaration
	var issues []EndpointContractIssue
	for _, module := range input.Modules {
		imports := webDirectiveImports(module)
		for _, class := range module.Classes {
			if !isEndpointClass(class) {
				continue
			}
			declared, classIssues := discoverEndpointDeclaration(module, class, imports)
			issues = append(issues, classIssues...)
			if len(classIssues) == 0 {
				result = append(result, declared)
			}
		}
	}
	return result, issues
}

func discoverEndpointDeclaration(module packageextension.ProjectModule, class packageextension.ProjectClass, imports map[string]bool) (endpointDeclaration, []EndpointContractIssue) {
	result := endpointDeclaration{Name: class.Name, ModulePath: module.ModulePath, Span: class.Span}
	var issues []EndpointContractIssue
	if len(class.TypeParameters) != 0 {
		issues = append(issues, endpointIssue(module.ModulePath, class.Span,
			fmt.Sprintf("trb/web endpoint contract %s cannot declare type parameters", class.Name)))
	}
	handlesCount := 0
	inputCount := 0
	statuses := map[int]bool{}
	for _, directive := range class.Directives {
		if !imports[directive.Name] {
			continue
		}
		switch directive.Name {
		case "handles":
			handlesCount++
			handler, issue := endpointHandler(module, class, directive)
			if issue != "" {
				issues = append(issues, endpointIssue(module.ModulePath, directive.Span, issue))
			} else {
				result.Handler = handler
			}
		case "input":
			inputCount++
			if len(directive.TypeArguments) != 1 || len(directive.Arguments) != 0 || directive.Block != nil {
				issues = append(issues, endpointIssue(module.ModulePath, directive.Span,
					fmt.Sprintf("trb/web endpoint contract %s input declaration must be input<T>()", class.Name)))
				continue
			}
			input := directive.TypeArguments[0]
			result.Input = &input
		case "response":
			response, issue := endpointResponse(class, directive)
			if issue != "" {
				issues = append(issues, endpointIssue(module.ModulePath, directive.Span, issue))
				continue
			}
			if statuses[response.Status] {
				issues = append(issues, endpointIssue(module.ModulePath, directive.Span,
					fmt.Sprintf("trb/web endpoint contract %s declares response status %d more than once", class.Name, response.Status)))
				continue
			}
			statuses[response.Status] = true
			result.Responses = append(result.Responses, response)
		}
	}
	if handlesCount == 0 {
		issues = append(issues, endpointIssue(module.ModulePath, class.Span,
			fmt.Sprintf("trb/web endpoint contract %s must declare handles(handler)", class.Name)))
	} else if handlesCount > 1 {
		issues = append(issues, endpointIssue(module.ModulePath, class.Span,
			fmt.Sprintf("trb/web endpoint contract %s declares handles more than once", class.Name)))
	}
	if inputCount > 1 {
		issues = append(issues, endpointIssue(module.ModulePath, class.Span,
			fmt.Sprintf("trb/web endpoint contract %s declares input more than once", class.Name)))
	}
	if len(result.Responses) == 0 {
		issues = append(issues, endpointIssue(module.ModulePath, class.Span,
			fmt.Sprintf("trb/web endpoint contract %s must declare at least one response<T>(status: code)", class.Name)))
	}
	return result, issues
}

func endpointHandler(module packageextension.ProjectModule, class packageextension.ProjectClass, directive packageextension.ProjectDirective) (string, string) {
	if len(directive.TypeArguments) != 0 || len(directive.Arguments) != 1 || directive.Arguments[0].Name != "" || directive.Arguments[0].Splat != "" || directive.Block != nil {
		return "", fmt.Sprintf("trb/web endpoint contract %s handles declaration must be handles(handler)", class.Name)
	}
	value := directive.Arguments[0].Value
	if value.Kind != "reference" || value.Name == "" || value.Reference != nil && value.Reference.ModulePath != module.ModulePath {
		return "", fmt.Sprintf("trb/web endpoint contract %s handles must reference a top-level function in the same module", class.Name)
	}
	for _, function := range module.Functions {
		if function.Name == value.Name {
			return value.Name, ""
		}
	}
	return "", fmt.Sprintf("trb/web endpoint contract %s handles must reference a top-level function in the same module", class.Name)
}

func endpointResponse(class packageextension.ProjectClass, directive packageextension.ProjectDirective) (EndpointResponse, string) {
	if len(directive.TypeArguments) != 1 || len(directive.Arguments) != 1 || directive.Arguments[0].Name != "status" || directive.Arguments[0].Splat != "" || directive.Block != nil {
		return EndpointResponse{}, fmt.Sprintf("trb/web endpoint contract %s response declaration must be response<T>(status: code)", class.Name)
	}
	value := directive.Arguments[0].Value
	if value.Kind != "integer" {
		return EndpointResponse{}, fmt.Sprintf("trb/web endpoint contract %s response status must be an integer literal from 100 through 599", class.Name)
	}
	parsed, err := strconv.ParseInt(strings.ReplaceAll(value.Raw, "_", ""), 10, 16)
	status := int(parsed)
	if err != nil || status < 100 || status > 599 {
		return EndpointResponse{}, fmt.Sprintf("trb/web endpoint contract %s response status must be an integer literal from 100 through 599", class.Name)
	}
	return EndpointResponse{Status: status, Type: directive.TypeArguments[0], Span: directive.Span}, ""
}

func isEndpointClass(class packageextension.ProjectClass) bool {
	if class.Superclass == nil {
		return false
	}
	for _, reference := range class.Superclass.ResolutionPath {
		if reference.Name == "Endpoint" && reference.ImportPath == PackageName {
			return true
		}
	}
	return false
}

func webDirectiveImports(module packageextension.ProjectModule) map[string]bool {
	result := map[string]bool{}
	for _, imported := range module.Imports {
		if imported.Path != PackageName && imported.ModulePath != ModulePath {
			continue
		}
		for _, symbol := range imported.Symbols {
			switch symbol {
			case "handles", "input", "response":
				result[symbol] = true
			}
		}
	}
	return result
}

func endpointIssue(modulePath string, span packageextension.SourceSpan, message string) EndpointContractIssue {
	return EndpointContractIssue{ModulePath: modulePath, Message: message, Span: span}
}

func validateEndpointInput(input packageextension.ProjectDeclarationInput) error {
	if err := packageextension.ValidateProjectDeclarationInput(input); err != nil {
		return err
	}
	if input.Provider != PackageName {
		return fmt.Errorf("trb/web received project declaration input for provider %s", input.Provider)
	}
	return nil
}
