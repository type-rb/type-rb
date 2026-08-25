package golang

import (
	pathpkg "path"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	webintegration "github.com/type-rb/type-rb/internal/web"
)

func (g *generator) integrations(extensions []ir.Extension) {
	if manifest := jobsintegration.ManifestFrom(extensions); manifest != nil {
		g.jobsRuntime(manifest)
	}
	if manifest := ormintegration.ManifestFrom(extensions); manifest != nil {
		g.ormRuntime(manifest)
	}
	if manifest := webintegration.ManifestFrom(extensions); manifest != nil {
		g.webDispatcher(manifest)
		g.webServer()
	}
}

func (g *generator) webDispatcher(manifest *webintegration.Manifest) {
	routes := manifest.Routes
	pathRoutes := webintegration.UniquePathRoutes(routes)
	pathIndexes := map[string]int{}
	for index, route := range pathRoutes {
		pathIndexes[route.Path] = index
	}
	rootMiddlewares := webintegration.RootMiddlewares(manifest.Middlewares)
	webPath := pathpkg.Join(g.goModule, "trb/web")
	g.requireImport(webPath, "web")
	g.requireImport(pathpkg.Join(g.goModule, "trb/http"), "__trb_http")
	// Headers owns an Array<Header>. The element type can therefore occur in
	// generated code even when callers only import Headers explicitly.
	if g.typeAliases["Headers"] != "" && g.typeAliases["Header"] == "" {
		g.typeAliases["Header"] = "__trb_http"
		g.typeKinds["Header"] = "record"
	}
	g.requireImport("net/url", "neturl")
	g.requireImport("context", "trbcontext")
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

	g.line("func trbWebDispatch(scope trbcontext.Context, request *web.Request) *web.Response {")
	g.indent++
	g.line("return trbWebDispatchWithBodyLimit(scope, request, trbWebDefaultMaxBodyBytes)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbWebDispatchWithBodyLimit(scope trbcontext.Context, request *web.Request, maxBodyBytes int) (response *web.Response) {")
	g.indent++
	g.line("request = trbWebNormalizeRequest(request)")
	g.line("headRequest := request.TrbFieldMethod.ToS() == \"HEAD\"")
	g.line("defer func() {")
	g.indent++
	g.line("if recovered := recover(); recovered != nil {")
	g.indent++
	g.line("if err, ok := recovered.(error); ok && (errors.Is(err, trbcontext.Canceled) || errors.Is(err, trbcontext.DeadlineExceeded)) { panic(recovered) }")
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
	g.line("response = web.NewResponse(map[string]any{\"status\": response.TrbFieldStatus, \"headers\": response.TrbFieldHeaders, \"body\": __trb_http.BodyEmpty()})")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}()")
	g.line("normalizedPath, validPath := trbWebNormalizePath(request.TrbFieldPath)")
	g.line("if !validPath {")
	g.indent++
	g.line("return trbWebDispatchProtocolResponse(scope, request, trbWebBadRequest())")
	g.indent--
	g.line("}")
	g.line("request = web.NewRequest(map[string]any{\"method\": request.TrbFieldMethod, \"path\": normalizedPath, \"query_string\": request.TrbFieldQueryString, \"headers\": request.TrbFieldHeaders, \"body\": request.TrbFieldBody})")
	g.line("context := web.NewContext(map[string]any{\"request\": request, \"path_parameters\": map[string]string{}})")
	g.line("handler := func(scope trbcontext.Context, context *web.Context) *web.Response {")
	g.indent++
	g.line("return trbWebDispatchCore(scope, context.TrbFieldRequest, maxBodyBytes)")
	g.indent--
	g.line("}")
	g.webMiddlewareChain(rootMiddlewares, "handler", "rootNextHandler", directories)
	g.line("return handler(scope, context)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbWebDispatchProtocolResponse(scope trbcontext.Context, request *web.Request, protocolResponse *web.Response) (response *web.Response) {")
	g.indent++
	g.line("request = trbWebNormalizeRequest(request)")
	g.line("if normalizedPath, validPath := trbWebNormalizePath(request.TrbFieldPath); validPath {")
	g.indent++
	g.line("request = web.NewRequest(map[string]any{\"method\": request.TrbFieldMethod, \"path\": normalizedPath, \"query_string\": request.TrbFieldQueryString, \"headers\": request.TrbFieldHeaders, \"body\": request.TrbFieldBody})")
	g.indent--
	g.line("}")
	g.line("headRequest := request.TrbFieldMethod.ToS() == \"HEAD\"")
	g.line("defer func() {")
	g.indent++
	g.line("if recovered := recover(); recovered != nil {")
	g.indent++
	g.line("if err, ok := recovered.(error); ok && (errors.Is(err, trbcontext.Canceled) || errors.Is(err, trbcontext.DeadlineExceeded)) { panic(recovered) }")
	g.line("response = trbWebInternalServerError()")
	g.indent--
	g.line("} else if !trbWebValidResponse(response) {")
	g.indent++
	g.line("response = trbWebInternalServerError()")
	g.indent--
	g.line("}")
	g.line("if headRequest {")
	g.indent++
	g.line("response = web.NewResponse(map[string]any{\"status\": response.TrbFieldStatus, \"headers\": response.TrbFieldHeaders, \"body\": __trb_http.BodyEmpty()})")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}()")
	g.line("context := web.NewContext(map[string]any{\"request\": request, \"path_parameters\": map[string]string{}})")
	g.line("handler := func(_ trbcontext.Context, _ *web.Context) *web.Response { return protocolResponse }")
	g.webMiddlewareChain(rootMiddlewares, "handler", "protocolNextHandler", directories)
	g.line("return handler(scope, context)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbWebDispatchCore(scope trbcontext.Context, request *web.Request, maxBodyBytes int) (response *web.Response) {")
	g.indent++
	g.line("defer func() {")
	g.indent++
	g.line("if recovered := recover(); recovered != nil {")
	g.indent++
	g.line("if err, ok := recovered.(error); ok && (errors.Is(err, trbcontext.Canceled) || errors.Is(err, trbcontext.DeadlineExceeded)) { panic(recovered) }")
	g.line("response = trbWebInternalServerError()")
	g.indent--
	g.line("} else if !trbWebValidResponse(response) {")
	g.indent++
	g.line("response = trbWebInternalServerError()")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}()")
	g.line("if len(request.TrbFieldBody.Bytes()) > maxBodyBytes {")
	g.indent++
	g.line("return trbWebPayloadTooLarge()")
	g.indent--
	g.line("}")
	if len(routes) == 0 {
		g.line("return web.NewResponse(map[string]any{\"status\": 404, \"headers\": trbWebHeaders(__trb_http.Header{Name: \"content-type\", Value: \"application/json; charset=utf-8\"}), \"body\": __trb_http.NewBody([]byte(\"{\\\"error\\\":\\\"not_found\\\"}\"))})")
		g.indent--
		g.line("}")
		g.b.WriteByte('\n')
		return
	}
	g.line("method := strings.ToUpper(request.TrbFieldMethod.ToS())")
	g.line("segments := []string{}")
	g.line("if request.TrbFieldPath != \"/\" {")
	g.indent++
	g.line("segments = strings.Split(strings.TrimPrefix(request.TrbFieldPath, \"/\"), \"/\")")
	g.indent--
	g.line("}")
	g.line("matchedPath := -1")
	for pathIndex, route := range pathRoutes {
		condition := goWebRouteConditions(webRouteSegments(route.Path))
		g.line("if matchedPath == -1 && " + strings.Join(condition, " && ") + " {")
		g.indent++
		g.line("matchedPath = " + strconv.Itoa(pathIndex))
		g.indent--
		g.line("}")
	}
	g.line("allowedMethods := []string{}")
	g.line("explicitHead := false")
	for _, route := range routes {
		condition := "matchedPath == " + strconv.Itoa(pathIndexes[route.Path])
		g.line("if " + condition + " {")
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
		condition := []string{"dispatchMethod == " + strconv.Quote(route.Method), "matchedPath == " + strconv.Itoa(pathIndexes[route.Path])}
		g.line("if " + strings.Join(condition, " && ") + " {")
		g.indent++
		g.line("pathParameters := map[string]string{}")
		for index, segment := range segments {
			if strings.HasPrefix(segment, ":") {
				g.line("pathParameters[" + strconv.Quote(strings.TrimPrefix(segment, ":")) + "] = segments[" + strconv.Itoa(index) + "]")
			} else if strings.HasPrefix(segment, "*") {
				g.line("pathParameters[" + strconv.Quote(strings.TrimPrefix(segment, "*")) + "] = strings.Join(segments[" + strconv.Itoa(index) + ":], \"/\")")
			}
		}
		contextName := "routeContext" + strconv.Itoa(routeIndex)
		handlerName := "routeHandler" + strconv.Itoa(routeIndex)
		g.line(contextName + " := web.NewContext(map[string]any{\"request\": request, \"path_parameters\": pathParameters})")
		g.line(handlerName + " := func(scope trbcontext.Context, context *web.Context) *web.Response {")
		g.indent++
		g.line("return " + g.webCallee(route.ModulePath, route.TargetHandler, directories) + "(scope, context)")
		g.indent--
		g.line("}")
		g.webMiddlewareChain(webintegration.NestedMiddlewares(route.Middlewares), handlerName, "nextHandler"+strconv.Itoa(routeIndex)+"_", directories)
		g.line("return " + handlerName + "(scope, " + contextName + ")")
		g.indent--
		g.line("}")
	}
	for routeIndex, route := range pathRoutes {
		segments := webRouteSegments(route.Path)
		condition := []string{"method == \"OPTIONS\"", "matchedPath == " + strconv.Itoa(routeIndex)}
		g.line("if " + strings.Join(condition, " && ") + " {")
		g.indent++
		g.line("pathParameters := map[string]string{}")
		for index, segment := range segments {
			if strings.HasPrefix(segment, ":") {
				g.line("pathParameters[" + strconv.Quote(strings.TrimPrefix(segment, ":")) + "] = segments[" + strconv.Itoa(index) + "]")
			} else if strings.HasPrefix(segment, "*") {
				g.line("pathParameters[" + strconv.Quote(strings.TrimPrefix(segment, "*")) + "] = strings.Join(segments[" + strconv.Itoa(index) + ":], \"/\")")
			}
		}
		contextName := "optionsContext" + strconv.Itoa(routeIndex)
		handlerName := "optionsHandler" + strconv.Itoa(routeIndex)
		g.line(contextName + " := web.NewContext(map[string]any{\"request\": request, \"path_parameters\": pathParameters})")
		g.line(handlerName + " := func(_ trbcontext.Context, context *web.Context) *web.Response {")
		g.indent++
		g.line("return web.NewResponse(map[string]any{\"status\": 204, \"headers\": trbWebHeaders(__trb_http.Header{Name: \"allow\", Value: strings.Join(allowedMethods, \", \")}), \"body\": __trb_http.BodyEmpty()})")
		g.indent--
		g.line("}")
		g.webMiddlewareChain(webintegration.NestedMiddlewares(route.Middlewares), handlerName, "optionsNextHandler"+strconv.Itoa(routeIndex)+"_", directories)
		g.line("return " + handlerName + "(scope, " + contextName + ")")
		g.indent--
		g.line("}")
	}
	g.line("if len(allowedMethods) > 0 {")
	g.indent++
	g.line("return web.NewResponse(map[string]any{\"status\": 405, \"headers\": trbWebHeaders(__trb_http.Header{Name: \"allow\", Value: strings.Join(allowedMethods, \", \")}, __trb_http.Header{Name: \"content-type\", Value: \"application/json; charset=utf-8\"}), \"body\": __trb_http.NewBody([]byte(\"{\\\"error\\\":\\\"method_not_allowed\\\"}\"))})")
	g.indent--
	g.line("}")
	g.line("return web.NewResponse(map[string]any{\"status\": 404, \"headers\": trbWebHeaders(__trb_http.Header{Name: \"content-type\", Value: \"application/json; charset=utf-8\"}), \"body\": __trb_http.NewBody([]byte(\"{\\\"error\\\":\\\"not_found\\\"}\"))})")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) webMiddlewareChain(middlewares []webintegration.Middleware, handlerName, nextPrefix string, directories map[string]string) {
	for middlewareIndex := len(middlewares) - 1; middlewareIndex >= 0; middlewareIndex-- {
		middleware := middlewares[middlewareIndex]
		nextName := nextPrefix + strconv.Itoa(middlewareIndex)
		g.line(nextName + " := " + handlerName)
		g.line(handlerName + " = func(scope trbcontext.Context, context *web.Context) *web.Response {")
		g.indent++
		g.line("return " + g.webCallee(middleware.ModulePath, middleware.TargetHandler, directories) + "(scope, context, &trbWebNext{handler: " + nextName + "})")
		g.indent--
		g.line("}")
	}
}

