package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/codegen"
	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/compilerservice"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/formatter"
	"github.com/type-rb/type-rb/internal/ir"
	jobssql "github.com/type-rb/type-rb/internal/jobs/sqladapter"
	"github.com/type-rb/type-rb/internal/languageservice"
	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/nativepackage"
	"github.com/type-rb/type-rb/internal/official"
	packageManager "github.com/type-rb/type-rb/internal/packages"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/playground"
	"github.com/type-rb/type-rb/internal/project"
	"github.com/type-rb/type-rb/internal/repl"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/sourcemap"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/testsuite"
	"github.com/type-rb/type-rb/internal/token"
)

// Version is a variable so release builds can inject the tag with Go's -X
// linker flag while local source builds retain a useful development version.
var Version = "0.3.14-dev"

type buildArtifactKind string

const (
	buildArtifactSource     buildArtifactKind = "source"
	buildArtifactExecutable buildArtifactKind = "executable"
)

type CLI struct {
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	terminal func(io.Reader, io.Writer) bool
}

type reportedError struct {
	cause error
}

func (e *reportedError) Error() string { return e.cause.Error() }
func (e *reportedError) Unwrap() error { return e.cause }

func New() *CLI { return &CLI{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr} }

func (c *CLI) Run(args []string) int {
	if len(args) == 0 {
		if !c.shouldStartREPL() {
			c.usage()
			return 2
		}
		args = []string{"repl"}
	}
	var err error
	switch args[0] {
	case "fmt":
		err = c.runFmt(args[1:])
	case "check":
		err = c.runCheck(args[1:])
	case "test":
		err = c.runTest(args[1:])
	case "build":
		err = c.runBuild(args[1:])
	case "run":
		err = c.runProgram(args[1:])
	case "clean":
		err = c.runClean(args[1:])
	case "repl":
		err = c.runRepl(args[1:])
	case "lsp":
		err = c.runLSP(args[1:])
	case "play":
		err = c.runPlay(args[1:])
	case "tour":
		err = c.runTour(args[1:])
	case "db":
		err = c.runDatabase(args[1:])
	case "jobs":
		err = c.runJobs(args[1:])
	case "init":
		err = c.runInit(args[1:])
	case "sync":
		err = c.runSync(args[1:])
	case "add":
		err = c.runAdd(args[1:])
	case "remove":
		err = c.runRemove(args[1:])
	case "update":
		err = c.runUpdate(args[1:])
	case "install":
		err = c.runInstall(args[1:])
	case "version", "--version", "-v":
		_, err = fmt.Fprintln(c.Stdout, Version)
	case "help", "--help", "-h":
		c.usage()
		return 0
	default:
		if filepath.Ext(args[0]) == ".trb" {
			err = c.runProgram(args)
		} else {
			err = fmt.Errorf("unknown command %q", args[0])
		}
	}
	if err != nil {
		var reported *reportedError
		if errors.As(err, &reported) {
			return 1
		}
		var compilation *compiler.CompileError
		if errors.As(err, &compilation) {
			c.writeHumanDiagnostics(compilation.Diagnostics)
		} else {
			fmt.Fprintln(c.Stderr, "trb:", err)
		}
		return 1
	}
	return 0
}

func (c *CLI) runCheck(args []string) error {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", "", "path to trbconfig.jsonc")
	format := flags.String("diagnostic-format", "human", "diagnostic output: human or json")
	mode := flags.String("mode", "", "standalone mode: ruby, go, or typescript")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return errors.New("check accepts at most one standalone .trb file")
	}
	if *format != "human" && *format != "json" {
		return fmt.Errorf("--diagnostic-format must be human or json; got %q", *format)
	}

	filename := ""
	configStart := "."
	if flags.NArg() == 1 {
		if filepath.Ext(flags.Arg(0)) != ".trb" {
			return errors.New("standalone check source must be a .trb file")
		}
		var err error
		filename, err = filepath.Abs(flags.Arg(0))
		if err != nil {
			return err
		}
		configStart = filename
	}
	config, standalone, err := loadCommandConfig("check", *configPath, configStart, filename, *mode, "")
	if err != nil {
		return c.reportCheckError(*format, diagnostic.ProjectError, err)
	}
	var (
		units   []compiler.SourceUnit
		options compiler.Options
		count   int
	)
	if standalone {
		graph, graphErr := loadFileRootSourceGraph(filename, os.ReadFile)
		if graphErr != nil {
			return c.reportCheckError(*format, diagnostic.ProjectError, graphErr)
		}
		count = len(graph.Sources)
		units, options, err = projectCompilationSources(config, graph.Sources)
	} else {
		files, collectErr := collectTRB([]string{config.SourcePath()}, config.OutputPath())
		if collectErr != nil {
			return c.reportCheckError(*format, diagnostic.ProjectError, collectErr)
		}
		if len(files) == 0 {
			return c.reportCheckError(*format, diagnostic.ProjectError, errors.New("no .trb files found"))
		}
		count = len(files)
		units, options, err = projectCompilation(config, files)
	}
	if err != nil {
		return c.reportCheckError(*format, diagnostic.ProjectError, err)
	}
	snapshot := compilerservice.New(units, options).Analyze()
	if snapshot.HasErrors() {
		return c.reportCheckDiagnostics(*format, snapshot.Diagnostics)
	}
	if *format == "json" {
		return c.writeJSONDiagnostics(nil)
	}
	_, err = fmt.Fprintf(c.Stdout, "checked %d file(s) for mode %s\n", count, config.Mode)
	return err
}

