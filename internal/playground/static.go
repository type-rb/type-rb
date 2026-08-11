//go:build !js || !wasm

package playground

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type StaticOptions struct {
	OutputDir string
	Version   string
}

func ExportStatic(options StaticOptions) error {
	if options.OutputDir == "" {
		return errors.New("static playground output directory is required")
	}
	if options.Version == "" {
		options.Version = "dev"
	}
	for _, directory := range []string{
		options.OutputDir,
		filepath.Join(options.OutputDir, "assets"),
		filepath.Join(options.OutputDir, "play"),
		filepath.Join(options.OutputDir, "tour"),
		filepath.Join(options.OutputDir, "type-rb"),
		filepath.Join(options.OutputDir, "type-rb", "play"),
		filepath.Join(options.OutputDir, "type-rb", "tour"),
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}

	index, err := webAssets.ReadFile("assets/index.html")
	if err != nil {
		return err
	}
	for _, path := range []string{"play/index.html", "tour/index.html"} {
		if err := writeStaticFile(options.OutputDir, path, index); err != nil {
			return err
		}
	}
	for _, name := range []string{"app.css", "app.js", "playground-worker.js"} {
		data, err := webAssets.ReadFile("assets/" + name)
		if err != nil {
			return err
		}
		if err := writeStaticFile(options.OutputDir, filepath.Join("assets", name), data); err != nil {
			return err
		}
	}

	config := Config{
		Transport: "wasm",
		Mode:      "go",
		Modes:     []string{"go", "ruby", "typescript"},
		Version:   options.Version,
	}
	if err := writeStaticJSON(options.OutputDir, "runtime.json", config); err != nil {
		return err
	}
	if err := writeStaticJSON(options.OutputDir, "tour.json", Tour()); err != nil {
		return err
	}
	landing, err := webAssets.ReadFile("assets/landing.html")
	if err != nil {
		return err
	}
	if err := writeStaticFile(options.OutputDir, "index.html", landing); err != nil {
		return err
	}
	for path, destination := range map[string]string{
		"type-rb/index.html":      "/",
		"type-rb/play/index.html": "/play/",
		"type-rb/tour/index.html": "/tour/",
	} {
		if err := writeStaticFile(options.OutputDir, path, legacyRedirectPage(destination)); err != nil {
			return err
		}
	}
	return nil
}

func legacyRedirectPage(destination string) []byte {
	return []byte(fmt.Sprintf(`<!doctype html>
<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta http-equiv="refresh" content="0; url=%s">
		<link rel="canonical" href="https://type-rb.github.io%s">
		<title>TypeRB</title>
		<script>location.replace(%q + location.search + location.hash)</script>
	</head>
	<body><a href="%s">Continue to TypeRB</a></body>
</html>
`, destination, destination, destination, destination))
}

func writeStaticJSON(outputDir, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "\t")
	if err != nil {
		return err
	}
	return writeStaticFile(outputDir, name, append(data, '\n'))
}

func writeStaticFile(outputDir, name string, data []byte) error {
	path := filepath.Join(outputDir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
