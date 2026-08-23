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
		return errors.New("adapter requires a subcommand: check or test")
	}
	switch args[0] {
	case "check":
		return c.runAdapterCheck(args[1:])
	case "test":
		return c.runAdapterTest(args[1:])
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
	checked := checkAdapterPackage(root)
	return c.finishAdapterCheck(*format, checked.Package, checked.Adapters, checked.Diagnostics)
}

type adapterPackageCheck struct {
	Root         string
	ManifestPath string
	Manifest     *packageManager.TypeRBManifest
	Package      *declarationadaptertooling.Package
	Adapters     []declarationadaptertooling.Adapter
	Diagnostics  []diagnostic.Diagnostic
}

func checkAdapterPackage(root string) adapterPackageCheck {
	result := adapterPackageCheck{Root: root, ManifestPath: filepath.Join(root, packageManager.TypeRBManifestName)}
	manifest, err := packageManager.ReadTypeRBManifest(root)
	if err != nil {
		result.Diagnostics = []diagnostic.Diagnostic{{
			Code: diagnostic.ProjectError, Severity: diagnostic.Error, Message: err.Error(), Path: result.ManifestPath,
		}}
		return result
	}
	result.Manifest = manifest
	result.Package = &declarationadaptertooling.Package{Name: manifest.Name, Version: manifest.Version, ManifestPath: result.ManifestPath}
	if len(manifest.DeclarationAdapters) == 0 {
		result.Diagnostics = []diagnostic.Diagnostic{{
			Code: diagnostic.ProjectIntegration, Severity: diagnostic.Error,
			Message: fmt.Sprintf("package %s declares no declaration adapters", manifest.Name), Path: result.ManifestPath,
		}}
		return result
	}

	modes := make([]string, 0, len(manifest.DeclarationAdapters))
	for mode := range manifest.DeclarationAdapters {
		modes = append(modes, mode)
	}
	sort.Strings(modes)
	result.Adapters = make([]declarationadaptertooling.Adapter, 0, len(modes))
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
			result.Diagnostics = append(result.Diagnostics, adapterCheckDiagnostic(adapterPath, readErr))
			result.Adapters = append(result.Adapters, checked)
			continue
		}

		dependencies := manifest.NativeDependenciesFor(mode)
		catalog := nativepackage.Empty(dependencies)
		applyErr := nativepackage.ApplyDeclarationAdapterFiles(catalog, []declarationadapterhost.Source{{
			Package: manifest.Name, Mode: mode, Path: adapterPath, Dependencies: dependencies,
		}})
		if applyErr != nil {
			result.Diagnostics = append(result.Diagnostics, adapterCheckDiagnostic(adapterPath, applyErr))
		} else {
			checked.Valid = true
		}
		result.Adapters = append(result.Adapters, checked)
	}
	return result
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