func (c *CLI) runTest(args []string) (resultErr error) {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", "", "path to trbconfig.jsonc")
	filter := flags.String("filter", "", "run tests whose full name contains this text")
	testFile := flags.String("file", "", "run tests declared in this project file")
	reporter := flags.String("reporter", "human", "test output: human or json")
	compile := flags.Bool("compile", false, "produce a test executable with the target toolchain")
	debug := flags.Bool("debug", false, "include source-level debugger information in a test executable")
	outfile := flags.String("outfile", "", "test executable output path relative to the project root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("test does not accept source paths; it discovers *_test.trb files in the configured project")
	}
	if *reporter != "human" && *reporter != "json" {
		return fmt.Errorf("--reporter must be human or json; got %q", *reporter)
	}
	if *debug && !*compile {
		return errors.New("--debug requires --compile")
	}
	if *outfile != "" && !*compile {
		return errors.New("--outfile requires --compile")
	}
	if *compile && (*filter != "" || *testFile != "" || *reporter != "human") {
		return errors.New("--filter, --file, and --reporter select a test execution and cannot be combined with --compile")
	}
	config, err := loadConfig(*configPath, ".")
	if err != nil {
		return err
	}
	if *compile && config.Mode != "go" {
		return fmt.Errorf("test --compile is supported only for mode go; project mode is %s", config.Mode)
	}
	selectedFile := ""
	if *testFile != "" {
		selectedFile = *testFile
		if !filepath.IsAbs(selectedFile) {
			selectedFile = filepath.Join(config.Root, selectedFile)
		}
		selectedFile, err = filepath.Abs(selectedFile)
		if err != nil {
			return err
		}
	}
	if config.Mode == "typescript" && config.TypeScript != nil && config.TypeScript.Runtime == project.TypeScriptRuntimeBrowser {
		return errors.New("trb test requires typescript.runtime bun or node; browser test execution is not available yet")
	}
	files, err := collectTRB([]string{config.SourcePath()}, config.OutputPath())
	if err != nil {
		return err
	}
	var testFiles []string
	for _, filename := range files {
		if testsuite.IsTestFile(filename) {
			testFiles = append(testFiles, filename)
		}
	}
	if len(testFiles) == 0 {
		return errors.New("no *_test.trb files found")
	}
	if selectedFile != "" {
		found := false
		for _, filename := range testFiles {
			absolute, absoluteErr := filepath.Abs(filename)
			if absoluteErr == nil && filepath.Clean(absolute) == filepath.Clean(selectedFile) {
				selectedFile = absolute
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("test file %s was not found in the configured project", *testFile)
		}
	}
	if config.ManagesPackages() {
		if _, err := syncProjectPackages(config, files); err != nil {
			return err
		}
	}
	units, options, err := projectCompilation(config, files)
	if err != nil {
		return err
	}
	var registrations []compiler.SourceUnit
	for index := range units {
		if units[index].ExternalPackage || units[index].CompilerOwned || units[index].Official {
			continue
		}
		if units[index].TestRegistration != "" {
			registrations = append(registrations, units[index])
		}
		sum := sha256.Sum256([]byte(units[index].ModulePath))
		units[index].MainReplacement = fmt.Sprintf("trb_test_application_main_%x", sum[:6])
	}
	sort.Slice(registrations, func(i, j int) bool { return registrations[i].ModulePath < registrations[j].ModulePath })
	runnerFilename := filepath.Join(config.SourcePath(), "__trb_test_main.trb")
	runnerModule := "trb_test_main"
	runnerPackage := ""
	if config.Go != nil {
		runnerPackage = config.Go.RootPackage
	}
	var runner strings.Builder
	runner.WriteString("import { finish } from trb/std/test\n")
	for _, unit := range registrations {
		fmt.Fprintf(&runner, "import { %s } from %s\n", unit.TestRegistration, unit.ModulePath)
	}
	runner.WriteString("\ndef main()\n")
	for _, unit := range registrations {
		fmt.Fprintf(&runner, "\t%s()\n", unit.TestRegistration)
	}
	runner.WriteString("\tfinish()\n\treturn\nend\n")
	units = append(units, compiler.SourceUnit{Filename: runnerFilename, Source: []byte(runner.String()), ModulePath: runnerModule, Package: runnerPackage, CompilerOwned: true})
	artifacts, err := compiler.CompileProject(units, options)
	if err != nil {
		return err
	}
	compiled := make(map[string]*compiler.Artifact, len(artifacts))
	for _, artifact := range artifacts {
		absolute, _ := filepath.Abs(artifact.Filename)
		outputKey := absolute
		if config.Mode == "go" && testsuite.IsTestFile(absolute) {
			stem := strings.TrimSuffix(filepath.Base(absolute), filepath.Ext(absolute))
			outputKey = filepath.Join(filepath.Dir(absolute), stem+"_trb.trb")
		}
		compiled[outputKey] = artifact
	}
	if *compile {
		return c.buildGoTestExecutable(config, compiled, runnerFilename, *outfile, *debug)
	}
	relay := newCommandSignalRelay()
	defer relay.Close()
	workspace, err := createGeneratedWorkspace(config.Root, "test")
	if err != nil {
		return err
	}
	defer c.closeGeneratedWorkspace(workspace, &resultErr)
	runRoot := workspace.Path()
	generated, err := writeCompiledTree(config, compiled, runRoot, false)
	if err != nil {
		return err
	}
	target := generated[runnerFilename]
	if target == "" {
		return errors.New("compiler did not produce the test runner")
	}
	if config.Mode == "go" {
		if err := copyGoModuleFiles(config, runRoot); err != nil {
			return err
		}
		if config.Go.Sqldef != nil {
			if err := c.applySqldef(config); err != nil {
				return err
			}
		}
	}
	var command *exec.Cmd
	switch config.Mode {
	case "ruby":
		command = rubyRunCommand(target, nil)
	case "go":
		command = exec.Command("go", "run", "-mod=mod", ".")
	case "typescript":
		command, err = typeScriptRunCommand(config.TypeScript.Runtime, target, nil)
		if err != nil {
			return err
		}
	}
	command.Dir = runRoot
	if config.Mode == "go" {
		command.Dir = filepath.Dir(target)
	}
	if config.Mode == "ruby" {
		command.Dir = config.Root
	}
	command.Stdin = c.Stdin
	command.Stdout = c.Stdout
	command.Stderr = c.Stderr
	command.Env = append(os.Environ(), "TRB_TEST_REPORTER="+*reporter, "TRB_TEST_FILTER="+*filter, "TRB_TEST_FILE="+selectedFile)
	if config.Mode == "go" && config.Go.Sqldef != nil {
		command.Env = append(command.Env, "TRB_DATABASE="+filepath.Join(config.Root, config.Go.Sqldef.Database))
	}
	if err := relay.Run(command); err != nil {
		return &reportedError{cause: err}
	}
	return nil
}

func (c *CLI) buildGoTestExecutable(config *project.Config, compiled map[string]*compiler.Artifact, runnerFilename, outfile string, debug bool) error {
	if outfile == "" {
		name := strings.TrimSpace(config.Name)
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\\`) {
			return fmt.Errorf("project name %q cannot be used as a test executable filename; pass --outfile", config.Name)
		}
		outfile = filepath.Join("bin", name+"-test")
	}
	output, err := executableOutputPath(config, outfile)
	if err != nil {
		return err
	}
	buildRoot, err := os.MkdirTemp("", "trb-test-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(buildRoot)
	generated, err := writeCompiledTree(config, compiled, buildRoot, debug)
	if err != nil {
		return err
	}
	if err := copyGoModuleFiles(config, buildRoot); err != nil {
		return err
	}
	target := generated[runnerFilename]
	if target == "" {
		return errors.New("compiler did not produce the test runner")
	}
	if info, statErr := os.Stat(output); statErr == nil && info.IsDir() {
		return fmt.Errorf("--outfile must name a file; %s is a directory", output)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	arguments := []string{"build", "-mod=mod"}
	if debug {
		arguments = append(arguments, "-gcflags=all=-N -l")
	}
	arguments = append(arguments, "-o", output, ".")
	command := exec.Command("go", arguments...)
	command.Dir = filepath.Dir(target)
	command.Stdout = c.Stdout
	command.Stderr = c.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("go build test: %w", err)
	}
	fmt.Fprintf(c.Stdout, "test executable -> %s\n", output)
	return nil
}

func (c *CLI) reportCheckError(format string, fallback diagnostic.Code, err error) error {
	var items []diagnostic.Diagnostic
	var compilation *compiler.CompileError
	if errors.As(err, &compilation) {
		items = diagnostic.Normalize(append([]diagnostic.Diagnostic(nil), compilation.Diagnostics...), compilation.Filename, fallback)
	} else {
		items = []diagnostic.Diagnostic{{Code: fallback, Severity: diagnostic.Error, Message: err.Error()}}
	}
	return c.reportCheckDiagnostics(format, items)
}

func (c *CLI) reportCheckDiagnostics(format string, items []diagnostic.Diagnostic) error {
	if format == "json" {
		if writeErr := c.writeJSONDiagnostics(items); writeErr != nil {
			return writeErr
		}
	} else {
		c.writeHumanDiagnostics(items)
	}
	return &reportedError{cause: errors.New("project check failed")}
}

func (c *CLI) writeJSONDiagnostics(items []diagnostic.Diagnostic) error {
	encoder := json.NewEncoder(c.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(diagnostic.NewJSONReport(items))
}

func (c *CLI) writeHumanDiagnostics(items []diagnostic.Diagnostic) {
	for _, item := range items {
		path := item.Path
		if path == "" {
			path = "<project>"
		}
		if item.Span.Start.Line > 0 {
			fmt.Fprintf(c.Stderr, "%s:%d:%d: %s[%s]: %s\n", path, item.Span.Start.Line, item.Span.Start.Column, item.Severity, item.Code, item.Message)
		} else {
			fmt.Fprintf(c.Stderr, "%s: %s[%s]: %s\n", path, item.Severity, item.Code, item.Message)
		}
		for _, related := range item.Related {
			relatedPath := related.Location.Path
			if relatedPath == "" {
				relatedPath = path
			}
			fmt.Fprintf(c.Stderr, "  %s:%d:%d: note: %s\n", relatedPath, related.Location.Span.Start.Line, related.Location.Span.Start.Column, related.Message)
		}
		for _, fix := range item.Fixes {
			fmt.Fprintf(c.Stderr, "  help: %s\n", fix.Message)
		}
	}
}

func (c *CLI) runFmt(args []string) error {
	flags := flag.NewFlagSet("fmt", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	check := flags.Bool("check", false, "check formatting without writing files")
	write := flags.Bool("w", true, "write result to files")
	if err := flags.Parse(args); err != nil {
		return err
	}
	paths := flags.Args()
	if len(paths) == 0 {
		if config, err := project.Find("."); err == nil {
			paths = []string{config.SourcePath()}
		} else {
			paths = []string{"."}
		}
	}
	if len(paths) == 1 && paths[0] == "-" {
		source, err := io.ReadAll(c.Stdin)
		if err != nil {
			return err
		}
		formatted, diagnostics := formatter.Format(source)
		if len(diagnostics) > 0 {
			return fmt.Errorf("stdin:%s", diagnostics[0])
		}
		_, err = c.Stdout.Write(formatted)
		return err
	}
	files, err := collectTRB(paths, "")
	if err != nil {
		return err
	}
	importContexts := newImportFormatContexts()
	var changed []string
	for _, name := range files {
		source, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		formatted, diagnostics := formatter.FormatWithOptions(source, formatter.Options{
			CanonicalImportPath: importContexts.canonicalizer(name),
		})
		if len(diagnostics) > 0 {
			return fmt.Errorf("%s:%s", name, diagnostics[0])
		}
		if bytes.Equal(source, formatted) {
			continue
		}
		changed = append(changed, name)
		if !*check && *write {
			info, err := os.Stat(name)
			if err != nil {
				return err
			}
			if err := os.WriteFile(name, formatted, info.Mode().Perm()); err != nil {
				return err
			}
		}
	}
	if *check && len(changed) > 0 {
		for _, name := range changed {
			fmt.Fprintln(c.Stderr, name)
		}
		return fmt.Errorf("%d file(s) are not formatted", len(changed))
	}
	for _, name := range changed {
		fmt.Fprintln(c.Stdout, name)
	}
	return nil
}

func (c *CLI) runBuild(args []string) error {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", "", "path to trbconfig.jsonc")
	outDirFlag := flags.String("out-dir", "", "override config outDir")
	flags.StringVar(outDirFlag, "o", "", "override config outDir")
	stdout := flags.Bool("stdout", false, "write one compiled file to stdout")
	copyFlag := flags.String("copy", "", "override config copyFiles (true or false)")
	compile := flags.Bool("compile", false, "produce an executable with the target toolchain")
	debug := flags.Bool("debug", false, "include source-level debugger information in an executable")
	outfile := flags.String("outfile", "", "executable output path relative to the project or entry directory")
	mode := flags.String("mode", "", "standalone executable mode (only go is supported)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	paths := flags.Args()
	kind := buildArtifactSource
	if *compile {
		kind = buildArtifactExecutable
	}
	var config *project.Config
	var sourceGraph *fileRootSourceGraph
	var err error
	if kind == buildArtifactExecutable && len(paths) == 1 && filepath.Ext(paths[0]) == ".trb" {
		filename, absoluteErr := filepath.Abs(paths[0])
		if absoluteErr != nil {
			return absoluteErr
		}
		var standalone bool
		config, standalone, err = loadCommandConfig("build --compile", *configPath, filename, filename, *mode, "")
		if err == nil && standalone {
			sourceGraph, err = loadFileRootSourceGraph(filename, os.ReadFile)
		}
	} else {
		if *mode != "" {
			return errors.New("--mode requires --compile FILE.trb")
		}
		config, err = loadConfig(*configPath, firstOr(paths, "."))
	}
	if err != nil {
		return err
	}
	if kind == buildArtifactExecutable {
		if config.Mode != "go" {
			return fmt.Errorf("--compile is supported only for mode go; selected mode is %s", config.Mode)
		}
		if sourceGraph != nil {
			if *stdout || *copyFlag != "" || *outDirFlag != "" {
				return errors.New("--compile cannot be combined with --stdout, --copy, or --out-dir")
			}
			return c.buildGoFileRootExecutable(config, sourceGraph, *outfile, *debug)
		}
		if len(paths) != 0 {
			return errors.New("--compile builds the configured project and does not accept source paths")
		}
		if *stdout || *copyFlag != "" || *outDirFlag != "" {
			return errors.New("--compile cannot be combined with --stdout, --copy, or --out-dir")
		}
		return c.buildGoExecutable(config, *outfile, *debug)
	}
	if *debug {
		return errors.New("--debug requires --compile")
	}
	if *outfile != "" {
		return errors.New("--outfile requires --compile")
	}
	sourceRoot := config.SourcePath()
	fullProjectBuild := len(paths) == 0
	if len(paths) == 0 {
		paths = []string{sourceRoot}
	}
	outDir := config.OutputPath()
	if *outDirFlag != "" {
		if filepath.IsAbs(*outDirFlag) {
			outDir = *outDirFlag
		} else {
			outDir = filepath.Join(config.Root, *outDirFlag)
		}
	}
	files, err := collectTRB(paths, outDir)
	if err != nil {
		return err
	}
	files = productionTRBFiles(files)
	if len(files) == 0 {
		return errors.New("no .trb files found")
	}
	if *stdout && len(files) != 1 {
		return errors.New("--stdout requires exactly one input file")
	}
	projectFiles, err := collectTRB([]string{sourceRoot}, config.OutputPath())
	if err != nil {
		return err
	}
	projectFiles = productionTRBFiles(projectFiles)
	compiled, err := compileProject(config, projectFiles)
	if err != nil {
		return err
	}
	type built struct {
		input  string
		output string
		data   []byte
	}
	artifacts := make([]built, 0, len(files))
	selected := map[string]bool{}
	for _, name := range files {
		absolute, _ := filepath.Abs(name)
		selected[absolute] = true
		rel, err := filepath.Rel(sourceRoot, name)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%s is outside configured sourceDir %s", name, config.SourceDir)
		}
		artifact := compiled[name]
		if artifact == nil {
			return fmt.Errorf("compiler did not produce an artifact for %s", name)
		}
		rel = generatedSourceRelativeForArtifact(config, rel, artifact)
		artifacts = append(artifacts, built{input: name, output: filepath.Join(outDir, rel), data: artifact.Output})
	}
	if *stdout {
		_, err := c.Stdout.Write(artifacts[0].data)
		return err
	}
	// Local portable packages are compiled into each target tree so Go and
	// TypeScript consume the exact same record declarations without generated
	// files being checked into either application.
	compiledNames := make([]string, 0, len(compiled))
	for name := range compiled {
		compiledNames = append(compiledNames, name)
	}
	sort.Strings(compiledNames)
	for _, name := range compiledNames {
		if selected[name] {
			continue
		}
		artifact := compiled[name]
		relative, local := generatedRelative(config, name, artifact)
		if !local {
			continue
		}
		artifacts = append(artifacts, built{input: name, output: filepath.Join(outDir, relative), data: artifact.Output})
	}
	manifest := ""
	if config.ManagesPackages() {
		manifest, err = syncProjectPackages(config, projectFiles)
		if err != nil {
			return err
		}
	}
	copyFiles := config.ShouldCopyFiles()
	if *copyFlag != "" {
		copyFiles, err = strconv.ParseBool(*copyFlag)
		if err != nil {
			return fmt.Errorf("--copy: %w", err)
		}
	}
	if fullProjectBuild {
		if err := cleanBuildOutput(config, outDir); err != nil {
			return err
		}
	}
	if copyFiles {
		if err := copyProjectFiles(config.Root, outDir); err != nil {
			return err
		}
	}
	for _, artifact := range artifacts {
		if err := os.MkdirAll(filepath.Dir(artifact.output), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(artifact.output, artifact.data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(c.Stdout, "%s -> %s\n", artifact.input, artifact.output)
	}
	if manifest != "" {
		fmt.Fprintf(c.Stdout, "packages -> %s\n", manifest)
	}
	return nil
}

func cleanBuildOutput(config *project.Config, outDir string) error {
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return err
	}
	source, err := filepath.Abs(config.SourcePath())
	if err != nil {
		return err
	}
	output, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	if output == root || output == source {
		return nil
	}
	relative, err := filepath.Rel(root, output)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil
	}
	return os.RemoveAll(output)
}

func (c *CLI) buildGoExecutable(config *project.Config, outfile string, debug bool) error {
	files, err := collectTRB([]string{config.SourcePath()}, config.OutputPath())
	if err != nil {
		return err
	}
	files = productionTRBFiles(files)
	if len(files) == 0 {
		return errors.New("no .trb files found")
	}
	compiled, err := compileProject(config, files)
	if err != nil {
		return err
	}
	entrySource := mainSource(compiled)
	if entrySource == "" {
		return errors.New("project has no top-level main(); define def main() before using --compile")
	}
	if config.ManagesPackages() {
		if _, err := syncProjectPackages(config, files); err != nil {
			return err
		}
	}
	output, err := executableOutputPath(config, outfile)
	if err != nil {
		return err
	}
	buildRoot, err := os.MkdirTemp("", "trb-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(buildRoot)
	generated, err := writeCompiledTree(config, compiled, buildRoot, debug)
	if err != nil {
		return err
	}
	if err := copyGoModuleFiles(config, buildRoot); err != nil {
		return err
	}
	target := generated[entrySource]
	if target == "" {
		return errors.New("compiler did not produce the top-level main() artifact")
	}
	return c.buildGoTarget(target, output, debug)
}

func executableOutputPath(config *project.Config, outfile string) (string, error) {
	if outfile == "" {
		name := strings.TrimSpace(config.Name)
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\\`) {
			return "", fmt.Errorf("project name %q cannot be used as an executable filename; pass --outfile", config.Name)
		}
		outfile = filepath.Join("bin", name)
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(outfile), ".exe") {
		outfile += ".exe"
	}
	if !filepath.IsAbs(outfile) {
		outfile = filepath.Join(config.Root, outfile)
	}
	return filepath.Abs(outfile)
}

