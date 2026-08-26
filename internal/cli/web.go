package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/compilerservice"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
	"github.com/type-rb/type-rb/internal/project"
	webintegration "github.com/type-rb/type-rb/internal/web"
)

type webContractProject struct {
	config       *project.Config
	manifest     *webintegration.Manifest
	declarations packageextension.ProjectDeclarationInput
	moduleFiles  map[string]string
}

func (c *CLI) runWeb(args []string) error {
	if len(args) == 0 {
		return errors.New("web requires a subcommand: client or openapi")
	}
	switch args[0] {
	case "client":
		return c.runWebClient(args[1:])
	case "openapi":
		return c.runWebOpenAPI(args[1:])
	default:
		return fmt.Errorf("unknown web command %q; expected client or openapi", args[0])
	}
}

func (c *CLI) runWebClient(args []string) error {
	flags := flag.NewFlagSet("web client", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", "", "path to trbconfig.jsonc")
	output := flags.String("output", "-", "write TypeRB source below the project root, or - for stdout")
	name := flags.String("name", webintegration.DefaultBrowserClientName, "generated TypeRB client class name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("web client does not accept positional arguments")
	}
	loaded, err := loadWebContractProject(*configPath, "client")
	if err != nil {
		return err
	}
	source, issues, err := webintegration.BuildBrowserClient(loaded.manifest.EndpointCatalog, loaded.declarations, webintegration.BrowserClientOptions{Name: *name})
	if err != nil {
		return err
	}
	if len(issues) != 0 {
		diagnostics := make([]diagnostic.Diagnostic, 0, len(issues))
		for _, issue := range issues {
			diagnostics = append(diagnostics, webContractDiagnostic(loaded, issue.ModulePath, issue.Message, issue.Span))
		}
		return compiler.NewCompileError(loaded.config.Path, diagnostic.ProjectIntegration, diagnostics)
	}
	if strings.TrimSpace(*output) == "" || *output == "-" {
		_, err = c.Stdout.Write([]byte(source))
		return err
	}
	path, err := projectOutputPath(loaded.config.Root, *output)
	if err != nil {
		return fmt.Errorf("web client --output: %w", err)
	}
	if err := atomicOutputWrite(path, []byte(source), 0o644); err != nil {
		return err
	}
	_, err = fmt.Fprintln(c.Stdout, path)
	return err
}

func (c *CLI) runWebOpenAPI(args []string) error {
	flags := flag.NewFlagSet("web openapi", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", "", "path to trbconfig.jsonc")
	output := flags.String("output", "-", "write JSON below the project root, or - for stdout")
	title := flags.String("title", "", "OpenAPI document title; defaults to the project name")
	apiVersion := flags.String("api-version", "", "OpenAPI API version; defaults to the project version")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("web openapi does not accept positional arguments")
	}
	loaded, err := loadWebContractProject(*configPath, "openapi")
	if err != nil {
		return err
	}
	documentTitle := strings.TrimSpace(*title)
	if documentTitle == "" {
		documentTitle = loaded.config.Name
	}
	documentVersion := strings.TrimSpace(*apiVersion)
	if documentVersion == "" {
		documentVersion = loaded.config.Version
	}
	document, issues, err := webintegration.BuildOpenAPI(loaded.manifest.EndpointCatalog, loaded.declarations, webintegration.OpenAPIOptions{
		Title: documentTitle, Version: documentVersion,
	})
	if err != nil {
		return err
	}
	if len(issues) != 0 {
		diagnostics := make([]diagnostic.Diagnostic, 0, len(issues))
		for _, issue := range issues {
			diagnostics = append(diagnostics, webContractDiagnostic(loaded, issue.ModulePath, issue.Message, issue.Span))
		}
		return compiler.NewCompileError(loaded.config.Path, diagnostic.ProjectIntegration, diagnostics)
	}

	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return err
	}
	if strings.TrimSpace(*output) == "" || *output == "-" {
		_, err = c.Stdout.Write(encoded.Bytes())
		return err
	}
	path, err := projectOutputPath(loaded.config.Root, *output)
	if err != nil {
		return fmt.Errorf("web openapi --output: %w", err)
	}
	if err := atomicOutputWrite(path, encoded.Bytes(), 0o644); err != nil {
		return err
	}
	_, err = fmt.Fprintln(c.Stdout, path)
	return err
}

func loadWebContractProject(configPath, subcommand string) (*webContractProject, error) {
	config, err := loadConfig(configPath, ".")
	if err != nil {
		return nil, err
	}
	files, err := collectTRB([]string{config.SourcePath()}, config.OutputPath())
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("no .trb files found")
	}
	units, options, err := projectCompilation(config, files)
	if err != nil {
		return nil, err
	}
	snapshot := compilerservice.New(units, options).Analyze()
	if snapshot.HasErrors() {
		return nil, compiler.NewCompileError("", diagnostic.ProjectIntegration, snapshot.Diagnostics)
	}

	var manifest *webintegration.Manifest
	programs := make([]*ast.Program, 0, len(snapshot.Artifacts))
	moduleFiles := map[string]string{}
	for _, artifact := range snapshot.Artifacts {
		if artifact == nil || artifact.AST == nil {
			continue
		}
		programs = append(programs, artifact.AST)
		moduleFiles[artifact.AST.ModulePath] = artifact.Filename
		if artifact.IR != nil {
			if candidate := webintegration.ManifestFrom(artifact.IR.Extensions); candidate != nil {
				manifest = candidate
			}
		}
	}
	if manifest == nil {
		return nil, fmt.Errorf("web %s requires a trb/web file-routing project", subcommand)
	}
	packageAliases := map[string]map[string]string{}
	for _, unit := range units {
		if len(unit.PackageAliases) == 0 {
			continue
		}
		aliases := make(map[string]string, len(unit.PackageAliases))
		for name, canonical := range unit.PackageAliases {
			aliases[name] = canonical
		}
		packageAliases[unit.ModulePath] = aliases
	}
	declarations, err := packageextensionhost.ExportProjectDeclarationInput(webintegration.PackageName, programs, packageextensionhost.ProjectDeclarationInputOptions{
		PackageAliasesByModule: packageAliases,
	})
	if err != nil {
		return nil, fmt.Errorf("build trb/web declaration input: %w", err)
	}
	return &webContractProject{config: config, manifest: manifest, declarations: declarations, moduleFiles: moduleFiles}, nil
}

func webContractDiagnostic(loaded *webContractProject, modulePath, message string, span packageextension.SourceSpan) diagnostic.Diagnostic {
	path := loaded.moduleFiles[modulePath]
	if path == "" {
		path = loaded.config.Path
	}
	return diagnostic.Diagnostic{
		Code: diagnostic.ProjectIntegration, Severity: diagnostic.Error, Message: message,
		Path: path, Span: packageextensionhost.ImportSourceSpan(span),
	}
}
