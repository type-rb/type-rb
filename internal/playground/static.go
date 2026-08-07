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
	return writeStaticFile(options.OutputDir, "index.html", landing)
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
