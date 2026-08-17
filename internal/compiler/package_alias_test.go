package compiler

import "testing"

func TestPackageAliasesAreScopedToTheirSourceUnit(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename:   "/packages/shared-a/index.trb",
			ModulePath: "github.com/acme/shared-a/index",
			Source:     []byte("def value_a(): String\n\treturn \"a\"\nend\n"),
		},
		{
			Filename:   "/packages/shared-b/index.trb",
			ModulePath: "github.com/acme/shared-b/index",
			Source:     []byte("def value_b(): String\n\treturn \"b\"\nend\n"),
		},
		{
			Filename:       "/packages/feature-a/index.trb",
			ModulePath:     "github.com/acme/feature-a/index",
			PackageAliases: map[string]string{"shared": "github.com/acme/shared-a"},
			Source: []byte(`import { value_a } from shared

def feature_a(): String
	return value_a()
end
`),
		},
		{
			Filename:       "/packages/feature-b/index.trb",
			ModulePath:     "github.com/acme/feature-b/index",
			PackageAliases: map[string]string{"shared": "github.com/acme/shared-b"},
			Source: []byte(`import { value_b } from shared

def feature_b(): String
	return value_b()
end
`),
		},
	}

	if _, err := CompileProject(sources, Options{Mode: "typescript"}); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryIndexFallbackDoesNotApplyANestedAlias(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename:   "/project/shared/index.trb",
			ModulePath: "shared/index",
			Source:     []byte("def local_value(): Integer\n\treturn 1\nend\n"),
		},
		{
			Filename:   "/packages/external.trb",
			ModulePath: "external/package",
			Source:     []byte("def external_value(): Integer\n\treturn 2\nend\n"),
		},
		{
			Filename:       "/project/main.trb",
			ModulePath:     "main",
			PackageAliases: map[string]string{"shared/index": "external/package"},
			Source: []byte(`import { local_value } from shared

def main()
	puts(local_value())
	return
end
`),
		},
	}

	if _, err := CompileProject(sources, Options{Mode: "typescript"}); err != nil {
		t.Fatal(err)
	}
}
