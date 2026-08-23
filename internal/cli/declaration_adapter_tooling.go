package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/type-rb/type-rb/internal/declarationadapterhost"
	"github.com/type-rb/type-rb/internal/declarationadaptertooling"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/nativepackage"
	"github.com/type-rb/type-rb/internal/packageextension"
	packageManager "github.com/type-rb/type-rb/internal/packages"
)

func (c *CLI) runAdapter(args []string) error {
	if len(args) == 0 {
		return errors.New("adapter requires a subcommand: check")
	}
	switch args[0] {
	case "check":
		return c.runAdapterCheck(args[1:])
	default:
		return fmt.Errorf("unknown adapter command %q", args[0])
	}
}

func (c *CLI) runAdapterCheck(args []string) error {
	flags := flag.NewFlagSet("adapter check", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	format := flags.String("format", "human", "output format: human or json")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *format != "human" && *format != "json" {
		return fmt.Errorf("--format must be human or json; got %q", *format)
	}
	if flags.NArg() > 1 {
		return errors.New("adapter check accepts at most one package root")
	}

	root := "."
	if flags.NArg() == 1 {
		root = flags.Arg(0)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	root = filepath.Clean(root)
	manifestPath := filepath.Join(root, packageManager.TypeRBManifestName)
	manifest, err := packageManager.ReadTypeRBManifest(root)
	if err != nil {
		return c.finishAdapterCheck(*format, nil, nil, []diagnostic.Diagnostic{{
			Code: diagnostic.ProjectError, Severity: diagnostic.Error, Message: err.Error(), Path: manifestPath,
		}})
	}

	packageInfo := &declarationadaptertooling.Package{Name: manifest.Name, Version: manifest.Version, ManifestPath: manifestPath}
	if len(manifest.DeclarationAdapters) == 0 {
		return c.finishAdapterCheck(*format, packageInfo, nil, []diagnostic.Diagnostic{{
			Code: diagnostic.ProjectIntegration, Severity: diagnostic.Error,
			Message: fmt.Sprintf("package %s declares no declaration adapters", manifest.Name), Path: manifestPath,
		}})
	}

	modes := make([]string, 0, len(manifest.DeclarationAdapters))
	for mode := range manifest.DeclarationAdapters {
		modes = append(modes, mode)
	}
	sort.Strings(modes)
	adapters := make([]declarationadaptertooling.Adapter, 0, len(modes))
	issues := []diagnostic.Diagnostic{}
	for _, mode := range modes {
		adapterPath := filepath.Join(root, manifest.DeclarationAdapterFor(mode))
		adapterPath = filepath.Clean(adapterPath)
		checked := declarationadaptertooling.Adapter{Mode: mode, Path: adapterPath}
		provided, readErr := declarationadapterhost.Read(adapterPath)
		if readErr == nil {
			checked.DeclarationProtocolVersion = provided.ProtocolVersion
			checked.Modules, checked.Exports, checked.SupportingRecords = declarationAdapterCounts(provided)
		}
		if readErr != nil {
			issues = append(issues, adapterCheckDiagnostic(adapterPath, readErr))
			adapters = append(adapters, checked)
			continue
		}

		dependencies := manifest.NativeDependenciesFor(mode)
		catalog := nativepackage.Empty(dependencies)
		applyErr := nativepackage.ApplyDeclarationAdapterFiles(catalog, []declarationadapterhost.Source{{
			Package: manifest.Name, Mode: mode, Path: adapterPath, Dependencies: dependencies,
		}})
		if applyErr != nil {
			issues = append(issues, adapterCheckDiagnostic(adapterPath, applyErr))
		} else {
			checked.Valid = true
		}
		adapters = append(adapters, checked)
	}
	return c.finishAdapterCheck(*format, packageInfo, adapters, issues)
}

func declarationAdapterCounts(catalog packageextension.DeclarationAdapterCatalog) (modules, exports, records int) {
	modules = len(catalog.Modules)
	for _, module := range catalog.Modules {
		exports += len(module.Exports)
		records += len(module.Records)
	}
	return modules, exports, records
}

func adapterCheckDiagnostic(path string, err error) diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Code: diagnostic.ProjectIntegration, Severity: diagnostic.Error, Message: err.Error(), Path: path,
	}
}

func (c *CLI) finishAdapterCheck(format string, packageInfo *declarationadaptertooling.Package, adapters []declarationadaptertooling.Adapter, issues []diagnostic.Diagnostic) error {
	report := declarationadaptertooling.Build(declarationadaptertooling.BuildOptions{
		CompilerVersion: Version, Package: packageInfo, Adapters: adapters, Diagnostics: issues,
	})
	if format == "json" {
		encoder := json.NewEncoder(c.Stdout)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return err
		}
		if len(issues) != 0 {
			return &reportedError{cause: errors.New("declaration adapter check failed")}
		}
		return nil
	}
	if len(issues) != 0 {
		c.writeHumanDiagnostics(issues)
		return &reportedError{cause: errors.New("declaration adapter check failed")}
	}
	_, err := fmt.Fprintf(c.Stdout, "checked %d declaration adapter(s) for package %s\n", len(adapters), packageInfo.Name)
	return err
}
