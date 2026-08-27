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
			Newtypes: []ProjectNewtype{{
				Name:   "JobID",
				Target: ProjectTypeUse{Authored: Type{Kind: "int", Name: "Integer"}, Resolved: Type{Kind: "int", Name: "Integer"}, Span: span},
				Span:   span,
			}},
			Records: []ProjectRecord{{
				Name: "JobPayload",
				Fields: []ProjectRecordField{{
					Name:       "id",
					Type:       ProjectTypeUse{Authored: Type{Kind: "int", Name: "Integer"}, Resolved: Type{Kind: "int", Name: "Integer"}, Span: span},
					HasDefault: true,
					Attributes: []ProjectAttribute{{
						Name: "json", Arguments: []ProjectDirectiveArgument{{Value: ProjectValue{Kind: "string", Raw: `"job_id"`}, Span: span}}, Span: span,
					}},
					Span: span,
				}},
				Span: span,
			}},
			Enums: []ProjectEnum{{
				Name: "DeliveryState",
				Members: []ProjectEnumMember{{
					Name: "Pending",
					Attributes: []ProjectAttribute{{
						Name: "json", Arguments: []ProjectDirectiveArgument{{Value: ProjectValue{Kind: "string", Raw: `"pending"`}, Span: span}}, Span: span,
					}},
					Span: span,
				}},
				Span: span,
			}},
			Classes: []ProjectClass{{
				Name: "ExampleJob",
				Superclass: &ProjectTypeUse{
					Authored:       Type{Kind: "named", Name: "Job", Definition: &Definition{ModulePath: "trb/jobs/index", ImportPath: "trb/jobs"}},
					Resolved:       Type{Kind: "named", Name: "Job", Definition: &Definition{ModulePath: "trb/jobs/index", ImportPath: "trb/jobs"}},
					ResolutionPath: []ProjectTypeReference{{Name: "Job", ModulePath: "trb/jobs/index", ImportPath: "trb/jobs"}},
					Span:           span,
				},
				Methods: []ProjectMethod{{Name: "perform", Span: span}},
				Directives: []ProjectDirective{{
					Name: "payload", TypeArguments: []ProjectTypeUse{{Authored: Type{Kind: "named", Name: "JobPayload"}, Resolved: Type{Kind: "named", Name: "JobPayload"}, Span: span}}, Span: span,
				}},
				Span: span,
			}},
			Functions: []ProjectMethod{{Name: "build_payload", Return: &ProjectTypeUse{Authored: Type{Kind: "named", Name: "JobPayload"}, Resolved: Type{Kind: "named", Name: "JobPayload"}, Span: span}, Span: span}},
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
	if !decoded.Modules[0].Records[0].Fields[0].HasDefault {
		t.Fatal("record default presence did not survive the JSON boundary")
	}
	if got := decoded.Modules[0].Enums[0].Members[0].Attributes[0].Arguments[0].Value.Raw; got != `"pending"` {
		t.Fatalf("enum member attributes did not survive the JSON boundary: %q", got)
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
		{name: "resolved import", mutate: func(input *ProjectDeclarationInput) {
			input.Modules[0].Imports = []ProjectImport{{Path: "contracts/ids"}}
		}, message: "has no resolved module path"},
		{name: "enum raw value", mutate: func(input *ProjectDeclarationInput) {
			input.Modules[0].Enums = []ProjectEnum{{Name: "State", Members: []ProjectEnumMember{{Name: "Ready", RawValue: &ProjectValue{Kind: "array"}}}}}
		}, message: "unsupported value kind"},
		{name: "enum attribute", mutate: func(input *ProjectDeclarationInput) {
			input.Modules[0].Enums = []ProjectEnum{{Name: "State", Members: []ProjectEnumMember{{
				Name: "Ready", Attributes: []ProjectAttribute{{Name: "json", Arguments: []ProjectDirectiveArgument{{Value: ProjectValue{Kind: "array"}}}}},
			}}}}
		}, message: "unsupported value kind"},
		{name: "directive reference", mutate: func(input *ProjectDeclarationInput) {
			input.Modules[0].Classes = []ProjectClass{{Name: "Model", Directives: []ProjectDirective{{
				Name: "belongs_to", Arguments: []ProjectDirectiveArgument{{Value: ProjectValue{Kind: "reference"}}},
			}}}}
		}, message: "empty reference"},
		{name: "directive block shape", mutate: func(input *ProjectDeclarationInput) {
			input.Modules[0].Classes = []ProjectClass{{Name: "Model", Directives: []ProjectDirective{{
				Name: "has_many", Block: &ProjectDirectiveBlock{StatementCount: 2, ResultExpression: true},
			}}}}
		}, message: "result expression requires one statement"},
		{name: "directive type argument", mutate: func(input *ProjectDeclarationInput) {
			input.Modules[0].Classes = []ProjectClass{{Name: "Model", Directives: []ProjectDirective{{
				Name: "payload", TypeArguments: []ProjectTypeUse{{Authored: Type{Kind: ""}, Resolved: Type{Kind: "int", Name: "Integer"}}},
			}}}}
		}, message: "type argument"},
		{name: "record attribute", mutate: func(input *ProjectDeclarationInput) {
			input.Modules[0].Records = []ProjectRecord{{Name: "Payload", Fields: []ProjectRecordField{{
				Name:       "id",
				Type:       ProjectTypeUse{Authored: Type{Kind: "int", Name: "Integer"}, Resolved: Type{Kind: "int", Name: "Integer"}},
				Attributes: []ProjectAttribute{{Name: "json", Arguments: []ProjectDirectiveArgument{{Value: ProjectValue{Kind: "array"}}}}},
			}}}}
		}, message: "unsupported value kind"},
		{name: "function parameter", mutate: func(input *ProjectDeclarationInput) {
			input.Modules[0].Functions = []ProjectMethod{{Name: "handle", Parameters: []ProjectParameter{{Type: ProjectTypeUse{Authored: Type{Kind: "int", Name: "Integer"}, Resolved: Type{Kind: "int", Name: "Integer"}}}}}}
		}, message: "unnamed parameter"},
		{name: "class function", mutate: func(input *ProjectDeclarationInput) {
			input.Modules[0].Functions = []ProjectMethod{{Name: "handle", Class: true}}
		}, message: "cannot be a class method"},
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
