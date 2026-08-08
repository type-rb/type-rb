package typescript

import (
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	webintegration "github.com/type-rb/type-rb/internal/web"
)

func (g *generator) integrationImports(extensions []ir.Extension) {
	if manifest := webintegration.ManifestFrom(extensions); manifest != nil {
		g.webRouteImports(manifest.Routes)
	}
}

func (g *generator) integrations(extensions []ir.Extension) {
	if manifest := webintegration.ManifestFrom(extensions); manifest != nil {
		g.webDispatcher(manifest.Routes)
	}
}

func (g *generator) webRouteImports(routes []webintegration.Route) {
	symbols := map[string][]string{}
	for _, route := range routes {
		symbols[route.ModulePath] = append(symbols[route.ModulePath], route.TargetHandler)
	}
	modulePaths := make([]string, 0, len(symbols))
	for modulePath := range symbols {
		modulePaths = append(modulePaths, modulePath)
	}
	sort.Strings(modulePaths)
	for _, modulePath := range modulePaths {
		names := symbols[modulePath]
		sort.Strings(names)
		g.line("import { " + strings.Join(names, ", ") + " } from " + strconv.Quote(tsImportPath(g.modulePath, modulePath)) + ";")
	}
}

func (g *generator) webDispatcher(routes []webintegration.Route) {
	g.line("function trb_web_dispatch(request: { method: string; path: string; headers: Record<string, string[]>; body: Uint8Array }) {")
	g.indent++
	g.line("const method = request.method.toUpperCase();")
	g.line(`const segments = request.path.split("/").filter((segment) => segment.length > 0);`)
	for _, route := range routes {
		segments := typescriptWebRouteSegments(route.Path)
		condition := []string{"method === " + strconv.Quote(route.Method), "segments.length === " + strconv.Itoa(len(segments))}
		for index, segment := range segments {
			if !strings.HasPrefix(segment, ":") {
				condition = append(condition, "segments["+strconv.Itoa(index)+"] === "+strconv.Quote(segment))
			}
		}
		g.line("if (" + strings.Join(condition, " && ") + ") {")
		g.indent++
		g.line("const path_parameters: Record<string, string> = {};")
		for index, segment := range segments {
			if strings.HasPrefix(segment, ":") {
				g.line("path_parameters[" + strconv.Quote(strings.TrimPrefix(segment, ":")) + "] = segments[" + strconv.Itoa(index) + "]!;")
			}
		}
		g.line("return " + route.TargetHandler + "({ request, path_parameters });")
		g.indent--
		g.line("}")
	}
	g.line(`return { status: 404, headers: { "content-type": ["application/json; charset=utf-8"] }, body: new TextEncoder().encode("{\"error\":\"not_found\"}") };`)
	g.indent--
	g.line("}")
}

func typescriptWebRouteSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}
