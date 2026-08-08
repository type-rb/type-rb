// Package web owns target-independent compilation support for trb/web.
package web

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/token"
)

const (
	ModulePath      = "trb/web/index"
	ProjectProvider = "trb.web.routes"
)

var handlerMethods = map[string]bool{
	"delete":  true,
	"get":     true,
	"head":    true,
	"options": true,
	"patch":   true,
	"post":    true,
	"put":     true,
}

type Source struct {
	Filename   string
	ModulePath string
	Program    *ast.Program
}

type Route struct {
	Filename       string
	Directory      string
	Method         string
	Path           string
	ModulePath     string
	Handler        string
	TargetHandler  string
	PathParameters []string
	Middlewares    []Middleware
	Span           token.Span
}

type Middleware struct {
	Filename      string
	Directory     string
	ModulePath    string
	Handler       string
	TargetHandler string
	Span          token.Span
}

type Issue struct {
	Filename string
	Message  string
	Span     token.Span
}

type Manifest struct {
	Routes      []Route
	Middlewares []Middleware
}

func (*Manifest) ExtensionName() string { return ProjectProvider }

// UniquePathRoutes returns the first route for each path in manifest order.
// Handlers in one route file share the same path parameters and middleware.
func UniquePathRoutes(routes []Route) []Route {
	seen := map[string]bool{}
	result := make([]Route, 0, len(routes))
	for _, route := range routes {
		if seen[route.Path] {
			continue
		}
		seen[route.Path] = true
		result = append(result, route)
	}
	return result
}

func ManifestFrom(extensions []ir.Extension) *Manifest {
	for _, extension := range extensions {
		if manifest, ok := extension.(*Manifest); ok {
			return manifest
		}
	}
	return nil
}

func Analyze(sources []Source, resolutions map[string]resolver.Result, sourceRoot string) (*Manifest, []Issue) {
	routes, issues := Discover(sources, sourceRoot)
	middlewares := discoverMiddlewares(sources, sourceRoot)
	if len(issues) > 0 {
		return &Manifest{Routes: routes, Middlewares: middlewares}, issues
	}
	for _, route := range routes {
		program := sourceProgram(sources, route.ModulePath)
		method := topLevelMethod(program, route.Handler)
		if method == nil || !validHandler(method, resolutions[route.ModulePath]) {
			span := program.Span()
			if method != nil {
				span = method.Span()
			}
			return &Manifest{Routes: routes, Middlewares: middlewares}, []Issue{{
				Filename: route.Filename,
				Message:  fmt.Sprintf("%s %s handler must have signature def %s(context: Context): Response", route.Method, route.Path, route.Handler),
				Span:     span,
			}}
		}
	}
	for _, middleware := range middlewares {
		program := sourceProgram(sources, middleware.ModulePath)
		method := topLevelMethod(program, middleware.Handler)
		if method == nil || !validMiddleware(method, resolutions[middleware.ModulePath]) {
			span := program.Span()
			if method != nil {
				span = method.Span()
			}
			return &Manifest{Routes: routes, Middlewares: middlewares}, []Issue{{
				Filename: middleware.Filename,
				Message:  "middleware must have signature def call(context: Context, next_handler: Next): Response",
				Span:     span,
			}}
		}
	}
	for routeIndex := range routes {
		for _, middleware := range middlewares {
			if appliesToRoute(middleware.Directory, routes[routeIndex].Directory) {
				routes[routeIndex].Middlewares = append(routes[routeIndex].Middlewares, middleware)
			}
		}
	}
	return &Manifest{Routes: routes, Middlewares: middlewares}, nil
}

func (m *Manifest) MethodTargets() map[string]map[string]string {
	result := map[string]map[string]string{}
	if m == nil {
		return result
	}
	for _, route := range m.Routes {
		if result[route.ModulePath] == nil {
			result[route.ModulePath] = map[string]string{}
		}
		result[route.ModulePath][route.Handler] = route.TargetHandler
	}
	for _, middleware := range m.Middlewares {
		if result[middleware.ModulePath] == nil {
			result[middleware.ModulePath] = map[string]string{}
		}
		result[middleware.ModulePath][middleware.Handler] = middleware.TargetHandler
	}
	return result
}

func sourceProgram(sources []Source, modulePath string) *ast.Program {
	for _, source := range sources {
		if source.ModulePath == modulePath {
			return source.Program
		}
	}
	return nil
}

func validHandler(method *ast.MethodStatement, resolved resolver.Result) bool {
	if method.Class || len(method.TypeParameters) != 0 || len(method.Parameters) != 1 {
		return false
	}
	parameter := method.Parameters[0]
	if parameter.Default != nil || parameter.Keyword || parameter.Rest || parameter.KeywordRest || !officialType(parameter.Type, "Context", resolved) {
		return false
	}
	return officialType(method.ReturnType, "Response", resolved)
}

