package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/codegen"
	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/formatter"
	packageManager "github.com/type-rb/type-rb/internal/packages"
	"github.com/type-rb/type-rb/internal/project"
)

// Version is a variable so release builds can inject the tag with Go's -X
// linker flag while local source builds retain a useful development version.
var Version = "0.1.0-dev"

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
	if err := flags.Parse(args); err != nil {
		return err
	}
	paths := flags.Args()
	config, err := loadConfig(*configPath, firstOr(paths, "."))
	if err != nil {
		return err
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

func (c *CLI) runProgram(args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", "", "path to trbconfig.jsonc")
	if err := flags.Parse(args); err != nil {
		return err
	}
	remaining := flags.Args()
	if len(remaining) == 0 {
		return errors.New("usage: trb run FILE.trb [-- program arguments]")
	}
	filename, err := filepath.Abs(remaining[0])
	if err != nil {
		return err
	}
	if filepath.Ext(filename) != ".trb" {
		return fmt.Errorf("%s is not a .trb file", remaining[0])
	}
	programArgs := remaining[1:]
	if len(programArgs) > 0 && programArgs[0] == "--" {
		programArgs = programArgs[1:]
	}
	config, err := loadConfig(*configPath, filename)
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
	compiledNames := make([]string, 0, len(compiled))
	for name := range compiled {
		compiledNames = append(compiledNames, name)
	}
	sort.Strings(compiledNames)
	for _, sourceName := range compiledNames {
		artifact := compiled[sourceName]
		relative, _ := generatedRelative(config, sourceName, artifact)
		generated := filepath.Join(runRoot, relative)
		if err := os.MkdirAll(filepath.Dir(generated), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(generated, artifact.Output, 0o644); err != nil {
			return err
		}
		if sourceName == filename {
			target = generated
		}
	}
	if target == "" {
		return fmt.Errorf("%s is outside configured sourceDir %s", filename, config.SourceDir)
	}
	if config.Mode == "go" {
		for _, manifest := range []string{"go.mod", "go.sum"} {
			sourcePath := filepath.Join(config.Root, manifest)
			data, readErr := os.ReadFile(sourcePath)
			if os.IsNotExist(readErr) {
				continue
			}
			if readErr != nil {
				return readErr
			}
			if err := os.WriteFile(filepath.Join(runRoot, manifest), data, 0o644); err != nil {
				return err
			}
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
		command = exec.Command("go", append([]string{"run", target}, programArgs...)...)
		if config.Go.Sqldef != nil {
			database := filepath.Join(config.Root, config.Go.Sqldef.Database)
			command.Env = append(os.Environ(), "TRB_DATABASE="+database)
		}
	case "typescript":
		command = exec.Command("node", append([]string{"--experimental-strip-types", target}, programArgs...)...)
	}
	command.Dir = runRoot
	if config.Mode == "ruby" {
		command.Dir = config.Root
	}
	command.Stdin = c.Stdin
	command.Stdout = c.Stdout
	command.Stderr = c.Stderr
	return command.Run()
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
	options := compiler.Options{Mode: config.Mode, EntryPoint: config.EntryPoint, SourceRoot: config.SourcePath(), ProjectRoot: config.Root}
	if config.Ruby != nil {
		options.RubyLoader = config.Ruby.Loader
	}
	if config.Go != nil {
		options.GoModule = config.Go.Module
	}
	return options
}

func firstOr(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return values[0]
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
	fmt.Fprintln(c.Stdout, "  trb run FILE.trb [-- arguments...]")
	fmt.Fprintln(c.Stdout, "  trb sync")
	fmt.Fprintln(c.Stdout, "  trb add [--dev] PACKAGE [VERSION]")
	fmt.Fprintln(c.Stdout, "  trb remove PACKAGE")
	fmt.Fprintln(c.Stdout, "  trb install")
	fmt.Fprintln(c.Stdout, "  trb version")
}
