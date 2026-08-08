package golang

import (
	pathpkg "path"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

func (g *generator) webDispatcher(routes []ir.WebRoute) {
	webPath := pathpkg.Join(g.goModule, "trb/web")
	g.requireImport(webPath, "web")
	g.requireImport("strings", "")

	directories := map[string]string{}
	for _, route := range routes {
		directory := pathpkg.Dir(route.ModulePath)
		if directory == "." || directory == g.currentDirectory() {
			continue
		}
		if _, exists := directories[directory]; !exists {
			directories[directory] = "trb_route_" + strconv.Itoa(len(directories))
		}
	}
	orderedDirectories := make([]string, 0, len(directories))
	for directory := range directories {
		orderedDirectories = append(orderedDirectories, directory)
	}
	sort.Strings(orderedDirectories)
	for index, directory := range orderedDirectories {
		directories[directory] = "trb_route_" + strconv.Itoa(index)
		g.requireImport(pathpkg.Join(g.goModule, directory), directories[directory])
	}

	g.line("func trbWebDispatch(request web.Request) web.Response {")
	g.indent++
	g.line("method := strings.ToUpper(request.Method)")
	g.line("cleanPath := strings.Trim(request.Path, \"/\")")
	g.line("segments := []string{}")
	g.line("if cleanPath != \"\" {")
	g.indent++
	g.line("segments = strings.Split(cleanPath, \"/\")")
	g.indent--
	g.line("}")
	for _, route := range routes {
		segments := webRouteSegments(route.Path)
		condition := []string{"method == " + strconv.Quote(route.Method), "len(segments) == " + strconv.Itoa(len(segments))}
		for index, segment := range segments {
			if !strings.HasPrefix(segment, ":") {
				condition = append(condition, "segments["+strconv.Itoa(index)+"] == "+strconv.Quote(segment))
			}
		}
		g.line("if " + strings.Join(condition, " && ") + " {")
		g.indent++
		g.line("pathParameters := map[string]string{}")
		for index, segment := range segments {
			if strings.HasPrefix(segment, ":") {
				g.line("pathParameters[" + strconv.Quote(strings.TrimPrefix(segment, ":")) + "] = segments[" + strconv.Itoa(index) + "]")
			}
		}
		callee := goMethodName(route.TargetHandler)
		if alias := directories[pathpkg.Dir(route.ModulePath)]; alias != "" {
			callee = goImportAlias(alias) + "." + callee
		}
		g.line("return " + callee + "(web.Context{Request: request, PathParameters: pathParameters})")
		g.indent--
		g.line("}")
	}
	g.line("return web.Response{Status: 404, Headers: map[string][]string{\"content-type\": []string{\"application/json; charset=utf-8\"}}, Body: []byte(\"{\\\"error\\\":\\\"not_found\\\"}\")}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func webRouteSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}
