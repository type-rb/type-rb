package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/testsuite"
)

// fileRootSource is an owned source snapshot used both to discover imports and
// to compile the resulting graph.
type fileRootSource struct {
	Filename string
	Source   []byte
}

// fileRootSourceGraph contains the project-local import closure rooted at one
// source file. Root is the synthetic source root used to resolve project
// imports when no trbconfig.jsonc is available.
type fileRootSourceGraph struct {
	Root    string
	Entry   string
	Sources []fileRootSource
}

// loadFileRootSourceGraph loads the project-local import closure for entry.
// readFile is injectable so callers such as the language server can layer open
// document contents over the filesystem. A source is read at most once, and
// the exact bytes used to discover its edges are returned in the graph.
func loadFileRootSourceGraph(entry string, readFile func(string) ([]byte, error)) (*fileRootSourceGraph, error) {
	if readFile == nil {
		return nil, errors.New("file-root source graph requires a source reader")
	}
	absoluteEntry, err := filepath.Abs(entry)
	if err != nil {
		return nil, err
	}
	absoluteEntry = filepath.Clean(absoluteEntry)
	root := filepath.Dir(absoluteEntry)

	type readResult struct {
		source []byte
		err    error
	}
	reads := map[string]readResult{}
	readSnapshot := func(filename string) ([]byte, error) {
		if result, ok := reads[filename]; ok {
			return result.source, result.err
		}
		source, readErr := readFile(filename)
		if readErr == nil {
			source = append([]byte(nil), source...)
		}
		reads[filename] = readResult{source: source, err: readErr}
		return source, readErr
	}

	entrySource, err := readSnapshot(absoluteEntry)
	if err != nil {
		return nil, fmt.Errorf("read entry source %s: %w", absoluteEntry, err)
	}
	sources := map[string][]byte{absoluteEntry: entrySource}
	queue := []string{absoluteEntry}
	for len(queue) > 0 {
		filename := queue[0]
		queue = queue[1:]
		program, _ := parser.Parse(sources[filename])
		imports := projectImportPaths(program.Statements)
		for _, importPath := range imports {
			resolved, source, found, resolveErr := readFileRootImport(root, importPath, readSnapshot)
			if resolveErr != nil {
				return nil, resolveErr
			}
			if !found {
				// The compiler owns unresolved-import diagnostics. Omitting a
				// missing edge lets it report the source span and canonical name.
				continue
			}
			if _, seen := sources[resolved]; seen {
				continue
			}
			sources[resolved] = source
			queue = append(queue, resolved)
		}
	}

	filenames := make([]string, 0, len(sources))
	for filename := range sources {
		filenames = append(filenames, filename)
	}
	sort.Strings(filenames)
	graphSources := make([]fileRootSource, 0, len(filenames))
	for _, filename := range filenames {
		graphSources = append(graphSources, fileRootSource{Filename: filename, Source: sources[filename]})
	}
	return &fileRootSourceGraph{Root: root, Entry: absoluteEntry, Sources: graphSources}, nil
}

func projectImportPaths(statements []ast.Statement) []string {
	seen := map[string]bool{}
	var paths []string
	for _, statement := range statements {
		importStatement, ok := statement.(*ast.ImportStatement)
		if !ok || stdlib.IsReservedPath(importStatement.Path) || seen[importStatement.Path] {
			continue
		}
		seen[importStatement.Path] = true
		paths = append(paths, importStatement.Path)
	}
	sort.Strings(paths)
	return paths
}

func readFileRootImport(root, importPath string, readFile func(string) ([]byte, error)) (string, []byte, bool, error) {
	modules, valid := resolver.ProjectImportModuleCandidates(importPath)
	if !valid {
		return "", nil, false, nil
	}
	candidates := make([]string, 0, len(modules))
	for _, module := range modules {
		candidates = append(candidates, filepath.Join(root, filepath.FromSlash(module)+".trb"))
	}
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		relative, err := filepath.Rel(root, candidate)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", nil, false, nil
		}
		if testsuite.IsTestFile(candidate) {
			continue
		}
		source, err := readFile(candidate)
		if err == nil {
			return candidate, source, true, nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		return "", nil, false, fmt.Errorf("read imported source %s: %w", candidate, err)
	}
	return "", nil, false, nil
}
