package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/formatter"
	packageManager "github.com/type-rb/type-rb/internal/packages"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/project"
	"github.com/type-rb/type-rb/internal/resolver"
)

type importFormatContext struct {
	modulePaths map[string]bool
	aliases     map[string]string
	catalog     *resolver.Catalog
	mode        string
	sourceRoot  string
}

type importFormatContexts struct {
	projects map[string]*importFormatContext
}

func (c *importFormatContexts) resolver(filename string) func(*ast.ImportStatement) formatter.ImportMetadata {
	config, err := project.Find(filename)
	if err != nil || !pathBelow(config.SourcePath(), filename) {
		return nil
	}
	context := c.projects[config.Path]
	if context == nil {
		context = loadImportFormatContext(config)
		c.projects[config.Path] = context
	}
	return func(node *ast.ImportStatement) formatter.ImportMetadata {
		canonicalPath := resolver.CanonicalProjectImportPath(node.Path, context.modulePaths, context.aliases)
		program := &ast.Program{Statements: []ast.Statement{node}}
		result, diagnostics := resolver.Resolve(program, resolver.Options{
			Mode: context.mode, SourceRoot: context.sourceRoot, Filename: filename,
			PackageAliases: context.aliases, Catalog: context.catalog,
		})
		if len(diagnostics) != 0 {
			return formatter.ImportMetadata{CanonicalPath: canonicalPath}
		}
		imported := result.Imports[node]
		root, _ := resolver.RootDeclaration(imported)
		return formatter.ImportMetadata{
			CanonicalPath: canonicalPath, Root: root, RootStable: imported != nil && imported.RootStable, Resolved: imported != nil,
		}
	}
}

func newImportFormatContexts() *importFormatContexts {
	return &importFormatContexts{projects: map[string]*importFormatContext{}}
}

func (c *importFormatContexts) canonicalizer(filename string) func(string) string {
	config, err := project.Find(filename)
	if err == nil && pathBelow(config.SourcePath(), filename) {
		context := c.projects[config.Path]
		if context == nil {
			context = loadImportFormatContext(config)
			c.projects[config.Path] = context
		}
		return func(importPath string) string {
			return resolver.CanonicalProjectImportPath(importPath, context.modulePaths, context.aliases)
		}
	}
	root := filepath.Dir(filename)
	return func(importPath string) string {
		return canonicalFileRootImportPath(root, importPath)
	}
}

func loadImportFormatContext(config *project.Config) *importFormatContext {
	context := &importFormatContext{modulePaths: map[string]bool{}, aliases: map[string]string{}, mode: config.Mode, sourceRoot: config.SourcePath()}
	files, err := collectTRB([]string{config.SourcePath()}, config.OutputPath())
	if err != nil {
		return context
	}
	for _, filename := range files {
		unit, unitErr := sourceUnit(config, filename, nil)
		if unitErr == nil {
			context.modulePaths[unit.ModulePath] = true
		}
	}

	resolvedPackages, err := packageManager.LoadTypeRBPackages(config)
	if err != nil {
		return context
	}
	for alias, canonical := range resolvedPackages.Aliases {
		context.aliases[alias] = canonical
	}
	units, err := projectSourceUnits(config, files, resolvedPackages)
	if err != nil {
		return context
	}
	modules := make([]resolver.Module, 0, len(units))
	for _, unit := range units {
		context.modulePaths[unit.ModulePath] = true
		program, diagnostics := parser.Parse(unit.Source)
		if len(diagnostics) != 0 {
			continue
		}
		modules = append(modules, resolver.Module{
			Path: unit.ModulePath, Filename: unit.Filename, Program: program,
			CompilerOwned: unit.CompilerOwned, Official: unit.Official, DeclarationProvider: unit.DeclarationProvider,
		})
	}
	context.catalog, _ = resolver.NewCatalog(modules)
	return context
}

func canonicalFileRootImportPath(root, importPath string) string {
	if !strings.HasSuffix(importPath, "/index") {
		return importPath
	}
	short := strings.TrimSuffix(importPath, "/index")
	resolved, _, found, err := readFileRootImport(root, importPath, os.ReadFile)
	if err != nil || !found {
		return importPath
	}
	shortResolved, _, found, err := readFileRootImport(root, short, os.ReadFile)
	if err != nil || !found || shortResolved != resolved {
		return importPath
	}
	return short
}

func pathBelow(root, filename string) bool {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absoluteFilename, err := filepath.Abs(filename)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(absoluteRoot, absoluteFilename)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
