// Package playground serves the local TypeRB playground and guided tour.
// Compilation and evaluation use the same compiler and typed IR as the CLI.
package playground

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"time"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/formatter"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/repl"
	"github.com/type-rb/type-rb/internal/types"
)

const (
	maxSourceBytes  = 256 * 1024
	evaluationLimit = 3 * time.Second
)

//go:embed assets/*
var webAssets embed.FS

type Options struct {
	Mode        string
	Page        string
	Port        int
	OpenBrowser bool
	Version     string
	Stdout      io.Writer
	Stderr      io.Writer
}

type request struct {
	Source string `json:"source"`
	Mode   string `json:"mode"`
}

type response struct {
	OK          bool         `json:"ok"`
	Output      string       `json:"output,omitempty"`
	Generated   string       `json:"generated,omitempty"`
	Formatted   string       `json:"formatted,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	DurationMS  int64        `json:"durationMs"`
}

type Diagnostic struct {
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	Line      int    `json:"line,omitempty"`
	Column    int    `json:"column,omitempty"`
	EndLine   int    `json:"endLine,omitempty"`
	EndColumn int    `json:"endColumn,omitempty"`
}

type configResponse struct {
	Mode    string   `json:"mode"`
	Modes   []string `json:"modes"`
	Page    string   `json:"page"`
	Version string   `json:"version"`
}

func Handler(options Options) http.Handler {
	if !validMode(options.Mode) {
		options.Mode = "go"
	}
	if options.Page != "tour" {
		options.Page = "play"
	}
	assets, err := fs.Sub(webAssets, "assets")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(writer http.ResponseWriter, req *http.Request) {
		http.Redirect(writer, req, "/play/", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("GET /play/", serveIndex)
	mux.HandleFunc("GET /tour/", serveIndex)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets))))
	mux.HandleFunc("GET /api/config", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, configResponse{
			Mode: options.Mode, Modes: []string{"go", "ruby", "typescript"}, Page: options.Page, Version: options.Version,
		})
	})
	mux.HandleFunc("GET /api/tour", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, Tour())
	})
	mux.HandleFunc("POST /api/run", func(writer http.ResponseWriter, req *http.Request) {
		input, ok := readRequest(writer, req)
		if !ok {
			return
		}
		started := time.Now()
		result := evaluate(req.Context(), input.Source, input.Mode)
		result.DurationMS = time.Since(started).Milliseconds()
		writeJSON(writer, http.StatusOK, result)
	})
	mux.HandleFunc("POST /api/transpile", func(writer http.ResponseWriter, req *http.Request) {
		input, ok := readRequest(writer, req)
		if !ok {
			return
		}
		started := time.Now()
		result := transpile(input.Source, input.Mode)
		result.DurationMS = time.Since(started).Milliseconds()
		writeJSON(writer, http.StatusOK, result)
	})
	mux.HandleFunc("POST /api/format", func(writer http.ResponseWriter, req *http.Request) {
		input, ok := readRequest(writer, req)
		if !ok {
			return
		}
		started := time.Now()
		result := format(input.Source)
		result.DurationMS = time.Since(started).Milliseconds()
		writeJSON(writer, http.StatusOK, result)
	})

	return securityHeaders(mux)
}

func Serve(options Options) error {
	if options.Port < 0 || options.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535; got %d", options.Port)
	}
	if !validMode(options.Mode) {
		return fmt.Errorf("mode must be go, ruby, or typescript; got %q", options.Mode)
	}
	if options.Stdout == nil {
		options.Stdout = os.Stdout
	}
	if options.Stderr == nil {
		options.Stderr = os.Stderr
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", options.Port))
	if err != nil {
		return err
	}
	defer listener.Close()

	page := options.Page
	if page != "tour" {
		page = "play"
	}
	url := fmt.Sprintf("http://%s/%s/", listener.Addr().String(), page)
	fmt.Fprintf(options.Stdout, "TypeRB %s is running at %s\n", pageName(page), url)
	fmt.Fprintln(options.Stdout, "Press Ctrl-C to stop.")
	if options.OpenBrowser {
		if err := openBrowser(url); err != nil {
			fmt.Fprintf(options.Stderr, "trb: could not open the browser: %s\n", err)
		}
	}

	server := &http.Server{
		Handler:           Handler(options),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	serveErrors := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrors <- err
			return
		}
		serveErrors <- nil
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	select {
	case err := <-serveErrors:
		return err
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return err
		}
		return <-serveErrors
	}
}

func serveIndex(writer http.ResponseWriter, _ *http.Request) {
	data, err := webAssets.ReadFile("assets/index.html")
	if err != nil {
		http.Error(writer, "playground asset is unavailable", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(data)
}

func readRequest(writer http.ResponseWriter, req *http.Request) (request, bool) {
	if !strings.HasPrefix(strings.ToLower(req.Header.Get("Content-Type")), "application/json") {
		writeJSON(writer, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"})
		return request{}, false
	}
	if origin := req.Header.Get("Origin"); origin != "" && origin != "http://"+req.Host && origin != "https://"+req.Host {
		writeJSON(writer, http.StatusForbidden, map[string]string{"error": "cross-origin requests are not allowed"})
		return request{}, false
	}
	body := http.MaxBytesReader(writer, req.Body, maxSourceBytes)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var input request
	if err := decoder.Decode(&input); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid request: " + err.Error()})
		return request{}, false
	}
	if !validMode(input.Mode) {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unsupported mode %q", input.Mode)})
		return request{}, false
	}
	return input, true
}

func transpile(source, mode string) response {
	_, session, err := compile(source, mode)
	if err != nil {
		return errorResponse(err)
	}
	return response{OK: true, Generated: string(session.Output)}
}

func evaluate(parent context.Context, source, mode string) response {
	programs, session, err := compile(source, mode)
	if err != nil {
		return errorResponse(err)
	}
	if item := disallowedRuntimeImport(session.AST); item != "" {
		return response{OK: false, Generated: string(session.Output), Diagnostics: []Diagnostic{{
			Severity: "error", Message: fmt.Sprintf("%s is not available in the browser playground", item),
		}}}
	}

	var output bytes.Buffer
	evaluator := repl.NewEvaluator(&output, mode)
	if err := evaluator.LoadProject(programs, session.IR.ModulePath); err != nil {
		return runtimeError(err, string(session.Output))
	}
	evaluator.LoadDefinitions(session.IR)
	ctx, cancel := context.WithTimeout(parent, evaluationLimit)
	defer cancel()
	result, err := evaluator.EvaluateContext(ctx, session.IR.Statements, session.IR.ModulePath)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("evaluation exceeded %s", evaluationLimit)
		}
		return runtimeError(err, string(session.Output))
	}
	if result.Display && result.Value.Type.Kind != types.Void {
		fmt.Fprintf(&output, "%s : %s\n", repl.Inspect(result.Value), result.Value.Type.String())
	}
	return response{OK: true, Output: output.String(), Generated: string(session.Output)}
}

func format(source string) response {
	formatted, diagnostics := formatter.Format([]byte(source))
	if hasDiagnosticErrors(diagnostics) {
		return response{OK: false, Diagnostics: diagnosticsFrom("playground.trb", diagnostics)}
	}
	return response{OK: true, Formatted: string(formatted)}
}

func compile(source, mode string) ([]*ir.Program, *compiler.Artifact, error) {
	packageName := ""
	if mode == "go" {
		packageName = "main"
	}
	const modulePath = "playground"
	artifacts, err := compiler.CompileProject([]compiler.SourceUnit{{
		Filename: "playground.trb", Source: []byte(source), ModulePath: modulePath, Package: packageName,
	}}, compiler.Options{
		Mode: mode, Package: packageName, ModulePath: modulePath, GoModule: "trb.local/playground", SourceRoot: ".", ProjectRoot: ".",
	})
	if err != nil {
		return nil, nil, err
	}
	programs := make([]*ir.Program, 0, len(artifacts))
	var session *compiler.Artifact
	for _, artifact := range artifacts {
		programs = append(programs, artifact.IR)
		if artifact.IR.ModulePath == modulePath {
			session = artifact
		}
	}
	if session == nil {
		return nil, nil, errors.New("compiler did not return the playground source")
	}
	return programs, session, nil
}

func disallowedRuntimeImport(program *ast.Program) string {
	for _, statement := range program.Statements {
		imported, ok := statement.(*ast.ImportStatement)
		if !ok {
			continue
		}
		if imported.Path == "trb/std/filesystem" || imported.Path == "trb/std/process" || strings.HasPrefix(imported.Path, "trb/platform/") {
			return imported.Path
		}
	}
	return ""
}

func errorResponse(err error) response {
	var compilation *compiler.CompileError
	if errors.As(err, &compilation) {
		return response{OK: false, Diagnostics: diagnosticsFrom(compilation.Filename, compilation.Diagnostics)}
	}
	return runtimeError(err, "")
}

func runtimeError(err error, generated string) response {
	return response{OK: false, Generated: generated, Diagnostics: []Diagnostic{{Severity: "error", Message: err.Error()}}}
}

func diagnosticsFrom(_ string, diagnostics []diagnostic.Diagnostic) []Diagnostic {
	result := make([]Diagnostic, 0, len(diagnostics))
	for _, item := range diagnostics {
		result = append(result, Diagnostic{
			Severity: string(item.Severity), Message: item.Message,
			Line: item.Span.Start.Line, Column: item.Span.Start.Column, EndLine: item.Span.End.Line, EndColumn: item.Span.End.Column,
		})
	}
	return result
}

func hasDiagnosticErrors(diagnostics []diagnostic.Diagnostic) bool {
	for _, item := range diagnostics {
		if item.Severity == diagnostic.Error {
			return true
		}
	}
	return false
}

func validMode(mode string) bool {
	return mode == "go" || mode == "ruby" || mode == "typescript"
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(writer, req)
	})
}

func openBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}

func pageName(page string) string {
	if page == "tour" {
		return "tour"
	}
	return "playground"
}