func validMiddleware(method *ast.MethodStatement, resolved resolver.Result) bool {
	if method.Class || len(method.TypeParameters) != 0 || len(method.Parameters) != 2 {
		return false
	}
	for index, expected := range []string{"Context", "Next"} {
		parameter := method.Parameters[index]
		if parameter.Default != nil || parameter.Keyword || parameter.Rest || parameter.KeywordRest || !officialType(parameter.Type, expected, resolved) {
			return false
		}
	}
	return officialType(method.ReturnType, "Response", resolved)
}

func officialType(ref ast.TypeRef, name string, resolved resolver.Result) bool {
	if ref.Name != name || ref.Nullable || ref.Array || len(ref.Arguments) != 0 || len(ref.Union) != 0 {
		return false
	}
	binding, imported := resolved.ImportedType(name)
	return imported && binding.Import != nil && binding.Import.RuntimePath() == ModulePath
}

func topLevelMethod(program *ast.Program, name string) *ast.MethodStatement {
	if program == nil {
		return nil
	}
	for _, statement := range program.Statements {
		if method, ok := statement.(*ast.MethodStatement); ok && method.Name == name {
			return method
		}
	}
	return nil
}

func Discover(sources []Source, sourceRoot string) ([]Route, []Issue) {
	if sourceRoot == "" {
		return nil, nil
	}
	routeRoot := filepath.Join(sourceRoot, "routes")
	var routes []Route
	var issues []Issue
	for _, source := range sources {
		relative, err := filepath.Rel(routeRoot, source.Filename)
		if err != nil || escapesRoot(relative) || filepath.Ext(relative) != ".trb" {
			continue
		}
		if filepath.Base(relative) == "_middleware.trb" {
			continue
		}
		path, parameters, pathIssue := routePath(relative)
		if pathIssue != "" {
			issues = append(issues, Issue{Filename: source.Filename, Message: pathIssue, Span: source.Program.Span()})
			continue
		}
		found := false
		for _, statement := range source.Program.Statements {
			method, ok := statement.(*ast.MethodStatement)
			if !ok || !handlerMethods[method.Name] {
				continue
			}
			found = true
			routes = append(routes, Route{
				Filename:       source.Filename,
				Directory:      relativeDirectory(relative),
				Method:         strings.ToUpper(method.Name),
				Path:           path,
				ModulePath:     source.ModulePath,
				Handler:        method.Name,
				PathParameters: append([]string(nil), parameters...),
				Span:           method.Span(),
			})
		}
		if !found {
			issues = append(issues, Issue{Filename: source.Filename, Message: "route file must declare at least one HTTP handler", Span: source.Program.Span()})
		}
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path != routes[j].Path {
			return routes[i].Path < routes[j].Path
		}
		if routes[i].Method != routes[j].Method {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].ModulePath < routes[j].ModulePath
	})
	for index := range routes {
		routes[index].TargetHandler = fmt.Sprintf("trb_web_route_%d", index)
	}
	issues = append(issues, conflictIssues(routes)...)
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Filename != issues[j].Filename {
			return issues[i].Filename < issues[j].Filename
		}
		if issues[i].Span.Start.Offset != issues[j].Span.Start.Offset {
			return issues[i].Span.Start.Offset < issues[j].Span.Start.Offset
		}
		return issues[i].Message < issues[j].Message
	})
	return routes, issues
}

func discoverMiddlewares(sources []Source, sourceRoot string) []Middleware {
	if sourceRoot == "" {
		return nil
	}
	routeRoot := filepath.Join(sourceRoot, "routes")
	var middlewares []Middleware
	for _, source := range sources {
		relative, err := filepath.Rel(routeRoot, source.Filename)
		if err != nil || escapesRoot(relative) || filepath.Base(relative) != "_middleware.trb" {
			continue
		}
		middlewares = append(middlewares, Middleware{
			Filename:   source.Filename,
			Directory:  relativeDirectory(relative),
			ModulePath: source.ModulePath,
			Handler:    "call",
			Span:       source.Program.Span(),
		})
	}
	sort.Slice(middlewares, func(i, j int) bool {
		leftDepth := directoryDepth(middlewares[i].Directory)
		rightDepth := directoryDepth(middlewares[j].Directory)
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		if middlewares[i].Directory != middlewares[j].Directory {
			return middlewares[i].Directory < middlewares[j].Directory
		}
		return middlewares[i].ModulePath < middlewares[j].ModulePath
	})
	for index := range middlewares {
		middlewares[index].TargetHandler = fmt.Sprintf("trb_web_middleware_%d", index)
	}
	return middlewares
}

