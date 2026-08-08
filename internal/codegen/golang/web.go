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
	g.requireImport("net/url", "neturl")
	g.requireImport("strings", "")
	g.requireImport("unicode/utf8", "")
	if len(routes) > 0 {
		g.requireImport("slices", "")
	}

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

	g.line("func trbWebDispatch(request web.Request) web.Response {")
	g.indent++
	g.line("return trbWebDispatchWithBodyLimit(request, trbWebDefaultMaxBodyBytes)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbWebDispatchWithBodyLimit(request web.Request, maxBodyBytes int) (response web.Response) {")
	g.indent++
	g.line("request = trbWebNormalizeRequest(request)")
	g.line("headRequest := request.Method == \"HEAD\"")
	g.line("defer func() {")
	g.indent++
	g.line("if recovered := recover(); recovered != nil {")
	g.indent++
	g.line("response = trbWebInternalServerError()")
	g.indent--
	g.line("}")
	g.line("if !trbWebValidResponse(response) {")
	g.indent++
	g.line("response = trbWebInternalServerError()")
	g.indent--
	g.line("}")
	g.line("if headRequest {")
	g.indent++
	g.line("response.Body = []byte{}")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}()")
	g.line("normalizedPath, validPath := trbWebNormalizePath(request.Path)")
	g.line("if !validPath {")
	g.indent++
	g.line("return trbWebBadRequest()")
	g.indent--
	g.line("}")
	g.line("request.Path = normalizedPath")
	g.line("if len(request.Body) > maxBodyBytes {")
	g.indent++
	g.line("return trbWebPayloadTooLarge()")
	g.indent--
	g.line("}")
	if len(routes) == 0 {
		g.line("return web.Response{Status: 404, Headers: map[string][]string{\"content-type\": []string{\"application/json; charset=utf-8\"}}, Body: []byte(\"{\\\"error\\\":\\\"not_found\\\"}\")}")
		g.indent--
		g.line("}")
		g.b.WriteByte('\n')
		return
	}
	g.line("method := strings.ToUpper(request.Method)")
	g.line("segments := []string{}")
	g.line("if request.Path != \"/\" {")
	g.indent++
	g.line("segments = strings.Split(strings.TrimPrefix(request.Path, \"/\"), \"/\")")
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
	g.line("if len(allowedMethods) > 0 {")
	g.indent++
	g.line("allowedMethods = append(allowedMethods, \"OPTIONS\")")
	g.indent--
	g.line("}")
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
	for routeIndex, route := range webintegration.UniquePathRoutes(routes) {
		segments := webRouteSegments(route.Path)
		condition := []string{"method == \"OPTIONS\"", "len(segments) == " + strconv.Itoa(len(segments))}
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
		contextName := "optionsContext" + strconv.Itoa(routeIndex)
		handlerName := "optionsHandler" + strconv.Itoa(routeIndex)
		g.line(contextName + " := web.Context{Request: request, PathParameters: pathParameters}")
		g.line(handlerName + " := func(context web.Context) web.Response {")
		g.indent++
		g.line("return web.Response{Status: 204, Headers: map[string][]string{\"allow\": []string{strings.Join(allowedMethods, \", \")}}, Body: []byte{}}")
		g.indent--
		g.line("}")
		for middlewareIndex := len(route.Middlewares) - 1; middlewareIndex >= 0; middlewareIndex-- {
			middleware := route.Middlewares[middlewareIndex]
			nextName := "optionsNextHandler" + strconv.Itoa(routeIndex) + "_" + strconv.Itoa(middlewareIndex)
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
	g.line("const trbWebDefaultMaxBodyBytes = " + strconv.Itoa(webintegration.MaxBodyBytes))
	g.b.WriteByte('\n')
	g.line("func trbWebNormalizeRequest(request web.Request) web.Request {")
	g.indent++
	g.line("headers := map[string][]string{}")
	g.line("for name, values := range request.Headers {")
	g.indent++
	g.line("normalizedName := strings.ToLower(name)")
	g.line("headers[normalizedName] = append(headers[normalizedName], values...)")
	g.indent--
	g.line("}")
	g.line("return web.Request{Method: strings.ToUpper(request.Method), Path: request.Path, QueryString: request.QueryString, Headers: headers, Body: request.Body}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbWebNormalizePath(path string) (string, bool) {")
	g.indent++
	g.line("if !strings.HasPrefix(path, \"/\") || strings.Contains(path, \"\\\\\") {")
	g.indent++
	g.line("return \"\", false")
	g.indent--
	g.line("}")
	g.line("segments := strings.Split(path, \"/\")")
	g.line("for index, segment := range segments {")
	g.indent++
	g.line("decoded, err := neturl.PathUnescape(segment)")
	g.line("if err != nil || !utf8.ValidString(decoded) || decoded == \".\" || decoded == \"..\" || strings.ContainsAny(decoded, \"/\\\\\") {")
	g.indent++
	g.line("return \"\", false")
	g.indent--
	g.line("}")
	g.line("segments[index] = decoded")
	g.indent--
	g.line("}")
	g.line("return strings.Join(segments, \"/\"), true")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbWebInternalServerError() web.Response {")
	g.indent++
	g.line("return web.Response{Status: 500, Headers: map[string][]string{\"content-type\": []string{\"application/json; charset=utf-8\"}}, Body: []byte(\"{\\\"error\\\":\\\"internal_server_error\\\"}\")}")
	g.indent--
	g.line("}")
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
	g.line("func trbWebValidResponse(response web.Response) bool {")
	g.indent++
	g.line("if response.Status < 100 || response.Status > 999 {")
	g.indent++
	g.line("return false")
	g.indent--
	g.line("}")
	g.line("for name, values := range response.Headers {")
	g.indent++
	g.line("if name == \"\" {")
	g.indent++
	g.line("return false")
	g.indent--
	g.line("}")
	g.line("for index := 0; index < len(name); index++ {")
	g.indent++
	g.line("character := name[index]")
	g.line("letterOrDigit := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9'")
	g.line("symbol := strings.ContainsRune(\"!#$%&'*+-.^_`|~\", rune(character))")
	g.line("if !letterOrDigit && !symbol {")
	g.indent++
	g.line("return false")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.line("for _, value := range values {")
	g.indent++
	g.line("if strings.ContainsAny(value, \"\\r\\n\") {")
	g.indent++
	g.line("return false")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.line("return true")
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
	g.requireImport("context", "")
	g.requireImport("errors", "")
	g.requireImport("io", "")
	g.requireImport("net", "")
	g.requireImport("net/http", "")
	g.requireImport("os", "")
	g.requireImport("os/signal", "signal")
	g.requireImport("strconv", "")
	g.requireImport("syscall", "")
	g.requireImport("time", "")

	g.line("func trbWebServe(config web.ServerConfig) {")
	g.indent++
	g.line("trbWebValidateServerConfig(config)")
	g.line("handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {")
	g.indent++
	g.line("body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, int64(config.BodyLimitBytes)))")
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
	g.line("response = trbWebDispatchWithBodyLimit(web.Request{")
	g.indent++
	g.line("Method: request.Method,")
	g.line("Path: request.URL.EscapedPath(),")
	g.line("QueryString: request.URL.RawQuery,")
	g.line("Headers: map[string][]string(request.Header.Clone()),")
	g.line("Body: body,")
	g.indent--
	g.line("}, config.BodyLimitBytes)")
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
	g.line("server := &http.Server{Addr: net.JoinHostPort(config.Host, strconv.Itoa(config.Port)), Handler: handler}")
	g.line("signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)")
	g.line("defer stopSignals()")
	g.line("serveErrors := make(chan error, 1)")
	g.line("go func() {")
	g.indent++
	g.line("serveErrors <- server.ListenAndServe()")
	g.indent--
	g.line("}()")
	g.line("select {")
	g.line("case err := <-serveErrors:")
	g.indent++
	g.line("if err != nil && !errors.Is(err, http.ErrServerClosed) {")
	g.indent++
	g.line("panic(err)")
	g.indent--
	g.line("}")
	g.indent--
	g.line("case <-signalContext.Done():")
	g.indent++
	g.line("shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Duration(config.ShutdownTimeoutMilliseconds)*time.Millisecond)")
	g.line("shutdownErr := server.Shutdown(shutdownContext)")
	g.line("cancelShutdown()")
	g.line("if shutdownErr != nil {")
	g.indent++
	g.line("_ = server.Close()")
	g.indent--
	g.line("}")
	g.line("if err := <-serveErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {")
	g.indent++
	g.line("panic(err)")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbWebValidateServerConfig(config web.ServerConfig) {")
	g.indent++
	g.line("if strings.TrimSpace(config.Host) == \"\" {")
	g.indent++
	g.line("panic(\"trb/web ServerConfig.host must not be empty\")")
	g.indent--
	g.line("}")
	g.line("if config.Port < 1 || config.Port > 65535 {")
	g.indent++
	g.line("panic(\"trb/web ServerConfig.port must be between 1 and 65535\")")
	g.indent--
	g.line("}")
	g.line("if config.BodyLimitBytes < 1 {")
	g.indent++
	g.line("panic(\"trb/web ServerConfig.body_limit_bytes must be greater than zero\")")
	g.indent--
	g.line("}")
	g.line("if config.ShutdownTimeoutMilliseconds < 0 {")
	g.indent++
	g.line("panic(\"trb/web ServerConfig.shutdown_timeout_milliseconds must not be negative\")")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}
