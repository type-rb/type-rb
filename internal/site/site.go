package site

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"html/template"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

//go:embed assets/*
var assets embed.FS

var (
	markdownLinkPattern  = regexp.MustCompile(`href="([^"]+)"`)
	markdownTitlePattern = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)
	titleMarkupPattern   = regexp.MustCompile("[`*_~]")
)

type Options struct {
	OutputDir string
	DocsDir   string
	Version   string
}

type manifest struct {
	Sections []manifestSection `json:"sections"`
	Exclude  []string          `json:"exclude"`
}

type manifestSection struct {
	Title string         `json:"title"`
	Pages []manifestPage `json:"pages"`
}

type manifestPage struct {
	Source string `json:"source"`
	Label  string `json:"label"`
}

type page struct {
	Source     string
	Title      string
	URL        string
	OutputPath string
	Body       template.HTML
}

type navSection struct {
	Title string
	Pages []navPage
}

type navPage struct {
	Label  string
	URL    string
	Active bool
}

type landingData struct {
	Version string
}

type docsData struct {
	Title     string
	URL       string
	SourceURL string
	Version   string
	Body      template.HTML
	Sections  []navSection
}

func Export(options Options) error {
	if options.OutputDir == "" {
		return errors.New("site output directory is required")
	}
	if options.DocsDir == "" {
		return errors.New("documentation source directory is required")
	}
	if options.Version == "" {
		options.Version = "dev"
	}

	configuration, err := loadManifest(options.DocsDir)
	if err != nil {
		return err
	}
	pages, err := loadPages(options.DocsDir, configuration)
	if err != nil {
		return err
	}
	if err := validateNavigation(configuration, pages); err != nil {
		return err
	}

	pageURLs := make(map[string]string, len(pages))
	for _, current := range pages {
		pageURLs[current.Source] = current.URL
	}
	for index := range pages {
		body, err := renderMarkdown(options.DocsDir, pages[index].Source, pageURLs)
		if err != nil {
			return err
		}
		pages[index].Body = template.HTML(body) // Repository documentation is trusted input rendered without raw HTML.
	}

	if err := os.MkdirAll(filepath.Join(options.OutputDir, "assets"), 0o755); err != nil {
		return err
	}
	for _, name := range []string{"site.css", "docs.js"} {
		asset, err := assets.ReadFile("assets/" + name)
		if err != nil {
			return err
		}
		if err := writeFile(options.OutputDir, "assets/"+name, asset); err != nil {
			return err
		}
	}
	if err := renderTemplate(options.OutputDir, "index.html", "landing.html", landingData{Version: options.Version}); err != nil {
		return err
	}

	for _, current := range pages {
		data := docsData{
			Title:     current.Title,
			URL:       current.URL,
			SourceURL: "https://github.com/type-rb/type-rb/blob/main/docs/" + current.Source,
			Version:   options.Version,
			Body:      current.Body,
			Sections:  navigation(configuration, pages, current.Source),
		}
		if err := renderTemplate(options.OutputDir, current.OutputPath, "docs.html", data); err != nil {
			return err
		}
	}
	return nil
}

func ValidateInternalLinks(outputDir string) error {
	return filepath.WalkDir(outputDir, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || path.Ext(entry.Name()) != ".html" {
			return nil
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		for _, match := range markdownLinkPattern.FindAllSubmatch(data, -1) {
			if len(match) != 2 {
				continue
			}
			destination := stdhtml.UnescapeString(string(match[1]))
			parsed, err := url.Parse(destination)
			if err != nil || parsed.IsAbs() || !strings.HasPrefix(parsed.Path, "/") {
				continue
			}
			target := strings.TrimPrefix(path.Clean(parsed.Path), "/")
			if parsed.Path == "/" {
				target = "index.html"
			} else if strings.HasSuffix(parsed.Path, "/") {
				target = path.Join(target, "index.html")
			}
			if _, err := os.Stat(filepath.Join(outputDir, filepath.FromSlash(target))); err != nil {
				relative, _ := filepath.Rel(outputDir, filePath)
				return fmt.Errorf("%s links to missing site path %s", filepath.ToSlash(relative), destination)
			}
		}
		return nil
	})
}

func loadManifest(docsDir string) (manifest, error) {
	data, err := os.ReadFile(filepath.Join(docsDir, "site.json"))
	if err != nil {
		return manifest{}, fmt.Errorf("read documentation site manifest: %w", err)
	}
	var result manifest
	if err := json.Unmarshal(data, &result); err != nil {
		return manifest{}, fmt.Errorf("parse documentation site manifest: %w", err)
	}
	if len(result.Sections) == 0 {
		return manifest{}, errors.New("documentation site manifest must define at least one section")
	}
	return result, nil
}