func (c *CLI) runProgram(args []string) (resultErr error) {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", "", "path to trbconfig.jsonc")
	keepGenerated := flags.Bool("keep-generated", false, "retain generated target source below .trb/generated")
	mode := flags.String("mode", "", "standalone mode: ruby, go, or typescript")
	typeScriptRuntime := flags.String("runtime", "", "standalone TypeScript runtime: node or bun")
	flagArgs := args
	var programArgs []string
	for index, argument := range args {
		if argument == "--" {
			flagArgs = args[:index]
			programArgs = append(programArgs, args[index+1:]...)
			break
		}
	}
	if err := flags.Parse(flagArgs); err != nil {
		return err
	}
	remaining := flags.Args()
	filename := ""
	configStart := "."
	if len(remaining) > 0 {
		if filepath.Ext(remaining[0]) != ".trb" {
			return fmt.Errorf("%s is not a .trb file; pass program arguments after --", remaining[0])
		}
		var err error
		filename, err = filepath.Abs(remaining[0])
		if err != nil {
			return err
		}
		configStart = filename
		programArgs = append(remaining[1:], programArgs...)
	}
	config, standalone, err := loadCommandConfig("run", *configPath, configStart, filename, *mode, *typeScriptRuntime)
	if err != nil {
		return err
	}
	files := []string{filename}
	var sourceGraph *fileRootSourceGraph
	if !standalone {
		files, err = collectTRB([]string{config.SourcePath()}, config.OutputPath())
		if err != nil {
			return err
		}
		files = productionTRBFiles(files)
	} else {
		sourceGraph, err = loadFileRootSourceGraph(filename, os.ReadFile)
		if err != nil {
			return err
		}
		files = make([]string, 0, len(sourceGraph.Sources))
		for _, source := range sourceGraph.Sources {
			files = append(files, source.Filename)
		}
	}
	if config.ManagesPackages() {
		if _, err := syncProjectPackages(config, files); err != nil {
			return err
		}
	}
	var compiled map[string]*compiler.Artifact
	if sourceGraph != nil {
		compiled, err = compileProjectSources(config, sourceGraph.Sources)
	} else {
		compiled, err = compileProject(config, files)
	}
	if err != nil {
		return err
	}
	relay := newCommandSignalRelay()
	defer relay.Close()
	var workspace *generatedWorkspace
	if standalone {
		workspace, err = createStandaloneGeneratedWorkspace(config.Root, "run")
	} else {
		workspace, err = createGeneratedWorkspace(config.Root, "run")
	}
	if err != nil {
		return err
	}
	if *keepGenerated {
		workspace.Keep()
	}
	defer c.closeGeneratedWorkspace(workspace, &resultErr)
	runRoot := workspace.Path()
	target := ""
	entrySource := filename
	if entrySource == "" {
		entrySource = mainSource(compiled)
		if entrySource == "" {
			return errors.New("project has no top-level main(); define def main() or pass a .trb file explicitly")
		}
	}
	if standalone && !artifactHasMain(compiled[entrySource]) {
		return errors.New("standalone file has no top-level main(); define def main()")
	}
	generated, err := writeCompiledTree(config, compiled, runRoot, false)
	if err != nil {
		return err
	}
	target = generated[entrySource]
	if target == "" {
		return fmt.Errorf("%s is outside configured sourceDir %s", entrySource, config.SourceDir)
	}
	if config.Mode == "go" {
		if standalone {
			err = writeStandaloneGoModule(config, runRoot)
		} else {
			err = copyGoModuleFiles(config, runRoot)
		}
		if err != nil {
			return err
		}
		if config.Go.Sqldef != nil {
			if err := c.applySqldef(config); err != nil {
				return err
			}
		}
	}
	var command *exec.Cmd
	switch config.Mode {
	case "ruby":
		command = rubyRunCommand(target, programArgs)
	case "go":
		command = exec.Command("go", append([]string{"run", "-mod=mod", "."}, programArgs...)...)
		if config.Go.Sqldef != nil {
			database := filepath.Join(config.Root, config.Go.Sqldef.Database)
			command.Env = append(os.Environ(), "TRB_DATABASE="+database)
		}
	case "typescript":
		command, err = typeScriptRunCommand(config.TypeScript.Runtime, target, programArgs)
		if err != nil {
			return err
		}
	}
	command.Dir = runRoot
	if config.Mode == "go" {
		command.Dir = filepath.Dir(target)
	}
	if config.Mode == "ruby" && !standalone {
		command.Dir = config.Root
	}
	command.Stdin = c.Stdin
	command.Stdout = c.Stdout
	command.Stderr = c.Stderr
	return relay.Run(command)
}

