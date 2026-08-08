package ruby

import (
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

func (g *generator) webDispatcher(routes []ir.WebRoute) {
	modules := map[string]bool{}
	for _, route := range routes {
		modules[route.ModulePath] = true
	}
	modulePaths := make([]string, 0, len(modules))
	for modulePath := range modules {
		modulePaths = append(modulePaths, modulePath)
	}
	sort.Strings(modulePaths)
	for _, modulePath := range modulePaths {
		g.line("require_relative "+strconv.Quote(rubyImportPath(g.modulePath, modulePath)), "")
	}
	if len(modulePaths) > 0 {
		g.b.WriteByte('\n')
	}

	g.line("def trb_web_dispatch(request)", "")
	g.indent++
	g.line("method = request.method.upcase", "")
	g.line(`segments = request.path.split("/").reject(&:empty?)`, "")
	for _, route := range routes {
		segments := rubyWebRouteSegments(route.Path)
		condition := []string{"method == " + strconv.Quote(route.Method), "segments.length == " + strconv.Itoa(len(segments))}
		for index, segment := range segments {
			if !strings.HasPrefix(segment, ":") {
				condition = append(condition, "segments["+strconv.Itoa(index)+"] == "+strconv.Quote(segment))
			}
		}
		g.line("if "+strings.Join(condition, " && "), "")
		g.indent++
		g.line("path_parameters = {}", "")
		for index, segment := range segments {
			if strings.HasPrefix(segment, ":") {
				g.line("path_parameters["+strconv.Quote(strings.TrimPrefix(segment, ":"))+"] = segments["+strconv.Itoa(index)+"]", "")
			}
		}
		g.line("return "+route.TargetHandler+"(Context.new(request: request, path_parameters: path_parameters))", "")
		g.indent--
		g.line("end", "")
	}
	g.line(`Response.new(status: 404, headers: { "content-type" => ["application/json; charset=utf-8"] }, body: "{\"error\":\"not_found\"}".b)`, "")
	g.indent--
	g.line("end", "")
}

func rubyWebRouteSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}
