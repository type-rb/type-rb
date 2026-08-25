package site

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

var documentationExampleMarker = regexp.MustCompile(`^<!-- trb-doc-test: ([a-z][a-z0-9-]*)(?: kind=(source|program))?(?: modes=([a-z,]+))? -->$`)

var portableDocumentationModes = []string{"go", "ruby", "typescript"}

type documentationExample struct {
	ID        string
	Path      string
	StartLine int
	Source    []byte
	Kind      string
	Modes     []string
}

func TestPublishedDocumentationExamplesCompile(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	examples, err := discoverDocumentationExamples(filepath.Join(repositoryRoot, "docs"))
	if err != nil {
		t.Fatal(err)
	}

	required := map[string]bool{
		"getting-started-project":       false,
		"getting-started-standalone":    false,
		"language-discriminated-union":  false,
		"language-generic-types":        false,
		"language-named-only":           false,
		"language-newtypes":             false,
		"language-payload-enum":         false,
		"language-result-case":          false,
		"react-first-component":         false,
		"shared-http-values":            false,
		"specification-result-baseline": false,
		"stdlib-scalars":                false,
		"web-update-todo":               false,
	}
	for _, example := range examples {
		if _, tracked := required[example.ID]; tracked {
			required[example.ID] = true
		}
		example := example
		t.Run(example.ID, func(t *testing.T) {
			for _, mode := range example.Modes {
				mode := mode
				t.Run(mode, func(t *testing.T) {
					err := compileDocumentationExample(example, mode, repositoryRoot)
					if err != nil {
						t.Fatalf("%s:%d (%s): documented example does not compile: %v", example.Path, example.StartLine, mode, err)
					}
				})
			}
		})
	}

	missing := make([]string, 0)
	for id, found := range required {
		if !found {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("required documented examples are no longer checked: %s", strings.Join(missing, ", "))
	}
}

func TestExtractDocumentationExamples(t *testing.T) {
	source := []byte("# Guide\n\n<!-- trb-doc-test: portable kind=program -->\n```trb\ndef main()\n\treturn\nend\n```\n\n<!-- trb-doc-test: browser modes=typescript -->\n```trb\nputs(\"browser\")\n```\n")
	examples, err := extractDocumentationExamples("docs/guide.md", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) != 2 {
		t.Fatalf("examples=%#v", examples)
	}
	if examples[0].ID != "portable" || examples[0].Kind != "program" || examples[0].StartLine != 5 || !bytes.Equal(examples[0].Source, []byte("def main()\n\treturn\nend\n")) || strings.Join(examples[0].Modes, ",") != "go,ruby,typescript" {
		t.Fatalf("portable example=%#v", examples[0])
	}
	if examples[1].ID != "browser" || examples[1].Kind != "source" || examples[1].StartLine != 12 || strings.Join(examples[1].Modes, ",") != "typescript" {
		t.Fatalf("browser example=%#v", examples[1])
	}
}

func TestExtractDocumentationExamplesRejectsInvalidAnnotations(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "malformed", source: "<!-- trb-doc-test portable -->\n```trb\n1\n```\n", want: "invalid trb-doc-test annotation"},
		{name: "unknown kind", source: "<!-- trb-doc-test: sample kind=snippet -->\n```trb\n1\n```\n", want: "invalid trb-doc-test annotation"},
		{name: "wrong fence", source: "<!-- trb-doc-test: sample -->\n```sh\ntrb check\n```\n", want: "must be followed immediately by a trb code fence"},
		{name: "unknown mode", source: "<!-- trb-doc-test: sample modes=python -->\n```trb\n1\n```\n", want: "unsupported documentation example mode"},
		{name: "duplicate mode", source: "<!-- trb-doc-test: sample modes=go,go -->\n```trb\n1\n```\n", want: "duplicate documentation example mode"},
		{name: "unterminated", source: "<!-- trb-doc-test: sample -->\n```trb\n1\n", want: "unterminated trb code fence"},
		{name: "empty", source: "<!-- trb-doc-test: sample -->\n```trb\n```\n", want: "cannot be empty"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := extractDocumentationExamples("docs/guide.md", []byte(test.source))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestCompileDocumentationExampleRequiresMainForPrograms(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	example := documentationExample{ID: "program", Path: "docs/guide.md", StartLine: 1, Source: []byte("puts(\"hello\")\n"), Kind: "program"}
	err = compileDocumentationExample(example, "go", repositoryRoot)
	if err == nil || !strings.Contains(err.Error(), "top-level def main()") {
		t.Fatalf("error=%v", err)
	}
}

func TestExtractDocumentationExamplesIgnoresAnnotationsInsideCodeFences(t *testing.T) {
	source := []byte("```text\n<!-- trb-doc-test: illustrative-only -->\n```\n")
	examples, err := extractDocumentationExamples("docs/guide.md", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) != 0 {
		t.Fatalf("examples=%#v", examples)
	}
}

func discoverDocumentationExamples(docsDir string) ([]documentationExample, error) {
	result := []documentationExample{}
	seen := map[string]string{}
	repositoryRoot := filepath.Dir(filepath.Clean(docsDir))
	err := filepath.WalkDir(docsDir, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(repositoryRoot, filePath)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		examples, err := extractDocumentationExamples(relative, data)
		if err != nil {
			return err
		}
		for _, example := range examples {
			if previous, duplicate := seen[example.ID]; duplicate {
				return fmt.Errorf("documentation example %q is duplicated in %s and %s", example.ID, previous, example.Path)
			}
			seen[example.ID] = example.Path
			result = append(result, example)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result, nil
}

func extractDocumentationExamples(filePath string, source []byte) ([]documentationExample, error) {
	lines := strings.SplitAfter(string(source), "\n")
	result := []documentationExample{}
	inFence := false
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if strings.HasPrefix(line, "```") {
			if !inFence {
				inFence = true
			} else if line == "```" {
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}
		if !strings.HasPrefix(line, "<!-- trb-doc-test") {
			continue
		}
		matches := documentationExampleMarker.FindStringSubmatch(line)
		if len(matches) != 4 {
			return nil, fmt.Errorf("%s:%d: invalid trb-doc-test annotation", filePath, index+1)
		}
		if index+1 >= len(lines) || strings.TrimSpace(lines[index+1]) != "```trb" {
			return nil, fmt.Errorf("%s:%d: trb-doc-test must be followed immediately by a trb code fence", filePath, index+1)
		}

		modes, err := documentationExampleModes(matches[3])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", filePath, index+1, err)
		}
		closing := index + 2
		for closing < len(lines) && strings.TrimSpace(lines[closing]) != "```" {
			closing++
		}
		if closing == len(lines) {
			return nil, fmt.Errorf("%s:%d: unterminated trb code fence", filePath, index+2)
		}
		contents := []byte(strings.Join(lines[index+2:closing], ""))
		if len(bytes.TrimSpace(contents)) == 0 {
			return nil, fmt.Errorf("%s:%d: documented example cannot be empty", filePath, index+3)
		}
		kind := matches[2]
		if kind == "" {
			kind = "source"
		}
		result = append(result, documentationExample{
			ID: matches[1], Path: filePath, StartLine: index + 3, Source: contents, Kind: kind, Modes: modes,
		})
		index = closing
	}
	return result, nil
}

func compileDocumentationExample(example documentationExample, mode, repositoryRoot string) error {
	source := append(bytes.Repeat([]byte{'\n'}, example.StartLine-1), example.Source...)
	artifacts, err := compiler.CompileProject([]compiler.SourceUnit{{
		Filename: example.Path, ModulePath: "main", Package: "main", Source: source,
	}}, compiler.Options{
		Mode: mode, GoModule: "example.com/type-rb/documentation", RubyLoader: "require_relative",
		TypeScriptRuntime: "bun", ProjectRoot: repositoryRoot, SourceRoot: filepath.Join(repositoryRoot, "docs"),
	})
	if err != nil {
		return err
	}
	if example.Kind != "program" {
		return nil
	}
	for _, artifact := range artifacts {
		if artifact.IR.ModulePath != "main" {
			continue
		}
		for _, statement := range artifact.IR.Statements {
			if method, ok := statement.(*ir.Method); ok && method.Name == compiler.MainFunction {
				return nil
			}
		}
	}
	return fmt.Errorf("documented program must define a top-level def main()")
}

func documentationExampleModes(value string) ([]string, error) {
	if value == "" {
		return append([]string(nil), portableDocumentationModes...), nil
	}
	allowed := map[string]bool{"go": true, "ruby": true, "typescript": true}
	seen := map[string]bool{}
	result := strings.Split(value, ",")
	for _, mode := range result {
		if !allowed[mode] {
			return nil, fmt.Errorf("unsupported documentation example mode %q", mode)
		}
		if seen[mode] {
			return nil, fmt.Errorf("duplicate documentation example mode %q", mode)
		}
		seen[mode] = true
	}
	return result, nil
}
