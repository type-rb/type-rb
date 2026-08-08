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
		g.webDispatcher(manifest)
		g.webServer()
	}
}

func (g *generator) webDispatcher(manifest *webintegration.Manifest) {
	routes := manifest.Routes
	webPath := pathpkg.Join(g.goModule, "trb/web")
	g.requireImport(webPath, "web")
	g.requireImport("slices", "")
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
	for _, middleware := range manifest.Middlewares {
		directory := pathpkg.Dir(middleware.ModulePath)
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
	if len(manifest.Middlewares) > 0 {
		g.webNext()
	}
	g.webProtocolResponses()

	g.line("func trbWebDispatch(request web.Request) (response web.Response) {")
	g.indent++
	g.line("headRequest := strings.EqualFold(request.Method, \"HEAD\")")
	g.line("defer func() {")
	g.indent++
	g.line("if recover() != nil {")
	g.indent++
	g.line("response = web.Response{Status: 500, Headers: map[string][]string{\"content-type\": []string{\"application/json; charset=utf-8\"}}, Body: []byte(\"{\\\"error\\\":\\\"internal_server_error\\\"}\")}")
	g.indent--
	g.line("}")
	g.line("if headRequest {")
	g.indent++
	g.line("response.Body = []byte{}")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}()")
	g.line("if len(request.Body) > trbWebMaxBodyBytes {")
	g.indent++
	g.line("return trbWebPayloadTooLarge()")
	g.indent--
	g.line("}")
	g.line("method := strings.ToUpper(request.Method)")
	g.line("cleanPath := strings.Trim(request.Path, \"/\")")
	g.line("segments := []string{}")
	g.line("if cleanPath != \"\" {")
	g.indent++
	g.line("segments = strings.Split(cleanPath, \"/\")")
	g.indent--
	g.line("}")
	g.line("allowedMethods := []string{}")
	g.line("explicitHead := false")
	for _, route := range routes {
		routeSegments := webRouteSegments(route.Path)
		condition := []string{"len(segments) == " + strconv.Itoa(len(routeSegments))}
		for segmentIndex, segment := range routeSegments {
			if !strings.HasPrefix(segment, ":") {
				condition = append(condition, "segments["+strconv.Itoa(segmentIndex)+"] == "+strconv.Quote(segment))
			}
		}
		g.line("if " + strings.Join(condition, " && ") + " {")
		g.indent++
		g.line("allowedMethods = append(allowedMethods, " + strconv.Quote(route.Method) + ")")
		if route.Method == "GET" {
			g.line("allowedMethods = append(allowedMethods, \"HEAD\")")
		}
		if route.Method == "HEAD" {
			g.line("explicitHead = true")
		}
		g.indent--
		g.line("}")
	}
	g.line("slices.Sort(allowedMethods)")
	g.line("allowedMethods = slices.Compact(allowedMethods)")
	g.line("dispatchMethod := method")
	g.line("if method == \"HEAD\" && !explicitHead && slices.Contains(allowedMethods, \"GET\") {")
	g.indent++
	g.line("dispatchMethod = \"GET\"")
	g.indent--
	g.line("}")
	for routeIndex, route := range routes {
		segments := webRouteSegments(route.Path)
		condition := []string{"dispatchMethod == " + strconv.Quote(route.Method), "len(segments) == " + strconv.Itoa(len(segments))}
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
		contextName := "routeContext" + strconv.Itoa(routeIndex)
		handlerName := "routeHandler" + strconv.Itoa(routeIndex)
		g.line(contextName + " := web.Context{Request: request, PathParameters: pathParameters}")
		g.line(handlerName + " := func(context web.Context) web.Response {")
		g.indent++
		g.line("return " + g.webCallee(route.ModulePath, route.TargetHandler, directories) + "(context)")
		g.indent--
		g.line("}")
		for middlewareIndex := len(route.Middlewares) - 1; middlewareIndex >= 0; middlewareIndex-- {
			middleware := route.Middlewares[middlewareIndex]
			nextName := "nextHandler" + strconv.Itoa(routeIndex) + "_" + strconv.Itoa(middlewareIndex)
			g.line(nextName + " := " + handlerName)
			g.line(handlerName + " = func(context web.Context) web.Response {")
			g.indent++
			g.line("return " + g.webCallee(middleware.ModulePath, middleware.TargetHandler, directories) + "(context, &trbWebNext{handler: " + nextName + "})")
			g.indent--
			g.line("}")
		}
		g.line("return " + handlerName + "(" + contextName + ")")
		g.indent--
		g.line("}")
	}
	g.line("if len(allowedMethods) > 0 {")
	g.indent++
	g.line("return web.Response{Status: 405, Headers: map[string][]string{\"allow\": []string{strings.Join(allowedMethods, \", \")}, \"content-type\": []string{\"application/json; charset=utf-8\"}}, Body: []byte(\"{\\\"error\\\":\\\"method_not_allowed\\\"}\")}")
	g.indent--
	g.line("}")
	g.line("return web.Response{Status: 404, Headers: map[string][]string{\"content-type\": []string{\"application/json; charset=utf-8\"}}, Body: []byte(\"{\\\"error\\\":\\\"not_found\\\"}\")}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) webProtocolResponses() {
	g.line("const trbWebMaxBodyBytes = " + strconv.Itoa(webintegration.MaxBodyBytes))
	g.b.WriteByte('\n')
	g.line("func trbWebBadRequest() web.Response {")
	g.indent++
	g.line("return web.Response{Status: 400, Headers: map[string][]string{\"content-type\": []string{\"application/json; charset=utf-8\"}}, Body: []byte(\"{\\\"error\\\":\\\"bad_request\\\"}\")}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbWebPayloadTooLarge() web.Response {")
	g.indent++
	g.line("return web.Response{Status: 413, Headers: map[string][]string{\"content-type\": []string{\"application/json; charset=utf-8\"}}, Body: []byte(\"{\\\"error\\\":\\\"payload_too_large\\\"}\")}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) webCallee(modulePath, target string, directories map[string]string) string {
	callee := goMethodName(target)
	if alias := directories[pathpkg.Dir(modulePath)]; alias != "" {
		callee = goImportAlias(alias) + "." + callee
	}
	return callee
}

func (g *generator) webNext() {
	g.line("type trbWebNext struct {")
	g.indent++
	g.line("called bool")
	g.line("handler func(web.Context) web.Response")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func (next *trbWebNext) Call(context web.Context) web.Response {")
	g.indent++
	g.line("if next.called {")
	g.indent++
	g.line("panic(\"trb/web Next.call may only be called once\")")
	g.indent--
	g.line("}")
	g.line("next.called = true")
	g.line("return next.handler(context)")
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
	g.requireImport("errors", "")
	g.requireImport("fmt", "")
	g.requireImport("io", "")
	g.requireImport("net/http", "")

	g.line("func trbWebServe(port int64) {")
	g.indent++
	g.line("handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {")
	g.indent++
	g.line("body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, trbWebMaxBodyBytes))")
	g.line("var response web.Response")
	g.line("if err != nil {")
	g.indent++
	g.line("var maxBytesError *http.MaxBytesError")
	g.line("if errors.As(err, &maxBytesError) {")
	g.indent++
	g.line("response = trbWebPayloadTooLarge()")
	g.indent--
	g.line("} else {")
	g.indent++
	g.line("response = trbWebBadRequest()")
	g.indent--
	g.line("}")
	g.indent--
	g.line("} else {")
	g.indent++
	g.line("response = trbWebDispatch(web.Request{")
	g.indent++
	g.line("Method: request.Method,")
	g.line("Path: request.URL.Path,")
	g.line("QueryString: request.URL.RawQuery,")
	g.line("Headers: map[string][]string(request.Header.Clone()),")
	g.line("Body: body,")
	g.indent--
	g.line("})")
	g.indent--
	g.line("}")
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
