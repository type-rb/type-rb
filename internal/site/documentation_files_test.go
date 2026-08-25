package site

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var documentationFileMarker = regexp.MustCompile(`^<!-- trb-doc-file: ([A-Za-z0-9][A-Za-z0-9._/\[\]-]*) -->$`)

type documentationFile struct {
	Path      string
	Document  string
	StartLine int
	Source    []byte
}

func TestPublishedDocumentationFilesMatchRepository(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	files, err := discoverDocumentationFiles(filepath.Join(repositoryRoot, "docs"))
	if err != nil {
		t.Fatal(err)
	}

	required := map[string]bool{
		"examples/tutorials/web-orm-jobs/db/schema.sql":                    false,
		"examples/tutorials/web-orm-jobs/src/config/jobs.trb":              false,
		"examples/tutorials/web-orm-jobs/src/http/errors.trb":              false,
		"examples/tutorials/web-orm-jobs/src/jobs/generate_report_job.trb": false,
		"examples/tutorials/web-orm-jobs/src/main.trb":                     false,
		"examples/tutorials/web-orm-jobs/src/models/report.trb":            false,
		"examples/tutorials/web-orm-jobs/src/report_api_test.trb":          false,
		"examples/tutorials/web-orm-jobs/src/routes/reports.trb":           false,
		"examples/tutorials/web-orm-jobs/src/routes/reports/[id].trb":      false,
		"examples/tutorials/web-orm-jobs/src/services/create_report.trb":   false,
		"examples/tutorials/web-orm-jobs/trbconfig.jsonc":                  false,
	}
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(file.Path)))
		if err != nil {
			t.Fatalf("%s:%d: read documented repository file %s: %v", file.Document, file.StartLine, file.Path, err)
		}
		if !bytes.Equal(file.Source, data) {
			t.Fatalf("%s:%d: documented source differs from %s", file.Document, file.StartLine, file.Path)
		}
		if _, tracked := required[file.Path]; tracked {
			required[file.Path] = true
		}
	}

	missing := make([]string, 0)
	for file, found := range required {
		if !found {
			missing = append(missing, file)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("tutorial files are no longer synchronized with documentation: %s", strings.Join(missing, ", "))
	}
}

func TestExtractDocumentationFiles(t *testing.T) {
	source := []byte("# Guide\n\n<!-- trb-doc-file: examples/app/main.trb -->\n```trb\ndef main()\n\treturn\nend\n```\n")
	files, err := extractDocumentationFiles("docs/guide.md", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "examples/app/main.trb" || files[0].StartLine != 5 || !bytes.Equal(files[0].Source, []byte("def main()\n\treturn\nend\n")) {
		t.Fatalf("files=%#v", files)
	}
}

func TestExtractDocumentationFilesRejectsInvalidAnnotations(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "malformed", source: "<!-- trb-doc-file examples/app.trb -->\n```trb\n1\n```\n", want: "invalid trb-doc-file annotation"},
		{name: "parent path", source: "<!-- trb-doc-file: examples/../app.trb -->\n```trb\n1\n```\n", want: "must use a clean repository-relative path"},
		{name: "missing language", source: "<!-- trb-doc-file: examples/app.trb -->\n```\n1\n```\n", want: "must be followed immediately by a typed code fence"},
		{name: "unterminated", source: "<!-- trb-doc-file: examples/app.trb -->\n```trb\n1\n", want: "unterminated code fence"},
		{name: "empty", source: "<!-- trb-doc-file: examples/app.trb -->\n```trb\n```\n", want: "cannot be empty"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := extractDocumentationFiles("docs/guide.md", []byte(test.source))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want %q", err, test.want)
			}
		})
	}
}

func discoverDocumentationFiles(docsDir string) ([]documentationFile, error) {
	result := []documentationFile{}
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
		files, err := extractDocumentationFiles(filepath.ToSlash(relative), data)
		if err != nil {
			return err
		}
		result = append(result, files...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func extractDocumentationFiles(filePath string, source []byte) ([]documentationFile, error) {
	lines := strings.SplitAfter(string(source), "\n")
	result := []documentationFile{}
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
		if inFence || !strings.HasPrefix(line, "<!-- trb-doc-file") {
			continue
		}
		matches := documentationFileMarker.FindStringSubmatch(line)
		if len(matches) != 2 {
			return nil, fmt.Errorf("%s:%d: invalid trb-doc-file annotation", filePath, index+1)
		}
		repositoryPath := matches[1]
		if path.IsAbs(repositoryPath) || path.Clean(repositoryPath) != repositoryPath || strings.HasPrefix(repositoryPath, "../") {
			return nil, fmt.Errorf("%s:%d: trb-doc-file must use a clean repository-relative path", filePath, index+1)
		}
		if index+1 >= len(lines) {
			return nil, fmt.Errorf("%s:%d: trb-doc-file must be followed immediately by a typed code fence", filePath, index+1)
		}
		opening := strings.TrimSpace(lines[index+1])
		if !strings.HasPrefix(opening, "```") || len(opening) == 3 || strings.ContainsAny(opening[3:], " \t") {
			return nil, fmt.Errorf("%s:%d: trb-doc-file must be followed immediately by a typed code fence", filePath, index+1)
		}
		closing := index + 2
		for closing < len(lines) && strings.TrimSpace(lines[closing]) != "```" {
			closing++
		}
		if closing == len(lines) {
			return nil, fmt.Errorf("%s:%d: unterminated code fence", filePath, index+2)
		}
		contents := []byte(strings.Join(lines[index+2:closing], ""))
		if len(bytes.TrimSpace(contents)) == 0 {
			return nil, fmt.Errorf("%s:%d: documented repository file cannot be empty", filePath, index+3)
		}
		result = append(result, documentationFile{
			Path: repositoryPath, Document: filePath, StartLine: index + 3, Source: contents,
		})
		index = closing
	}
	return result, nil
}