func loadCommandConfig(command, explicit, start, filename, mode, runtimeName string) (*project.Config, bool, error) {
	config, err := loadConfig(explicit, start)
	if err == nil {
		if mode != "" || runtimeName != "" {
			return nil, false, errors.New("--mode and --runtime are available only when trbconfig.jsonc is unavailable")
		}
		return config, false, nil
	}
	if !errors.Is(err, project.ErrConfigNotFound) {
		return nil, false, err
	}
	if explicit != "" {
		return nil, false, err
	}
	if filename == "" {
		return nil, false, fmt.Errorf("%s requires FILE.trb when trbconfig.jsonc is unavailable", command)
	}
	if mode == "" {
		mode = "go"
	}
	if mode != "ruby" && mode != "go" && mode != "typescript" {
		return nil, false, fmt.Errorf("standalone mode must be ruby, go, or typescript; got %q", mode)
	}
	if runtimeName != "" && mode != "typescript" {
		return nil, false, errors.New("--runtime requires --mode typescript")
	}
	if runtimeName != "" && runtimeName != project.TypeScriptRuntimeNode && runtimeName != project.TypeScriptRuntimeBun {
		return nil, false, fmt.Errorf("standalone TypeScript runtime must be node or bun; got %q", runtimeName)
	}
	config = project.New(filepath.Dir(filename), mode)
	config.Name = "standalone"
	config.PackageManagement = project.ExternalPackages
	if config.Go != nil {
		config.Go.Module = "trb.local/standalone"
	}
	if config.TypeScript != nil && runtimeName != "" {
		config.TypeScript.Runtime = runtimeName
		if runtimeName == project.TypeScriptRuntimeBun {
			config.TypeScript.PackageManager = "bun"
		}
	}
	if err := config.Validate(); err != nil {
		return nil, false, err
	}
	return config, true, nil
}

func writeStandaloneGoModule(config *project.Config, root string) error {
	version := project.DefaultGoVersion
	if config.Go != nil && config.Go.Version != "" {
		version = config.Go.Version
	}
	module := fmt.Sprintf("module trb.local/standalone\n\ngo %s\n", version)
	return os.WriteFile(filepath.Join(root, "go.mod"), []byte(module), 0o644)
}

func rubyRunCommand(target string, programArgs []string) *exec.Cmd {
	return exec.Command("ruby", append([]string{"-rbundler/setup", target}, programArgs...)...)
}

func typeScriptRunCommand(runtimeName, target string, programArgs []string) (*exec.Cmd, error) {
	switch runtimeName {
	case "", project.TypeScriptRuntimeNode:
		return exec.Command("node", append([]string{"--experimental-strip-types", target}, programArgs...)...), nil
	case project.TypeScriptRuntimeBun:
		return exec.Command("bun", append([]string{"run", target}, programArgs...)...), nil
	case project.TypeScriptRuntimeBrowser:
		return nil, errors.New("trb run is unavailable for typescript.runtime browser; use a browser application entrypoint")
	default:
		return nil, fmt.Errorf("unsupported TypeScript runtime %q", runtimeName)
	}
}

func (c *CLI) runRepl(args []string) error {
	flags := flag.NewFlagSet("repl", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", "", "path to trbconfig.jsonc")
	mode := flags.String("mode", "", "REPL mode: ruby, go, or typescript")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("repl does not accept source arguments; use :load FILE inside the REPL")
	}
	config, projectAware, err := loadReplConfig(*configPath, *mode)
	if err != nil {
		return err
	}
	var files []string
	if projectAware {
		files, err = collectTRB([]string{config.SourcePath()}, config.OutputPath())
		if err != nil {
			return err
		}
		files = productionTRBFiles(files)
	}
	sessionFilename := filepath.Join(config.SourcePath(), ".trb-repl.trb")
	sessionModule := "__trb_repl__"
	sessionPackage := ""
	if config.Go != nil {
		sessionPackage = config.Go.RootPackage
	}
	analyzer := compiler.NewAnalyzer()
	compileSource := func(source string) (*repl.Compilation, error) {
		units, options, err := projectCompilation(config, files)
		if err != nil {
			return nil, err
		}
		units = append(units, compiler.SourceUnit{
			Filename:       sessionFilename,
			Source:         []byte(source),
			ModulePath:     sessionModule,
			Package:        sessionPackage,
			PackageAliases: nil,
		})
		options.AllowUnusedImports = true
		options.InteractiveModule = sessionModule
		artifacts, err := analyzer.AnalyzeProject(units, options)
		if err != nil {
			return nil, err
		}
		compilation := &repl.Compilation{Artifacts: artifacts, Programs: make([]*ir.Program, 0, len(artifacts))}
		for _, artifact := range artifacts {
			compilation.Programs = append(compilation.Programs, artifact.IR)
			if artifact.IR.ModulePath == sessionModule {
				compilation.Session = artifact
			}
		}
		if compilation.Session == nil {
			return nil, errors.New("compiler did not return the REPL session")
		}
		return compilation, nil
	}
	initial, err := compileSource("")
	if err != nil {
		return err
	}
	availableImports := uniqueReplImports(initial.Artifacts, sessionModule, config.Mode)
	candidates, err := replStandardCandidates(config, availableImports, sessionPackage)
	if err != nil {
		return err
	}
	compile := func(source string) (*repl.Compilation, error) {
		hiddenPrelude := replPrelude(availableImports, source)
		hiddenPreludeLines := strings.Count(hiddenPrelude, "\n")
		compilation, err := compileSource(hiddenPrelude + source)
		if err != nil {
			return nil, hideReplPreludeDiagnostics(err, sessionFilename, hiddenPreludeLines, len(hiddenPrelude))
		}
		return compilation, nil
	}
	historyFile := ""
	if projectAware {
		historyFile = filepath.Join(config.Root, ".trb", "repl_history")
	} else if cacheRoot, cacheErr := os.UserCacheDir(); cacheErr == nil {
		historyFile = filepath.Join(cacheRoot, "trb", "repl_history_"+config.Mode)
	}
	return repl.Run(repl.Options{
		Mode:        config.Mode,
		ProjectName: config.Name,
		Version:     Version,
		Stdin:       c.Stdin,
		Stdout:      c.Stdout,
		Stderr:      c.Stderr,
		Interactive: interactiveTerminal(c.Stdin, c.Stdout),
		HistoryFile: historyFile,
		Compile:     compile,
		Initial:     initial,
		Candidates:  candidates,
	})
}

func analyzeReplProject(units []compiler.SourceUnit, options compiler.Options) ([]*compiler.Artifact, error) {
	return compiler.AnalyzeProject(units, options)
}

type replImport struct {
	path     string
	symbols  []string
	standard bool
}

func uniqueReplImports(artifacts []*compiler.Artifact, modulePath, mode string) []replImport {
	type origin struct {
		path     string
		standard bool
	}
	byName := map[string][]origin{}
	for _, artifact := range artifacts {
		if artifact == nil || artifact.AST == nil || artifact.AST.ModulePath == modulePath || artifact.CompilerOwned || artifact.Official {
			continue
		}
		for name := range resolver.CollectExports(artifact.AST.Statements) {
			if name != "main" {
				byName[name] = append(byName[name], origin{path: artifact.AST.ModulePath})
			}
		}
	}
	for _, definition := range stdlib.RuntimeExportPackages(mode) {
		for _, exported := range definition.RuntimeExports {
			byName[exported.Name] = append(byName[exported.Name], origin{path: definition.Path, standard: true})
		}
	}
	type importKey struct {
		path     string
		standard bool
	}
	byPath := map[importKey][]string{}
	for name, origins := range byName {
		if len(origins) == 1 {
			key := importKey{path: origins[0].path, standard: origins[0].standard}
			byPath[key] = append(byPath[key], name)
		}
	}
	keys := make([]importKey, 0, len(byPath))
	for key := range byPath {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left].path < keys[right].path })
	result := make([]replImport, 0, len(keys))
	for _, key := range keys {
		sort.Strings(byPath[key])
		result = append(result, replImport{path: key.path, symbols: byPath[key], standard: key.standard})
	}
	return result
}

func replPrelude(imports []replImport, sessionSource string) string {
	explicit := map[string]bool{}
	explicitPaths := map[string]bool{}
	referenced := map[string]bool{}
	tokens, _ := lexer.Lex([]byte(sessionSource))
	for _, item := range tokens {
		if item.Kind == token.Identifier {
			referenced[item.Lexeme] = true
		}
	}
	if program, diagnostics := parser.Parse([]byte(sessionSource)); !hasDiagnosticErrors(diagnostics) {
		for name := range resolver.CollectExports(program.Statements) {
			explicit[name] = true
		}
		for _, statement := range program.Statements {
			imported, ok := statement.(*ast.ImportStatement)
			if !ok {
				continue
			}
			if len(imported.Symbols) == 0 && imported.Alias == "" {
				explicitPaths[imported.Path] = true
			}
			for _, name := range imported.Symbols {
				explicit[name] = true
			}
		}
	}
	var source strings.Builder
	for _, imported := range imports {
		if explicitPaths[imported.path] {
			continue
		}
		symbols := make([]string, 0, len(imported.symbols))
		for _, name := range imported.symbols {
			if !explicit[name] && (!imported.standard || referenced[name]) {
				symbols = append(symbols, name)
			}
		}
		if len(symbols) == 0 {
			continue
		}
		fmt.Fprintf(&source, "import { %s } from %s\n", strings.Join(symbols, ", "), imported.path)
	}
	return source.String()
}

func replStandardCandidates(config *project.Config, imports []replImport, sessionPackage string) (languageservice.Context, error) {
	const modulePath = "__trb_repl_standard_candidates__"
	var source strings.Builder
	for _, imported := range imports {
		if imported.standard {
			fmt.Fprintf(&source, "import { %s } from %s\n", strings.Join(imported.symbols, ", "), imported.path)
		}
	}
	if source.Len() == 0 {
		return languageservice.Context{}, nil
	}
	options, err := compilerOptions(config)
	if err != nil {
		return languageservice.Context{}, err
	}
	options.AllowUnusedImports = true
	artifacts, err := analyzeReplProject([]compiler.SourceUnit{{
		Filename:   filepath.Join(config.SourcePath(), ".trb-repl-standard-candidates.trb"),
		Source:     []byte(source.String()),
		ModulePath: modulePath,
		Package:    sessionPackage,
	}}, options)
	if err != nil {
		return languageservice.Context{}, err
	}
	programs := make([]*ir.Program, 0, len(artifacts))
	for _, artifact := range artifacts {
		programs = append(programs, artifact.IR)
	}
	return languageservice.BuildContext(programs, modulePath), nil
}

