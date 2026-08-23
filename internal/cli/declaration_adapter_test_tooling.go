package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/declarationadaptertooling"
	"github.com/type-rb/type-rb/internal/diagnostic"
	packageManager "github.com/type-rb/type-rb/internal/packages"
	"github.com/type-rb/type-rb/internal/project"
)

func (c *CLI) runAdapterTest(args []string) error {
	flags := flag.NewFlagSet("adapter test", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	format := flags.String("format", "human", "output format: human or json")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *format != "human" && *format != "json" {
		return fmt.Errorf("--format must be human or json; got %q", *format)
	}
	if flags.NArg() > 1 {
		return errors.New("adapter test accepts at most one package root")
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
	if checked.Manifest == nil {
		return c.finishAdapterTest(*format, checked.Package, nil, checked.Diagnostics)
	}

	issues := append([]diagnostic.Diagnostic(nil), checked.Diagnostics...)
	manifest := checked.Manifest
	if len(manifest.AdapterTests) == 0 {
		issues = append(issues, adapterTestDiagnostic(checked.ManifestPath, fmt.Errorf("package %s declares no adapter tests", manifest.Name)))
		return c.finishAdapterTest(*format, checked.Package, nil, issues)
	}
	adapterModes := make([]string, 0, len(manifest.DeclarationAdapters))
	for mode := range manifest.DeclarationAdapters {
		adapterModes = append(adapterModes, mode)
	}
	sort.Strings(adapterModes)
	for _, mode := range adapterModes {
		if _, exists := manifest.AdapterTests[mode]; !exists {
			issues = append(issues, adapterTestDiagnostic(checked.ManifestPath, fmt.Errorf("declaration adapter mode %s declares no adapter test", mode)))
		}
	}

	validAdapters := map[string]bool{}
	for _, adapter := range checked.Adapters {
		validAdapters[adapter.Mode] = adapter.Valid
	}
	modes := make([]string, 0, len(manifest.AdapterTests))
	for mode := range manifest.AdapterTests {
		modes = append(modes, mode)
	}
	sort.Strings(modes)
	tests := make([]declarationadaptertooling.Test, 0, len(modes))
	for _, mode := range modes {
		definition := manifest.AdapterTests[mode]
		configPath := filepath.Clean(filepath.Join(root, definition.Config))
		result := declarationadaptertooling.Test{
			Mode: mode, ConfigPath: configPath, Command: append([]string(nil), definition.Command...),
			Phases: []declarationadaptertooling.TestPhase{
				{Name: declarationadaptertooling.TestPhaseAdapterCheck, Status: declarationadaptertooling.TestStatusNotRun},
				{Name: declarationadaptertooling.TestPhaseBuild, Status: declarationadaptertooling.TestStatusNotRun},
				{Name: declarationadaptertooling.TestPhaseNativeCheck, Status: declarationadaptertooling.TestStatusNotRun},
			},
		}
		if !validAdapters[mode] {
			result.Phases[0].Status = declarationadaptertooling.TestStatusFailed
			tests = append(tests, result)
			continue
		}
		result.Phases[0].Status = declarationadaptertooling.TestStatusPassed

		config, loadErr := project.Load(configPath)
		if loadErr != nil {
			result.Phases[1].Status = declarationadaptertooling.TestStatusFailed
			issues = append(issues, adapterTestDiagnostic(configPath, fmt.Errorf("adapter test %s project: %w", mode, loadErr)))
			tests = append(tests, result)
			continue
		}
		if config.Mode != mode {
			result.Phases[1].Status = declarationadaptertooling.TestStatusFailed
			issues = append(issues, adapterTestDiagnostic(configPath, fmt.Errorf("adapter test %s project selects mode %s", mode, config.Mode)))
			tests = append(tests, result)
			continue
		}
		resolved, loadErr := packageManager.LoadTypeRBPackages(config)
		if loadErr != nil {
			result.Phases[1].Status = declarationadaptertooling.TestStatusFailed
			issues = append(issues, adapterTestDiagnostic(configPath, fmt.Errorf("adapter test %s project is not installed: %w", mode, loadErr)))
			tests = append(tests, result)
			continue
		}
		if !adapterTestUsesPackage(resolved, manifest.Name, root) {
			result.Phases[1].Status = declarationadaptertooling.TestStatusFailed
			issues = append(issues, adapterTestDiagnostic(configPath, fmt.Errorf("adapter test %s project must install package %s from this package root", mode, manifest.Name)))
			tests = append(tests, result)
			continue
		}

		var buildStdout, buildStderr bytes.Buffer
		buildCLI := &CLI{Stdin: c.Stdin, Stdout: &buildStdout, Stderr: &buildStderr, terminal: c.terminal}
		if buildErr := buildCLI.runBuild([]string{"--config", configPath}); buildErr != nil {
			result.Phases[1].Status = declarationadaptertooling.TestStatusFailed
			writeAdapterTestFailureOutput(c, &buildStdout, &buildStderr)
			issues = append(issues, adapterTestDiagnostic(configPath, fmt.Errorf("adapter test %s build failed: %w", mode, buildErr)))
			tests = append(tests, result)
			continue
		}
		result.Phases[1].Status = declarationadaptertooling.TestStatusPassed

		commandArguments := append([]string(nil), definition.Command...)
		executable := commandArguments[0]
		if strings.Contains(executable, "/") || strings.Contains(executable, `\`) {
			executable = filepath.Join(config.Root, filepath.FromSlash(executable))
		}
		command := exec.Command(executable, commandArguments[1:]...)
		command.Dir = config.Root
		command.Stdin = c.Stdin
		var nativeStdout, nativeStderr bytes.Buffer
		command.Stdout = &nativeStdout
		command.Stderr = &nativeStderr
		if commandErr := command.Run(); commandErr != nil {
			result.Phases[2].Status = declarationadaptertooling.TestStatusFailed
			writeAdapterTestFailureOutput(c, &nativeStdout, &nativeStderr)
			issues = append(issues, adapterTestDiagnostic(configPath, fmt.Errorf("adapter test %s native check failed: %w", mode, commandErr)))
			tests = append(tests, result)
			continue
		}
		result.Phases[2].Status = declarationadaptertooling.TestStatusPassed
		result.Passed = true
		tests = append(tests, result)
	}
	return c.finishAdapterTest(*format, checked.Package, tests, issues)
}

func adapterTestUsesPackage(resolved *packageManager.TypeRBPackages, name, root string) bool {
	if resolved == nil {
		return false
	}
	want := canonicalAdapterTestPath(root)
	for _, installed := range resolved.Packages {
		if installed.Name == name && canonicalAdapterTestPath(installed.Root) == want {
			return true
		}
	}
	return false
}

func canonicalAdapterTestPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
		absolute = resolved
	}
	return filepath.Clean(absolute)
}

func adapterTestDiagnostic(path string, err error) diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Code: diagnostic.ProjectIntegration, Severity: diagnostic.Error, Message: err.Error(), Path: path,
	}
}

func writeAdapterTestFailureOutput(c *CLI, stdout, stderr *bytes.Buffer) {
	if stdout.Len() > 0 {
		_, _ = c.Stderr.Write(stdout.Bytes())
	}
	if stderr.Len() > 0 {
		_, _ = c.Stderr.Write(stderr.Bytes())
	}
}

func (c *CLI) finishAdapterTest(format string, packageInfo *declarationadaptertooling.Package, tests []declarationadaptertooling.Test, issues []diagnostic.Diagnostic) error {
	report := declarationadaptertooling.BuildTest(declarationadaptertooling.TestBuildOptions{
		CompilerVersion: Version, Package: packageInfo, Tests: tests, Diagnostics: issues,
	})
	if format == "json" {
		encoder := json.NewEncoder(c.Stdout)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return err
		}
		if len(issues) != 0 {
			return &reportedError{cause: errors.New("declaration adapter test failed")}
		}
		return nil
	}
	if len(issues) != 0 {
		c.writeHumanDiagnostics(issues)
		return &reportedError{cause: errors.New("declaration adapter test failed")}
	}
	_, err := fmt.Fprintf(c.Stdout, "tested %d declaration adapter(s) for package %s\n", len(tests), packageInfo.Name)
	return err
}
