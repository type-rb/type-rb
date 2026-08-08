// Package web owns target-independent compilation support for trb/web.
package web

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/token"
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
	Method         string
	Path           string
	ModulePath     string
	Handler        string
	PathParameters []string
	Span           token.Span
}

type Issue struct {
	Filename string
	Message  string
	Span     token.Span
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
			return "", nil, "catch-all route segments are not supported yet"
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
	leftSegments := strings.Split(strings.TrimPrefix(left, "/"), "/")
	rightSegments := strings.Split(strings.TrimPrefix(right, "/"), "/")
	if len(leftSegments) != len(rightSegments) {
		return false
	}
	for index := range leftSegments {
		if leftSegments[index] == rightSegments[index] || strings.HasPrefix(leftSegments[index], ":") || strings.HasPrefix(rightSegments[index], ":") {
			continue
		}
		return false
	}
	return true
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