func hasDiagnosticErrors(diagnostics []diagnostic.Diagnostic) bool {
	for _, item := range diagnostics {
		if item.Severity == diagnostic.Error {
			return true
		}
	}
	return false
}

func hideReplPreludeDiagnostics(err error, filename string, lines, bytes int) error {
	var compilation *compiler.CompileError
	if lines == 0 || !errors.As(err, &compilation) || compilation.Filename != filename {
		return err
	}
	adjusted := &compiler.CompileError{Filename: compilation.Filename, Diagnostics: append([]diagnostic.Diagnostic(nil), compilation.Diagnostics...)}
	for index := range adjusted.Diagnostics {
		adjustReplLocation := func(path string, span *token.Span) {
			if path != "" && path != filename || span.Start.Line <= lines {
				return
			}
			span.Start.Line -= lines
			span.End.Line -= lines
			span.Start.Offset -= bytes
			span.End.Offset -= bytes
		}
		adjustReplLocation(adjusted.Diagnostics[index].Path, &adjusted.Diagnostics[index].Span)
		for relatedIndex := range adjusted.Diagnostics[index].Related {
			related := &adjusted.Diagnostics[index].Related[relatedIndex].Location
			adjustReplLocation(related.Path, &related.Span)
		}
		for fixIndex := range adjusted.Diagnostics[index].Fixes {
			for editIndex := range adjusted.Diagnostics[index].Fixes[fixIndex].Edits {
				edit := &adjusted.Diagnostics[index].Fixes[fixIndex].Edits[editIndex].Location
				adjustReplLocation(edit.Path, &edit.Span)
			}
		}
	}
	return adjusted
}

func (c *CLI) runPlay(args []string) error {
	return c.runBrowserTool("play", args)
}

func (c *CLI) runTour(args []string) error {
	return c.runBrowserTool("tour", args)
}

func (c *CLI) runBrowserTool(page string, args []string) error {
	flags := flag.NewFlagSet(page, flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	mode := flags.String("mode", "", "initial mode: ruby, go, or typescript")
	port := flags.Int("port", 0, "local HTTP port; zero chooses an available port")
	noOpen := flags.Bool("no-open", false, "serve without opening a browser")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("%s does not accept source arguments", page)
	}
	initialMode, err := playgroundMode(*mode)
	if err != nil {
		return err
	}
	return playground.Serve(playground.Options{
		Mode: initialMode, Page: page, Port: *port, OpenBrowser: !*noOpen, Version: Version, Stdout: c.Stdout, Stderr: c.Stderr,
	})
}

func playgroundMode(requested string) (string, error) {
	if requested != "" {
		if requested != "ruby" && requested != "go" && requested != "typescript" {
			return "", fmt.Errorf("--mode must be ruby, go, or typescript; got %q", requested)
		}
		return requested, nil
	}
	config, err := project.Find(".")
	if err == nil {
		return config.Mode, nil
	}
	if !errors.Is(err, project.ErrConfigNotFound) {
		return "", err
	}
	return "go", nil
}

func loadReplConfig(explicit, requestedMode string) (*project.Config, bool, error) {
	if requestedMode != "" && requestedMode != "ruby" && requestedMode != "go" && requestedMode != "typescript" {
		return nil, false, fmt.Errorf("repl --mode must be ruby, go, or typescript; got %q", requestedMode)
	}
	config, err := loadConfig(explicit, ".")
	if err == nil {
		if requestedMode != "" && requestedMode != config.Mode {
			config = replConfigForMode(config, requestedMode)
			if err := config.Validate(); err != nil {
				return nil, false, err
			}
		}
		return config, true, nil
	}
	if explicit != "" || !errors.Is(err, project.ErrConfigNotFound) {
		return nil, false, err
	}
	if requestedMode == "" {
		requestedMode = "go"
	}
	root, absoluteErr := filepath.Abs(".")
	if absoluteErr != nil {
		return nil, false, absoluteErr
	}
	config = project.New(root, requestedMode)
	config.Name = "scratch"
	config.PackageManagement = project.ExternalPackages
	if config.Go != nil {
		config.Go.Module = "trb.local/repl"
	}
	if err := config.Validate(); err != nil {
		return nil, false, err
	}
	return config, false, nil
}

func replConfigForMode(base *project.Config, mode string) *project.Config {
	config := *base
	config.Mode = mode
	config.Ruby = nil
	config.Go = nil
	config.TypeScript = nil
	switch mode {
	case "ruby":
		config.Ruby = &project.RubyConfig{Source: "https://rubygems.org", Loader: "require_relative"}
		if base.Ruby != nil {
			clone := *base.Ruby
			config.Ruby = &clone
		}
	case "go":
		config.Go = &project.GoConfig{Module: "trb.local/repl", Version: project.DefaultGoVersion, RootPackage: "main", IndirectDependencies: map[string]string{}}
		if base.Go != nil {
			clone := *base.Go
			config.Go = &clone
			if config.Go.Module == "" {
				config.Go.Module = "trb.local/repl"
			}
		}
	case "typescript":
		config.TypeScript = &project.TypeScriptConfig{PackageManager: "npm", ModuleType: "module", Runtime: project.DefaultTypeScriptRuntime, Scripts: map[string]string{}}
		if base.TypeScript != nil {
			clone := *base.TypeScript
			config.TypeScript = &clone
		}
	}
	return &config
}

func (c *CLI) applySqldef(config *project.Config) error {
	definition := config.Go.Sqldef
	schema, err := os.Open(filepath.Join(config.Root, definition.Schema))
	if err != nil {
		return fmt.Errorf("sqldef schema: %w", err)
	}
	defer schema.Close()
	arguments := append([]string(nil), definition.Arguments...)
	arguments = append(arguments, filepath.Join(config.Root, definition.Database))
	command := exec.Command(definition.Command, arguments...)
	command.Dir = config.Root
	command.Stdin = schema
	command.Stdout = c.Stdout
	command.Stderr = c.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("sqldef: %w", err)
	}
	return nil
}

