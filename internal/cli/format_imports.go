package cli

import (
	"os"
	"path/filepath"
	"strings"

	packageManager "github.com/type-rb/type-rb/internal/packages"
	"github.com/type-rb/type-rb/internal/project"
	"github.com/type-rb/type-rb/internal/resolver"
)

type importFormatContext struct {
	modulePaths map[string]bool
	aliases     map[string]string
}

type importFormatContexts struct {
	projects map[string]*importFormatContext
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
	context := &importFormatContext{modulePaths: map[string]bool{}, aliases: map[string]string{}}
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
	for _, unit := range units {
		context.modulePaths[unit.ModulePath] = true
	}
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