func relativeDirectory(relative string) string {
	directory := filepath.ToSlash(filepath.Dir(relative))
	if directory == "." {
		return ""
	}
	return directory
}

func directoryDepth(directory string) int {
	if directory == "" {
		return 0
	}
	return strings.Count(directory, "/") + 1
}

func appliesToRoute(middlewareDirectory, routeDirectory string) bool {
	return middlewareDirectory == "" || routeDirectory == middlewareDirectory || strings.HasPrefix(routeDirectory, middlewareDirectory+"/")
}

func routePath(relative string) (string, []string, string) {
	withoutExtension := strings.TrimSuffix(filepath.ToSlash(relative), ".trb")
	segments := strings.Split(withoutExtension, "/")
	if len(segments) > 0 && segments[len(segments)-1] == "index" {
		segments = segments[:len(segments)-1]
	}
	parameters := []string{}
	seen := map[string]bool{}
	for index, segment := range segments {
		if strings.HasPrefix(segment, "[...") && strings.HasSuffix(segment, "]") {
			name := segment[4 : len(segment)-1]
			if !validParameterName(name) {
				return "", nil, fmt.Sprintf("invalid catch-all route parameter %q", name)
			}
			if index != len(segments)-1 {
				return "", nil, fmt.Sprintf("catch-all route parameter %q must be the final segment", name)
			}
			if seen[name] {
				return "", nil, fmt.Sprintf("route parameter %q is duplicated", name)
			}
			seen[name] = true
			parameters = append(parameters, name)
			segments[index] = "*" + name
			continue
		}
		if strings.HasPrefix(segment, "[") || strings.HasSuffix(segment, "]") {
			if len(segment) < 3 || segment[0] != '[' || segment[len(segment)-1] != ']' {
				return "", nil, fmt.Sprintf("invalid route segment %q", segment)
			}
			name := segment[1 : len(segment)-1]
			if !validParameterName(name) {
				return "", nil, fmt.Sprintf("invalid route parameter %q", name)
			}
			if seen[name] {
				return "", nil, fmt.Sprintf("route parameter %q is duplicated", name)
			}
			seen[name] = true
			parameters = append(parameters, name)
			segments[index] = ":" + name
		}
	}
	if len(segments) == 0 {
		return "/", parameters, ""
	}
	return "/" + strings.Join(segments, "/"), parameters, ""
}

func conflictIssues(routes []Route) []Issue {
	var issues []Issue
	for current := 0; current < len(routes); current++ {
		for previous := 0; previous < current; previous++ {
			if routes[current].Method != routes[previous].Method || !pathsOverlap(routes[current].Path, routes[previous].Path) {
				continue
			}
			issues = append(issues, Issue{
				Filename: routes[current].Filename,
				Message:  fmt.Sprintf("%s %s conflicts with route %s", routes[current].Method, routes[current].Path, routes[previous].Path),
				Span:     routes[current].Span,
			})
		}
	}
	return issues
}

func pathsOverlap(left, right string) bool {
	leftSegments := routePatternSegments(left)
	rightSegments := routePatternSegments(right)
	leftCatchAll := catchAllSegmentIndex(leftSegments)
	rightCatchAll := catchAllSegmentIndex(rightSegments)
	if leftCatchAll < 0 && rightCatchAll < 0 && len(leftSegments) != len(rightSegments) {
		return false
	}
	sharedPrefix := min(len(leftSegments), len(rightSegments))
	if leftCatchAll >= 0 {
		sharedPrefix = min(sharedPrefix, leftCatchAll)
	}
	if rightCatchAll >= 0 {
		sharedPrefix = min(sharedPrefix, rightCatchAll)
	}
	for index := 0; index < sharedPrefix; index++ {
		if leftSegments[index] == rightSegments[index] || strings.HasPrefix(leftSegments[index], ":") || strings.HasPrefix(rightSegments[index], ":") {
			continue
		}
		return false
	}
	if leftCatchAll >= 0 && len(rightSegments) < leftCatchAll+1 {
		return false
	}
	if rightCatchAll >= 0 && len(leftSegments) < rightCatchAll+1 {
		return false
	}
	return leftCatchAll >= 0 || rightCatchAll >= 0 || len(leftSegments) == len(rightSegments)
}

func routePatternSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func catchAllSegmentIndex(segments []string) int {
	for index, segment := range segments {
		if strings.HasPrefix(segment, "*") {
			return index
		}
	}
	return -1
}

func validParameterName(name string) bool {
	for index, character := range name {
		if character == '_' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || index > 0 && character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return name != ""
}

func escapesRoot(path string) bool {
	return path == ".." || filepath.IsAbs(path) || strings.HasPrefix(path, ".."+string(filepath.Separator))
}
