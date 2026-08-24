package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportBuildsLandingAndPublicDocumentation(t *testing.T) {
	docsDir := t.TempDir()
	writeTestFile(t, docsDir, "site.json", `{
		"sections": [{
			"title": "Tooling",
			"pages": [
				{ "source": "README.md", "label": "Documentation" },
				{ "source": "lint-rules/index.md", "label": "Lint rules" }
			]
		}],
		"exclude": ["development.md"]
	}`)
	writeTestFile(t, docsDir, "README.md", `# Documentation

[Lint rules](lint-rules/index.md)
[Maintainer documentation](development.md)
`)
	writeTestFile(t, docsDir, "lint-rules/index.md", `# Lint rules

| Rule | Default |
| --- | --- |
| [Prefer conditional transfer](prefer-conditional-transfer.md) | warning |
`)
	writeTestFile(t, docsDir, "lint-rules/prefer-conditional-transfer.md", `# trb/prefer-conditional-transfer

Use conditional transfer for a short guard.
`)
	writeTestFile(t, docsDir, "development.md", "# Development\n")

	output := t.TempDir()
	if err := Export(Options{OutputDir: output, DocsDir: docsDir, Version: "1.2.3-test"}); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		"index.html",
		"assets/site.css",
		"docs/index.html",
		"docs/lint-rules/index.html",
		"docs/lint-rules/prefer-conditional-transfer/index.html",
	} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("missing generated site file %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(output, "docs/development/index.html")); !os.IsNotExist(err) {
		t.Fatalf("maintainer documentation should not be published, stat error=%v", err)
	}

	landing := readTestFile(t, output, "index.html")
	for _, link := range []string{`href="/docs/"`, `href="/tour/"`, `href="/play/"`} {
		if !strings.Contains(landing, link) {
			t.Fatalf("landing page is missing %s:\n%s", link, landing)
		}
	}
	if !strings.Contains(landing, "v1.2.3-test") {
		t.Fatalf("landing page is missing the compiler version:\n%s", landing)
	}
	if !strings.Contains(landing, `<h1>TypeRB</h1>`) || !strings.Contains(landing, `class="hero-statement">One portable language.`) {
		t.Fatalf("landing page is missing the product-first hero hierarchy:\n%s", landing)
	}
	if !strings.Contains(landing, `<span class="code-keyword">def</span> main()`) {
		t.Fatalf("landing example is missing the required runnable entrypoint:\n%s", landing)
	}
	if !strings.Contains(landing, `class="entry-kicker">Start here</span>`) {
		t.Fatalf("landing page does not identify the primary documentation entry:\n%s", landing)
	}

	index := readTestFile(t, output, "docs/index.html")
	if !strings.Contains(index, `name="color-scheme" content="light"`) {
		t.Fatalf("documentation does not declare its light color scheme:\n%s", index)
	}
	if !strings.Contains(index, `href="/docs/lint-rules/"`) {
		t.Fatalf("public Markdown link was not rewritten:\n%s", index)
	}
	if !strings.Contains(index, `href="https://github.com/type-rb/type-rb/blob/main/docs/development.md"`) {
		t.Fatalf("excluded documentation link should target GitHub:\n%s", index)
	}
	if !strings.Contains(index, `aria-current="page"`) {
		t.Fatalf("active documentation navigation is missing:\n%s", index)
	}
	if !strings.Contains(index, `class="docs-navigation"`) || !strings.Contains(index, `class="docs-menu"`) {
		t.Fatalf("documentation is missing desktop or mobile navigation:\n%s", index)
	}

	rules := readTestFile(t, output, "docs/lint-rules/index.html")
	if !strings.Contains(rules, `href="/docs/lint-rules/prefer-conditional-transfer/"`) {
		t.Fatalf("rule link was not rewritten:\n%s", rules)
	}
	if !strings.Contains(rules, "<table>") {
		t.Fatalf("GFM table was not rendered:\n%s", rules)
	}
}

func TestExportRejectsNavigationToUnpublishedPage(t *testing.T) {
	docsDir := t.TempDir()
	writeTestFile(t, docsDir, "site.json", `{
		"sections": [{
			"title": "Missing",
			"pages": [{ "source": "missing.md", "label": "Missing" }]
		}]
	}`)
	writeTestFile(t, docsDir, "README.md", "# Documentation\n")

	err := Export(Options{OutputDir: t.TempDir(), DocsDir: docsDir})
	if err == nil || !strings.Contains(err.Error(), `references unpublished page "missing.md"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInternalLinksRejectsMissingRootPath(t *testing.T) {
	output := t.TempDir()
	writeTestFile(t, output, "index.html", `<a href="/docs/missing/">Missing</a>`)

	err := ValidateInternalLinks(output)
	if err == nil || !strings.Contains(err.Error(), "links to missing site path /docs/missing/") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeTestFile(t *testing.T, root, name, contents string) {
	t.Helper()
	destination := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
