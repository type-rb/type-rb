package golang

import (
	pathpkg "path"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	webintegration "github.com/type-rb/type-rb/internal/web"
)

func (g *generator) integrations(extensions []ir.Extension) {
	if manifest := webintegration.ManifestFrom(extensions); manifest != nil {
		g.webDispatcher(manifest.Routes)
		g.webServer()
	}
}

func (g *generator) webDispatcher(routes []webintegration.Route) {
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

	g.line("func trbWebDispatch(request web.Request) (response web.Response) {")
	g.indent++
	g.line("defer func() {")
	g.indent++
	g.line("if recover() != nil {")
	g.indent++
	g.line("response = web.Response{Status: 500, Headers: map[string][]string{\"content-type\": []string{\"application/json; charset=utf-8\"}}, Body: []byte(\"{\\\"error\\\":\\\"internal_server_error\\\"}\")}")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}()")
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

func (g *generator) webServer() {
	g.requireImport("fmt", "")
	g.requireImport("io", "")
	g.requireImport("net/http", "")

	g.line("func trbWebServe(port int64) {")
	g.indent++
	g.line("handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {")
	g.indent++
	g.line("body, err := io.ReadAll(request.Body)")
	g.line("if err != nil {")
	g.indent++
	g.line("http.Error(writer, \"invalid request body\", http.StatusBadRequest)")
	g.line("return")
	g.indent--
	g.line("}")
	g.line("response := trbWebDispatch(web.Request{")
	g.indent++
	g.line("Method: request.Method,")
	g.line("Path: request.URL.Path,")
	g.line("Headers: map[string][]string(request.Header.Clone()),")
	g.line("Body: body,")
	g.indent--
	g.line("})")
	g.line("for name, values := range response.Headers {")
	g.indent++
	g.line("for _, value := range values {")
	g.indent++
	g.line("writer.Header().Add(name, value)")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.line("writer.WriteHeader(int(response.Status))")
	g.line("_, _ = writer.Write(response.Body)")
	g.indent--
	g.line("})")
	g.line("if err := http.ListenAndServe(fmt.Sprintf(\":%d\", port), handler); err != nil {")
	g.indent++
	g.line("panic(err)")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}
