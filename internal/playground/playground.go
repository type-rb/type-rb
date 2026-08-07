//go:build !js || !wasm

// Package playground serves the local TypeRB playground and guided tour.
// Compilation and evaluation use the same compiler and typed IR as the CLI.
package playground

import (
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
)

const maxSourceBytes = 256 * 1024

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

func Handler(options Options) http.Handler {
	if !ValidMode(options.Mode) {
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
	serveConfig := func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, Config{
			Transport: "http", Mode: options.Mode, Modes: []string{"go", "ruby", "typescript"}, Page: options.Page, Version: options.Version,
		})
	}
	mux.HandleFunc("GET /runtime.json", serveConfig)
	mux.HandleFunc("GET /api/config", serveConfig)
	mux.HandleFunc("GET /api/tour", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, Tour())
	})
	mux.HandleFunc("POST /api/run", func(writer http.ResponseWriter, req *http.Request) {
		input, ok := readRequest(writer, req)
		if !ok {
			return
		}
		writeJSON(writer, http.StatusOK, Run(req.Context(), input.Source, input.Mode))
	})
	mux.HandleFunc("POST /api/transpile", func(writer http.ResponseWriter, req *http.Request) {
		input, ok := readRequest(writer, req)
		if !ok {
			return
		}
		writeJSON(writer, http.StatusOK, Transpile(input.Source, input.Mode))
	})
	mux.HandleFunc("POST /api/format", func(writer http.ResponseWriter, req *http.Request) {
		input, ok := readRequest(writer, req)
		if !ok {
			return
		}
		writeJSON(writer, http.StatusOK, Format(input.Source))
	})

	return securityHeaders(mux)
}

func Serve(options Options) error {
	if options.Port < 0 || options.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535; got %d", options.Port)
	}
	if !ValidMode(options.Mode) {
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

func readRequest(writer http.ResponseWriter, req *http.Request) (Request, bool) {
	if !strings.HasPrefix(strings.ToLower(req.Header.Get("Content-Type")), "application/json") {
		writeJSON(writer, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"})
		return Request{}, false
	}
	if origin := req.Header.Get("Origin"); origin != "" && origin != "http://"+req.Host && origin != "https://"+req.Host {
		writeJSON(writer, http.StatusForbidden, map[string]string{"error": "cross-origin requests are not allowed"})
		return Request{}, false
	}
	body := http.MaxBytesReader(writer, req.Body, maxSourceBytes)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var input Request
	if err := decoder.Decode(&input); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid request: " + err.Error()})
		return Request{}, false
	}
	if !ValidMode(input.Mode) {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unsupported mode %q", input.Mode)})
		return Request{}, false
	}
	return input, true
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self'; img-src 'self' data:; connect-src 'self'; worker-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
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