func (c *CLI) runInit(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	mode := flags.String("mode", "", "ruby, go, or typescript")
	module := flags.String("module", "", "Go module path")
	typeScriptRuntime := flags.String("runtime", "", "TypeScript runtime: browser, bun, or node")
	template := flags.String("template", "", "project template (web)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	root := "."
	if flags.NArg() > 1 {
		return errors.New("init accepts at most one directory")
	}
	if flags.NArg() == 1 {
		root = flags.Arg(0)
	}
	if *mode == "" {
		return errors.New("init requires --mode ruby, --mode go, or --mode typescript")
	}
	if *typeScriptRuntime != "" && *mode != "typescript" {
		return errors.New("init --runtime is supported only for mode typescript")
	}
	if *template != "" && *template != "web" {
		return fmt.Errorf("unknown project template %q; available template: web", *template)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	config := project.New(root, *mode)
	if *typeScriptRuntime != "" {
		config.TypeScript.Runtime = *typeScriptRuntime
		if *typeScriptRuntime == project.TypeScriptRuntimeBun {
			config.TypeScript.PackageManager = "bun"
		}
	}
	if *template == "web" {
		config.SourceDir = "src"
	}
	if *mode == "go" {
		config.Go.Module = *module
		if config.Go.Module == "" {
			config.Go.Module = config.Name
		}
	}
	if _, err := os.Stat(config.Path); err == nil {
		return fmt.Errorf("%s already exists", config.Path)
	} else if !os.IsNotExist(err) {
		return err
	}
	templateFiles := initTemplateFiles(config, *template)
	if err := checkInitTemplateTargets(templateFiles); err != nil {
		return err
	}
	if err := config.Save(); err != nil {
		return err
	}
	manifest, err := packageManager.Sync(config)
	if err != nil {
		return err
	}
	if err := writeInitTemplate(templateFiles); err != nil {
		return err
	}
	fmt.Fprintln(c.Stdout, config.Path)
	fmt.Fprintln(c.Stdout, manifest)
	for _, file := range templateFiles {
		fmt.Fprintln(c.Stdout, file.Path)
	}
	return nil
}

func (c *CLI) runSync(args []string) error {
	flags := flag.NewFlagSet("sync", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", "", "path to trbconfig.jsonc")
	if err := flags.Parse(args); err != nil {
		return err
	}
	config, err := loadConfig(*configPath, ".")
	if err != nil {
		return err
	}
	files, collectErr := collectDependencyTRB(config)
	if collectErr != nil {
		return collectErr
	}
	manifest, err := syncProjectPackages(config, files)
	if err == nil {
		fmt.Fprintln(c.Stdout, manifest)
	}
	return err
}

func (c *CLI) runAdd(args []string) error {
	flags := flag.NewFlagSet("add", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	dev := flags.Bool("dev", false, "add a native development dependency")
	native := flags.Bool("native", false, "add a target-language dependency")
	source := flags.String("source", "", "Git source for a TypeRB package")
	packagePath := flags.String("path", "", "local TypeRB package directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 1 || flags.NArg() > 2 {
		return errors.New("usage: trb add [--source GIT | --path DIRECTORY] PACKAGE [VERSION]")
	}
	config, err := project.Find(".")
	if err != nil {
		return err
	}
	if *native {
		if *source != "" || *packagePath != "" {
			return errors.New("--native cannot be combined with --source or --path")
		}
		return c.addNativeDependency(config, flags.Args(), *dev)
	}
	if *dev {
		return errors.New("--dev currently applies only to --native dependencies")
	}
	if *source != "" && *packagePath != "" {
		return errors.New("--source and --path are mutually exclusive")
	}
	name, version := dependencySpec(flags.Args())
	if *packagePath != "" {
		if version != "" {
			return errors.New("a local TypeRB package path cannot have a version")
		}
		config.Packages[name] = project.PackageRequirement{Path: *packagePath}
	} else {
		if version == "" {
			version = "latest"
		}
		config.Packages[name] = project.PackageRequirement{Source: *source, Version: version}
	}
	if err := config.Validate(); err != nil {
		return err
	}
	resolved, err := packageManager.ResolveTypeRBPackages(config, packageManager.TypeRBResolveOptions{Update: true})
	if err != nil {
		return err
	}
	if err := config.Save(); err != nil {
		return err
	}
	if config.ManagesPackages() {
		files, collectErr := collectDependencyTRB(config)
		if collectErr != nil {
			return collectErr
		}
		if _, err := syncProjectPackages(config, files); err != nil {
			return err
		}
	}
	canonical := resolved.Aliases[name]
	locked := resolved.Lock.Packages[canonical]
	fmt.Fprintf(c.Stdout, "%s %s -> %s\n", name, locked.Version, packageManager.TypeRBLockPath(config))
	return nil
}

func (c *CLI) addNativeDependency(config *project.Config, arguments []string, dev bool) error {
	if !config.ManagesPackages() {
		return errors.New("native package management is external; edit dependencies in the host project")
	}
	name, version := dependencySpec(arguments)
	if version == "" && config.Mode == "typescript" {
		version = "latest"
	}
	if version == "" && config.Mode == "go" {
		return errors.New("Go dependencies require an explicit version")
	}
	delete(config.Dependencies, name)
	delete(config.DevDependencies, name)
	if dev {
		config.DevDependencies[name] = version
	} else {
		config.Dependencies[name] = version
	}
	if err := config.Save(); err != nil {
		return err
	}
	files, collectErr := collectDependencyTRB(config)
	if collectErr != nil {
		return collectErr
	}
	manifest, err := syncProjectPackages(config, files)
	if err == nil {
		fmt.Fprintf(c.Stdout, "%s %s -> %s\n", name, version, manifest)
	}
	return err
}

func (c *CLI) runRemove(args []string) error {
	flags := flag.NewFlagSet("remove", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	native := flags.Bool("native", false, "remove a target-language dependency")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: trb remove [--native] PACKAGE")
	}
	config, err := project.Find(".")
	if err != nil {
		return err
	}
	name := flags.Arg(0)
	if *native {
		if !config.ManagesPackages() {
			return errors.New("native package management is external; edit dependencies in the host project")
		}
		delete(config.Dependencies, name)
		delete(config.DevDependencies, name)
	} else {
		delete(config.Packages, name)
		if _, err := packageManager.ResolveTypeRBPackages(config, packageManager.TypeRBResolveOptions{Update: true}); err != nil {
			return err
		}
	}
	if err := config.Save(); err != nil {
		return err
	}
	if config.ManagesPackages() {
		files, collectErr := collectDependencyTRB(config)
		if collectErr != nil {
			return collectErr
		}
		if _, err := syncProjectPackages(config, files); err != nil {
			return err
		}
	}
	fmt.Fprintf(c.Stdout, "%s -> %s\n", name, config.Path)
	return nil
}

func (c *CLI) runInstall(args []string) error {
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", "", "path to trbconfig.jsonc")
	frozen := flags.Bool("frozen", false, "require trb.lock to match the project configuration")
	offline := flags.Bool("offline", false, "use only the local TypeRB package cache")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("install does not accept package arguments")
	}
	config, err := loadConfig(*configPath, ".")
	if err != nil {
		return err
	}
	resolved, err := packageManager.ResolveTypeRBPackages(config, packageManager.TypeRBResolveOptions{Frozen: *frozen, Offline: *offline})
	if err != nil {
		return err
	}
	if resolved.Lock != nil {
		fmt.Fprintf(c.Stdout, "resolved %d TypeRB package(s) -> %s\n", len(resolved.Packages), packageManager.TypeRBLockPath(config))
	}
	if !config.ManagesPackages() {
		fmt.Fprintln(c.Stdout, "native package management is external")
		return c.indexNativeTypeScriptPackages(config, resolved)
	}
	files, err := collectDependencyTRB(config)
	if err != nil {
		return err
	}
	dependencies, err := projectPackageDependencies(config, files)
	if err != nil {
		return err
	}
	if err := packageManager.InstallWithDependencies(config, dependencies, c.Stdin, c.Stdout, c.Stderr); err != nil {
		return err
	}
	return c.indexNativeTypeScriptPackages(config, resolved)
}

func (c *CLI) indexNativeTypeScriptPackages(config *project.Config, resolved *packageManager.TypeRBPackages) error {
	if config.Mode != "typescript" || config.TypeScript == nil {
		return nil
	}
	dependencies, err := nativeTypeScriptDependencies(config, resolved)
	if err != nil {
		return err
	}
	modules, err := nativeTypeScriptModules(config, resolved, dependencies)
	if err != nil {
		return err
	}
	catalog, err := nativepackage.GenerateModules(config.Root, config.TypeScript.PackageManager, dependencies, modules)
	if err != nil {
		return err
	}
	providers := nativeTypeProviderSources(resolved)
	if err := nativepackage.ApplyProviderFiles(catalog, providers); err != nil {
		return err
	}
	if err := nativepackage.Write(config.Root, catalog); err != nil {
		return err
	}
	if len(dependencies) > 0 {
		fmt.Fprintf(c.Stdout, "indexed %d native TypeScript module(s) from %d package(s) -> %s\n", len(catalog.Modules), len(dependencies), nativepackage.IndexPath(config.Root))
	}
	return nil
}

func nativeTypeScriptModules(config *project.Config, resolved *packageManager.TypeRBPackages, dependencies map[string]string) ([]string, error) {
	modules := make([]string, 0, len(dependencies))
	seen := map[string]bool{}
	for name := range dependencies {
		seen[name] = true
		modules = append(modules, name)
	}
	files, err := collectDependencyTRB(config)
	if err != nil {
		return nil, err
	}
	units, err := projectSourceUnits(config, files, resolved)
	if err != nil {
		return nil, err
	}
	for _, unit := range units {
		program, _ := parser.Parse(unit.Source)
		for _, statement := range program.Statements {
			imported, ok := statement.(*ast.ImportStatement)
			if !ok || seen[imported.Path] || !nativeDependencyOwns(dependencies, imported.Path) {
				continue
			}
			seen[imported.Path] = true
			modules = append(modules, imported.Path)
		}
	}
	sort.Strings(modules)
	return modules, nil
}

func nativeDependencyOwns(dependencies map[string]string, importPath string) bool {
	for dependency := range dependencies {
		if importPath == dependency || strings.HasPrefix(importPath, dependency+"/") {
			return true
		}
	}
	return false
}

func nativeTypeScriptDependencies(config *project.Config, resolved *packageManager.TypeRBPackages) (map[string]string, error) {
	dependencies := make(map[string]string, len(config.Dependencies))
	for name, version := range config.Dependencies {
		dependencies[name] = version
	}
	if resolved != nil {
		for name, version := range resolved.NativeDependencies {
			if existing, ok := dependencies[name]; ok && existing != version {
				return nil, fmt.Errorf("native TypeScript dependency %s has conflicting versions %s and %s", name, existing, version)
			}
			dependencies[name] = version
		}
	}
	return dependencies, nil
}

func nativeTypeProviderSources(resolved *packageManager.TypeRBPackages) []nativepackage.ProviderSource {
	if resolved == nil {
		return nil
	}
	providers := make([]nativepackage.ProviderSource, 0, len(resolved.NativeTypeProviders))
	for _, provider := range resolved.NativeTypeProviders {
		providers = append(providers, nativepackage.ProviderSource{Package: provider.Package, Path: provider.Path, Dependencies: provider.Dependencies})
	}
	return providers
}

func (c *CLI) runUpdate(args []string) error {
	if len(args) != 0 {
		return errors.New("update does not accept package arguments yet")
	}
	config, err := project.Find(".")
	if err != nil {
		return err
	}
	resolved, err := packageManager.ResolveTypeRBPackages(config, packageManager.TypeRBResolveOptions{Update: true})
	if err != nil {
		return err
	}
	if config.ManagesPackages() {
		files, err := collectDependencyTRB(config)
		if err != nil {
			return err
		}
		if _, err := syncProjectPackages(config, files); err != nil {
			return err
		}
	}
	fmt.Fprintf(c.Stdout, "updated %d TypeRB package(s) -> %s\n", len(resolved.Packages), packageManager.TypeRBLockPath(config))
	return nil
}

func syncProjectPackages(config *project.Config, files []string) (string, error) {
	dependencies, err := projectPackageDependencies(config, files)
	if err != nil {
		return "", err
	}
	return packageManager.SyncWithDependencies(config, dependencies)
}

func collectDependencyTRB(config *project.Config) ([]string, error) {
	files, err := collectTRB([]string{config.SourcePath()}, config.OutputPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return files, err
}

func projectPackageDependencies(config *project.Config, files []string) (map[string]string, error) {
	resolvedPackages, err := packageManager.LoadTypeRBPackages(config)
	if err != nil {
		return nil, err
	}
	units, err := projectSourceUnits(config, files, resolvedPackages)
	if err != nil {
		return nil, err
	}
	sources := make([][]byte, 0, len(units))
	for _, unit := range units {
		sources = append(sources, unit.Source)
	}
	dependencies := map[string]string{}
	for name, version := range resolvedPackages.NativeDependencies {
		dependencies[name] = version
	}
	seen := map[string]bool{}
	for len(sources) > 0 {
		source := sources[0]
		sources = sources[1:]
		program, _ := parser.Parse(source)
		for _, statement := range program.Statements {
			imported, ok := statement.(*ast.ImportStatement)
			if !ok {
				continue
			}
			bundled, exists := official.Lookup(imported.Path)
			if !exists || seen[bundled.Name] {
				continue
			}
			seen[bundled.Name] = true
			options := config.PackageOptions[bundled.Name]
			if bundled.Name == jobssql.PackageName {
				options, err = jobsSQLNativeOptions(config)
				if err != nil {
					return nil, err
				}
			}
			required, err := bundled.NativeDependenciesFor(config.Mode, options)
			if err != nil {
				return nil, err
			}
			for name, version := range required {
				if existing, present := dependencies[name]; present && existing != version {
					return nil, fmt.Errorf("TypeRB packages require conflicting versions of %s: %s and %s", name, existing, version)
				}
				dependencies[name] = version
			}
			sources = append(sources, []byte(bundled.Definition.Source))
		}
	}
	return dependencies, nil
}

func loadConfig(explicit, start string) (*project.Config, error) {
	if explicit != "" {
		return project.Load(explicit)
	}
	return project.Find(start)
}

func compileProject(config *project.Config, files []string) (map[string]*compiler.Artifact, error) {
	units, options, err := projectCompilation(config, files)
	if err != nil {
		return nil, err
	}
	return compileSourceUnits(units, options)
}

func compileProjectSources(config *project.Config, sources []fileRootSource) (map[string]*compiler.Artifact, error) {
	units, options, err := projectCompilationSources(config, sources)
	if err != nil {
		return nil, err
	}
	return compileSourceUnits(units, options)
}

func compileSourceUnits(units []compiler.SourceUnit, options compiler.Options) (map[string]*compiler.Artifact, error) {
	artifacts, err := compiler.CompileProject(units, options)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*compiler.Artifact, len(artifacts))
	for _, artifact := range artifacts {
		absolute, _ := filepath.Abs(artifact.Filename)
		result[absolute] = artifact
	}
	return result, nil
}

func projectCompilation(config *project.Config, files []string) ([]compiler.SourceUnit, compiler.Options, error) {
	resolvedPackages, err := packageManager.LoadTypeRBPackages(config)
	if err != nil {
		return nil, compiler.Options{}, err
	}
	units, err := projectSourceUnits(config, files, resolvedPackages)
	if err != nil {
		return nil, compiler.Options{}, err
	}
	options, err := compilerOptionsWithPackages(config, resolvedPackages)
	if err != nil {
		return nil, compiler.Options{}, err
	}
	return units, options, nil
}

func projectCompilationSources(config *project.Config, sources []fileRootSource) ([]compiler.SourceUnit, compiler.Options, error) {
	resolvedPackages, err := packageManager.LoadTypeRBPackages(config)
	if err != nil {
		return nil, compiler.Options{}, err
	}
	units, err := projectSourceUnitsFromSources(config, sources, resolvedPackages)
	if err != nil {
		return nil, compiler.Options{}, err
	}
	options, err := compilerOptionsWithPackages(config, resolvedPackages)
	if err != nil {
		return nil, compiler.Options{}, err
	}
	return units, options, nil
}

func projectSourceUnits(config *project.Config, files []string, resolvedPackages *packageManager.TypeRBPackages) ([]compiler.SourceUnit, error) {
	sources := make([]fileRootSource, 0, len(files))
	for _, filename := range files {
		source, err := os.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		sources = append(sources, fileRootSource{Filename: filename, Source: source})
	}
	return projectSourceUnitsFromSources(config, sources, resolvedPackages)
}

func projectSourceUnitsFromSources(config *project.Config, sources []fileRootSource, resolvedPackages *packageManager.TypeRBPackages) ([]compiler.SourceUnit, error) {
	units := make([]compiler.SourceUnit, 0, len(sources))
	for _, source := range sources {
		unit, err := sourceUnit(config, source.Filename, source.Source)
		if err != nil {
			return nil, err
		}
		units = append(units, unit)
	}
	packageNames := make([]string, 0, len(config.LocalPackages))
	for name := range config.LocalPackages {
		packageNames = append(packageNames, name)
	}
	sort.Strings(packageNames)
	for _, packageName := range packageNames {
		packageRoot := config.LocalPackages[packageName]
		if !filepath.IsAbs(packageRoot) {
			packageRoot = filepath.Join(config.Root, packageRoot)
		}
		packageRoot, _ = filepath.Abs(packageRoot)
		packageFiles, err := collectTRB([]string{packageRoot}, config.OutputPath())
		if err != nil {
			return nil, fmt.Errorf("local package %s: %w", packageName, err)
		}
		if len(packageFiles) == 0 {
			return nil, fmt.Errorf("local package %s has no .trb files at %s", packageName, packageRoot)
		}
		for _, filename := range packageFiles {
			source, err := os.ReadFile(filename)
			if err != nil {
				return nil, err
			}
			unit, err := localSourceUnit(config, packageName, packageRoot, filename, source)
			if err != nil {
				return nil, err
			}
			units = append(units, unit)
		}
	}
	for _, resolved := range resolvedPackages.Packages {
		sourceRoot := filepath.Join(resolved.Root, resolved.Manifest.SourceDir)
		packageAliases := map[string]string{}
		if resolvedPackages.Lock != nil {
			for alias, canonical := range resolvedPackages.Lock.Packages[resolved.Name].Dependencies {
				packageAliases[alias] = canonical
			}
		}
		packageFiles, err := collectTRB([]string{sourceRoot}, "")
		if err != nil {
			return nil, fmt.Errorf("TypeRB package %s: %w", resolved.Name, err)
		}
		if len(packageFiles) == 0 {
			return nil, fmt.Errorf("TypeRB package %s has no .trb files below %s", resolved.Name, sourceRoot)
		}
		for _, filename := range packageFiles {
			source, err := os.ReadFile(filename)
			if err != nil {
				return nil, err
			}
			unit, err := localSourceUnit(config, resolved.Name, sourceRoot, filename, source)
			if err != nil {
				return nil, err
			}
			unit.PackageAliases = packageAliases
			units = append(units, unit)
		}
	}
	return units, nil
}

func localSourceUnit(config *project.Config, packageName, packageRoot, filename string, source []byte) (compiler.SourceUnit, error) {
	absolute, err := filepath.Abs(filename)
	if err != nil {
		return compiler.SourceUnit{}, err
	}
	relative, err := filepath.Rel(packageRoot, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return compiler.SourceUnit{}, fmt.Errorf("%s is outside local package %s", filename, packageName)
	}
	modulePath := filepath.ToSlash(filepath.Join(filepath.FromSlash(packageName), strings.TrimSuffix(relative, filepath.Ext(relative))))
	goPackage := ""
	if config.Go != nil {
		directory := filepath.ToSlash(filepath.Dir(modulePath))
		goPackage = filepath.Base(directory)
	}
	return compiler.SourceUnit{Filename: absolute, Source: source, ModulePath: modulePath, Package: goPackage, ExternalPackage: true}, nil
}

func generatedRelative(config *project.Config, filename string, artifact *compiler.Artifact) (string, bool) {
	extension := generatedExtension(config, artifact)
	if artifact.CompilerOwned || artifact.Official {
		return filepath.FromSlash(artifact.IR.ModulePath) + extension, true
	}
	if artifact.ExternalPackage {
		return filepath.FromSlash(artifact.IR.ModulePath) + extension, true
	}
	relative, err := filepath.Rel(config.SourcePath(), filename)
	local := err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))
	if local {
		relative = filepath.FromSlash(artifact.IR.ModulePath) + extension
	} else {
		relative = generatedSourceRelativeForArtifact(config, relative, artifact)
	}
	return relative, local
}

func generatedSourceRelativeForArtifact(config *project.Config, relative string, artifact *compiler.Artifact) string {
	if config.Mode != "typescript" || artifact == nil || artifact.IR == nil || !artifact.IR.UsesJSX {
		return generatedSourceRelative(config, relative)
	}
	extension := ".tsx"
	stem := strings.TrimSuffix(relative, filepath.Ext(relative))
	if filepath.Ext(stem) == extension {
		return stem
	}
	return stem + extension
}

func generatedExtension(config *project.Config, artifact *compiler.Artifact) string {
	if config.Mode == "typescript" && artifact != nil && artifact.IR != nil && artifact.IR.UsesJSX {
		return ".tsx"
	}
	return codegen.Extension(config.Mode)
}

func generatedSourceRelative(config *project.Config, relative string) string {
	extension := codegen.Extension(config.Mode)
	stem := strings.TrimSuffix(relative, filepath.Ext(relative))
	if filepath.Ext(stem) == extension {
		relative = stem
	} else {
		relative = stem + extension
	}
	if config.Mode == "go" {
		relative = goGeneratedSourceRelative(relative)
	}
	return relative
}

func goGeneratedSourceRelative(relative string) string {
	directory := filepath.Dir(relative)
	base := strings.TrimSuffix(filepath.Base(relative), ".go")
	if strings.HasPrefix(base, "[") && strings.HasSuffix(base, "]") {
		parameter := strings.TrimSuffix(strings.TrimPrefix(base, "["), "]")
		prefix := "route_param_"
		if strings.HasPrefix(parameter, "...") {
			parameter = strings.TrimPrefix(parameter, "...")
			prefix = "route_catch_all_"
		}
		base = prefix + parameter
	}
	var safe strings.Builder
	for _, character := range base {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' {
			safe.WriteRune(character)
		} else {
			safe.WriteByte('_')
		}
	}
	base = safe.String()
	if base == "" || strings.HasPrefix(base, "_") || strings.HasPrefix(base, ".") {
		base = "trb_" + base
	}
	filename := base + ".go"
	if directory == "." {
		return filename
	}
	return filepath.Join(directory, filename)
}