func loadPages(docsDir string, configuration manifest) ([]page, error) {
	result := []page{}
	err := filepath.WalkDir(docsDir, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(docsDir, filePath)
		if err != nil {
			return err
		}
		source := filepath.ToSlash(relative)
		if path.Ext(source) != ".md" || isExcluded(source, configuration.Exclude) {
			return nil
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		title, err := markdownTitle(data)
		if err != nil {
			return fmt.Errorf("%s: %w", source, err)
		}
		outputPath, pageURL := documentationLocation(source)
		result = append(result, page{Source: source, Title: title, OutputPath: outputPath, URL: pageURL})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover documentation: %w", err)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Source < result[j].Source })
	return result, nil
}

func isExcluded(source string, excludes []string) bool {
	for _, excluded := range excludes {
		clean := strings.TrimSuffix(path.Clean(excluded), "/")
		if source == clean || strings.HasPrefix(source, clean+"/") {
			return true
		}
	}
	return false
}

func markdownTitle(source []byte) (string, error) {
	match := markdownTitlePattern.FindSubmatch(source)
	if len(match) != 2 {
		return "", errors.New("public documentation must start with a level-one heading")
	}
	title := titleMarkupPattern.ReplaceAllString(strings.TrimSpace(string(match[1])), "")
	return title, nil
}

func documentationLocation(source string) (string, string) {
	if source == "README.md" {
		return "docs/index.html", "/docs/"
	}
	directory, name := path.Split(source)
	stem := strings.TrimSuffix(name, ".md")
	if stem == "index" {
		cleanDirectory := strings.TrimSuffix(directory, "/")
		return path.Join("docs", cleanDirectory, "index.html"), "/" + path.Join("docs", cleanDirectory) + "/"
	}
	return path.Join("docs", directory, stem, "index.html"), "/" + path.Join("docs", directory, stem) + "/"
}

func validateNavigation(configuration manifest, pages []page) error {
	available := make(map[string]bool, len(pages))
	for _, current := range pages {
		available[current.Source] = true
	}
	seen := map[string]bool{}
	for _, section := range configuration.Sections {
		if strings.TrimSpace(section.Title) == "" || len(section.Pages) == 0 {
			return errors.New("every documentation navigation section needs a title and pages")
		}
		for _, item := range section.Pages {
			if !available[item.Source] {
				return fmt.Errorf("documentation navigation references unpublished page %q", item.Source)
			}
			if strings.TrimSpace(item.Label) == "" {
				return fmt.Errorf("documentation navigation page %q needs a label", item.Source)
			}
			if seen[item.Source] {
				return fmt.Errorf("documentation navigation contains duplicate page %q", item.Source)
			}
			seen[item.Source] = true
		}
	}
	return nil
}

func renderMarkdown(docsDir, source string, pageURLs map[string]string) (string, error) {
	data, err := os.ReadFile(filepath.Join(docsDir, filepath.FromSlash(source)))
	if err != nil {
		return "", err
	}
	converter := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	var output bytes.Buffer
	if err := converter.Convert(data, &output); err != nil {
		return "", fmt.Errorf("render %s: %w", source, err)
	}
	return rewriteMarkdownLinks(output.String(), source, pageURLs), nil
}

func rewriteMarkdownLinks(rendered, source string, pageURLs map[string]string) string {
	return markdownLinkPattern.ReplaceAllStringFunc(rendered, func(attribute string) string {
		match := markdownLinkPattern.FindStringSubmatch(attribute)
		if len(match) != 2 {
			return attribute
		}
		destination := stdhtml.UnescapeString(match[1])
		parsed, err := url.Parse(destination)
		if err != nil || parsed.IsAbs() || strings.HasPrefix(destination, "/") || parsed.Path == "" {
			return attribute
		}
		if path.Ext(parsed.Path) != ".md" {
			return attribute
		}

		target := path.Clean(path.Join(path.Dir(source), parsed.Path))
		if publicURL, ok := pageURLs[target]; ok {
			parsed.Path = publicURL
			return `href="` + stdhtml.EscapeString(parsed.String()) + `"`
		}

		repositoryPath := path.Clean(path.Join("docs", path.Dir(source), parsed.Path))
		githubURL := url.URL{
			Scheme:   "https",
			Host:     "github.com",
			Path:     "/type-rb/type-rb/blob/main/" + repositoryPath,
			Fragment: parsed.Fragment,
		}
		return `href="` + stdhtml.EscapeString(githubURL.String()) + `"`
	})
}

func navigation(configuration manifest, pages []page, active string) []navSection {
	bySource := make(map[string]page, len(pages))
	for _, current := range pages {
		bySource[current.Source] = current
	}
	result := make([]navSection, 0, len(configuration.Sections))
	for _, section := range configuration.Sections {
		current := navSection{Title: section.Title, Pages: make([]navPage, 0, len(section.Pages))}
		for _, item := range section.Pages {
			current.Pages = append(current.Pages, navPage{
				Label:  item.Label,
				URL:    bySource[item.Source].URL,
				Active: item.Source == active,
			})
		}
		result = append(result, current)
	}
	return result
}

func renderTemplate(outputDir, outputPath, templateName string, data any) error {
	parsed, err := template.ParseFS(assets, "assets/"+templateName)
	if err != nil {
		return err
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return err
	}
	return writeFile(outputDir, outputPath, output.Bytes())
}

func writeFile(outputDir, name string, data []byte) error {
	destination := filepath.Join(outputDir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(destination, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
