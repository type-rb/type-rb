package packageextension

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectDeclarationInputIsVersionedAndSerializable(t *testing.T) {
	span := SourceSpan{
		Start: SourcePosition{Offset: 0, Line: 1, Column: 1},
		End:   SourcePosition{Offset: 8, Line: 1, Column: 9},
	}
	input := ProjectDeclarationInput{
		ProtocolVersion: ProjectDeclarationInputProtocolVersion,
		Provider:        "trb/jobs",
		Modules: []ProjectModule{{
			ModulePath: "jobs/example",
			Classes: []ProjectClass{{
				Name: "ExampleJob",
				Superclass: &ProjectTypeUse{
					Authored:       Type{Kind: "named", Name: "Job", Definition: &Definition{ModulePath: "trb/jobs/index", ImportPath: "trb/jobs"}},
					Resolved:       Type{Kind: "named", Name: "Job", Definition: &Definition{ModulePath: "trb/jobs/index", ImportPath: "trb/jobs"}},
					ResolutionPath: []ProjectTypeReference{{Name: "Job", ModulePath: "trb/jobs/index", ImportPath: "trb/jobs"}},
					Span:           span,
				},
				Methods: []ProjectMethod{{Name: "perform", Span: span}},
				Span:    span,
			}},
		}},
	}
	if err := ValidateProjectDeclarationInput(input); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ProjectDeclarationInput
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProjectDeclarationInput(decoded); err != nil {
		t.Fatal(err)
	}
}

func TestProjectDeclarationInputRejectsInvalidBoundaryData(t *testing.T) {
	valid := func() ProjectDeclarationInput {
		return ProjectDeclarationInput{
			ProtocolVersion: ProjectDeclarationInputProtocolVersion,
			Provider:        "trb/jobs",
			Modules:         []ProjectModule{{ModulePath: "jobs/example"}},
		}
	}
	tests := []struct {
		name    string
		mutate  func(*ProjectDeclarationInput)
		message string
	}{
		{name: "version", mutate: func(input *ProjectDeclarationInput) { input.ProtocolVersion++ }, message: "unsupported project declaration input protocol version"},
		{name: "provider", mutate: func(input *ProjectDeclarationInput) { input.Provider = "" }, message: "provider is missing"},
		{name: "duplicate module", mutate: func(input *ProjectDeclarationInput) { input.Modules = append(input.Modules, input.Modules[0]) }, message: "empty or duplicate module"},
		{name: "definition module", mutate: func(input *ProjectDeclarationInput) {
			input.Modules[0].TypeAliases = []ProjectTypeAlias{{
				Name: "ID",
				Target: ProjectTypeUse{
					Authored: Type{Kind: "named", Name: "External", Definition: &Definition{ImportPath: "external"}},
					Resolved: Type{Kind: "named", Name: "External"},
				},
			}}
		}, message: "source definition module path is missing"},
		{name: "record inspection metadata", mutate: func(input *ProjectDeclarationInput) {
			input.Modules[0].TypeAliases = []ProjectTypeAlias{{
				Name: "Payload",
				Target: ProjectTypeUse{
					Authored: Type{Kind: "named", Name: "Payload", Record: &Record{}},
					Resolved: Type{Kind: "named", Name: "Payload"},
				},
			}}
		}, message: "record inspection metadata is not valid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid()
			test.mutate(&input)
			err := ValidateProjectDeclarationInput(input)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q, got %v", test.message, err)
			}
		})
	}
}