func (g *generator) webProtocolResponses() {
	g.line("const trbWebDefaultMaxBodyBytes = " + strconv.Itoa(webintegration.MaxBodyBytes))
	g.b.WriteByte('\n')
	g.line("func trbWebHeaders(entries ...__trb_http.Header) *__trb_http.Headers {")
	g.indent++
	g.line("return __trb_http.NewHeaders(entries)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbWebHeadersFromMap(values map[string][]string) *__trb_http.Headers {")
	g.indent++
	g.line("entries := []__trb_http.Header{}")
	g.line("for name, headerValues := range values {")
	g.indent++
	g.line("for _, value := range headerValues {")
	g.indent++
	g.line("entries = append(entries, __trb_http.Header{Name: name, Value: value})")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.line("return __trb_http.NewHeaders(entries)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbWebNormalizeRequest(request *web.Request) *web.Request {")
	g.indent++
	g.line("headers := []__trb_http.Header{}")
	g.line("for _, header := range request.TrbFieldHeaders.Entries() {")
	g.indent++
	g.line("headers = append(headers, __trb_http.Header{Name: strings.ToLower(header.Name), Value: header.Value})")
	g.indent--
	g.line("}")
	g.line("return web.NewRequest(map[string]any{\"method\": __trb_http.NewHttpMethod(strings.ToUpper(request.TrbFieldMethod.ToS())), \"path\": request.TrbFieldPath, \"query_string\": request.TrbFieldQueryString, \"headers\": __trb_http.NewHeaders(headers), \"body\": request.TrbFieldBody})")
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
	g.line("func trbWebInternalServerError() *web.Response {")
	g.indent++
	g.line("return web.NewResponse(map[string]any{\"status\": 500, \"headers\": trbWebHeaders(__trb_http.Header{Name: \"content-type\", Value: \"application/json; charset=utf-8\"}), \"body\": __trb_http.NewBody([]byte(\"{\\\"error\\\":\\\"internal_server_error\\\"}\"))})")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbWebBadRequest() *web.Response {")
	g.indent++
	g.line("return web.NewResponse(map[string]any{\"status\": 400, \"headers\": trbWebHeaders(__trb_http.Header{Name: \"content-type\", Value: \"application/json; charset=utf-8\"}), \"body\": __trb_http.NewBody([]byte(\"{\\\"error\\\":\\\"bad_request\\\"}\"))})")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbWebPayloadTooLarge() *web.Response {")
	g.indent++
	g.line("return web.NewResponse(map[string]any{\"status\": 413, \"headers\": trbWebHeaders(__trb_http.Header{Name: \"content-type\", Value: \"application/json; charset=utf-8\"}), \"body\": __trb_http.NewBody([]byte(\"{\\\"error\\\":\\\"payload_too_large\\\"}\"))})")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbWebValidResponse(response *web.Response) bool {")
	g.indent++
	g.line("if response == nil || response.TrbFieldStatus < 100 || response.TrbFieldStatus > 999 {")
	g.indent++
	g.line("return false")
	g.indent--
	g.line("}")
	g.line("if response.TrbFieldHeaders == nil || response.TrbFieldBody == nil {")
	g.indent++
	g.line("return false")
	g.indent--
	g.line("}")
	g.line("for _, header := range response.TrbFieldHeaders.Entries() {")
	g.indent++
	g.line("name := header.Name")
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
	g.line("if strings.ContainsAny(header.Value, \"\\r\\n\") {")
	g.indent++
	g.line("return false")
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
	callee := g.projectFunctionName(modulePath, target)
	if alias := directories[pathpkg.Dir(modulePath)]; alias != "" {
		callee = goImportAlias(alias) + "." + callee
	}
	return callee
}

func (g *generator) webNext() {
	g.line("type trbWebNext struct {")
	g.indent++
	g.line("called bool")
	g.line("handler func(trbcontext.Context, *web.Context) *web.Response")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func (next *trbWebNext) Call(scope trbcontext.Context, context *web.Context) *web.Response {")
	g.indent++
	g.line("if next.called {")
	g.indent++
	g.line("panic(\"trb/web Next.call may only be called once\")")
	g.indent--
	g.line("}")
	g.line("next.called = true")
	g.line("return next.handler(scope, context)")
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

func goWebRouteConditions(segments []string) []string {
	lengthOperator := "=="
	if len(segments) > 0 && strings.HasPrefix(segments[len(segments)-1], "*") {
		lengthOperator = ">="
	}
	conditions := []string{"len(segments) " + lengthOperator + " " + strconv.Itoa(len(segments))}
	for index, segment := range segments {
		if strings.HasPrefix(segment, ":") || strings.HasPrefix(segment, "*") {
			continue
		}
		conditions = append(conditions, "segments["+strconv.Itoa(index)+"] == "+strconv.Quote(segment))
	}
	return conditions
}

func (g *generator) webServer() {
	g.requireImport("context", "trbcontext")
	g.requireImport("errors", "")
	g.requireImport("io", "")
	g.requireImport("net", "")
	g.requireImport("net/http", "nethttp")
	g.requireImport("os", "")
	g.requireImport("os/signal", "signal")
	g.requireImport("strconv", "")
	g.requireImport("syscall", "")
	g.requireImport("time", "stdtime")

	g.line("func trbWebServe(config web.ServerConfig) {")
	g.indent++
	g.line("trbWebValidateServerConfig(config)")
	g.line("handler := nethttp.HandlerFunc(func(writer nethttp.ResponseWriter, request *nethttp.Request) {")
	g.indent++
	g.line("body, err := io.ReadAll(nethttp.MaxBytesReader(writer, request.Body, int64(config.BodyLimitBytes)))")
	g.line("webRequest := web.NewRequest(map[string]any{\"method\": __trb_http.NewHttpMethod(request.Method), \"path\": request.URL.EscapedPath(), \"query_string\": request.URL.RawQuery, \"headers\": trbWebHeadersFromMap(map[string][]string(request.Header.Clone())), \"body\": __trb_http.NewBody(body)})")
	g.line("var response *web.Response")
	g.line("if err != nil {")
	g.indent++
	g.line("var maxBytesError *nethttp.MaxBytesError")
	g.line("if errors.As(err, &maxBytesError) {")
	g.indent++
	g.line("response = trbWebDispatchProtocolResponse(request.Context(), webRequest, trbWebPayloadTooLarge())")
	g.indent--
	g.line("} else {")
	g.indent++
	g.line("response = trbWebDispatchProtocolResponse(request.Context(), webRequest, trbWebBadRequest())")
	g.indent--
	g.line("}")
	g.indent--
	g.line("} else {")
	g.indent++
	g.line("response = trbWebDispatchWithBodyLimit(request.Context(), webRequest, config.BodyLimitBytes)")
	g.indent--
	g.line("}")
	g.line("for _, header := range response.TrbFieldHeaders.Entries() {")
	g.indent++
	g.line("writer.Header().Add(header.Name, header.Value)")
	g.indent--
	g.line("}")
	g.line("writer.WriteHeader(response.TrbFieldStatus)")
	g.line("_, _ = writer.Write(response.TrbFieldBody.Bytes())")
	g.indent--
	g.line("})")
	g.line("server := &nethttp.Server{Addr: net.JoinHostPort(config.Host, strconv.Itoa(config.Port)), Handler: handler}")
	g.line("signalContext, stopSignals := signal.NotifyContext(trbcontext.Background(), os.Interrupt, syscall.SIGTERM)")
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
	g.line("if err != nil && !errors.Is(err, nethttp.ErrServerClosed) {")
	g.indent++
	g.line("panic(err)")
	g.indent--
	g.line("}")
	g.indent--
	g.line("case <-signalContext.Done():")
	g.indent++
	g.line("shutdownContext, cancelShutdown := trbcontext.WithTimeout(trbcontext.Background(), stdtime.Duration(config.ShutdownTimeoutMilliseconds)*stdtime.Millisecond)")
	g.line("shutdownErr := server.Shutdown(shutdownContext)")
	g.line("cancelShutdown()")
	g.line("if shutdownErr != nil {")
	g.indent++
	g.line("_ = server.Close()")
	g.indent--
	g.line("}")
	g.line("if err := <-serveErrors; err != nil && !errors.Is(err, nethttp.ErrServerClosed) {")
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
