package ruby

import (
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

func (g *generator) webServer() {
	g.line("", "")
	g.line("def trb_web_serve(port)", "")
	g.indent++
	g.line(`require "socket"`, "")
	g.line(`server = TCPServer.new("0.0.0.0", port)`, "")
	g.line("loop do", "")
	g.indent++
	g.line("client = server.accept", "")
	g.line("Thread.new(client) do |connection|", "")
	g.indent++
	g.line("begin", "")
	g.indent++
	g.line("loop do", "")
	g.indent++
	g.line(`request_line = connection.gets("\r\n")`, "")
	g.line("break if request_line.nil?", "")
	g.line("request_line = request_line.strip", "")
	g.line("next if request_line.empty?", "")
	g.line(`method, target, version = request_line.split(" ", 3)`, "")
	g.line("headers = {}", "")
	g.line(`while (line = connection.gets("\r\n"))`, "")
	g.indent++
	g.line(`line = line.delete_suffix("\r\n")`, "")
	g.line("break if line.empty?", "")
	g.line(`name, value = line.split(":", 2)`, "")
	g.line("next if value.nil?", "")
	g.line("key = name.downcase", "")
	g.line("(headers[key] ||= []) << value.strip", "")
	g.indent--
	g.line("end", "")
	g.line(`content_length = (headers["content-length"]&.first || "0").to_i`, "")
	g.line(`body = content_length.positive? ? connection.read(content_length) : "".b`, "")
	g.line(`path = target.split("?", 2).first`, "")
	g.line("response = trb_web_dispatch(Request.new(method: method, path: path, headers: headers, body: body))", "")
	g.line(`reason = { 200 => "OK", 201 => "Created", 204 => "No Content", 400 => "Bad Request", 404 => "Not Found", 500 => "Internal Server Error" }[response.status] || "Response"`, "")
	g.line("response_headers = response.headers.dup", "")
	g.line(`response_headers["content-length"] ||= [response.body.bytesize.to_s]`, "")
	g.line(`keep_alive = version == "HTTP/1.1" && headers.fetch("connection", [""]).first.downcase != "close"`, "")
	g.line(`response_headers["connection"] ||= [keep_alive ? "keep-alive" : "close"]`, "")
	g.line(`connection.write("HTTP/1.1 #{response.status} #{reason}\r\n")`, "")
	g.line("response_headers.each do |name, values|", "")
	g.indent++
	g.line("values.each do |value|", "")
	g.indent++
	g.line(`connection.write("#{name}: #{value}\r\n")`, "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.line(`connection.write("\r\n")`, "")
	g.line("connection.write(response.body)", "")
	g.line("break unless keep_alive", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("rescue StandardError", "")
	g.line("ensure", "")
	g.indent++
	g.line("connection.close", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("rescue Interrupt", "")
	g.indent++
	g.line("nil", "")
	g.indent--
	g.line("ensure", "")
	g.indent++
	g.line("server&.close", "")
	g.indent--
	g.line("end", "")
}
