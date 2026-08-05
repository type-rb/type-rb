package cli

import (
	"bytes"
	"context"
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

	"github.com/type-rb/type-rb/internal/codegen"
	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/formatter"
	"github.com/type-rb/type-rb/internal/ir"
	packageManager "github.com/type-rb/type-rb/internal/packages"
	"github.com/type-rb/type-rb/internal/playground"
	"github.com/type-rb/type-rb/internal/project"
	"github.com/type-rb/type-rb/internal/repl"
)

// Version is a variable so release builds can inject the tag with Go's -X
// linker flag while local source builds retain a useful development version.
var Version = "0.1.4-dev"

type buildArtifactKind string

const (
	buildArtifactSource     buildArtifactKind = "source"
	buildArtifactExecutable buildArtifactKind = "executable"
)

type CLI struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func New() *CLI { return &CLI{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr} }

func (c *CLI) Run(args []string) int {
	if len(args) == 0 {
		c.usage()
		return 2
	}
	var err error
	switch args[0] {
	case "fmt":
		err = c.runFmt(args[1:])
	case "build":
		err = c.runBuild(args[1:])
	case "run":
		err = c.runProgram(args[1:])
	case "repl":
		err = c.runRepl(args[1:])
	case "play":
		err = c.runPlay(args[1:])
	case "tour":
		err = c.runTour(args[1:])
	case "init":
		err = c.runInit(args[1:])
	case "sync":
		err = c.runSync(args[1:])
	case "add":
		err = c.runAdd(args[1:])
	case "remove":
		err = c.runRemove(args[1:])
	case "install":
		err = c.runInstall(args[1:])
	case "version", "--version", "-v":
		_, err = fmt.Fprintln(c.Stdout, "trb "+Version)
	case "help", "--help", "-h":
		c.usage()
		return 0
	default:
		err = fmt.Errorf("unknown command %q", args[0])
	}
	if err != nil {
		fmt.Fprintln(c.Stderr, "trb:", err)
		if compilation, ok := err.(*compiler.CompileError); ok {
			for i, item := range compilation.Diagnostics {
				if i == 0 {
					continue
				}
				fmt.Fprintf(c.Stderr, "%s:%d:%d: %s: %s\n", compilation.Filename, item.Span.Start.Line, item.Span.Start.Column, item.Severity, item.Message)
			}
		}
		return 1
	}
	return 0
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
	var changed []string
	for _, name := range files {
		source, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		formatted, diagnostics := formatter.Format(source)
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
	check := flags.Bool("check", false, "compile without writing output")
	copyFlag := flags.String("copy", "", "override config copyFiles (true or false)")
	compile := flags.Bool("compile", false, "produce an executable with the target toolchain")
	outfile := flags.String("outfile", "", "executable output path relative to the project root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	paths := flags.Args()
	config, err := loadConfig(*configPath, firstOr(paths, "."))
	if err != nil {
		return err
	}
	kind := buildArtifactSource
	if *compile {
		kind = buildArtifactExecutable
	}
	if kind == buildArtifactExecutable {
		if config.Mode != "go" {
			return fmt.Errorf("--compile is supported only for mode go; project mode is %s", config.Mode)
		}
		if len(paths) != 0 {
			return errors.New("--compile builds the configured project and does not accept source paths")
		}
		if *stdout || *check || *copyFlag != "" || *outDirFlag != "" {
			return errors.New("--compile cannot be combined with --stdout, --check, --copy, or --out-dir")
		}
		return c.buildGoExecutable(config, *outfile)
	}
	if *outfile != "" {
		return errors.New("--outfile requires --compile")
	}
	sourceRoot := config.SourcePath()
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
		ext := codegen.Extension(config.Mode)
		stem := strings.TrimSuffix(rel, filepath.Ext(rel))
		if filepath.Ext(stem) == ext {
			rel = stem
		} else {
			rel = stem + ext
		}
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
	if *check {
		fmt.Fprintf(c.Stdout, "checked %d file(s) for mode %s\n", len(artifacts), config.Mode)
		return nil
	}
	manifest := ""
	if config.ManagesPackages() {
		manifest, err = packageManager.Sync(config)
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

func (c *CLI) buildGoExecutable(config *project.Config, outfile string) error {
	files, err := collectTRB([]string{config.SourcePath()}, config.OutputPath())
	if err != nil {
		return err
	}
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
		if _, err := packageManager.Sync(config); err != nil {
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
	generated, err := writeCompiledTree(config, compiled, buildRoot)
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
	if info, statErr := os.Stat(output); statErr == nil && info.IsDir() {
		return fmt.Errorf("--outfile must name a file; %s is a directory", output)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	command := exec.Command("go", "build", "-o", output, ".")
	command.Dir = filepath.Dir(target)
	command.Stdout = c.Stdout
	command.Stderr = c.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}
	fmt.Fprintf(c.Stdout, "executable -> %s\n", output)
	return nil
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

func (c *CLI) runProgram(args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", "", "path to trbconfig.jsonc")
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
	config, err := loadConfig(*configPath, configStart)
	if err != nil {
		return err
	}
	if config.ManagesPackages() {
		if _, err := packageManager.Sync(config); err != nil {
			return err
		}
	}
	files, err := collectTRB([]string{config.SourcePath()}, config.OutputPath())
	if err != nil {
		return err
	}
	compiled, err := compileProject(config, files)
	if err != nil {
		return err
	}
	runRoot, err := os.MkdirTemp(config.Root, "trb-run-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(runRoot)
	target := ""
	entrySource := filename
	if entrySource == "" {
		entrySource = mainSource(compiled)
		if entrySource == "" {
			return errors.New("project has no top-level main(); define def main() or pass a .trb file explicitly")
		}
	}
	generated, err := writeCompiledTree(config, compiled, runRoot)
	if err != nil {
		return err
	}
	target = generated[entrySource]
	if target == "" {
		return fmt.Errorf("%s is outside configured sourceDir %s", entrySource, config.SourceDir)
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
		command = exec.Command("bundle", append([]string{"exec", "ruby", target}, programArgs...)...)
	case "go":
		command = exec.Command("go", append([]string{"run", "."}, programArgs...)...)
		if config.Go.Sqldef != nil {
			database := filepath.Join(config.Root, config.Go.Sqldef.Database)
			command.Env = append(os.Environ(), "TRB_DATABASE="+database)
		}
	case "typescript":
		command = exec.Command("node", append([]string{"--experimental-strip-types", target}, programArgs...)...)
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
	return command.Run()
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
	}
	sessionFilename := filepath.Join(config.SourcePath(), ".trb-repl.trb")
	sessionModule := "__trb_repl__"
	sessionPackage := ""
	if config.Go != nil {
		sessionPackage = config.Go.RootPackage
	}
	compile := func(source string) (*repl.Compilation, error) {
		units, err := projectSourceUnits(config, files)
		if err != nil {
			return nil, err
		}
		units = append(units, compiler.SourceUnit{
			Filename:   sessionFilename,
			Source:     []byte(source),
			ModulePath: sessionModule,
			Package:    sessionPackage,
		})
		artifacts, err := compiler.CompileProject(units, compilerOptions(config))
		if err != nil {
			return nil, err
		}
		compilation := &repl.Compilation{Programs: make([]*ir.Program, 0, len(artifacts))}
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
	})
}

func (c *CLI) runPlay(args []string) error {
	return c.runBrowserTool("play", args, false)
}

func (c *CLI) runTour(args []string) error {
	return c.runBrowserTool("tour", args, true)
}

func (c *CLI) runBrowserTool(page string, args []string, allowCheck bool) error {
	flags := flag.NewFlagSet(page, flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	mode := flags.String("mode", "", "initial mode: ruby, go, or typescript")
	port := flags.Int("port", 0, "local HTTP port; zero chooses an available port")
	noOpen := flags.Bool("no-open", false, "serve without opening a browser")
	var check *bool
	if allowCheck {
		check = flags.Bool("check", false, "validate every tour lesson without opening a browser")
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("%s does not accept source arguments", page)
	}
	if check != nil && *check {
		if *mode != "" || *port != 0 || *noOpen {
			return errors.New("tour --check cannot be combined with --mode, --port, or --no-open")
		}
		count, err := playground.ValidateTour(context.Background())
		if err != nil {
			return err
		}
		fmt.Fprintf(c.Stdout, "checked %d tour lesson execution(s)\n", count)
		return nil
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
		config.Go = &project.GoConfig{Module: "trb.local/repl", Version: "1.26", RootPackage: "main", IndirectDependencies: map[string]string{}}
		if base.Go != nil {
			clone := *base.Go
			config.Go = &clone
			if config.Go.Module == "" {
				config.Go.Module = "trb.local/repl"
			}
		}
	case "typescript":
		config.TypeScript = &project.TypeScriptConfig{PackageManager: "npm", ModuleType: "module", Scripts: map[string]string{}}
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
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	config := project.New(root, *mode)
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
	if err := config.Save(); err != nil {
		return err
	}
	manifest, err := packageManager.Sync(config)
	if err != nil {
		return err
	}
	fmt.Fprintln(c.Stdout, config.Path)
	fmt.Fprintln(c.Stdout, manifest)
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
	manifest, err := packageManager.Sync(config)
	if err == nil {
		fmt.Fprintln(c.Stdout, manifest)
	}
	return err
}

func (c *CLI) runAdd(args []string) error {
	flags := flag.NewFlagSet("add", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	dev := flags.Bool("dev", false, "add a development dependency")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 1 || flags.NArg() > 2 {
		return errors.New("usage: trb add [--dev] PACKAGE [VERSION]")
	}
	config, err := project.Find(".")
	if err != nil {
		return err
	}
	if !config.ManagesPackages() {
		return errors.New("package management is external; edit dependencies in the host project")
	}
	name, version := dependencySpec(flags.Args())
	if version == "" && config.Mode == "typescript" {
		version = "latest"
	}
	if version == "" && config.Mode == "go" {
		return errors.New("Go dependencies require an explicit version")
	}
	delete(config.Dependencies, name)
	delete(config.DevDependencies, name)
	if *dev {
		config.DevDependencies[name] = version
	} else {
		config.Dependencies[name] = version
	}
	if err := config.Save(); err != nil {
		return err
	}
	manifest, err := packageManager.Sync(config)
	if err == nil {
		fmt.Fprintf(c.Stdout, "%s %s -> %s\n", name, version, manifest)
	}
	return err
}

func (c *CLI) runRemove(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: trb remove PACKAGE")
	}
	config, err := project.Find(".")
	if err != nil {
		return err
	}
	if !config.ManagesPackages() {
		return errors.New("package management is external; edit dependencies in the host project")
	}
	delete(config.Dependencies, args[0])
	delete(config.DevDependencies, args[0])
	if err := config.Save(); err != nil {
		return err
	}
	manifest, err := packageManager.Sync(config)
	if err == nil {
		fmt.Fprintf(c.Stdout, "%s -> %s\n", args[0], manifest)
	}
	return err
}

func (c *CLI) runInstall(args []string) error {
	if len(args) != 0 {
		return errors.New("install does not accept arguments")
	}
	config, err := project.Find(".")
	if err != nil {
		return err
	}
	return packageManager.Install(config, c.Stdin, c.Stdout, c.Stderr)
}

func loadConfig(explicit, start string) (*project.Config, error) {
	if explicit != "" {
		return project.Load(explicit)
	}
	return project.Find(start)
}

func compileProject(config *project.Config, files []string) (map[string]*compiler.Artifact, error) {
	units, err := projectSourceUnits(config, files)
	if err != nil {
		return nil, err
	}
	artifacts, err := compiler.CompileProject(units, compilerOptions(config))
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

func projectSourceUnits(config *project.Config, files []string) ([]compiler.SourceUnit, error) {
	units := make([]compiler.SourceUnit, 0, len(files))
	for _, filename := range files {
		source, err := os.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		unit, err := sourceUnit(config, filename, source)
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
	return compiler.SourceUnit{Filename: absolute, Source: source, ModulePath: modulePath, Package: goPackage}, nil
}

func generatedRelative(config *project.Config, filename string, artifact *compiler.Artifact) (string, bool) {
	if artifact.CompilerOwned {
		return filepath.FromSlash(artifact.IR.ModulePath) + codegen.Extension(config.Mode), true
	}
	relative, err := filepath.Rel(config.SourcePath(), filename)
	local := err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))
	if local {
		relative = filepath.FromSlash(artifact.IR.ModulePath) + codegen.Extension(config.Mode)
	} else {
		extension := codegen.Extension(config.Mode)
		stem := strings.TrimSuffix(relative, filepath.Ext(relative))
		if filepath.Ext(stem) == extension {
			relative = stem
		} else {
			relative = stem + extension
		}
	}
	return relative, local
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
	return compiler.SourceUnit{Filename: absolute, Source: source, ModulePath: modulePath, Package: packageName}, nil
}

func compilerOptions(config *project.Config) compiler.Options {
	options := compiler.Options{Mode: config.Mode, SourceRoot: config.SourcePath(), ProjectRoot: config.Root}
	if config.Ruby != nil {
		options.RubyLoader = config.Ruby.Loader
	}
	if config.Go != nil {
		options.GoModule = config.Go.Module
	}
	return options
}

func writeCompiledTree(config *project.Config, compiled map[string]*compiler.Artifact, root string) (map[string]string, error) {
	generated := make(map[string]string, len(compiled))
	for _, sourceName := range sortedArtifactNames(compiled) {
		artifact := compiled[sourceName]
		relative, _ := generatedRelative(config, sourceName, artifact)
		output := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(output, artifact.Output, 0o644); err != nil {
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
				if name != absolutePath && (entry.Name() == ".git" || entry.Name() == "node_modules" || absolute == excludedAbs) {
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

func copyProjectFiles(root, outDir string) error {
	outAbs, _ := filepath.Abs(outDir)
	rootAbs, _ := filepath.Abs(root)
	return filepath.WalkDir(rootAbs, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		absolute, _ := filepath.Abs(name)
		if entry.IsDir() {
			if name != rootAbs && (entry.Name() == ".git" || entry.Name() == "node_modules" || absolute == outAbs) {
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
	fmt.Fprintln(c.Stdout, "  trb init --mode ruby|go|typescript [directory]")
	fmt.Fprintln(c.Stdout, "  trb fmt [--check] [paths...]")
	fmt.Fprintln(c.Stdout, "  trb build [--check] [paths...]")
	fmt.Fprintln(c.Stdout, "  trb build --compile [--outfile FILE]")
	fmt.Fprintln(c.Stdout, "  trb run [FILE.trb] [-- arguments...]")
	fmt.Fprintln(c.Stdout, "  trb repl [--mode ruby|go|typescript] [--config trbconfig.jsonc]")
	fmt.Fprintln(c.Stdout, "  trb play [--mode ruby|go|typescript] [--port PORT] [--no-open]")
	fmt.Fprintln(c.Stdout, "  trb tour [--mode ruby|go|typescript] [--port PORT] [--no-open]")
	fmt.Fprintln(c.Stdout, "  trb tour --check")
	fmt.Fprintln(c.Stdout, "  trb sync")
	fmt.Fprintln(c.Stdout, "  trb add [--dev] PACKAGE [VERSION]")
	fmt.Fprintln(c.Stdout, "  trb remove PACKAGE")
	fmt.Fprintln(c.Stdout, "  trb install")
	fmt.Fprintln(c.Stdout, "  trb version")
}