func sourceUnit(config *project.Config, filename string, source []byte) (compiler.SourceUnit, error) {
	absolute, err := filepath.Abs(filename)
	if err != nil {
		return compiler.SourceUnit{}, err
	}
	relative, err := filepath.Rel(config.SourcePath(), absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return compiler.SourceUnit{}, fmt.Errorf("%s is outside configured sourceDir %s", filename, config.SourceDir)
	}
	modulePath := filepath.ToSlash(strings.TrimSuffix(relative, filepath.Ext(relative)))
	packageName := ""
	if config.Go != nil {
		directory := filepath.ToSlash(filepath.Dir(modulePath))
		if directory == "." || directory == "" {
			packageName = config.Go.RootPackage
		} else {
			packageName = filepath.Base(directory)
		}
	}
	unit := compiler.SourceUnit{Filename: absolute, Source: source, ModulePath: modulePath, Package: packageName}
	if testsuite.IsTestFile(absolute) {
		sum := sha256.Sum256([]byte(modulePath))
		unit.TestRegistration = fmt.Sprintf("trb_test_register_%x", sum[:6])
	}
	return unit, nil
}

func compilerOptions(config *project.Config) (compiler.Options, error) {
	resolvedPackages, err := packageManager.LoadTypeRBPackages(config)
	if err != nil {
		return compiler.Options{}, err
	}
	return compilerOptionsWithPackages(config, resolvedPackages)
}

