package compiler

import (
	"strings"
	"testing"
)

func TestOpaqueStandardTypesCannotBeSuperclassesAcrossPortableBackends(t *testing.T) {
	tests := map[string]struct {
		source     string
		diagnostic string
	}{
		"file direct": {
			source: `import { File } from trb/std/file

class Child < File
end
`,
			diagnostic: "File cannot be used as a superclass because it is nonconstructible",
		},
		"file imported alias": {
			source: `import { File as Resource } from trb/std/file

class Child < Resource
end
`,
			diagnostic: "Resource cannot be used as a superclass because it is nonconstructible",
		},
		"file declaration root alias": {
			source: `import trb/std/file as Resource

class Child < Resource
end
`,
			diagnostic: "Resource cannot be used as a superclass because it is nonconstructible",
		},
		"file transparent alias": {
			source: `import { File } from trb/std/file

alias Resource = File

class Child < Resource
end
`,
			diagnostic: "scoped File may only be introduced as the File.open() block parameter",
		},
		"directory direct": {
			source: `import { Dir } from trb/std/dir

class Child < Dir
end
`,
			diagnostic: "Dir cannot be used as a superclass because it is nonconstructible",
		},
		"directory imported alias": {
			source: `import { Dir as Resource } from trb/std/dir

class Child < Resource
end
`,
			diagnostic: "Resource cannot be used as a superclass because it is nonconstructible",
		},
		"directory declaration root alias": {
			source: `import trb/std/dir as Resource

class Child < Resource
end
`,
			diagnostic: "Resource cannot be used as a superclass because it is nonconstructible",
		},
		"directory transparent alias": {
			source: `import { Dir } from trb/std/dir

alias Resource = Dir

class Child < Resource
end
`,
			diagnostic: "Resource cannot be used as a superclass because it is nonconstructible",
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		for name, test := range tests {
			t.Run(mode+"/"+name, func(t *testing.T) {
				_, err := Compile("main.trb", []byte(test.source), mode)
				if err == nil || !strings.Contains(err.Error(), test.diagnostic) {
					t.Fatalf("Compile() error = %v, want %q", err, test.diagnostic)
				}
			})
		}
	}
}

func TestUnrelatedFileAndDirClassesRemainValidSuperclassesAcrossPortableBackends(t *testing.T) {
	source := []byte(`class File
end

class FileChild < File
end

class Dir
end

class DirChild < Dir
end

file := FileChild.new()
directory := DirChild.new()
`)

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := Compile("main.trb", source, mode); err != nil {
				t.Fatalf("Compile() error = %v", err)
			}
		})
	}
}

func TestImportedTransparentAliasesToOpaqueStandardTypesCannotBeSuperclasses(t *testing.T) {
	tests := map[string]struct {
		aliases    string
		consumer   string
		diagnostic string
	}{
		"file": {
			aliases: `import { File } from trb/std/file

alias Resource = File
`,
			consumer: `import { Resource } from resources

class Child < Resource
end
`,
			diagnostic: "Resource cannot be used as a superclass because it is nonconstructible",
		},
		"directory": {
			aliases: `import { Dir } from trb/std/dir

alias Resource = Dir
`,
			consumer: `import { Resource } from resources

class Child < Resource
end
`,
			diagnostic: "Resource cannot be used as a superclass because it is nonconstructible",
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		for name, test := range tests {
			t.Run(mode+"/"+name, func(t *testing.T) {
				aliases := SourceUnit{
					Filename: "resources.trb", ModulePath: "resources", Package: "resources", Source: []byte(test.aliases),
				}
				consumer := SourceUnit{
					Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(test.consumer),
				}
				_, err := CompileProject([]SourceUnit{aliases, consumer}, Options{
					Mode: mode, GoModule: "example.com/opaque-inheritance",
					SourceRoot: "/project", ProjectRoot: "/project",
				})
				if err == nil || !strings.Contains(err.Error(), test.diagnostic) {
					t.Fatalf("CompileProject() error = %v, want %q", err, test.diagnostic)
				}
			})
		}
	}
}