func compilerOptionsWithPackages(config *project.Config, resolvedPackages *packageManager.TypeRBPackages) (compiler.Options, error) {
	packageOptions := make(map[string][]byte, len(config.PackageOptions))
	for name, value := range config.PackageOptions {
		packageOptions[name] = append([]byte(nil), value...)
	}
	options := compiler.Options{Mode: config.Mode, SourceRoot: config.SourcePath(), ProjectRoot: config.Root, PackageOptions: packageOptions, PackageAliases: resolvedPackages.Aliases}
	if config.Jobs != nil {
		options.JobsConfiguration = config.Jobs.Configuration
	}
	if config.Ruby != nil {
		options.RubyLoader = config.Ruby.Loader
	}
	if config.Go != nil {
		options.GoModule = config.Go.Module
	}
	if config.TypeScript != nil {
		options.TypeScriptRuntime = config.TypeScript.Runtime
		dependencies, err := nativeTypeScriptDependencies(config, resolvedPackages)
		if err != nil {
			return compiler.Options{}, err
		}
		options.NativePackages, err = nativepackage.LoadWithProviders(config.Root, dependencies, nativeTypeProviderSources(resolvedPackages))
		if err != nil {
			return compiler.Options{}, err
		}
	}
	return options, nil
}

func jobsSQLNativeOptions(config *project.Config) (json.RawMessage, error) {
	if config == nil || config.Jobs == nil {
		return nil, errors.New("trb/jobs/sql requires jobs.configuration in trbconfig.jsonc")
	}
	filename := filepath.Join(config.SourcePath(), filepath.FromSlash(config.Jobs.Configuration)+".trb")
	source, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	program, diagnostics := parser.Parse(source)
	for _, item := range diagnostics {
		if item.Severity == diagnostic.Error {
			return nil, fmt.Errorf("%s: %s", filename, item.Message)
		}
	}
	SQLConfig, err := jobssql.ParseConfiguration(program)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	return json.Marshal(map[string]string{"dialect": SQLConfig.Dialect})
}

func writeCompiledTree(config *project.Config, compiled map[string]*compiler.Artifact, root string, debug bool) (map[string]string, error) {
	generated := make(map[string]string, len(compiled))
	for _, sourceName := range sortedArtifactNames(compiled) {
		artifact := compiled[sourceName]
		relative, _ := generatedRelative(config, sourceName, artifact)
		output := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return nil, err
		}
		data := artifact.Output
		if debug && config.Mode == "go" {
			data = []byte(sourcemap.WithGoLineDirectives(string(data), output, artifact.SourceMap))
		}
		if err := os.WriteFile(output, data, 0o644); err != nil {
			return nil, err
		}
		generated[sourceName] = output
	}
	return generated, nil
}

func copyGoModuleFiles(config *project.Config, root string) error {
	for _, manifest := range []string{"go.mod", "go.sum"} {
		sourcePath := filepath.Join(config.Root, manifest)
		data, err := os.ReadFile(sourcePath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, manifest), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func sortedArtifactNames(compiled map[string]*compiler.Artifact) []string {
	names := make([]string, 0, len(compiled))
	for name := range compiled {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func mainSource(compiled map[string]*compiler.Artifact) string {
	for _, sourceName := range sortedArtifactNames(compiled) {
		if artifactHasMain(compiled[sourceName]) {
			return sourceName
		}
	}
	return ""
}

func artifactHasMain(artifact *compiler.Artifact) bool {
	if artifact == nil || artifact.IR == nil {
		return false
	}
	for _, statement := range artifact.IR.Statements {
		if method, ok := statement.(*ir.Method); ok && method.Name == compiler.MainFunction {
			return true
		}
	}
	return false
}

func firstOr(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return values[0]
}

func interactiveTerminal(reader io.Reader, writer io.Writer) bool {
	return characterDevice(reader) && characterDevice(writer)
}

func (c *CLI) shouldStartREPL() bool {
	detect := c.terminal
	if detect == nil {
		detect = interactiveTerminal
	}
	return detect(c.Stdin, c.Stdout)
}

func characterDevice(stream any) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func dependencySpec(args []string) (string, string) {
	if len(args) == 2 {
		return args[0], args[1]
	}
	value := args[0]
	if at := strings.LastIndex(value, "@"); at > 0 {
		return value[:at], value[at+1:]
	}
	return value, ""
}

func collectTRB(paths []string, excluded string) ([]string, error) {
	var files []string
	excludedAbs, _ := filepath.Abs(excluded)
	for _, path := range paths {
		absolutePath, _ := filepath.Abs(path)
		info, err := os.Stat(absolutePath)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if filepath.Ext(absolutePath) != ".trb" {
				return nil, fmt.Errorf("%s is not a .trb file", path)
			}
			files = append(files, absolutePath)
			continue
		}
		err = filepath.WalkDir(absolutePath, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				absolute, _ := filepath.Abs(name)
				if name != absolutePath && (entry.Name() == ".git" || entry.Name() == ".trb" || entry.Name() == "node_modules" || absolute == excludedAbs) {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(name) == ".trb" {
				files = append(files, name)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return unique(files), nil
}

func productionTRBFiles(files []string) []string {
	result := make([]string, 0, len(files))
	for _, filename := range files {
		if !testsuite.IsTestFile(filename) {
			result = append(result, filename)
		}
	}
	return result
}

func copyProjectFiles(root, outDir string) error {
	outAbs, _ := filepath.Abs(outDir)
	rootAbs, _ := filepath.Abs(root)
	return filepath.WalkDir(rootAbs, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		absolute, _ := filepath.Abs(name)
		if entry.IsDir() {
			legacyGenerated := strings.HasPrefix(entry.Name(), "trb-run-") || strings.HasPrefix(entry.Name(), "trb-test-")
			if name != rootAbs && (entry.Name() == ".git" || entry.Name() == ".trb" || entry.Name() == "node_modules" || legacyGenerated || absolute == outAbs) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(name) == ".trb" || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(rootAbs, absolute)
		if err != nil {
			return err
		}
		destination := filepath.Join(outAbs, rel)
		if destination == absolute {
			return nil
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		return os.WriteFile(destination, data, info.Mode().Perm())
	})
}

func unique(items []string) []string {
	if len(items) == 0 {
		return items
	}
	result := items[:1]
	for _, item := range items[1:] {
		if item != result[len(result)-1] {
			result = append(result, item)
		}
	}
	return result
}

func (c *CLI) usage() {
	fmt.Fprintln(c.Stdout, "TypeRB compiler and package manager")
	fmt.Fprintln(c.Stdout, "")
	fmt.Fprintln(c.Stdout, "Usage:")
	fmt.Fprintln(c.Stdout, "  trb")
	fmt.Fprintln(c.Stdout, "  trb init --mode ruby|go|typescript [--runtime browser|bun|node] [--template web] [directory]")
	fmt.Fprintln(c.Stdout, "  trb fmt [--check] [paths...]")
	fmt.Fprintln(c.Stdout, "  trb check [--diagnostic-format human|json] [--config trbconfig.jsonc]")
	fmt.Fprintln(c.Stdout, "  trb check [--diagnostic-format human|json] [--mode MODE] FILE.trb")
	fmt.Fprintln(c.Stdout, "  trb test [--filter TEXT] [--file FILE] [--reporter human|json] [--compile [--debug] [--outfile FILE]] [--config trbconfig.jsonc]")
	fmt.Fprintln(c.Stdout, "  trb build [paths...]")
	fmt.Fprintln(c.Stdout, "  trb build --compile [--debug] [--outfile FILE]")
	fmt.Fprintln(c.Stdout, "  trb build --compile [--debug] [--outfile FILE] [--mode go] FILE.trb")
	fmt.Fprintln(c.Stdout, "  trb run [--keep-generated] [--mode MODE] [--runtime RUNTIME] [FILE.trb] [-- arguments...]")
	fmt.Fprintln(c.Stdout, "  trb FILE.trb [-- arguments...]")
	fmt.Fprintln(c.Stdout, "  trb clean [--build] [--cache] [--generated] [--config trbconfig.jsonc]")
	fmt.Fprintln(c.Stdout, "  trb repl [--mode ruby|go|typescript] [--config trbconfig.jsonc]")
	fmt.Fprintln(c.Stdout, "  trb lsp [--config trbconfig.jsonc] [--mode MODE] [--runtime RUNTIME] [FILE.trb]")
	fmt.Fprintln(c.Stdout, "  trb play [--mode ruby|go|typescript] [--port PORT] [--no-open]")
	fmt.Fprintln(c.Stdout, "  trb tour [--mode ruby|go|typescript] [--port PORT] [--no-open]")
	fmt.Fprintln(c.Stdout, "  trb db plan|apply|export|lock|check [options]")
	fmt.Fprintln(c.Stdout, "  trb jobs start [--once] [--queue NAME] [--config trbconfig.jsonc]")
	fmt.Fprintln(c.Stdout, "  trb jobs list [--config trbconfig.jsonc]")
	fmt.Fprintln(c.Stdout, "  trb jobs retry|discard JOB_ID [--config trbconfig.jsonc]")
	fmt.Fprintln(c.Stdout, "  trb sync")
	fmt.Fprintln(c.Stdout, "  trb add [--source GIT | --path DIRECTORY] PACKAGE [VERSION]")
	fmt.Fprintln(c.Stdout, "  trb add --native [--dev] PACKAGE [VERSION]")
	fmt.Fprintln(c.Stdout, "  trb remove [--native] PACKAGE")
	fmt.Fprintln(c.Stdout, "  trb install [--frozen] [--offline] [--config trbconfig.jsonc]")
	fmt.Fprintln(c.Stdout, "  trb update")
	fmt.Fprintln(c.Stdout, "  trb version")
}
